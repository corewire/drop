# Feature: CRD Naming and Structure Proposals

## Goal
Propose clean, minimal CRD names and structure for an operator whose sole purpose is pulling images onto nodes. Policies are always separate resources (single concern). The `puller.corewire.io` API group already communicates the domain, so resource names should be concise.

---

## Kubernetes operator naming principles applied

1. **Single concern per CRD** — separate "what to pull" from "how fast to pull".
2. **Singular nouns** for Kind names.
3. **Owner references** — parent owns children for lifecycle/GC.
4. **API group carries context** — within `puller.corewire.io`, names don't need to repeat "pull" or "pre-pull".
5. **Patterns from core k8s:**
   - Workload: `Deployment`, `Job`, `DaemonSet`
   - Collection: `ReplicaSet`, `StatefulSet`
   - Policy: `NetworkPolicy`, `PodDisruptionBudget`, `ResourceQuota`

---

## Proposal A (recommended): `Image` + `ImageSet` + `PullPolicy`

The simplest naming. The API group (`puller.corewire.io`) already says "this is the puller operator" — no need for `PrePull` prefix on every resource.

### Kinds

| Kind | Scope | Single concern |
|------|-------|----------------|
| `Image` | Namespaced | "Pull this one image onto these nodes" |
| `ImageSet` | Namespaced | "Manage this group of images (static or discovered)" |
| `PullPolicy` | Namespaced | "Control pacing/safety for pulls" |

### Resource hierarchy

```
PullPolicy        → "how fast/safe" (reusable across sets)
    ↑ referenced by
ImageSet          → "which images as a group" + discovery config
    │ owns
    ↓
Image             → "one image on target nodes" (leaf resource)
```

### Example: Static set on build nodes, one image at a time

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PullPolicy
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
kind: ImageSet
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

### Example: Discovery-driven set with Prometheus

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: ImageSet
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

### Example: Standalone image (no set needed)

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: Image
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
- Shortest, cleanest names.
- API group provides full context — no redundancy.
- Three focused CRDs, each with one concern.
- Matches k8s patterns (Deployment/ReplicaSet/Pod, PDB separate from workload).

### Cons
- `Image` is a very common word; could be confused with OCI image objects in conversation (but the API group disambiguates at the k8s API level).

---

## Proposal B: `NodeImage` + `NodeImageSet` + `PullPolicy`

Adds `Node` prefix to emphasize that these resources represent images *on nodes* (not in a registry or pod spec).

| Kind | Single concern |
|------|----------------|
| `NodeImage` | "This image should exist on these nodes" |
| `NodeImageSet` | "This group of images should exist on these nodes" |
| `PullPolicy` | "Control pull pacing/safety" |

### Example

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PullPolicy
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
kind: NodeImageSet
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

### Pros
- `NodeImage` clearly conveys "an image that lives on a node" vs. a registry image.
- Still concise — no `PrePull` prefix.
- Policy stays separate.

### Cons
- Slightly longer than Proposal A.
- `Node` prefix might imply cluster-scoped (it's not).

---

## Proposal C: `CachedImage` + `CachedImageSet` + `PullPolicy`

Uses "cached" to describe the desired state: the image is cached on nodes.

| Kind | Single concern |
|------|----------------|
| `CachedImage` | "This image should be cached on these nodes" |
| `CachedImageSet` | "This group of images should be cached" |
| `PullPolicy` | "Control pull pacing/safety" |

### Example

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: build-essentials
spec:
  policyRef:
    name: build-safe
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  images:
    - image: registry.example.com/team/image-a
      tag: "1.2.3"
    - image: registry.example.com/team/image-b
      tag: "4.5.6"
  pullPolicy: IfNotPresent
  repullPolicy: Never
```

### Pros
- Describes desired state (image is "cached"), which is idiomatic for k8s specs.
- No ambiguity with OCI Image objects.

### Cons
- "Cached" implies read-only/ephemeral; actual behavior is "ensure present".
- Slightly less intuitive than `NodeImage`.

---

## Proposal D: `PrePullImage` + `PrePullImageSet` + `PrePullPolicy`

Keep the original `PrePull` prefix on all resources for maximum explicitness.

### Pros
- Self-describing even without API group context.
- No clash risk whatsoever.

### Cons
- Verbose and repetitive — the API group already communicates "puller".
- `PrePull` is an action verb prefix; k8s conventionally uses nouns for Kinds.

---

## Recommendation

**Proposal A** (`Image` + `ImageSet` + `PullPolicy`) for maximum simplicity, or **Proposal B** (`NodeImage` + `NodeImageSet` + `PullPolicy`) if disambiguation from generic "image" is preferred.

Both keep policy separate (single concern), use the API group for context, and follow k8s ownership patterns.

### Summary of resource responsibilities

| Resource | Answers | Owns |
|----------|---------|------|
| `PullPolicy` | "How fast/safe do we pull?" | nothing |
| `ImageSet` / `NodeImageSet` | "Which images as a group? Discovered how?" | child `Image`/`NodeImage` resources |
| `Image` / `NodeImage` | "Which single image on which nodes?" | nothing (leaf) |
