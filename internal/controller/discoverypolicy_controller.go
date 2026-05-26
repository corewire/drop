/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dropv1alpha1 "github.com/Breee/drop/api/v1alpha1"
	"github.com/Breee/drop/internal/discovery"
	dropmetrics "github.com/Breee/drop/internal/metrics"
)

// DiscoveryPolicyReconciler reconciles a DiscoveryPolicy object
type DiscoveryPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	reasonDNSError          = "DNSError"
	reasonConnectionRefused = "ConnectionRefused"
)

// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile queries discovery sources and updates the DiscoveryPolicy status.
func (r *DiscoveryPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch DiscoveryPolicy
	dp := &dropv1alpha1.DiscoveryPolicy{}
	if err := r.Get(ctx, req.NamespacedName, dp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Query each source
	patch := client.MergeFrom(dp.DeepCopy())
	var allResults []discovery.ImageResult
	allSourcesHealthy := true
	var lastFailReason, lastFailMessage string

	for i, src := range dp.Spec.Sources {
		source, err := r.buildSource(ctx, src)
		if err != nil {
			log.Error(err, "building source", "index", i, "type", src.Type)
			allSourcesHealthy = false
			lastFailReason, lastFailMessage = classifyError(err)
			dropmetrics.DiscoverySourceHealth.WithLabelValues(dp.Name, src.Type, sourceEndpoint(src)).Set(0)
			continue
		}

		start := time.Now()
		results, err := source.Fetch(ctx)
		elapsed := time.Since(start).Seconds()
		dropmetrics.DiscoverySourceLatencySeconds.WithLabelValues(dp.Name, src.Type).Observe(elapsed)

		if err != nil {
			log.Error(err, "fetching from source", "index", i, "type", src.Type)
			allSourcesHealthy = false
			lastFailReason, lastFailMessage = classifyError(err)
			dropmetrics.DiscoverySourceHealth.WithLabelValues(dp.Name, src.Type, sourceEndpoint(src)).Set(0)
			continue
		}

		dropmetrics.DiscoverySourceHealth.WithLabelValues(dp.Name, src.Type, sourceEndpoint(src)).Set(1)

		// Tag results with source type
		for j := range results {
			results[j] = discovery.ImageResult{
				Image: results[j].Image,
				Score: results[j].Score,
			}
		}
		dropmetrics.DiscoveryImagesFound.WithLabelValues(dp.Name, src.Type).Set(float64(len(results)))
		allResults = append(allResults, results...)
	}

	// 3. Merge results (deduplicate by image, keep highest score)
	merged := deduplicateResults(allResults)

	// 4. Apply image filter
	if dp.Spec.ImageFilter != "" {
		re, err := regexp.Compile(dp.Spec.ImageFilter)
		if err != nil {
			log.Error(err, "compiling image filter regex")
		} else {
			var filtered []discovery.ImageResult
			for _, r := range merged {
				if re.MatchString(r.Image) {
					filtered = append(filtered, r)
				}
			}
			merged = filtered
		}
	}

	// 5. Sort by score descending, truncate to maxImages
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].Image < merged[j].Image
	})

	maxImages := dp.Spec.MaxImages
	if maxImages <= 0 {
		maxImages = 50
	}
	if int32(len(merged)) > maxImages {
		merged = merged[:maxImages]
	}

	// 6. Write status
	// On total failure and previous results exist, keep last good results
	if len(merged) == 0 && !allSourcesHealthy && len(dp.Status.DiscoveredImages) > 0 {
		log.Info("all sources failed, keeping previous discovery results")
	} else {
		discoveredImages := make([]dropv1alpha1.DiscoveredImage, 0, len(merged))
		for _, r := range merged {
			discoveredImages = append(discoveredImages, dropv1alpha1.DiscoveredImage{
				Image:  r.Image,
				Score:  r.Score,
				Source: "discovery",
			})
		}
		dp.Status.DiscoveredImages = discoveredImages
	}

	now := metav1.Now()
	if allSourcesHealthy || len(merged) > 0 {
		dp.Status.LastSyncTime = &now
	}

	// 7. Set conditions
	sourceCondition := metav1.Condition{
		Type:               "SourceHealthy",
		ObservedGeneration: dp.Generation,
		LastTransitionTime: now,
	}
	if allSourcesHealthy {
		sourceCondition.Status = metav1.ConditionTrue
		sourceCondition.Reason = "AllSourcesHealthy"
		sourceCondition.Message = "All discovery sources responded successfully"
	} else {
		sourceCondition.Status = metav1.ConditionFalse
		sourceCondition.Reason = "SourceError"
		sourceCondition.Message = "One or more sources failed to respond"
	}
	meta.SetStatusCondition(&dp.Status.Conditions, sourceCondition)

	readyCondition := metav1.Condition{
		Type:               conditionTypeReady,
		ObservedGeneration: dp.Generation,
		LastTransitionTime: now,
	}
	if allSourcesHealthy {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "Synced"
		readyCondition.Message = fmt.Sprintf("Discovered %d images", len(dp.Status.DiscoveredImages))
	} else if len(dp.Status.DiscoveredImages) > 0 {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "PartiallyFailed"
		readyCondition.Message = fmt.Sprintf("Discovered %d images, but some sources failed: %s", len(dp.Status.DiscoveredImages), lastFailMessage)
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = lastFailReason
		if lastFailReason == "" {
			readyCondition.Reason = "SyncFailed"
		}
		if lastFailMessage != "" {
			readyCondition.Message = lastFailMessage
		} else {
			readyCondition.Message = "All sources failed, no images discovered"
		}
	}
	meta.SetStatusCondition(&dp.Status.Conditions, readyCondition)

	// Set scalar counts for printer columns
	dp.Status.SourceCount = int32(len(dp.Spec.Sources))
	dp.Status.ImageCount = int32(len(dp.Status.DiscoveredImages))

	if err := r.Status().Patch(ctx, dp, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching status: %w", err)
	}

	// 8. Requeue after sync interval
	syncInterval := dp.Spec.SyncInterval.Duration
	if syncInterval == 0 {
		syncInterval = 30 * time.Minute
	}

	// If sources failed, return error → controller-runtime rate limiter
	// applies exponential backoff (standard k8s pattern).
	if !allSourcesHealthy && len(dp.Status.DiscoveredImages) == 0 {
		return ctrl.Result{}, fmt.Errorf("discovery sync failed: %s", lastFailMessage)
	}

	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// buildSource creates the appropriate Source implementation from a DiscoverySource config.
func (r *DiscoveryPolicyReconciler) buildSource(ctx context.Context, src dropv1alpha1.DiscoverySource) (discovery.Source, error) {
	httpClient, err := r.buildHTTPClient(ctx, src.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("building HTTP client: %w", err)
	}

	switch src.Type {
	case "prometheus":
		if src.Prometheus == nil {
			return nil, fmt.Errorf("prometheus config is required when type=prometheus")
		}
		var lookback time.Duration
		if src.Prometheus.Lookback != nil {
			lookback = src.Prometheus.Lookback.Duration
		}
		return discovery.NewPrometheusSource(src.Prometheus.Endpoint, src.Prometheus.Query, lookback, src.Prometheus.Step, httpClient), nil
	case "registry":
		if src.Registry == nil {
			return nil, fmt.Errorf("registry config is required when type=registry")
		}
		return discovery.NewRegistrySource(
			src.Registry.URL,
			src.Registry.Repositories,
			src.Registry.TagFilter,
			src.Registry.TopX,
			src.Registry.ImageTemplate,
			httpClient,
		), nil
	default:
		return nil, fmt.Errorf("unsupported source type: %s", src.Type)
	}
}

// buildHTTPClient creates an HTTP client with auth/TLS from a Secret.
func (r *DiscoveryPolicyReconciler) buildHTTPClient(ctx context.Context, secretRef *corev1.LocalObjectReference) (*http.Client, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	if secretRef == nil {
		return client, nil
	}

	secret := &corev1.Secret{}
	// Secrets are namespaced; use kube-system for operator secrets
	key := types.NamespacedName{Name: secretRef.Name, Namespace: "kube-system"}
	if err := r.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("fetching secret %s: %w", secretRef.Name, err)
	}

	transport := &authTransport{
		base:   http.DefaultTransport,
		secret: secret,
	}

	// Configure TLS if cert data is present
	if caCert, ok := secret.Data["ca.crt"]; ok {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)

		tlsConfig := &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}

		if cert, ok := secret.Data["tls.crt"]; ok {
			if key, ok := secret.Data["tls.key"]; ok {
				clientCert, err := tls.X509KeyPair(cert, key)
				if err == nil {
					tlsConfig.Certificates = []tls.Certificate{clientCert}
				}
			}
		}

		transport.base = &http.Transport{TLSClientConfig: tlsConfig}
	}

	client.Transport = transport
	return client, nil
}

// authTransport adds authentication headers from a Secret to HTTP requests.
type authTransport struct {
	base   http.RoundTripper
	secret *corev1.Secret
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Bearer token auth
	if token, ok := t.secret.Data["token"]; ok {
		req.Header.Set("Authorization", "Bearer "+string(token))
	}

	// Basic auth
	if username, ok := t.secret.Data["username"]; ok {
		if password, ok := t.secret.Data["password"]; ok {
			req.SetBasicAuth(string(username), string(password))
		}
	}

	// Custom headers (headers.<name>)
	for key, value := range t.secret.Data {
		if len(key) > 8 && key[:8] == "headers." {
			headerName := key[8:]
			req.Header.Set(headerName, string(value))
		}
	}

	return t.base.RoundTrip(req)
}

// deduplicateResults merges results, keeping the highest score per image.
func deduplicateResults(results []discovery.ImageResult) []discovery.ImageResult {
	seen := make(map[string]discovery.ImageResult, len(results))
	for _, r := range results {
		if existing, ok := seen[r.Image]; ok {
			if r.Score > existing.Score {
				seen[r.Image] = r
			}
		} else {
			seen[r.Image] = r
		}
	}

	deduplicated := make([]discovery.ImageResult, 0, len(seen))
	for _, r := range seen {
		deduplicated = append(deduplicated, r)
	}
	return deduplicated
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiscoveryPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dropv1alpha1.DiscoveryPolicy{}).
		Named("discoverypolicy").
		Complete(r)
}

// sourceEndpoint returns the endpoint URL for a discovery source (for metric labels).
func sourceEndpoint(src dropv1alpha1.DiscoverySource) string {
	switch src.Type {
	case "prometheus":
		if src.Prometheus != nil {
			return src.Prometheus.Endpoint
		}
	case "registry":
		if src.Registry != nil {
			return src.Registry.URL
		}
	}
	return "unknown"
}

// classifyError maps a source fetch error into a k8s-style reason and human-readable message.
func classifyError(err error) (reason, message string) {
	if err == nil {
		return "", ""
	}

	errStr := err.Error()

	// Network-level errors (typed)
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "Timeout", cleanMessage(errStr)
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return reasonDNSError, fmt.Sprintf("cannot resolve host %q", dnsErr.Name)
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			// Check if the underlying error is DNS
			if strings.Contains(opErr.Err.Error(), "lookup") || strings.Contains(opErr.Err.Error(), "no such host") || strings.Contains(opErr.Err.Error(), "server misbehaving") {
				host := extractHost(errStr)
				return reasonDNSError, fmt.Sprintf("cannot resolve host %q", host)
			}
			host := extractHost(errStr)
			return reasonConnectionRefused, fmt.Sprintf("cannot connect to %s", host)
		}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		inner := urlErr.Err.Error()
		if strings.Contains(inner, "no such host") || strings.Contains(inner, "server misbehaving") || strings.Contains(inner, "lookup") {
			host := extractHost(errStr)
			return reasonDNSError, fmt.Sprintf("cannot resolve host %q", host)
		}
		if strings.Contains(inner, "connection refused") {
			host := extractHost(errStr)
			return reasonConnectionRefused, fmt.Sprintf("cannot connect to %s", host)
		}
	}

	// HTTP status-based errors
	if strings.Contains(errStr, "status 401") {
		return "Unauthorized", cleanMessage(errStr)
	}
	if strings.Contains(errStr, "status 403") {
		return "Forbidden", cleanMessage(errStr)
	}
	if strings.Contains(errStr, "status 404") {
		return "NotFound", cleanMessage(errStr)
	}
	if strings.Contains(errStr, "status 5") {
		return "ServerError", cleanMessage(errStr)
	}

	// String-based fallbacks
	if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "server misbehaving") {
		host := extractHost(errStr)
		return reasonDNSError, fmt.Sprintf("cannot resolve host %q", host)
	}
	if strings.Contains(errStr, "connection refused") {
		host := extractHost(errStr)
		return reasonConnectionRefused, fmt.Sprintf("cannot connect to %s", host)
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "Timeout", cleanMessage(errStr)
	}
	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "x509") {
		return "TLSError", cleanMessage(errStr)
	}
	if strings.Contains(errStr, "decoding") || strings.Contains(errStr, "unmarshal") || strings.Contains(errStr, "invalid") {
		return "InvalidResponse", cleanMessage(errStr)
	}

	return "SyncFailed", cleanMessage(errStr)
}

// extractHost pulls the hostname (or host:port) from a Go error string like
// "... lookup nonexistent-prometheus on 10.96.0.10:53 ..." or
// "... dial tcp nonexistent-registry:5000 ..."
func extractHost(errStr string) string {
	// Try "lookup <host> on" pattern (DNS errors)
	if idx := strings.Index(errStr, "lookup "); idx != -1 {
		rest := errStr[idx+len("lookup "):]
		if end := strings.IndexAny(rest, " :"); end != -1 {
			return rest[:end]
		}
		return rest
	}
	// Try to extract from URL pattern "://<host>..."
	if idx := strings.Index(errStr, "://"); idx != -1 {
		rest := errStr[idx+3:]
		if end := strings.IndexAny(rest, "/?"); end != -1 {
			return rest[:end]
		}
		return rest
	}
	return "unknown"
}

// cleanMessage truncates verbose Go error chains for human display.
func cleanMessage(errStr string) string {
	// Take the last meaningful segment after the last colon-space
	parts := strings.Split(errStr, ": ")
	if len(parts) > 2 {
		// Keep last 2 segments for context
		return strings.Join(parts[len(parts)-2:], ": ")
	}
	if len(errStr) > 120 {
		return errStr[:120] + "..."
	}
	return errStr
}
