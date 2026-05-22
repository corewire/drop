# puller
K8s Operator that pre-pulls images onto Kubernetes nodes without destroying Containerd

## AI Docs

- See `/ai-docs/README.md` for feature-sliced planning documents and `/ai-docs/progress.md` for tracking.
- CRD field reference: `/ai-docs/09-crd-reference.md`.
- Pull policy design: `/ai-docs/10-policy-redesign-proposals.md`.
- Example scenarios: `/ai-docs/11-example-scenarios.md`.
- Naming decision: `/ai-docs/12-naming-structure-proposals.md`.

## Draft Plan

### 1) API / CRDs (`puller.corewire.io/v1alpha1`, all cluster-scoped)

- `CachedImage`: declarative record for a single image that should be cached on selected nodes.
  - Spec: `image`, optional `tag`/`digest`, `pullPolicy`, `repullPolicy`, `nodeSelector`, `tolerations`, `priority`, `policyRef`.
    - `pullPolicy`: image pull behavior (`IfNotPresent`/`Always`).
    - `repullPolicy`: refresh behavior for moving tags (`Never`/`OnSchedule`/`Always`).
    - no per-image concurrency knob: node-level image layer parallelism is already handled by the container runtime.
  - Status: `observedGeneration`, `phase`, `lastPulledAt`, `nodesTargeted`, `nodesReady`, `conditions`.

- `CachedImageSet`: declares a group of images to cache, with shared config.
  - Spec: `policyRef`, `discoveryPolicyRef`, `nodeSelector`, `tolerations`, `images` (static list), `pullPolicy`, `repullPolicy`.
  - Owns child `CachedImage` resources via ownerReferences for GC.
  - Status: `phase`, `imagesManaged`, `imagesReady`, `conditions`.

- `PullPolicy`: shared execution policy for pacing and safety.
  - Spec: `maxConcurrentNodes`, `minDelayBetweenPulls`, `failureBackoff`, `repullPolicyDefault`, `nodeSelector`, `tolerations`.
  - Referenced by `CachedImage`/`CachedImageSet` via `policyRef`.

- `DiscoveryPolicy`: declares how dynamic image lists are produced.
  - Spec: `source` (prometheus query/registry), `imageFilter`, `syncInterval`, `maxImages`.
  - Referenced by `CachedImageSet` via `discoveryPolicyRef`.
  - Status: `lastSyncTime`, `discoveredImages`, `conditions`.

### 2) Operator Control Loops
- Reconciler A (`CachedImage`):
  - Ensures a DaemonSet/Job-based pull mechanism exists for each declared image.
  - Throttles rollout via referenced `PullPolicy` (`maxConcurrentNodes`, backoff, jitter).
  - Updates status from node-level pull completion signals.
- Reconciler B (`CachedImageSet`):
  - Manages child `CachedImage` resources (create/update/delete).
  - Reads discovered images from referenced `DiscoveryPolicy` status if configured.
- Reconciler C (`DiscoveryPolicy`):
  - Periodically executes Prometheus queries or registry lookups.
  - Reports discovered images in status for `CachedImageSet` to consume.

### 3) Prometheus Integration
- Query source metrics from kube-state-metrics/cAdvisor/container runtime metrics (cluster dependent).
- Provide configurable query templates, for example:
  - “Top images used in namespaces N over last T hours”.
  - “Top gitlab helper images over last T hours”.
- Normalize image names (registry/repo/tag), deduplicate, and rank by usage frequency.

### 4) Registry Top-X Tag Discovery
- Add registry client support (OCI distribution API) to list tags for a repository.
- Filter tags (regex/semver/channel), sort by recency or semantic version, select top X.
- Use auth via Kubernetes Secret references.
- Feed selected tags into managed `CachedImage` resources.

### 5) Safe Pulling Strategy
- Use init containers in a managed DaemonSet for ordered pulls, one image per init step.
- Cap concurrent pulls across cluster via `PullPolicy` (global rate limits).
- Retry with exponential backoff; quarantine failing images via status conditions.

### 6) Observability & Operations
- Expose operator metrics: reconcile duration, discovery errors, pull success/failure, queue depth.
- Emit Kubernetes events for failures and policy drift.
- Add dashboards/alerts for:
  - Node pull lag
  - Repeated image pull failures
  - Discovery sync failures

### 7) Delivery Phases
1. Bootstrap CRDs + static `CachedImage` reconciliation.
2. Add safe/throttled DaemonSet pull orchestration with `PullPolicy`.
3. Add `CachedImageSet` with static image lists.
4. Add `DiscoveryPolicy` with Prometheus integration.
5. Add registry tag discovery.
6. Harden RBAC, leader election, and SLO-based alerting.

### Example `CachedImage`
```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: gitlab-runner-helper
spec:
  image: gitlab/gitlab-runner-helper
  tag: v17.0.0
  pullPolicy: IfNotPresent
  repullPolicy: Always
  nodeSelector:
    node-role.kubernetes.io/ci: "true"
  tolerations:
    - key: "node-role.kubernetes.io/ci"
      operator: "Exists"
      effect: "NoSchedule"
  policyRef:
    name: safe-default
```
