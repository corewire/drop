package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ImagesCachedTotal counts the total number of images successfully cached on nodes.
	ImagesCachedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "drop_images_cached_total",
			Help: "Total number of images successfully cached on nodes.",
		},
		[]string{"image", "node"},
	)

	// PullDurationSeconds tracks the duration of image pull operations.
	PullDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "drop_pull_duration_seconds",
			Help:    "Duration of image pull operations in seconds.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~68min
		},
		[]string{"image"},
	)

	// PullErrorsTotal counts the total number of failed image pull attempts.
	PullErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "drop_pull_errors_total",
			Help: "Total number of failed image pull attempts.",
		},
		[]string{"image", "node"},
	)

	// DiscoveryImagesFound reports the number of images found by each discovery source.
	DiscoveryImagesFound = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "drop_discovery_images_found",
			Help: "Number of images found by a discovery policy.",
		},
		[]string{"policy", "source_type"},
	)

	// ActivePulls reports the current number of active pull Pods.
	ActivePulls = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "drop_active_pulls",
			Help: "Current number of active image pull Pods.",
		},
	)

	// ReconcileTotal counts reconciliation attempts per controller and result.
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "drop_reconcile_total",
			Help: "Total number of reconciliation attempts.",
		},
		[]string{"controller", "result"},
	)

	// DiscoverySourceHealth reports whether a discovery source is reachable (1=healthy, 0=unhealthy).
	DiscoverySourceHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "drop_discovery_source_health",
			Help: "Whether a discovery source is reachable and queryable (1=healthy, 0=unhealthy).",
		},
		[]string{"policy", "source_type", "endpoint"},
	)

	// DiscoverySourceLatencySeconds tracks the query duration per source.
	DiscoverySourceLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "drop_discovery_source_latency_seconds",
			Help:    "Latency of discovery source queries in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"policy", "source_type"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ImagesCachedTotal,
		PullDurationSeconds,
		PullErrorsTotal,
		DiscoveryImagesFound,
		ActivePulls,
		ReconcileTotal,
		DiscoverySourceHealth,
		DiscoverySourceLatencySeconds,
	)
}
