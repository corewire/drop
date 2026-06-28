/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
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

	dropv1alpha1 "github.com/corewire/drop/api/v1alpha1"
	"github.com/corewire/drop/internal/discovery"
	dropmetrics "github.com/corewire/drop/internal/metrics"
)

// DiscoveryPolicyReconciler reconciles a DiscoveryPolicy object
type DiscoveryPolicyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SecretNamespace string
}

const (
	reasonDNSError          = "DNSError"
	reasonConnectionRefused = "ConnectionRefused"
	secretHeaderPrefix      = "headers."
)

// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile executes the query/signal/ranking pipeline for a DiscoveryPolicy and updates status.
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

	log.Info("reconciling DiscoveryPolicy",
		"queries", len(dp.Spec.Queries),
		"signals", len(dp.Spec.Signals),
	)

	// 2. Execute pipeline
	httpClientFunc := r.buildHTTPClientFunc(dp)
	result := discovery.ExecutePipeline(ctx, dp.Spec, httpClientFunc)

	// 3. Build status patch
	patch := client.MergeFrom(dp.DeepCopy())
	now := metav1.Now()

	dp.Status.LastSyncTime = &now
	dp.Status.QueryResults = result.QueryResults
	dp.Status.DiscoveredImages = result.Images
	dp.Status.ImageCount = int32(len(result.Images))

	// Determine overall health from query results
	allHealthy, failReason, failMsg := summarizeQueryResults(result.QueryResults)

	// Emit per-query metrics
	for _, qr := range result.QueryResults {
		healthy := float64(0)
		if qr.Status == dropv1alpha1.QueryResultStatusSuccess {
			healthy = 1
		}
		dropmetrics.DiscoverySourceHealth.WithLabelValues(dp.Name, string(qr.Type), qr.Name).Set(healthy)
	}

	// 4. Set Ready condition
	readyCondition := metav1.Condition{
		Type:               conditionTypeReady,
		ObservedGeneration: dp.Generation,
		LastTransitionTime: now,
	}
	if allHealthy || len(result.Images) > 0 {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "Synced"
		readyCondition.Message = fmt.Sprintf("Pipeline executed successfully; %d images discovered.", len(result.Images))
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = failReason
		readyCondition.Message = failMsg
	}
	meta.SetStatusCondition(&dp.Status.Conditions, readyCondition)

	if err := r.Status().Patch(ctx, dp, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching status: %w", err)
	}

	// 5. Requeue after sync interval
	syncInterval := dp.Spec.SyncInterval.Duration
	if syncInterval == 0 {
		syncInterval = 30 * time.Minute
	}

	// Return an error to trigger rate-limited backoff when all queries failed and no images available.
	if !allHealthy && len(result.Images) == 0 {
		return ctrl.Result{}, fmt.Errorf("discovery sync failed: %s", failMsg)
	}

	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// buildHTTPClientFunc returns a discovery.HTTPClientFunc that provides per-query auth/TLS clients.
func (r *DiscoveryPolicyReconciler) buildHTTPClientFunc(dp *dropv1alpha1.DiscoveryPolicy) discovery.HTTPClientFunc {
	// Build a name → secretRef index for quick lookup
	secretIndex := make(map[string]*corev1.LocalObjectReference, len(dp.Spec.Queries))
	for _, q := range dp.Spec.Queries {
		if q.SecretRef != nil {
			secretIndex[q.Name] = q.SecretRef
		}
	}

	return func(innerCtx context.Context, queryName string) (*http.Client, error) {
		secretRef, hasSecret := secretIndex[queryName]
		if !hasSecret {
			return &http.Client{Timeout: 30 * time.Second}, nil
		}
		return r.buildHTTPClient(innerCtx, secretRef)
	}
}

// summarizeQueryResults determines overall health and a human-readable reason/message.
func summarizeQueryResults(qrs []dropv1alpha1.QueryResult) (allHealthy bool, reason, message string) {
	if len(qrs) == 0 {
		return true, "Synced", "No queries configured."
	}

	var failures []string
	for _, qr := range qrs {
		if qr.Status != dropv1alpha1.QueryResultStatusSuccess {
			failures = append(failures, fmt.Sprintf("%s: %s", qr.Name, qr.Message))
		}
	}

	if len(failures) == 0 {
		return true, "Synced", ""
	}

	// Classify the first failure for the Reason field
	reason = classifyReason(failures[0])
	message = strings.Join(failures, "; ")
	return false, reason, message
}

// classifyReason maps a failure message to a k8s-style reason string.
func classifyReason(msg string) string {
	switch {
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "server misbehaving") || strings.Contains(msg, "lookup"):
		return reasonDNSError
	case strings.Contains(msg, "connection refused"):
		return reasonConnectionRefused
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return "Timeout"
	case strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized"):
		return "Unauthorized"
	case strings.Contains(msg, "403") || strings.Contains(msg, "Forbidden"):
		return "Forbidden"
	case strings.Contains(msg, "404") || strings.Contains(msg, "NotFound"):
		return "NotFound"
	case strings.Contains(msg, "certificate") || strings.Contains(msg, "x509"):
		return "TLSError"
	default:
		return "SyncFailed"
	}
}

// buildHTTPClient creates an HTTP client with auth/TLS from a Secret.
func (r *DiscoveryPolicyReconciler) buildHTTPClient(ctx context.Context, secretRef *corev1.LocalObjectReference) (*http.Client, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	if secretRef == nil {
		return httpClient, nil
	}

	secret := &corev1.Secret{}
	secretNamespace := r.SecretNamespace
	if secretNamespace == "" {
		secretNamespace = "drop-system"
	}
	key := types.NamespacedName{Name: secretRef.Name, Namespace: secretNamespace}
	if err := r.Get(ctx, key, secret); err != nil {
		return nil, fmt.Errorf("fetching secret %s/%s: %w", secretNamespace, secretRef.Name, err)
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

	httpClient.Transport = transport
	return httpClient, nil
}

// authTransport adds authentication headers from a Secret to HTTP requests.
type authTransport struct {
	base   http.RoundTripper
	secret *corev1.Secret
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// ****** auth
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
		if strings.HasPrefix(key, secretHeaderPrefix) {
			headerName := key[len(secretHeaderPrefix):]
			req.Header.Set(headerName, string(value))
		}
	}

	return t.base.RoundTrip(req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiscoveryPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dropv1alpha1.DiscoveryPolicy{}).
		Named("discoverypolicy").
		Complete(r)
}
