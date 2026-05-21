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
- `concurrency` (int, default: `1`)
  - **Maximum parallel pulls per node for this resource**.
  - `1` means strictly sequential pulling on each node (safe default).
  - Higher values increase pull speed but also containerd/network pressure.
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

1. **Per-node sequential default**
   - `concurrency: 1` by default.
2. **Rate limiting between pulls**
   - Enforce minimum spacing (`maxPullRate` / backoff window) between launches.
3. **Bounded rollout across nodes**
   - Use DaemonSet rollout controls (e.g. `maxUnavailable`) to prevent cluster-wide bursts.
4. **Backoff + jitter**
   - On failures, retry with exponential backoff and jitter.
5. **Policy-based refresh**
   - Moving tags (`latest`) should be controlled via `repullPolicy`, not uncontrolled constant pulls.

## Recommended safe defaults
```yaml
pullPolicy: IfNotPresent
repullPolicy: OnSchedule
concurrency: 1
```

These defaults prioritize node stability over fastest pull completion.
