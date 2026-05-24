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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DiscoveryPolicySpec defines the desired state of DiscoveryPolicy.
type DiscoveryPolicySpec struct {
	// Sources is the list of discovery backends to query.
	// +kubebuilder:validation:MinItems=1
	Sources []DiscoverySource `json:"sources"`
	// ImageFilter is a regex to filter discovered images.
	// +optional
	ImageFilter string `json:"imageFilter,omitempty"`
	// SyncInterval is how often to re-query sources.
	// +kubebuilder:default="30m"
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
	// MaxImages caps the number of discovered images.
	// +kubebuilder:default=50
	// +kubebuilder:validation:Minimum=1
	MaxImages int32 `json:"maxImages,omitempty"`
}

// DiscoverySource defines a single discovery backend.
type DiscoverySource struct {
	// Type identifies the backend.
	// +kubebuilder:validation:Enum=prometheus;registry
	Type string `json:"type"`
	// Prometheus config (when type=prometheus).
	// +optional
	Prometheus *PrometheusSource `json:"prometheus,omitempty"`
	// Registry config (when type=registry).
	// +optional
	Registry *RegistrySource `json:"registry,omitempty"`
	// SecretRef references a Secret for auth/TLS for this source.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// PrometheusSource defines Prometheus query configuration.
type PrometheusSource struct {
	// Endpoint is the Prometheus API URL.
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Query is the PromQL query that must return an 'image' label.
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
	// Lookback is the time window to aggregate over (e.g. "7d", "24h").
	// When set, uses query_range and sums values to rank by total usage.
	// When unset, uses an instant query (point-in-time).
	// +optional
	Lookback *metav1.Duration `json:"lookback,omitempty"`
	// Step is the query resolution step for range queries.
	// +kubebuilder:default="5m"
	// +optional
	Step string `json:"step,omitempty"`
}

// RegistrySource defines OCI registry tag listing configuration.
type RegistrySource struct {
	// URL is the registry base URL.
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// Repositories is the list of repositories to query.
	// +kubebuilder:validation:MinItems=1
	Repositories []string `json:"repositories"`
	// TagFilter is a regex to filter tags.
	// +optional
	TagFilter string `json:"tagFilter,omitempty"`
	// TopX limits the number of tags to fetch per repository.
	// +optional
	// +kubebuilder:validation:Minimum=1
	TopX int32 `json:"topX,omitempty"`
	// ImageTemplate is a Go text/template for constructing the full image reference.
	// Available variables: .Registry, .Repository, .Tag
	// +optional
	ImageTemplate string `json:"imageTemplate,omitempty"`
}

// DiscoveryPolicyStatus defines the observed state of DiscoveryPolicy.
type DiscoveryPolicyStatus struct {
	// LastSyncTime is the timestamp of the last successful sync.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
	// DiscoveredImages is the list of discovered images from all sources.
	// +optional
	DiscoveredImages []DiscoveredImage `json:"discoveredImages,omitempty"`
	// ImageCount is the number of discovered images.
	// +optional
	ImageCount int32 `json:"imageCount,omitempty"`
	// SourceCount is the number of configured sources.
	// +optional
	SourceCount int32 `json:"sourceCount,omitempty"`
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DiscoveredImage represents a single discovered image with metadata.
type DiscoveredImage struct {
	// Image is the fully qualified image reference.
	Image string `json:"image"`
	// Score is the ranking score from the source (higher = more relevant).
	Score int64 `json:"score"`
	// Source identifies which discovery source produced this image.
	Source string `json:"source"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=puller
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.status.sourceCount`
// +kubebuilder:printcolumn:name="Images",type=integer,JSONPath=`.status.imageCount`
// +kubebuilder:printcolumn:name="LastSync",type=date,JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DiscoveryPolicy is the Schema for the discoverypolicies API.
type DiscoveryPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DiscoveryPolicySpec   `json:"spec,omitempty"`
	Status DiscoveryPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DiscoveryPolicyList contains a list of DiscoveryPolicy.
type DiscoveryPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiscoveryPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DiscoveryPolicy{}, &DiscoveryPolicyList{})
}
