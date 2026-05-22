# Feature: CRD Naming and Structure Proposals

## Goal
Propose naming conventions and resource hierarchy following Kubernetes operator best practices, evaluating `PrePullImage` + `PrePullImageSet` as the core resource pair.

---

## Kubernetes operator naming best practices (reference)

1. **Singular nouns** for Kind names (`Pod`, not `Pods`).
2. **Group resources by lifecycle** — if objects are created/deleted together, they belong in one resource or one owns the other.
3. **Owner references** — parent resources own children; garbage collection follows naturally.
4. **Spec/Status split** — spec is desired state, status is observed state.
5. **Keep CRDs focused** — one resource = one concern. Avoid "god objects".
6. **Use label selectors** for loose coupling (like Deployment → ReplicaSet → Pod).
7. **Naming patterns from core k8s:**
   - Single item: `Pod`, `Service`, `Secret`
   - Set/collection: `ReplicaSet`, `DaemonSet`, `StatefulSet`
   - Policy/config: `NetworkPolicy`, `PodDisruptionBudget`, `LimitRange`

---

## Proposal A (recommended): `PrePullImage` + `PrePullImageSet` + `PrePullPolicy`

### Resource hierarchy

```
PrePullPolicy          (cluster-wide or per-pool pacing controls)
    ↑ referenced by
PrePullImageSet        (logical group of images + discovery attachment point)
    │ owns
    ↓
PrePullImage           (single image intent, may be manually created or auto-generated)
```

### Kinds

| Kind | Scope | Purpose |
|------|-------|---------|
| `PrePullImage` | Namespaced | Single image to keep warm on target nodes |
| `PrePullImageSet` | Namespaced | Group of images managed together; discovery attaches here |
| `PrePullPolicy` | Namespaced | Pacing/safety controls for a node pool or cluster-wide |

### How they relate
- `PrePullImageSet` lists images inline **or** references a discovery source.
- The set owns the individual `PrePullImage` resources it generates (owner references → GC).
- `PrePullImageSet` references a `PrePullPolicy` for pacing.
- Standalone `PrePullImage` can also exist without a set (manual one-offs), and reference a policy directly.

### Example: Static set with two images on build nodes

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullPolicy
metadata:
  name: build-safe
spec:
  maxConcurrentNodes: 1
  minDelayBetweenPulls: 20s
  maxUnavailableNodes: 1
  failureBackoff:
    initial: 10s
    max: 5m
---
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImageSet
metadata:
  name: build-essentials
spec:
  policyRef:
    name: build-safe
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
  images:
    - image: registry.example.com/team/image-a
      tag: "1.2.3"
    - image: registry.example.com/team/image-b
      tag: "4.5.6"
  pullPolicy: IfNotPresent
  repullPolicy: Never
```

**Operator behavior:**
1. Reconciler creates two `PrePullImage` owned by `build-essentials`.
2. Pacing follows `build-safe` policy: 1 node at a time, 20s delay.
3. Images processed sequentially within the set.

### Example: Discovery-driven set with Prometheus

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImageSet
metadata:
  name: popular-ci-images
spec:
  policyRef:
    name: build-safe
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
  discovery:
    prometheus:
      endpoint: http://prometheus.monitoring.svc:9090
      query: |
        topk(5,
          count by (image) (
            kube_pod_container_info{image=~"registry.example.com/team/image-c.*"}
          )
        )
    syncInterval: 30m
  pullPolicy: IfNotPresent
  repullPolicy: OnSchedule
```

**Operator behavior:**
1. Every 30m, query Prometheus for top 5 images.
2. Materialize/update `PrePullImage` resources owned by this set.
3. Removed images are garbage-collected via owner references.
4. Pacing controlled by `build-safe` policy.

### Example: Standalone image (no set)

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImage
metadata:
  name: cuda-base
spec:
  image: nvcr.io/nvidia/cuda
  tag: "12.4.0-runtime-ubuntu22.04"
  policyRef:
    name: gpu-fast
  nodeSelector:
    gpu: "true"
  tolerations:
    - key: "nvidia.com/gpu"
      operator: "Exists"
      effect: "NoSchedule"
  pullPolicy: IfNotPresent
  repullPolicy: Never
```

### Pros
- Matches k8s patterns: Deployment→ReplicaSet→Pod, DaemonSet→Pod.
- Discovery is a property of a set, not a separate CRD (fewer resources).
- Standalone `PrePullImage` still works for simple cases.
- Owner references give clean GC semantics.

### Cons
- Three CRD kinds to understand (but each is focused).

---

## Proposal B: `PrePullImage` + `PrePullImageSet` (no separate Policy kind)

Merge pacing into `PrePullImageSet` directly.

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImageSet
metadata:
  name: build-essentials
spec:
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
  pacing:
    maxConcurrentNodes: 1
    minDelayBetweenPulls: 20s
    failureBackoff:
      initial: 10s
      max: 5m
  images:
    - image: registry.example.com/team/image-a
      tag: "1.2.3"
    - image: registry.example.com/team/image-b
      tag: "4.5.6"
  pullPolicy: IfNotPresent
  repullPolicy: Never
```

### Pros
- Two CRD kinds only (simpler mental model).
- Self-contained: one resource defines what, where, and how fast.

### Cons
- Pacing duplicated across sets targeting the same pool.
- Cannot share a single policy across multiple sets without duplication.
- Doesn't follow the k8s pattern of separating policy from workload (cf. `PodDisruptionBudget` is separate from `Deployment`).

---

## Proposal C: `PrePullImage` + `ImageSet` + `PullPolicy` (shorter names)

Drop the `Pre` prefix on set and policy for brevity; keep `PrePullImage` because it describes the action.

| Kind | Purpose |
|------|---------|
| `PrePullImage` | Single image intent |
| `ImageSet` | Group + discovery |
| `PullPolicy` | Pacing controls |

### Cons
- `ImageSet` and `PullPolicy` are generic names that could clash with other operators.
- Losing the `PrePull` prefix makes the API group do more naming work.

---

## Recommendation

**Proposal A** (`PrePullImage` + `PrePullImageSet` + `PrePullPolicy`):
- Follows k8s separation of concerns (workload vs. policy).
- Discovery naturally attaches to sets.
- Standalone images still work without a set.
- Policies are reusable across sets and standalone images.
- Names are self-describing and unlikely to clash.

### Summary of resource responsibilities

| Resource | Answers | Owns |
|----------|---------|------|
| `PrePullPolicy` | "How fast/safe do we pull?" | nothing |
| `PrePullImageSet` | "Which images as a group?" + "Discovered how?" | `PrePullImage` children |
| `PrePullImage` | "Which single image on which nodes?" | nothing (leaf) |
