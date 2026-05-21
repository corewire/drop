# Feature: CRD Reference and Pull-Rate Safety

## Goal
Make CRD settings explicit so users can predict pull behavior and avoid containerd overload.

## `PrePullImage` (`puller.corewire.io/v1alpha1`)

### Spec fields
- `image` (string, required)
  - Repository/image name to pre-pull.
- `tag` (string, optional)
  - Tag to use. Prefer pinned versions for reproducibility.
- `digest` (string, optional)
  - Immutable digest (preferred over moving tags where possible).
- `pullPolicy` (`IfNotPresent` | `Always`)
  - Initial pull behavior.
  - `IfNotPresent`: pull only when image is missing on node.
  - `Always`: force remote check/pull on each reconcile pull attempt.
- `repullPolicy` (`Never` | `OnSchedule` | `Always`)
  - Controls refresh after first successful pull.
  - `Never`: do not refresh unless spec changes.
  - `OnSchedule`: refresh only on discovery/sync interval boundaries.
  - `Always`: refresh every reconcile cycle (use carefully).
- `nodeSelector` (map, optional)
  - Restricts target nodes.
- `tolerations` (list, optional)
  - Allows targeting tainted nodes.
- `priority` (int, optional)
  - Pull ordering hint (lower first or higher first, implementation-defined but documented).
- `maxPullRate` (duration/int, optional)
  - Rate-limit guardrail between pull starts.

### Status fields
- `phase`, `conditions`, `lastPulledAt`, `nodesTargeted`, `nodesReady`, `observedGeneration`.

## `ImageDiscoveryPolicy` (`puller.corewire.io/v1alpha1`)

### Spec fields
- `namespaces`, `lookbackWindow`, `topX` for Prometheus-driven selection.
- Optional registry source for helper images (`registry`, `repository`, `tagFilter`, `topX`, auth secret refs).
- `syncInterval` to control discovery and refresh cadence.

## Slow-pull safety model
To avoid "10 images at once" behavior, operator logic should enforce:

1. **Policy-driven global pacing**
   - A dedicated pull policy should cap concurrent pull work across nodes.
2. **Rate limiting between pulls**
   - Enforce minimum spacing (`maxPullRate` / backoff window) between launches.
3. **Bounded rollout across nodes**
   - Use DaemonSet rollout controls (e.g. `maxUnavailable`) to prevent cluster-wide bursts.
4. **Backoff + jitter**
   - On failures, retry with exponential backoff and jitter.
5. **Policy-based refresh**
   - Moving tags (`latest`) should be controlled via `repullPolicy`, not uncontrolled constant pulls.

## Parallel pull workers: simplified model
`PrePullImage` no longer includes a separate `concurrency` setting in the plan.

- `runtime parallelism`: container runtimes (containerd/cri) already download image layers concurrently for a single image pull.
- `operator workers`: would add parallel *image pull tasks* on the same node.
- `design choice`: remove this from the plan for now because it duplicates runtime behavior and adds tuning complexity before benchmarks exist.

Operator pacing should instead focus on cluster-safe controls:
- limit how many nodes pull at once,
- add spacing/backoff between pull starts,
- keep rollout bounded (`maxUnavailable` style limits).

## Recommended safe defaults
```yaml
pullPolicy: IfNotPresent
repullPolicy: OnSchedule
```

These defaults prioritize node stability over fastest pull completion.

See `/ai-docs/10-policy-redesign-proposals.md` for proposed API redesign options that separate image intent from pull-rate policy.
