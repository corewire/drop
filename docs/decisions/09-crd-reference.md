# Feature: CRD Reference and Pull-Rate Safety

## Goal
Make CRD settings explicit so users can predict pull behavior and avoid containerd overload.

## `CachedImage` (`puller.corewire.io/v1alpha1`) — Cluster-scoped

### Spec fields
- `image` (string, required)
  - Repository/image name to cache on nodes.
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
- `policyRef` (object, optional)
  - Reference to a `PullPolicy` resource for pacing controls.

### Status fields
- `phase`, `conditions`, `lastPulledAt`, `nodesTargeted`, `nodesReady`, `observedGeneration`.

## `CachedImageSet` (`puller.corewire.io/v1alpha1`) — Cluster-scoped

### Spec fields
- `policyRef` (object, optional) — reference to a `PullPolicy`.
- `discoveryPolicyRef` (object, optional) — reference to a `DiscoveryPolicy`.
- `nodeSelector` (map, optional) — target nodes for all images in the set.
- `tolerations` (list, optional) — tolerate taints on target nodes.
- `images` (list, optional) — static list of images (each with `image`, `tag`/`digest`).
- `pullPolicy` — default for child `CachedImage` resources.
- `repullPolicy` — default for child `CachedImage` resources.

### Status fields
- `phase`, `imagesManaged`, `imagesReady`, `observedGeneration`, `conditions`.

## `PullPolicy` (`puller.corewire.io/v1alpha1`) — Cluster-scoped

### Spec fields
- `maxConcurrentNodes` (int) — max nodes pulling simultaneously.
- `minDelayBetweenPulls` (duration) — minimum spacing between pull starts.
- `failureBackoff` (object) — `initial` and `max` retry delays.
- `repullPolicyDefault` (string) — default repull behavior for referencing images.
- `nodeSelector` (map, optional) — scope policy to a node pool.
- `tolerations` (list, optional) — match tainted nodes in pool.

## `DiscoveryPolicy` (`puller.corewire.io/v1alpha1`) — Cluster-scoped

Extensible design: `sources` is a list supporting multiple backend types. New source types can be added without schema changes.

### Spec fields
- `sources` (list) — discovery backends, each with:
  - `type` (string) — source type identifier (`prometheus`, `registry`, future: `graphite`, `datadog`, `webhook`, `argocd`).
  - `prometheus` (object, when type=prometheus) — `endpoint`, `query`, `interval`.
  - `registry` (object, when type=registry) — `url`, `repositories` (list), `tagFilter`, `topX`.
  - `secretRef` (object, optional) — reference to a k8s Secret for auth/TLS/headers for this source.
    - Well-known Secret keys: `token`, `username`, `password`, `ca.crt`, `tls.crt`, `tls.key`, `headers.<name>`.
- `imageFilter` (object) — regex pattern to filter discovered images.
- `syncInterval` (duration) — how often to reconcile discovered images.
- `maxImages` (int) — cap on number of discovered images.

### Status fields
- `lastSyncTime`, `discoveredImages`, `conditions`.

## Slow-pull safety model
To avoid "10 images at once" behavior, operator logic should enforce:

1. **Policy-driven global pacing**
   - `PullPolicy` caps concurrent pull work across nodes via `maxConcurrentNodes`.
2. **Rate limiting between pulls**
   - Enforce minimum spacing (`minDelayBetweenPulls`) between pull launches.
3. **Backoff + jitter**
   - On failures, retry with exponential backoff and jitter.
4. **Policy-based refresh**
   - Moving tags (`latest`) should be controlled via `repullPolicy`, not uncontrolled constant pulls.

## Non-disruptive pull guarantee
Image pulls **never** affect node schedulability. The operator does not cordon, drain, or mark nodes as unavailable during pulls. Pulls are a background operation with no impact on workload scheduling. The operator may also place images on nodes before they are marked Ready (e.g. during node bootstrap).

## Parallel pull workers: simplified model
No separate `concurrency` setting is needed.

- `runtime parallelism`: container runtimes (containerd/cri) already download image layers concurrently for a single image pull.
- `design choice`: no per-image parallel worker field needed because it duplicates runtime behavior and adds tuning complexity.

Operator pacing focuses on cluster-safe controls:
- limit how many nodes pull at once (`maxConcurrentNodes`),
- add spacing or backoff between pull starts (`minDelayBetweenPulls`, `failureBackoff`).

## Recommended safe defaults
```yaml
pullPolicy: IfNotPresent
repullPolicy: OnSchedule
```

These defaults prioritize node stability over fastest pull completion.

See `/ai-docs/10-policy-redesign-proposals.md` for the policy design rationale and `/ai-docs/12-naming-structure-proposals.md` for the naming decision.
