---
title: Observability
weight: 4
description: Monitoring the drop operator with Prometheus and Kubernetes events.
llmsDescription: |
  Observability for drop: Prometheus metrics (drop_images_cached_total,
  drop_pull_errors_total, drop_pull_duration_seconds, etc.), Kubernetes
  events on CachedImage/CachedImageSet, and metav1.Condition status with
  type Ready. ServiceMonitor included for Prometheus Operator integration.
---

The drop operator provides comprehensive observability through Prometheus metrics, Kubernetes events, and status conditions.

## Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `drop_images_cached_total` | Counter | `image`, `node` | Total images successfully cached |
| `drop_pull_duration_seconds` | Histogram | `image` | Duration of pull operations |
| `drop_pull_errors_total` | Counter | `image`, `node` | Total failed pull attempts |
| `drop_discovery_images_found` | Gauge | `policy`, `source_type` | Images found per discovery source |
| `drop_active_pulls` | Gauge | — | Currently active pull Pods |
| `drop_reconcile_total` | Counter | `controller`, `result` | Reconciliation attempts |

### Enabling Metrics

Metrics are enabled by default on port 8443 with secure serving. To scrape with Prometheus Operator:

```bash
helm install drop oci://ghcr.io/breee/charts/drop \
  --set serviceMonitor.enabled=true
```

### Example Grafana Queries

```promql
# Pull success rate over last hour
rate(drop_images_cached_total[1h])

# Average pull duration
histogram_quantile(0.95, rate(drop_pull_duration_seconds_bucket[1h]))

# Error rate by image
rate(drop_pull_errors_total[1h])

# Active pulls right now
drop_active_pulls
```

## Kubernetes Events

The operator emits events on CachedImage resources:

| Event | Type | Reason | Description |
|-------|------|--------|-------------|
| Pull started | Normal | `PullStarted` | Image pull Pod created on a node |
| Pull succeeded | Normal | `PullSucceeded` | Image successfully cached on a node |
| Pull failed | Warning | `PullFailed` | Image pull failed on a node |

View events:

```bash
kubectl get events --field-selector involvedObject.kind=CachedImage
```

## Status Conditions

All resources maintain standard Kubernetes conditions:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Cached
      message: "Image cached on all 5 target nodes"
      lastTransitionTime: "2024-01-15T10:30:00Z"
```

## Health Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| `/healthz` | 8081 | Liveness probe |
| `/readyz` | 8081 | Readiness probe |
