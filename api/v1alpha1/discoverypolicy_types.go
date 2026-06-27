/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DiscoveryPolicySpec defines the desired state of DiscoveryPolicy.
type DiscoveryPolicySpec struct {
	// Queries is the list of named raw-data sources. Each query is referenced by name from signals.
	// +optional
	Queries []DiscoveryQuery `json:"queries,omitempty"`
	// Signals is the list of named per-image metrics derived from query results.
	// Each signal is referenced by name from the ranking configuration.
	// +optional
	Signals []DiscoverySignal `json:"signals,omitempty"`
	// Ranking defines how signals are combined into a final ordered image list.
	// +optional
	Ranking *DiscoveryRanking `json:"ranking,omitempty"`
	// ImageFilter is a regex applied to discovered image references. Only matching images are kept.
	// Example: "registry.example.com/team/.*" (only keep images from that registry path)
	// +optional
	ImageFilter string `json:"imageFilter,omitempty"`
	// SyncInterval is how often the operator re-runs the pipeline and updates status.discoveredImages.
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

// ============================================================
// Stage 1 — Queries
// ============================================================

// DiscoveryQueryType identifies the backend for a named query.
// +kubebuilder:validation:Enum=prometheus;loki
type DiscoveryQueryType string

const (
	// DiscoveryQueryTypePrometheus fetches time-series data from a Prometheus-compatible API.
	DiscoveryQueryTypePrometheus DiscoveryQueryType = "prometheus"
	// DiscoveryQueryTypeLoki fetches log event data from a Loki-compatible API.
	DiscoveryQueryTypeLoki DiscoveryQueryType = "loki"
)

// DiscoveryQuery defines a named raw-data source referenced by signals.
type DiscoveryQuery struct {
	// Name is the unique identifier for this query within the policy.
	// Signals reference queries by this name via queryRef.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Type selects the backend. Must be "prometheus" or "loki".
	// +kubebuilder:validation:Enum=prometheus;loki
	Type DiscoveryQueryType `json:"type"`
	// Prometheus contains the configuration when type=prometheus.
	// +optional
	Prometheus *DiscoveryPrometheusQuery `json:"prometheus,omitempty"`
	// Loki contains the configuration when type=loki.
	// +optional
	Loki *DiscoveryLokiQuery `json:"loki,omitempty"`
	// SecretRef references a Secret in the pod namespace (default "drop-system") for auth/TLS.
	// Supported Secret keys: token, username, password, ca.crt, tls.crt, tls.key, headers.<name>.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// QueryType defines how the Prometheus query is executed.
// +kubebuilder:validation:Enum=range;instant
type QueryType string

const (
	// QueryTypeRange uses /api/v1/query_range with a time window defined by lookback.
	// Returns multiple data points which are aggregated at the signal stage.
	QueryTypeRange QueryType = "range"
	// QueryTypeInstant uses /api/v1/query for a single point-in-time result.
	// The returned value is used directly as the raw sample value.
	QueryTypeInstant QueryType = "instant"
)

// DiscoveryPrometheusQuery defines the Prometheus-specific query parameters.
// The PromQL result MUST carry an "image" label; that label value is the image reference.
type DiscoveryPrometheusQuery struct {
	// Endpoint is the Prometheus-compatible API URL (Prometheus, Thanos, Mimir, VictoriaMetrics).
	// Example: "http://prometheus.monitoring.svc:9090", "https://mimir.example.com"
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Query is the PromQL expression. Must return results with an "image" label.
	// Example: count(container_memory_working_set_bytes{namespace="gitlab-runner"}) by (image)
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
	// QueryType controls how the query is executed: "range" or "instant". Default: "range".
	// +kubebuilder:default="range"
	// +optional
	QueryType QueryType `json:"queryType,omitempty"`
	// Lookback is the time window for range queries (start=now-lookback, end=now).
	// Required when queryType is "range". Ignored when queryType is "instant".
	// Example: "168h" (7 days), "24h", "72h"
	// +optional
	Lookback *metav1.Duration `json:"lookback,omitempty"`
	// Step is the resolution step for range queries.
	// Smaller steps increase data-point density but also increase Prometheus load.
	// Default: 5m. Example: "1m", "15m"
	// +optional
	Step *metav1.Duration `json:"step,omitempty"`
}

// LokiQueryType defines how the Loki query is executed.
// +kubebuilder:validation:Enum=range
type LokiQueryType string

const (
	// LokiQueryTypeRange uses /loki/api/v1/query_range with a lookback window.
	LokiQueryTypeRange LokiQueryType = "range"
)

// DiscoveryLokiQuery defines the Loki-specific query parameters.
type DiscoveryLokiQuery struct {
	// Endpoint is the Loki API URL.
	// Example: "https://loki.example.com"
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// Query is the LogQL expression.
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`
	// QueryType controls how the query is executed. Currently only "range" is supported.
	// +kubebuilder:default="range"
	// +optional
	QueryType LokiQueryType `json:"queryType,omitempty"`
	// Lookback is the time window for the query (start=now-lookback, end=now).
	// Example: "168h" (7 days), "24h"
	// +optional
	Lookback *metav1.Duration `json:"lookback,omitempty"`
	// Parser configures how log lines are parsed into structured event records.
	// +optional
	Parser *LokiParser `json:"parser,omitempty"`
}

// LokiParserType identifies how Loki log lines are parsed.
// +kubebuilder:validation:Enum=kubernetesEvents
type LokiParserType string

const (
	// LokiParserTypeKubernetesEvents parses Kubernetes Event log lines,
	// extracting pod name, reason, message, and image reference.
	LokiParserTypeKubernetesEvents LokiParserType = "kubernetesEvents"
)

// LokiParser configures structured parsing of Loki log entries.
type LokiParser struct {
	// Type selects the parser. Currently only "kubernetesEvents" is supported.
	// +kubebuilder:validation:Enum=kubernetesEvents
	Type LokiParserType `json:"type"`
	// PodField is the log label or field that contains the pod name.
	// Example: "involvedObject_name"
	// +optional
	PodField string `json:"podField,omitempty"`
	// ReasonField is the log label or field that contains the event reason.
	// Example: "reason"
	// +optional
	ReasonField string `json:"reasonField,omitempty"`
	// MessageField is the log label or field that contains the event message.
	// Example: "message"
	// +optional
	MessageField string `json:"messageField,omitempty"`
	// ImageField is the log label or field from which the image reference is extracted.
	// For kubernetesEvents, the image is parsed out of the message text.
	// Example: "message"
	// +optional
	ImageField string `json:"imageField,omitempty"`
}

// ============================================================
// Stage 2 — Signals
// ============================================================

// SignalType identifies the derivation method for a named signal.
// +kubebuilder:validation:Enum=aggregate;timeWeightedAggregate;windowAggregate;eventPullTime
type SignalType string

const (
	// SignalTypeAggregate aggregates all samples per image using a single method (sum, max, avg, count, min).
	SignalTypeAggregate SignalType = "aggregate"
	// SignalTypeTimeWeightedAggregate applies per-hour-window weights before aggregation.
	SignalTypeTimeWeightedAggregate SignalType = "timeWeightedAggregate"
	// SignalTypeWindowAggregate aggregates only the samples within a specific time sub-window.
	SignalTypeWindowAggregate SignalType = "windowAggregate"
	// SignalTypeEventPullTime derives image pull-time statistics from Loki event records.
	SignalTypeEventPullTime SignalType = "eventPullTime"
)

// AggregationMethod defines how data-point values are combined into a single per-image number.
// +kubebuilder:validation:Enum=sum;count;avg;max;min
type AggregationMethod string

const (
	// AggregationSum adds all data-point values.
	AggregationSum AggregationMethod = "sum"
	// AggregationCount counts the number of data points.
	AggregationCount AggregationMethod = "count"
	// AggregationAvg computes the arithmetic mean of all data-point values.
	AggregationAvg AggregationMethod = "avg"
	// AggregationMax takes the highest single data-point value.
	AggregationMax AggregationMethod = "max"
	// AggregationMin takes the lowest single data-point value.
	AggregationMin AggregationMethod = "min"
)

// DiscoverySignal defines a named per-image metric derived from a single query.
type DiscoverySignal struct {
	// Name is the unique identifier for this signal within the policy.
	// Ranking configurations reference signals by this name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// QueryRef is the name of the query that provides raw data for this signal.
	// Must match a queries[].name within the same policy.
	// +kubebuilder:validation:MinLength=1
	QueryRef string `json:"queryRef"`
	// Type selects the signal derivation method.
	// +kubebuilder:validation:Enum=aggregate;timeWeightedAggregate;windowAggregate;eventPullTime
	Type SignalType `json:"type"`
	// Aggregate is required when type=aggregate.
	// +optional
	Aggregate *AggregateSignalConfig `json:"aggregate,omitempty"`
	// TimeWeightedAggregate is required when type=timeWeightedAggregate.
	// +optional
	TimeWeightedAggregate *TimeWeightedAggregateSignalConfig `json:"timeWeightedAggregate,omitempty"`
	// WindowAggregate is required when type=windowAggregate.
	// +optional
	WindowAggregate *WindowAggregateSignalConfig `json:"windowAggregate,omitempty"`
	// EventPullTime is required when type=eventPullTime.
	// +optional
	EventPullTime *EventPullTimeSignalConfig `json:"eventPullTime,omitempty"`
}

// AggregateSignalConfig configures the aggregate signal type.
type AggregateSignalConfig struct {
	// Method is the aggregation function applied to all samples per image.
	// +kubebuilder:validation:Enum=sum;count;avg;max;min
	Method AggregationMethod `json:"method"`
}

// TimeWeightedAggregateSignalConfig configures the timeWeightedAggregate signal type.
// Each sample value is multiplied by the weight of the matching time window before aggregation.
type TimeWeightedAggregateSignalConfig struct {
	// Method is the aggregation function applied after weighting (currently only "sum" is meaningful).
	// +kubebuilder:validation:Enum=sum;count;avg;max;min
	Method AggregationMethod `json:"method"`
	// Timezone is the IANA time zone used to evaluate window boundaries (wall-clock hours).
	// Example: "Europe/Berlin", "America/New_York", "UTC"
	// +kubebuilder:validation:MinLength=1
	Timezone string `json:"timezone"`
	// DefaultWeight is applied to samples that do not fall in any configured window.
	// Use "0" to exclude off-hours samples entirely.
	DefaultWeight resource.Quantity `json:"defaultWeight"`
	// Windows is the list of hour-of-day windows with associated weights.
	// +kubebuilder:validation:MinItems=1
	Windows []TimeWeightedWindow `json:"windows"`
}

// TimeWeightedWindow defines a wall-clock hour range and its weight factor.
type TimeWeightedWindow struct {
	// StartHour is the inclusive start of the window in local time (0–23).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	StartHour int32 `json:"startHour"`
	// EndHour is the exclusive end of the window in local time (1–24).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=24
	EndHour int32 `json:"endHour"`
	// Weight is the factor applied to sample values within this window.
	// Use "1.0" for full weight, "0.3" for partial, "0" to exclude.
	Weight resource.Quantity `json:"weight"`
}

// WindowAggregateSignalConfig configures the windowAggregate signal type.
// Exactly one of relativeWindow or (window + timezone) must be set.
type WindowAggregateSignalConfig struct {
	// Method is the aggregation function applied to the windowed samples.
	// +kubebuilder:validation:Enum=sum;count;avg;max;min
	Method AggregationMethod `json:"method"`
	// RelativeWindow aggregates only samples from the last N duration before now.
	// Mutually exclusive with window + timezone.
	// Example: "2h" (last 2 hours)
	// +optional
	RelativeWindow *metav1.Duration `json:"relativeWindow,omitempty"`
	// Timezone is the IANA time zone for evaluating wall-clock window boundaries.
	// Required when window is set.
	// +optional
	Timezone string `json:"timezone,omitempty"`
	// Window defines fixed wall-clock start/end times within each day.
	// Mutually exclusive with relativeWindow.
	// +optional
	Window *TimeOfDayWindow `json:"window,omitempty"`
}

// TimeOfDayWindow defines a fixed wall-clock time range within each day.
type TimeOfDayWindow struct {
	// Start is the inclusive start time in "HH:MM" format (24-hour, local time).
	// Example: "09:00"
	// +kubebuilder:validation:Pattern=`^([01][0-9]|2[0-3]):[0-5][0-9]$`
	Start string `json:"start"`
	// End is the exclusive end time in "HH:MM" format (24-hour, local time).
	// Example: "17:00"
	// +kubebuilder:validation:Pattern=`^([01][0-9]|2[0-3]):[0-5][0-9]$`
	End string `json:"end"`
}

// EventPullTimeStatistic defines which pull-time statistic to derive from event records.
// +kubebuilder:validation:Enum=p50;p90;p95;avg;max;count;failureCount;cacheHitCount
type EventPullTimeStatistic string

const (
	// EventPullTimeStatisticP50 is the median cold-pull duration.
	EventPullTimeStatisticP50 EventPullTimeStatistic = "p50"
	// EventPullTimeStatisticP90 is the 90th-percentile cold-pull duration.
	EventPullTimeStatisticP90 EventPullTimeStatistic = "p90"
	// EventPullTimeStatisticP95 is the 95th-percentile cold-pull duration.
	EventPullTimeStatisticP95 EventPullTimeStatistic = "p95"
	// EventPullTimeStatisticAvg is the mean cold-pull duration.
	EventPullTimeStatisticAvg EventPullTimeStatistic = "avg"
	// EventPullTimeStatisticMax is the maximum observed cold-pull duration.
	EventPullTimeStatisticMax EventPullTimeStatistic = "max"
	// EventPullTimeStatisticCount is the total number of cold-pull events.
	EventPullTimeStatisticCount EventPullTimeStatistic = "count"
	// EventPullTimeStatisticFailureCount is the total number of pull failures.
	EventPullTimeStatisticFailureCount EventPullTimeStatistic = "failureCount"
	// EventPullTimeStatisticCacheHitCount is the number of cache-hit events.
	EventPullTimeStatisticCacheHitCount EventPullTimeStatistic = "cacheHitCount"
)

// DurationMode defines how pull duration is extracted from event records.
// +kubebuilder:validation:Enum=eventPair;messageDuration
type DurationMode string

const (
	// DurationModeEventPair computes duration as Pulled.timestamp - Pulling.timestamp
	// for the same Pod/image pair.
	DurationModeEventPair DurationMode = "eventPair"
	// DurationModeMessageDuration parses the duration directly from the Pulled event message
	// (e.g., "Successfully pulled image ... in 42.3s").
	DurationModeMessageDuration DurationMode = "messageDuration"
)

// EventPullTimeSignalConfig configures the eventPullTime signal type.
// The referenced query must be a Loki query.
type EventPullTimeSignalConfig struct {
	// Statistic selects which pull-time metric to compute.
	// +kubebuilder:validation:Enum=p50;p90;p95;avg;max;count;failureCount;cacheHitCount
	Statistic EventPullTimeStatistic `json:"statistic"`
	// IncludeCacheHits controls whether "already present on machine" events are included
	// in cold-pull duration statistics. Set to false to exclude cache hits.
	// +kubebuilder:default=false
	IncludeCacheHits bool `json:"includeCacheHits"`
	// DurationMode controls how pull duration is extracted from event records.
	// +kubebuilder:validation:Enum=eventPair;messageDuration
	DurationMode DurationMode `json:"durationMode"`
}

// ============================================================
// Stage 3 — Ranking
// ============================================================

// RankingStrategy identifies which ranking algorithm is applied.
// +kubebuilder:validation:Enum=signal;weightedSum;modelExposure
type RankingStrategy string

const (
	// RankingStrategySignal ranks images directly by the value of a single signal.
	RankingStrategySignal RankingStrategy = "signal"
	// RankingStrategyWeightedSum combines normalized signals using a weighted sum.
	RankingStrategyWeightedSum RankingStrategy = "weightedSum"
	// RankingStrategyModelExposure ranks images by expected post-rotation cold-node exposure.
	RankingStrategyModelExposure RankingStrategy = "modelExposure"
)

// DiscoveryRanking defines how signals are combined into the final ordered image list.
type DiscoveryRanking struct {
	// Strategy selects the ranking algorithm.
	// +kubebuilder:validation:Enum=signal;weightedSum;modelExposure
	Strategy RankingStrategy `json:"strategy"`
	// Signal is required when strategy=signal.
	// +optional
	Signal *SignalRankingConfig `json:"signal,omitempty"`
	// WeightedSum is required when strategy=weightedSum.
	// +optional
	WeightedSum *WeightedSumRankingConfig `json:"weightedSum,omitempty"`
	// ModelExposure is required when strategy=modelExposure.
	// +optional
	ModelExposure *ModelExposureRankingConfig `json:"modelExposure,omitempty"`
}

// SignalRankingConfig configures the signal ranking strategy.
type SignalRankingConfig struct {
	// SignalRef is the name of the signal whose values determine image rank.
	// Must match a signals[].name within the same policy.
	// +kubebuilder:validation:MinLength=1
	SignalRef string `json:"signalRef"`
}

// NormalizeMethod defines how signal values are normalized before weighted combination.
// +kubebuilder:validation:Enum=minMax
type NormalizeMethod string

const (
	// NormalizeMethodMinMax applies min-max normalization: (x - min) / (max - min).
	// When all values are equal, normalized(x) = 1.
	NormalizeMethodMinMax NormalizeMethod = "minMax"
)

// MissingSignalBehavior defines what happens when an image has no value for a required signal.
// +kubebuilder:validation:Enum=zero;drop
type MissingSignalBehavior string

const (
	// MissingSignalBehaviorZero treats a missing signal value as zero.
	MissingSignalBehaviorZero MissingSignalBehavior = "zero"
	// MissingSignalBehaviorDrop removes the image from ranking if any required signal is missing.
	MissingSignalBehaviorDrop MissingSignalBehavior = "drop"
)

// WeightedSumTerm defines one signal contribution in a weightedSum ranking.
type WeightedSumTerm struct {
	// SignalRef is the name of the signal to include in the weighted sum.
	// Must match a signals[].name within the same policy.
	// +kubebuilder:validation:MinLength=1
	SignalRef string `json:"signalRef"`
	// Weight is the factor applied to the normalized signal value.
	// All weights should be non-negative; they do not need to sum to 1.
	// Example: "0.7"
	Weight resource.Quantity `json:"weight"`
}

// WeightedSumRankingConfig configures the weightedSum ranking strategy.
// Score = Σ weight_k * normalize(signal_k(image)).
type WeightedSumRankingConfig struct {
	// Normalize selects the normalization method applied to each signal before weighting.
	// Currently only "minMax" is supported.
	// +kubebuilder:validation:Enum=minMax
	// +kubebuilder:default="minMax"
	Normalize NormalizeMethod `json:"normalize"`
	// MissingSignal controls behavior when an image has no value for a required signal.
	// "zero" treats missing as 0; "drop" removes the image from ranking.
	// +kubebuilder:validation:Enum=zero;drop
	// +kubebuilder:default="zero"
	MissingSignal MissingSignalBehavior `json:"missingSignal"`
	// Terms is the list of signals and their weights.
	// +kubebuilder:validation:MinItems=1
	Terms []WeightedSumTerm `json:"terms"`
}

// ModelExposureRankingConfig configures the modelExposure ranking strategy.
// Score = J_target(I) * (1 - 1/N)^J_pre(I) * p_hat(I)
// where N=nodeCount, J_pre is pre-window usage, J_target is target-window usage,
// and p_hat is the pull-time signal value.
type ModelExposureRankingConfig struct {
	// NodeCount is the number of eligible CI nodes (N in the exposure formula).
	// +kubebuilder:validation:Minimum=1
	NodeCount int32 `json:"nodeCount"`
	// PreWindowUsageSignalRef is the name of the signal representing usage before the target window.
	// Must match a signals[].name within the same policy.
	// +kubebuilder:validation:MinLength=1
	PreWindowUsageSignalRef string `json:"preWindowUsageSignalRef"`
	// TargetWindowUsageSignalRef is the name of the signal representing usage during the target window.
	// Must match a signals[].name within the same policy.
	// +kubebuilder:validation:MinLength=1
	TargetWindowUsageSignalRef string `json:"targetWindowUsageSignalRef"`
	// PullTimeSignalRef is the name of the signal providing per-image pull-time estimates.
	// Must match a signals[].name within the same policy.
	// +kubebuilder:validation:MinLength=1
	PullTimeSignalRef string `json:"pullTimeSignalRef"`
}

// ============================================================
// Status
// ============================================================

// QueryResultStatus reports whether a named query succeeded or failed.
// +kubebuilder:validation:Enum=success;failed
type QueryResultStatus string

const (
	// QueryResultStatusSuccess indicates the query executed without errors.
	QueryResultStatusSuccess QueryResultStatus = "success"
	// QueryResultStatusFailed indicates the query encountered an error.
	QueryResultStatusFailed QueryResultStatus = "failed"
)

// QueryResult reports the outcome of a single named query execution.
type QueryResult struct {
	// Name matches the queries[].name that produced this result.
	Name string `json:"name"`
	// Type is the query backend type (prometheus or loki).
	Type DiscoveryQueryType `json:"type"`
	// Series is the number of time-series returned (Prometheus queries only).
	// +optional
	Series *int32 `json:"series,omitempty"`
	// Samples is the total number of data points across all series (Prometheus range queries only).
	// +optional
	Samples *int64 `json:"samples,omitempty"`
	// Records is the number of log records returned (Loki queries only).
	// +optional
	Records *int64 `json:"records,omitempty"`
	// Status is "success" or "failed".
	Status QueryResultStatus `json:"status"`
	// Message describes the failure reason when status=failed.
	// +optional
	Message string `json:"message,omitempty"`
}

// SignalResult reports the outcome of a single signal derivation.
type SignalResult struct {
	// Name matches the signals[].name that produced this result.
	Name string `json:"name"`
	// Images is the number of images for which this signal produced a value.
	Images int32 `json:"images"`
	// Status is "success" or "failed".
	Status string `json:"status"`
	// Message describes the failure reason when status=failed.
	// +optional
	Message string `json:"message,omitempty"`
}

// ImageSignalValue records the raw and normalized value of a signal for one image.
type ImageSignalValue struct {
	// Name is the signal name.
	Name string `json:"name"`
	// RawValue is the unscaled signal value as a decimal string.
	RawValue string `json:"rawValue"`
	// NormalizedValue is the normalized value (after minMax or other normalization) as a decimal string.
	// Only populated for signals used in a weightedSum ranking.
	// +optional
	NormalizedValue string `json:"normalizedValue,omitempty"`
}

// RankingTerm records the contribution of one signal to the final score of an image.
type RankingTerm struct {
	// Signal is the signal name.
	Signal string `json:"signal"`
	// Weight is the configured weight as a decimal string.
	Weight string `json:"weight"`
	// Contribution is weight * normalizedValue as a decimal string.
	Contribution string `json:"contribution"`
}

// ImageRankingDetail explains how the final score was computed for one image.
type ImageRankingDetail struct {
	// Strategy is the ranking strategy that produced this detail.
	Strategy string `json:"strategy"`
	// Terms lists the per-signal contributions (populated for weightedSum and modelExposure).
	// +optional
	Terms []RankingTerm `json:"terms,omitempty"`
}

// DiscoveredImage represents a single discovered and ranked image.
type DiscoveredImage struct {
	// Image is the fully qualified image reference.
	Image string `json:"image"`
	// Rank is the position of this image in the final ordered list (1 = highest score).
	Rank int32 `json:"rank"`
	// FinalScore is the computed ranking score as a decimal string.
	FinalScore string `json:"finalScore"`
	// Selected is true when this image is within the maxImages cap and will be
	// propagated to dependent CachedImageSet resources.
	Selected bool `json:"selected"`
	// Signals lists the per-signal values used during ranking (for observability).
	// +optional
	Signals []ImageSignalValue `json:"signals,omitempty"`
	// Ranking explains how the final score was computed.
	// +optional
	Ranking *ImageRankingDetail `json:"ranking,omitempty"`
}

// DiscoveryPolicyStatus defines the observed state of DiscoveryPolicy.
type DiscoveryPolicyStatus struct {
	// LastSyncTime is the timestamp of the last reconciliation attempt.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
	// QueryResults reports the outcome of each named query execution.
	// +optional
	QueryResults []QueryResult `json:"queryResults,omitempty"`
	// SignalResults reports the outcome of each signal derivation.
	// +optional
	SignalResults []SignalResult `json:"signalResults,omitempty"`
	// DiscoveredImages is the ordered list of discovered and ranked images.
	// Only images with selected=true are propagated to dependent CachedImageSet resources.
	// +optional
	DiscoveredImages []DiscoveredImage `json:"discoveredImages,omitempty"`
	// ImageCount is the number of selected discovered images.
	// +optional
	ImageCount int32 `json:"imageCount,omitempty"`
	// QueryCount is the number of configured queries.
	// +optional
	QueryCount int32 `json:"queryCount,omitempty"`
	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=drop
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Queries",type=integer,JSONPath=`.status.queryCount`
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
