/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DiscoveryPolicySpec defines the desired state of DiscoveryPolicy.
type DiscoveryPolicySpec struct {
	// Sources is the list of discovery backends to query. At least one source is required.
	// Multiple sources are merged and ranked together before maxImages is applied.
	// +kubebuilder:validation:MinItems=1
	Sources []DiscoverySource `json:"sources"`
	// ImageFilter is a regex applied to discovered image references. Only matching images are kept.
	// Example: "registry.example.com/team/.*" (only keep images from that registry path)
	// +optional
	ImageFilter string `json:"imageFilter,omitempty"`
	// SyncInterval is how often the operator re-queries all sources and updates status.discoveredImages.
	// Default: "30m". Example: "1h", "15m"
	// +kubebuilder:default="30m"
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
	// MaxImages caps the total number of images stored in status.discoveredImages.
	// Images are ranked by score; lowest-scoring images are dropped when the cap is exceeded.
	// Default: 50. Example: 30, 100
	// +kubebuilder:default=50
	// +kubebuilder:validation:Minimum=1
	MaxImages int32 `json:"maxImages,omitempty"`
}

// DiscoverySource defines a single discovery backend.
type DiscoverySource struct {
	// Type identifies the discovery backend. Must be "prometheus" or "registry".
	// +kubebuilder:validation:Enum=prometheus;registry
	Type string `json:"type"`
	// Prometheus contains the configuration when type=prometheus.
	// +optional
	Prometheus *PrometheusSource `json:"prometheus,omitempty"`
	// Registry contains the configuration when type=registry.
	// +optional
	Registry *RegistrySource `json:"registry,omitempty"`
	// SecretRef references a Secret in the namespace where Drop creates pull Pods.
	// The default namespace is "drop-system" unless the controller is started with a different --pod-namespace.
	// Supported Secret keys: token, username, password, ca.crt, tls.crt, tls.key, headers.<name>.
	// Example: {name: "prometheus-creds"}
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// AggregationMethod defines how range query values are aggregated into a score.
// +kubebuilder:validation:Enum=sum;count;avg;max
type AggregationMethod string

const (
	// AggregationSum adds all data-point values over the lookback window.
	// Use when the query returns a gauge/counter and the total magnitude matters
	// (e.g., total memory usage across the window).
	AggregationSum AggregationMethod = "sum"
	// AggregationCount counts the number of non-zero data points over the lookback window.
	// Use when you want to rank by how frequently an image appears
	// (e.g., number of sample intervals where the image was running).
	AggregationCount AggregationMethod = "count"
	// AggregationAvg computes the arithmetic mean of all data-point values.
	// Use when you want the average magnitude regardless of how many samples exist.
	AggregationAvg AggregationMethod = "avg"
	// AggregationMax takes the highest single data-point value.
	// Use when peak usage is more relevant than cumulative usage.
	AggregationMax AggregationMethod = "max"
)

// QueryType defines how the Prometheus query is executed.
// +kubebuilder:validation:Enum=range;instant
type QueryType string

const (
	// QueryTypeRange uses /api/v1/query_range with a time window defined by lookback.
	// Returns multiple data points which are aggregated using the aggregationMethod.
	QueryTypeRange QueryType = "range"
	// QueryTypeInstant uses /api/v1/query for a single point-in-time result.
	// The returned value is used directly as the score.
	QueryTypeInstant QueryType = "instant"
)

// PrometheusSource defines Prometheus query configuration for image discovery.
type PrometheusSource struct {
	// Endpoint is the Prometheus-compatible API URL (Prometheus, Thanos, Mimir, VictoriaMetrics).
	// Example: "http://prometheus.monitoring.svc:9090", "https://mimir.example.com"
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Query is the PromQL expression. It MUST return results with an "image" label —
	// that label value is used as the discovered image reference.
	// The query result value is used as the ranking score (higher = more relevant).
	// Example: count(container_memory_working_set_bytes{container!="",container!="POD",namespace="gitlab-runner"}) by (image)
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
	// QueryType controls how the Prometheus query is executed.
	// "range" uses /api/v1/query_range with a time window defined by lookback.
	// "instant" uses /api/v1/query for a single point-in-time result.
	// Default: "range".
	// +kubebuilder:default="range"
	// +optional
	QueryType QueryType `json:"queryType,omitempty"`
	// Lookback is the time window for range queries. When queryType is "range",
	// the operator queries (start=now-lookback, end=now) and aggregates all returned values per image.
	// The aggregation function is controlled by the aggregationMethod field.
	// Required when queryType is "range". Ignored when queryType is "instant".
	// Example: "168h" (7 days), "24h", "72h"
	// +optional
	Lookback *metav1.Duration `json:"lookback,omitempty"`
	// AggregationMethod controls how data points from a range query are combined into a single score.
	// Only used when queryType is "range". Ignored for instant queries.
	// Default: "sum". Options: "sum", "count", "avg", "max"
	// +kubebuilder:default="sum"
	// +optional
	AggregationMethod AggregationMethod `json:"aggregationMethod,omitempty"`
	// Step is the resolution step for range queries (only used when lookback is set).
	// Smaller steps = more data points = more accurate aggregation but higher Prometheus load.
	// Default: "5m". Example: "1m", "15m"
	// +kubebuilder:default="5m"
	// +optional
	Step string `json:"step,omitempty"`
}

// RegistrySource defines OCI registry tag listing configuration for image discovery.
type RegistrySource struct {
	// URL is the registry base URL (without repository path).
	// Example: "https://registry.example.com", "https://ghcr.io"
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
	// Repositories is the list of repository paths to list tags from.
	// Example: ["team/app", "team/worker", "infra/tools"]
	// +kubebuilder:validation:MinItems=1
	Repositories []string `json:"repositories"`
	// TagFilter is a regex applied to tag names. Only matching tags are discovered.
	// Example: "^v[0-9]+\\." (semver tags only), "^main-" (main branch builds)
	// +optional
	TagFilter string `json:"tagFilter,omitempty"`
	// TopX limits the number of tags kept per repository after tagFilter is applied.
	// The registry API does not provide creation timestamps here; Drop keeps the last N tags returned by the registry.
	// Example: 3 (keep the last 3 matching tags returned per repo)
	// +optional
	// +kubebuilder:validation:Minimum=1
	TopX int32 `json:"topX,omitempty"`
	// ImageTemplate is a Go text/template for constructing the full image reference from discovered tags.
	// Available variables: {{.Registry}}, {{.Repository}}, {{.Tag}}
	// Default (when unset): "{{.Registry}}/{{.Repository}}:{{.Tag}}"
	// Example: "{{.Registry}}/{{.Repository}}@{{.Tag}}" (if tags are actually digests)
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
// +kubebuilder:resource:scope=Cluster,categories=drop
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Sources",type=integer,JSONPath=`.status.sourceCount`
// +kubebuilder:printcolumn:name="Images",type=integer,JSONPath=`.status.imageCount`
// +kubebuilder:printcolumn:name="LastSync",type=date,JSONPath=`.status.lastSyncTime`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DiscoveryPolicy automatically discovers images from registries or Prometheus metrics.
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
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &DiscoveryPolicy{}, &DiscoveryPolicyList{})
		return nil
	})
}
