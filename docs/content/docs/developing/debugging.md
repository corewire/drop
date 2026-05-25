---
title: Debugging
weight: 4
description: Logs, common issues, pacing diagnostics, and Delve.
llmsDescription: |
  Debugging guide for drop. Check operator logs, inspect CachedImage status,
  list drop Pods. Common issues: Pending pods (nodeSelector), ErrImagePull (auth),
  stuck Pulling (pacing), Degraded (consecutive failures). Use Delve for local debugging.
---

## Operator Logs

```bash
kubectl logs -n drop-system deploy/drop-controller-manager -f
```

The operator logs structured JSON. Look for `"controller"` and `"reconcileID"` fields to trace a specific reconciliation.

## Inspect a CachedImage

```bash
kubectl get cachedimage <name> -o yaml
```

Key status fields:
- `phase`: Pending → Pulling → Ready (or Degraded)
- `conditions[type=Ready]`: The definitive health signal
- `cachedNodes`: Which nodes have the image
- `nodesTargeted` / `nodesReady`: Progress tracking
- `consecutiveFailures`: Backoff trigger

## Inspect Drop Pods

```bash
kubectl get pods -l app.kubernetes.io/managed-by=drop -o wide
```

Pods should be `Succeeded` (image pulled) or `Failed` (pull error). Check events for details:

```bash
kubectl describe pod <drop-pod-name>
```

## Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| Pod stuck `Pending` | Node selector doesn't match any node | Check `nodeSelector` on CachedImage |
| Pod `ErrImagePull` | Wrong image name or missing auth | Check `imagePullSecrets`, verify image ref exists |
| CachedImage stays `Pulling` | Pacing engine throttling | Check PullPolicy `maxConcurrentNodes` / `minDelayBetweenPulls` |
| CachedImage `Degraded` | Consecutive failures exceeded | Check Pod events, increase backoff in PullPolicy |
| DiscoveryPolicy no images | Prometheus query returns empty | Run query manually in Prometheus UI, check for `image` label |
| DiscoveryPolicy `DNSError` | Source endpoint unreachable | Check network policies, DNS, service name |

## Pacing Engine Diagnostics

The pacing engine (in `internal/pacing/`) blocks new pulls when:
1. Active (Pending/Running) Pods ≥ `maxConcurrentNodes`
2. Time since last Pod creation < `minDelayBetweenPulls`

Pods stuck in `ErrImagePull`/`ImagePullBackOff` are **excluded** from the active count (so they don't block other pulls).

To check pacing state:
```bash
# Count active drop pods
kubectl get pods -l app.kubernetes.io/managed-by=drop --field-selector=status.phase!=Succeeded,status.phase!=Failed

# Check the metric
curl -s localhost:8443/metrics | grep drop_active_pulls
```

## Delve Debugging

```bash
# Run the operator locally with delve:
dlv debug ./cmd/ -- --metrics-bind-address=:8443

# Or attach to a running process:
dlv attach <pid>
```

When running locally, the operator uses your `~/.kube/config` context.

### Useful breakpoints

| Location | Why |
|----------|-----|
| `cachedimage_controller.go:Reconcile` | Entry point for the core loop |
| `pacing.go:CanStartPull` | Pacing decision point |
| `builder.go:BuildDropPod` | Pod spec construction |
| `discoverypolicy_controller.go:buildSource` | Source creation |

## Metrics for Debugging

```bash
curl -s localhost:8443/metrics | grep drop_
```

| Metric | What it tells you |
|--------|-------------------|
| `drop_active_pulls` | How many Pods are in-flight right now |
| `drop_pull_errors_total` | Which images/nodes are failing |
| `drop_pull_duration_seconds` | How long pulls take |
| `drop_reconcile_total{result="error"}` | Controller errors |
| `drop_discovery_source_health` | Whether sources are reachable |
