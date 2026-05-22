/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pullerv1alpha1 "github.com/Breee/puller/api/v1alpha1"
	"github.com/Breee/puller/internal/discovery"
)

// DiscoveryPolicyReconciler reconciles a DiscoveryPolicy object
type DiscoveryPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=puller.corewire.io,resources=discoverypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=puller.corewire.io,resources=discoverypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=puller.corewire.io,resources=discoverypolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile queries discovery sources and updates the DiscoveryPolicy status.
func (r *DiscoveryPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch DiscoveryPolicy
	dp := &pullerv1alpha1.DiscoveryPolicy{}
	if err := r.Get(ctx, req.NamespacedName, dp); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Query each source
	var allResults []discovery.ImageResult
	allSourcesHealthy := true

	for i, src := range dp.Spec.Sources {
		source, err := r.buildSource(ctx, src)
		if err != nil {
			log.Error(err, "building source", "index", i, "type", src.Type)
			allSourcesHealthy = false
			continue
		}

		results, err := source.Fetch(ctx)
		if err != nil {
			log.Error(err, "fetching from source", "index", i, "type", src.Type)
			allSourcesHealthy = false
			continue
		}

		// Tag results with source type
		for j := range results {
			_ = j
		}
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
		return merged[i].Score > merged[j].Score
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
		discoveredImages := make([]pullerv1alpha1.DiscoveredImage, 0, len(merged))
		for _, r := range merged {
			discoveredImages = append(discoveredImages, pullerv1alpha1.DiscoveredImage{
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
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            fmt.Sprintf("Discovered %d images", len(dp.Status.DiscoveredImages)),
	}
	meta.SetStatusCondition(&dp.Status.Conditions, readyCondition)

	if err := r.Status().Update(ctx, dp); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	// 8. Requeue after sync interval
	syncInterval := dp.Spec.SyncInterval.Duration
	if syncInterval == 0 {
		syncInterval = 30 * time.Minute
	}

	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// buildSource creates the appropriate Source implementation from a DiscoverySource config.
func (r *DiscoveryPolicyReconciler) buildSource(ctx context.Context, src pullerv1alpha1.DiscoverySource) (discovery.Source, error) {
	httpClient, err := r.buildHTTPClient(ctx, src.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("building HTTP client: %w", err)
	}

	switch src.Type {
	case "prometheus":
		if src.Prometheus == nil {
			return nil, fmt.Errorf("prometheus config is required when type=prometheus")
		}
		return discovery.NewPrometheusSource(src.Prometheus.Endpoint, src.Prometheus.Query, httpClient), nil
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
		For(&pullerv1alpha1.DiscoveryPolicy{}).
		Named("discoverypolicy").
		Complete(r)
}
