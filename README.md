# puller
K8s Operator that pre-pulls images onto Kubernetes nodes without destroying Containerd

## Draft Plan

### 1) API / CRDs
- `PrePullImage` (namespaced): declarative record for a single image that should be kept warm on selected nodes.
  - API group/version: `puller.corewire.io/v1alpha1`.
  - Spec: `image`, optional `tag`/`digest`, `pullPolicy`, `repullPolicy`, `concurrency`, `nodeSelector`, `tolerations`, `priority`, `maxPullRate`.
  - Status: `observedGeneration`, `phase`, `lastPulledAt`, `nodesTargeted`, `nodesReady`, `conditions`.
- `ImageDiscoveryPolicy` (namespaced): declares how dynamic image lists are produced.
  - Spec:
    - Prometheus query settings (namespace filters, time window, query templates, topX).
    - Optional registry source settings for helper images (registry/repository, auth secret, tag filters, topX).
    - Sync cadence and limits.
  - Status: last sync time, discovered images, errors, and conditions.

### 2) Operator Control Loops
- Reconciler A (`PrePullImage`):
  - Ensures a DaemonSet/Job-based pull mechanism exists for each declared image.
  - Throttles rollout (`maxUnavailable`, pull backoff, jitter) to avoid containerd overload.
  - Updates status from node-level pull completion signals.
- Reconciler B (`ImageDiscoveryPolicy`):
  - Periodically executes Prometheus queries for image usage in target namespaces/time ranges.
  - Computes top-X images and materializes/updates `PrePullImage` objects.
  - Optionally enriches with registry-derived helper images.

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
- Feed selected tags into managed `PrePullImage` resources (e.g. `gitlab/gitlab-runner-helper`).

### 5) Safe Pulling Strategy
- Use init containers in a managed DaemonSet for ordered pulls, one image per init step.
- Cap concurrent pulls per node and across cluster (global and node-local rate limits).
- Retry with exponential backoff; quarantine failing images via status conditions.

### 6) Observability & Operations
- Expose operator metrics: reconcile duration, discovery errors, pull success/failure, queue depth.
- Emit Kubernetes events for failures and policy drift.
- Add dashboards/alerts for:
  - Node pull lag
  - Repeated image pull failures
  - Discovery sync failures

### 7) Delivery Phases
1. Bootstrap CRDs + static `PrePullImage` reconciliation.
2. Add safe/throttled DaemonSet pull orchestration.
3. Add Prometheus discovery and top-X materialization.
4. Add registry tag discovery and helper image automation.
5. Harden RBAC, leader election, and SLO-based alerting.

### Example `PrePullImage`
```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImage
metadata:
  name: gitlab-runner-helper
spec:
  image: gitlab/gitlab-runner-helper
  tag: latest
  pullPolicy: IfNotPresent
  repullPolicy: Always
  concurrency: 1
  nodeSelector:
    node-role.kubernetes.io/ci: "true"
  tolerations:
    - key: "node-role.kubernetes.io/ci"
      operator: "Exists"
      effect: "NoSchedule"
```
