---
title: Monitoring
weight: 4
aliases:
  - /drop/docs/observability/
description: Prometheus metrics, events, and health checks.
llmsDescription: |
  Monitoring for drop: Prometheus metrics (drop_images_cached_total,
  drop_pull_errors_total, drop_pull_duration_seconds, etc.), Kubernetes
  events on CachedImage/CachedImageSet, and metav1.Condition status with
  type Ready. ServiceMonitor included for Prometheus Operator integration.
---

## Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `drop_images_cached_total` | Counter | `image`, `node` | Total images successfully cached |
| `drop_pull_duration_seconds` | Histogram | `image` | Duration of pull operations |
| `drop_pull_errors_total` | Counter | `image`, `node` | Total failed pull attempts |
| `drop_discovery_images_found` | Gauge | `policy`, `source_type` | Images found per discovery source |
| `drop_active_pulls` | Gauge | — | Currently active pull Pods |
| `drop_reconcile_total` | Counter | `controller`, `result` | Reconciliation attempts |

### Enable ServiceMonitor

```bash
helm install drop oci://ghcr.io/corewire/charts/drop \
  --set serviceMonitor.enabled=true
```

### Example Queries

```promql
# Pull rate by image (success/min)
sum by (image) (rate(drop_images_cached_total[5m])) * 60

# Pull duration percentiles per image
histogram_quantile(0.50, sum by (le, image) (rate(drop_pull_duration_seconds_bucket[30m])))
histogram_quantile(0.90, sum by (le, image) (rate(drop_pull_duration_seconds_bucket[30m])))
histogram_quantile(0.95, sum by (le, image) (rate(drop_pull_duration_seconds_bucket[30m])))

# Images with p90 pull time > 30s (slow images worth caching)
histogram_quantile(0.90, sum by (le, image) (rate(drop_pull_duration_seconds_bucket[1h]))) > 30

# Pull success ratio (0–1)
sum(rate(drop_images_cached_total[5m])) /
  (sum(rate(drop_images_cached_total[5m])) + sum(rate(drop_pull_errors_total[5m])))

# Cache coverage per CachedImage (nodes cached / nodes targeted)
sum by (cachedimage) (drop_nodes_cached)
  / sum by (cachedimage) (drop_nodes_targeted)

# Nodes with incomplete coverage
sum by (cachedimage) (drop_nodes_targeted - drop_nodes_cached) > 0

# Active pulls right now
drop_active_pulls

# Discovery source health
drop_discovery_source_health
```

## Kubernetes Events

| Reason | Type | Description |
|--------|------|-------------|
| `PullStarted` | Normal | Image pull Pod created on a node |
| `PullSucceeded` | Normal | Image successfully cached on a node |
| `PullFailed` | Warning | Image pull failed on a node |

```bash
kubectl get events --field-selector involvedObject.kind=CachedImage
```

## Loki Queries

`eventPullTime` signals read Kubernetes pull events from Loki. These LogQL queries let you explore the same raw data the operator uses for discovery.

```logql
# All Pulled events (the source for eventPullTime signals)
{job="kubernetes-events"} | json | reason = "Pulled"

# All Pulling events (in-progress pulls)
{job="kubernetes-events"} | json | reason = "Pulling"

# Pull events in a specific namespace
{job="kubernetes-events", namespace="production"} | json | reason = "Pulled"

# Pulls for a specific image
{job="kubernetes-events"} | json | reason = "Pulled"
  | message =~ `"nginx:1.25`

# Slow pulls — duration string has 2+ digits before the decimal (≥10s)
{job="kubernetes-events"} | json | reason = "Pulled"
  | message =~ `in [1-9][0-9]+\.`

# Pull failures and backoffs
{job="kubernetes-events"} | json
  | reason =~ "Failed|BackOff"
  | involvedObject_kind = "Pod"

# Pull event rate by namespace (metric query)
sum by (namespace) (
  rate({job="kubernetes-events"} | json | reason = "Pulled" [5m])
)
```

## Status Conditions

All resources use `metav1.Condition` with type `Ready`:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Cached
      message: "Image cached on all 5 target nodes"
```

## Health Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe |
| `/readyz` | 8081 | Readiness probe |
