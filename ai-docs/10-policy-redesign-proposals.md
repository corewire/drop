# Feature: Pull Policy Design (Simplified)

## Problem statement
`PrePullImage` describes *what* to pull, but cluster stability depends on *how fast* pulling happens across many nodes.
Putting all pacing controls on `PrePullImage` is not enough for large clusters.

## Recommended design: Split intent and execution policy

### APIs
- `PrePullImage`: image intent only (image/tag/digest/selectors/priority).
- `PrePullPolicy`: shared execution policy applied to many `PrePullImage` resources.

### `PrePullPolicy` fields
- `maxConcurrentNodes`: max nodes pulling at once cluster-wide.
- `minDelayBetweenPulls`: spacing between pull starts per node.
- `failureBackoff`: retry backoff config.
- `repullPolicyDefault`: default behavior for moving tags.
- `maxUnavailableNodes`: maximum nodes simultaneously marked busy by rollout for this pull operation.

`maxConcurrentNodes` controls active pull throughput.  
`maxUnavailableNodes` controls rollout disruption budget (how many nodes can be taken out of normal scheduling posture for pull work at once).

### Why
- Clear separation of concerns.
- One place to tune rollout safety for entire cluster.
- Easier ops: update one policy instead of many image objects.
- Avoids redundant per-image worker tuning when runtimes already parallelize layer pulls.

## Parallel pull worker semantics
- A single image pull already performs concurrent layer downloads in containerd/cri.
- Additional operator-level parallel workers on one node would run multiple image pull tasks at once.
- For v1 planning, prefer **no dedicated per-image `concurrency` field**; keep pacing in `PrePullPolicy` with node rollout and delay controls.

## Scope note
No migration path is needed at this stage because implementation has not started.

## Example
```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullPolicy
metadata:
  name: safe-default
spec:
  maxConcurrentNodes: 2
  minDelayBetweenPulls: 30s
  maxUnavailableNodes: 1
  failureBackoff:
    initial: 15s
    max: 10m
  repullPolicyDefault: OnSchedule
---
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullImage
metadata:
  name: gitlab-runner-helper
spec:
  image: gitlab/gitlab-runner-helper
  tag: v17.0.0
  nodeSelector:
    node-role.kubernetes.io/ci: "true"
  policyRef:
    name: safe-default
```
