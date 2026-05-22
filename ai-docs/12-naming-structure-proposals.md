# CRD Naming and Structure — Decision

## Chosen: `CachedImage` + `CachedImageSet` + `PullPolicy` + `DiscoveryPolicy`

Decision: Proposal C. "Cached" describes the desired state (image is cached on nodes), which is idiomatic for Kubernetes declarative specs. All resources are **cluster-scoped** since they target nodes (which are cluster-scoped).

---

## Design principles

1. **Single concern per CRD** — separate "what to cache", "how fast to pull", and "how to discover".
2. **Singular nouns** for Kind names.
3. **Owner references** — `CachedImageSet` owns child `CachedImage` resources for lifecycle/GC.
4. **API group carries context** — within `puller.corewire.io`, names don't need to repeat "pull" or "pre-pull".
5. **Cluster-scoped** — nodes are cluster-scoped, so image caching resources are too.
6. **Policy separation** — `PullPolicy` and `DiscoveryPolicy` are independent resources with single concerns.

---

## Resource overview

| Kind | API Group/Version | Scope | Single concern |
|------|-------------------|-------|----------------|
| `CachedImage` | `puller.corewire.io/v1alpha1` | Cluster | "This image should be cached on these nodes" |
| `CachedImageSet` | `puller.corewire.io/v1alpha1` | Cluster | "This group of images should be cached on these nodes" |
| `PullPolicy` | `puller.corewire.io/v1alpha1` | Cluster | "Control pull pacing and safety" |
| `DiscoveryPolicy` | `puller.corewire.io/v1alpha1` | Cluster | "How to discover images dynamically" |

---

## Resource hierarchy

```
PullPolicy          → "how fast/safe do we pull?" (reusable, referenced by sets/images)
DiscoveryPolicy     → "how do we find images?" (attached to a CachedImageSet)
    ↑ referenced by
CachedImageSet      → "which images as a group" (static list or discovery-driven)
    │ owns (ownerReferences)
    ↓
CachedImage         → "one image on target nodes" (leaf resource, reconciled individually)
```

---

## CRD field definitions

### `CachedImage`

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: cuda-base    # cluster-scoped, no namespace
spec:
  image: nvcr.io/nvidia/cuda
  tag: "12.4.0-runtime-ubuntu22.04"       # optional, mutually exclusive with digest
  digest: ""                                # optional, preferred for immutable refs
  pullPolicy: IfNotPresent                  # IfNotPresent | Always
  repullPolicy: Never                       # Never | OnSchedule | Always
  policyRef:
    name: gpu-fast                          # reference to a PullPolicy
  nodeSelector:                             # target specific nodes
    gpu: "true"
  tolerations:                              # tolerate taints on target nodes
    - key: "nvidia.com/gpu"
      operator: "Exists"
      effect: "NoSchedule"
  priority: 10                              # optional ordering hint (lower = pulled first)
status:
  phase: Ready                              # Pending | Pulling | Ready | Failed
  nodesTargeted: 5
  nodesReady: 5
  lastPulledAt: "2026-05-22T05:00:00Z"
  observedGeneration: 1
  conditions: []
```

### `CachedImageSet`

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: build-essentials
spec:
  policyRef:
    name: build-safe                        # reference to a PullPolicy
  discoveryPolicyRef:
    name: discover-ci-images                # optional, reference to a DiscoveryPolicy
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
  images:                                   # static image list (used when no discoveryPolicyRef)
    - image: registry.example.com/team/image-a
      tag: "1.2.3"
    - image: registry.example.com/team/image-b
      tag: "4.5.6"
  pullPolicy: IfNotPresent                  # default for child CachedImages
  repullPolicy: Never                       # default for child CachedImages
status:
  phase: Ready
  imagesManaged: 2
  imagesReady: 2
  observedGeneration: 1
  conditions: []
```

### `PullPolicy`

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PullPolicy
metadata:
  name: build-safe
spec:
  maxConcurrentNodes: 1                     # max nodes pulling at once
  minDelayBetweenPulls: 20s                 # spacing between pull starts
  maxUnavailableNodes: 1                    # max nodes simultaneously busy with pull work
  failureBackoff:
    initial: 10s                            # first retry delay
    max: 5m                                 # max retry delay
  repullPolicyDefault: OnSchedule           # default repull behavior for referencing images
  nodeSelector:                             # optional: scope policy to a node pool
    node-role.kubernetes.io/build: "true"
  tolerations:                              # optional: match tainted nodes in pool
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
```

### `DiscoveryPolicy`

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: discover-ci-images
spec:
  source:
    prometheus:
      endpoint: http://prometheus.monitoring.svc:9090
      query: |
        topk(5,
          count by (image) (
            kube_pod_container_info{image=~"registry.example.com/team/.*"}
          )
        )
      interval: 1h                          # how often to run the query
    registry:                               # optional alternative/additional source
      url: https://registry.example.com
      repository: team/image-c
      tagFilter: "^v[0-9]+\\."
      topX: 3
      authSecretRef:
        name: registry-creds
  imageFilter:
    pattern: "registry.example.com/team/.*" # regex filter on discovered images
  syncInterval: 30m                         # how often to reconcile discovered set
  maxImages: 10                             # cap on discovered images
status:
  lastSyncTime: "2026-05-22T05:00:00Z"
  discoveredImages: 5
  conditions: []
```

---

## Why this design

- **"Cached" describes desired state** — idiomatic for k8s (you declare what should be true).
- **No ambiguity** — "CachedImage" clearly differs from OCI Image manifests or container image refs.
- **Cluster-scoped** — nodes are cluster-scoped; images cached on nodes logically belong at cluster level.
- **Discovery is separate** — `DiscoveryPolicy` has its own reconciliation loop, sync interval, and failure modes. Keeping it separate from `CachedImageSet` follows single-concern principle and allows reuse.
- **Policy is separate** — `PullPolicy` can be shared across many sets/images, tuned independently by platform teams.
- **Owner references for GC** — when a `CachedImageSet` is deleted, its child `CachedImage` resources are garbage-collected automatically.

---

## Alternatives considered (rejected)

| Proposal | Names | Why rejected |
|----------|-------|--------------|
| A | `Image` + `ImageSet` + `PullPolicy` | "Image" too generic, confusing in conversation |
| B | `NodeImage` + `NodeImageSet` + `PullPolicy` | Less intuitive than "Cached" for desired state |
| D | `PrePullImage` + `PrePullImageSet` + `PrePullPolicy` | Verbose, redundant within `puller.corewire.io` group |
