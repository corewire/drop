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
helm install drop oci://ghcr.io/breee/charts/drop \
  --set serviceMonitor.enabled=true
```

### Example Queries

```promql
# Pull success rate
rate(drop_images_cached_total[1h])

# p95 pull duration
histogram_quantile(0.95, rate(drop_pull_duration_seconds_bucket[1h]))

# Error rate by image
rate(drop_pull_errors_total[1h])

# Active pulls right now
drop_active_pulls
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
