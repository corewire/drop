# Feature: Example CR Scenarios

## Goal
Define concrete Custom Resource examples that demonstrate real operator behavior ("write the code you wish to have").

---

## Scenario 1: Pull two images onto build nodes, one at a time

Pull `image-a` and `image-b` onto all nodes with taint `node-role.kubernetes.io/build`, pacing to maximum one image pulling at a time across the pool.

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullPolicy
metadata:
  name: build-pool-safe
spec:
  maxConcurrentNodes: 1          # only 1 node pulls at a time
  minDelayBetweenPulls: 20s      # 20s pause between pull starts
  maxUnavailableNodes: 1
  failureBackoff:
    initial: 10s
    max: 5m
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
---
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImage
metadata:
  name: image-a
spec:
  image: registry.example.com/team/image-a
  tag: "1.2.3"
  pullPolicy: IfNotPresent
  repullPolicy: Never
  policyRef:
    name: build-pool-safe
---
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImage
metadata:
  name: image-b
spec:
  image: registry.example.com/team/image-b
  tag: "4.5.6"
  pullPolicy: IfNotPresent
  repullPolicy: Never
  policyRef:
    name: build-pool-safe
```

**Operator behavior:**
1. Reconciler sees two `PrePullImage` resources bound to `build-pool-safe`.
2. Policy limits pulling to 1 node at a time with 20s spacing.
3. Operator picks `image-a` first (alphabetical or by `priority` if set), pulls it onto node-1, waits 20s, pulls onto node-2, etc.
4. Once `image-a` is complete on all targeted nodes, moves to `image-b` and repeats.
5. At no point are two images or two nodes pulling simultaneously.

---

## Scenario 2: GPU pool with relaxed pacing

GPU nodes have fast storage and network; allow 3 nodes to pull at once.

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullPolicy
metadata:
  name: gpu-pool-fast
spec:
  maxConcurrentNodes: 3
  minDelayBetweenPulls: 5s
  maxUnavailableNodes: 3
  failureBackoff:
    initial: 5s
    max: 2m
  nodeSelector:
    gpu: "true"
  tolerations:
    - key: "nvidia.com/gpu"
      operator: "Exists"
      effect: "NoSchedule"
---
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImage
metadata:
  name: cuda-base
spec:
  image: nvcr.io/nvidia/cuda
  tag: "12.4.0-runtime-ubuntu22.04"
  pullPolicy: IfNotPresent
  repullPolicy: Never
  policyRef:
    name: gpu-pool-fast
```

**Operator behavior:**
1. Up to 3 GPU nodes pull `cuda-base` concurrently.
2. 5s delay between each new node starting its pull.
3. If a pull fails, backs off starting at 5s up to 2m.

---

## Scenario 3: Prometheus-driven discovery for dynamic images

Automatically discover the top 5 most-used images named matching `image-c*` via a Prometheus query, then pre-pull them onto build nodes using the safe policy.

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullPolicy
metadata:
  name: build-pool-safe
spec:
  maxConcurrentNodes: 1
  minDelayBetweenPulls: 20s
  maxUnavailableNodes: 1
  failureBackoff:
    initial: 10s
    max: 5m
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
---
apiVersion: puller.corewire.io/v1alpha1
kind: ImageDiscoveryPolicy
metadata:
  name: discover-image-c
spec:
  source:
    prometheus:
      endpoint: http://prometheus.monitoring.svc:9090
      query: |
        topk(5,
          count by (image) (
            kube_pod_container_info{image=~"registry.example.com/team/image-c.*"}
          )
        )
      interval: 1h
  imageFilter:
    pattern: "registry.example.com/team/image-c.*"
  target:
    pullPolicy: IfNotPresent
    repullPolicy: OnSchedule
    policyRef:
      name: build-pool-safe
  syncInterval: 30m
```

**Operator behavior:**
1. Every 30 minutes, reconciler executes the Prometheus query.
2. Query returns top 5 images matching `image-c*` by pod usage count.
3. Operator materializes/updates up to 5 `PrePullImage` resources automatically.
4. Each generated `PrePullImage` inherits `policyRef: build-pool-safe`, so pulls respect the one-node-at-a-time pacing.
5. If an image drops out of the top 5, its `PrePullImage` is garbage-collected on the next sync.

---

## Design notes

### Per-pool policy binding
`PrePullPolicy` carries `nodeSelector` and `tolerations` to bind it to a specific node pool. This allows heterogeneous clusters to have different pacing per pool:
- Slow/safe policy for large CI build pools.
- Fast/relaxed policy for GPU or burst pools with better I/O.
- Default cluster-wide policy for general workloads.

Multiple policies can coexist; each `PrePullImage` references the appropriate policy via `policyRef`.

### Ordering within a policy
When multiple `PrePullImage` resources share the same policy, the operator processes them sequentially by default (one image fully rolled out before starting the next). A `priority` field on `PrePullImage` controls ordering.

### Moving tags
For images using moving tags (e.g. `latest`), set `repullPolicy: OnSchedule` on the `PrePullImage` or let the policy default apply. The operator re-checks on each sync interval.
