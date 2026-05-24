---
title: Monitoring
weight: 4
aliases:
  - /puller/docs/observability/
description: Prometheus metrics, events, and health checks.
llmsDescription: |
  Monitoring for puller: Prometheus metrics (puller_images_cached_total,
  puller_pull_errors_total, puller_pull_duration_seconds, etc.), Kubernetes
  events on CachedImage/CachedImageSet, and metav1.Condition status with
  type Ready. ServiceMonitor included for Prometheus Operator integration.
---

## Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `puller_images_cached_total` | Counter | `image`, `node` | Total images successfully cached |
| `puller_pull_duration_seconds` | Histogram | `image` | Duration of pull operations |
| `puller_pull_errors_total` | Counter | `image`, `node` | Total failed pull attempts |
| `puller_discovery_images_found` | Gauge | `policy`, `source_type` | Images found per discovery source |
| `puller_active_pulls` | Gauge | — | Currently active pull Pods |
| `puller_reconcile_total` | Counter | `controller`, `result` | Reconciliation attempts |

### Enable ServiceMonitor

```bash
helm install puller oci://ghcr.io/breee/charts/puller \
  --set serviceMonitor.enabled=true
```

### Example Queries

```promql
# Pull success rate
rate(puller_images_cached_total[1h])

# p95 pull duration
histogram_quantile(0.95, rate(puller_pull_duration_seconds_bucket[1h]))

# Error rate by image
rate(puller_pull_errors_total[1h])

# Active pulls right now
puller_active_pulls
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
