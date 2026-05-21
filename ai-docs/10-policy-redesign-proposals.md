# Feature: Policy Redesign Proposals

## Problem statement
`PrePullImage` describes *what* to pull, but cluster stability depends on *how fast* pulling happens across many nodes.
Putting all pacing controls on `PrePullImage` is not enough for large clusters.

## Proposal A (recommended): Split intent and execution policy

### APIs
- `PrePullImage`: image intent only (image/tag/digest/selectors/priority).
- `PrePullPolicy`: shared execution policy applied to many `PrePullImage` resources.

### Example fields for `PrePullPolicy`
- `maxConcurrentNodes`: max nodes pulling at once cluster-wide.
- `maxConcurrentPullsPerNode`: max parallel pulls per node.
- `minDelayBetweenPulls`: spacing between pull starts per node.
- `failureBackoff`: retry backoff config.
- `repullPolicyDefault`: default behavior for moving tags.

### Why
- Clear separation of concerns.
- One place to tune rollout safety for entire cluster.
- Easier ops: update one policy instead of many image objects.

## Proposal B: Per-pool policy binding
- Add `NodePullPolicy` and bind by node pool/label set.
- Better if infra has heterogeneous node classes (build, gpu, burst pools).
- More complex than Proposal A but gives fine-grained control.

## Proposal C: Queue-first model
- Introduce `PrePullQueue` as orchestrator object.
- Queue controls ordering/budgets; `PrePullImage` just enqueues desired images.
- Powerful but largest design and implementation effort.

## Recommended direction
1. Implement **Proposal A** first (lowest complexity, high impact).
2. Add optional pool-specific override later (Proposal B style).
3. Keep queue-first approach as future scaling path.

## Migration sketch
1. Keep `PrePullImage` backward-compatible.
2. Add `spec.policyRef` on `PrePullImage`.
3. If no `policyRef`, fall back to a namespace/global default `PrePullPolicy`.
4. Deprecate image-level pacing fields over time in favor of policy object settings.

## Example
```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PrePullPolicy
metadata:
  name: safe-default
spec:
  maxConcurrentNodes: 2
  maxConcurrentPullsPerNode: 1
  minDelayBetweenPulls: 30s
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
