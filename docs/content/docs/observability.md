---
title: Observability
weight: 4
---

# Observability

The puller operator provides comprehensive observability through Prometheus metrics, Kubernetes events, and status conditions.

## Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `puller_images_cached_total` | Counter | `image`, `node` | Total images successfully cached |
| `puller_pull_duration_seconds` | Histogram | `image` | Duration of pull operations |
| `puller_pull_errors_total` | Counter | `image`, `node` | Total failed pull attempts |
| `puller_discovery_images_found` | Gauge | `policy`, `source_type` | Images found per discovery source |
| `puller_active_pulls` | Gauge | — | Currently active pull Pods |
| `puller_reconcile_total` | Counter | `controller`, `result` | Reconciliation attempts |

### Enabling Metrics

Metrics are enabled by default on port 8443 with secure serving. To scrape with Prometheus Operator:

```bash
helm install puller oci://ghcr.io/breee/charts/puller \
  --set serviceMonitor.enabled=true
```

### Example Grafana Queries

```promql
# Pull success rate over last hour
rate(puller_images_cached_total[1h])

# Average pull duration
histogram_quantile(0.95, rate(puller_pull_duration_seconds_bucket[1h]))

# Error rate by image
rate(puller_pull_errors_total[1h])

# Active pulls right now
puller_active_pulls
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
