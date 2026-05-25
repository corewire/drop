---
title: Architecture
weight: 1
description: How the operator is structured internally.
llmsDescription: |
  Architecture of drop operator. Three reconcilers (CachedImage, CachedImageSet,
  DiscoveryPolicy), shared pacing engine, pure pod builder, discovery sources
  (Prometheus, Registry). All CRDs cluster-scoped. Pods use nodeName + command: ["true"].
---

Drop is a Kubernetes operator that pre-caches container images on cluster nodes by creating short-lived Pods.
It uses **kubelet-based image pulls** (no CRI socket, no privileged containers).

## High-Level Flow

```
CachedImageSet ──owns──▶ CachedImage[] ──creates──▶ Pod (per node)
                              ▲                         │
                              │                    image pulled by
DiscoveryPolicy ──discovers───┘                      kubelet
       │
       ├── PrometheusSource (PromQL query)
       └── RegistrySource   (OCI tag list)
```

## Package Dependency Graph

```
cmd/main.go
  └── internal/controller/
        ├── cachedimage_controller.go    (core pull loop)
        ├── cachedimageset_controller.go (child management)
        └── discoverypolicy_controller.go (image discovery)
              │
              ├── internal/pacing/       (rate-limiting engine)
              ├── internal/podbuilder/   (pure Pod construction)
              ├── internal/discovery/    (source interface + impls)
              └── internal/metrics/      (Prometheus counters/gauges)

api/v1alpha1/   (CRD type definitions — imported by all)
```

## Reconciler Responsibilities

### CachedImage Controller

The core pull loop. For each CachedImage:
1. Resolve target nodes (by nodeSelector + toleration compatibility)
2. Fetch referenced PullPolicy for pacing config
3. Build per-node state from owned Pods
4. Mark nodes for re-pull if repull interval elapsed
5. Process Pod states (succeeded → mark ready, failed → mark degraded)
6. Schedule pulls respecting pacing engine
7. Update status with phase, ready count, conditions
8. Requeue based on backoff or repull interval

### CachedImageSet Controller

Child management. For each CachedImageSet:
1. Build desired image list (static + discovered via DiscoveryPolicy)
2. List existing child CachedImages (by ownerReference)
3. Diff: create missing, delete unwanted children
4. Update status: count ready, propagate failure reasons

### DiscoveryPolicy Controller

Image discovery. For each DiscoveryPolicy:
1. Query each source (Prometheus or Registry), measure latency
2. Merge results, deduplicate by highest score
3. Apply image filter (regex)
4. Sort by score, truncate to maxImages
5. Set status: DiscoveredImages, conditions
6. Requeue after SyncInterval

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| One controller per CRD | Single responsibility; easier to reason about |
| Shared pacing engine | Prevents thundering herd across all CachedImages |
| Pod builder is a pure function | No k8s client = easy to unit test |
| `command: ["true"]` Pods | Kubelet pulls the image, Pod exits immediately |
| `nodeName` placement | Guarantees scheduling to the target node |
| Cluster-scoped CRDs | Images are node-level; namespaces don't apply |
| `metav1.Condition` status | Standard K8s pattern for Ready/Degraded states |
| ownerReferences | CachedImageSet→CachedImage, CachedImage→Pod for GC |

## Pacing Engine

Located in `internal/pacing/`. Shared across all CachedImage reconciliations.

Blocks new pulls when:
- Active (Pending/Running) Pods ≥ `maxConcurrentNodes`
- Time since last Pod creation < `minDelayBetweenPulls`

Pods stuck in `ErrImagePull`/`ImagePullBackOff` are excluded from the active count.

## Pod Builder

Located in `internal/podbuilder/`. A pure function (`BuildDropPod`) with no k8s client dependency.

Produces Pods with:
- Labels: `app.kubernetes.io/managed-by=drop`, `drop.corewire.io/cachedimage=<name>`, `drop.corewire.io/node=<node>`
- `command: ["true"]` (no-op, image pull is the side effect)
- `RestartPolicy: Never`, `AutomountServiceAccountToken: false`
- `TerminationGracePeriodSeconds: 0`
- Tolerations + ImagePullSecrets propagated from CachedImage

## Discovery Sources

Located in `internal/discovery/`. Implements the `Source` interface:

```go
type Source interface {
    Fetch(ctx context.Context) ([]ImageResult, error)
}
```

**PrometheusSource:** Queries Prometheus for container images (requires `image` label in results). Supports instant and range queries.

**RegistrySource:** Lists tags from an OCI registry via `/v2/<repo>/tags/list`. Filters by regex, limits to TopX most recent.
