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
)

// DiscoveryPolicyReconciler reconciles a DiscoveryPolicy object
type DiscoveryPolicyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SecretNamespace string
}

// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=drop.corewire.io,resources=discoverypolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile updates the DiscoveryPolicy status.
// NOTE: Query/signal/ranking execution is not yet implemented. The controller sets a
// NotImplemented condition and requeues after syncInterval until a future release adds execution.
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

	log.Info("reconciling DiscoveryPolicy (pipeline execution not yet implemented)",
		"queries", len(dp.Spec.Queries),
		"signals", len(dp.Spec.Signals),
	)

	// 2. Update status with query/image counts and NotImplemented condition.
	patch := client.MergeFrom(dp.DeepCopy())

	now := metav1.Now()
	dp.Status.LastSyncTime = &now
	dp.Status.QueryCount = int32(len(dp.Spec.Queries))
	dp.Status.ImageCount = int32(len(dp.Status.DiscoveredImages))

	readyCondition := metav1.Condition{
		Type:               conditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             "NotImplemented",
		Message:            "Query/signal/ranking pipeline execution is not yet implemented; discovered images will be populated in a future release.",
		ObservedGeneration: dp.Generation,
		LastTransitionTime: now,
	}
	meta.SetStatusCondition(&dp.Status.Conditions, readyCondition)

	if err := r.Status().Patch(ctx, dp, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching status: %w", err)
	}

	// 3. Requeue after sync interval.
	syncInterval := dp.Spec.SyncInterval.Duration
	if syncInterval == 0 {
		syncInterval = 30 * time.Minute
	}
	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// buildHTTPClient creates an HTTP client with auth/TLS from a Secret.
// This is retained for use by future query execution (Issues 2 and 8).
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
		if strings.HasPrefix(key, "headers.") {
			headerName := key[len("headers."):]
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
