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
- `concurrency` (int, optional)
  - Optional **per-node** parallelism hint for this single resource.
  - Useful for local pacing, but not sufficient for cluster-wide burst control by itself.
  - Operator usage:
    - `1`: run one pull worker per targeted node for this image (safest default).
    - `2+`: allow limited parallel pull workers on each targeted node for this image.
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

## Real `concurrency` use cases (3 examples)
`concurrency` only changes **node-local behavior** for one `PrePullImage`.  
Cluster-wide pacing still comes from policy-level controls (`PrePullPolicy` proposal).

1. **Small CI node pool, one very large base image**
   - Situation: each CI node frequently needs `ghcr.io/acme/build-base:2026.05` (~8 GB).
   - Setting: `concurrency: 1`.
   - Operator behavior: on each targeted node, reconciler starts one pull worker for this image, keeping disk/network pressure predictable.

2. **High-throughput GPU nodes with spare bandwidth**
   - Situation: GPU nodes have fast NVMe + 25GbE and can safely overlap chunk downloads.
   - Setting: `concurrency: 2`.
   - Operator behavior: per targeted GPU node, up to two pull workers for this image can run in parallel, reducing warm-up time without opening full burst mode.

3. **Moving tag refresh during low-traffic window**
   - Situation: `my-registry/runner-helper:latest` is refreshed nightly.
   - Setting: `repullPolicy: OnSchedule` + `concurrency: 1`.
   - Operator behavior: at schedule time, each node refreshes this image sequentially (one worker), preventing local I/O spikes while still updating moving tags.

## Recommended safe defaults
```yaml
pullPolicy: IfNotPresent
repullPolicy: OnSchedule
concurrency: 1 # optional local hint
```

These defaults prioritize node stability over fastest pull completion.

See `/ai-docs/10-policy-redesign-proposals.md` for proposed API redesign options that separate image intent from pull-rate policy.
