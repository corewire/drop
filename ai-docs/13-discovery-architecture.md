# Feature: Discovery Architecture

## Goal

Replace legacy bash-script-based image discovery (Prometheus queries + registry tag fetching + DaemonSet YAML generation) with a declarative, operator-managed flow. The operator handles querying, filtering, ranking, and materializing `CachedImage` resources — no scripts, no manual `jq`/`yq`/`curl` pipelines.

---

## How it replaces legacy scripts

| Legacy step | Operator equivalent |
|-------------|-------------------|
| `curl` Prometheus with basic auth | `DiscoveryPolicy` source `type: prometheus` with `secretRef` |
| `jq` to parse response, rank by count | Operator parses Prometheus response, ranks internally |
| `curl` GitLab/registry API for tags | `DiscoveryPolicy` source `type: registry` with `secretRef` |
| Build image refs from tag+commit | Operator uses `imageTemplate` to construct full image refs |
| `jq -s sort_by | reverse | [:30]` | `topX` field on source + `maxImages` on policy |
| Generate DaemonSet YAML with `yq` | Operator creates/updates `CachedImage` resources (owned by `CachedImageSet`) |
| Manual re-run / cron | `syncInterval` triggers automatic periodic reconciliation |

---

## Reconciliation flow

```
┌─────────────────────────────────────────────────────────────────┐
│  DiscoveryPolicy Reconciler                                     │
│                                                                 │
│  1. For each source in spec.sources:                            │
│     a. Build HTTP client (endpoint + secretRef → auth/TLS)      │
│     b. Execute query/request                                    │
│     c. Parse response into unified ImageResult list             │
│                                                                 │
│  2. Merge results from all sources                              │
│  3. Apply imageFilter (regex)                                   │
│  4. Rank by score (descending), truncate to maxImages           │
│  5. Write discovered images to status.discoveredImages[]        │
│  6. Requeue after syncInterval                                  │
└──────────────────────────────────────┬──────────────────────────┘
                                       │
                                       ▼
┌─────────────────────────────────────────────────────────────────┐
│  CachedImageSet Reconciler                                      │
│                                                                 │
│  1. If discoveryPolicyRef set:                                  │
│     a. Read DiscoveryPolicy.status.discoveredImages[]           │
│     b. Diff against existing child CachedImage resources        │
│     c. Create new CachedImage for newly discovered images       │
│     d. Delete CachedImage for images no longer discovered       │
│     e. All children have ownerReference → set for GC            │
│                                                                 │
│  2. If static images[] set:                                     │
│     a. Reconcile child CachedImage list to match spec           │
└─────────────────────────────────────────────────────────────────┘
```

---

## Query result contract

Every source type must produce a **unified internal result**: a list of `ImageResult` items. The operator normalizes all backend responses into this shape.

### `ImageResult` (internal, not a CRD)

```go
type ImageResult struct {
    Image string  // fully qualified image reference (registry/repo:tag or @sha256:...)
    Score float64 // ranking score (higher = more important, e.g. usage count)
}
```

### What the Prometheus query must return

The operator expects Prometheus to return results where **each result has a label called `image`** containing the full image reference. The associated value is used as the score for ranking.

**Required label:** `image` — the fully qualified image reference.

**Score source:**
- For `query` (instant query): the current value of each result series.
- For `query_range`: the operator sums all values in the range (total usage).

**Example query — top 30 images by container count over 7 days:**

```promql
topk(30,
  count by (image) (
    container_memory_working_set_bytes{
      container!="",
      container!="POD",
      namespace="build-stuff",
      cluster="mycluster",
      pod=~"runner-.*",
      image!~".+\\.ecr\\.eu-central-1\\.amazonaws\\.com.+"
    }
  )
)
```

The operator will:
1. Execute this query against the configured endpoint (with auth from `secretRef`).
2. Parse the response: extract `image` label → `ImageResult.Image`, metric value → `ImageResult.Score`.
3. Results are already ranked by Prometheus (`topk`), but operator re-sorts by score anyway for consistency.

**Prometheus response format (standard `/api/v1/query` JSON):**

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      { "metric": { "image": "registry.example.com/team/runner:v1.2.3" }, "value": [1716368400, "42"] },
      { "metric": { "image": "registry.example.com/team/helper:latest" }, "value": [1716368400, "38"] }
    ]
  }
}
```

The operator reads `result[].metric.image` and `result[].value[1]` (as float64 score).

---

### What the registry source returns

The operator queries OCI Distribution API (`GET /v2/<repo>/tags/list`) for each configured repository, then:
1. Filters tags by `tagFilter` regex.
2. Sorts by semver (if parseable) or lexicographic/date order.
3. Takes top X per repository.
4. Constructs full image refs: `<url>/<repository>:<tag>`.
5. Optionally applies `imageTemplate` for complex ref construction (e.g. GitLab helper images with commit-based tags).

**Registry response format (OCI standard):**

```json
{
  "name": "gitlab-org/gitlab-runner/gitlab-runner-helper",
  "tags": ["v17.0.0", "v16.11.0", "v16.10.0", "x86_64-abc1234", "x86_64-v17.0.0"]
}
```

---

## Image template (for complex image ref construction)

Some registries use non-standard tag formats (e.g. GitLab runner helper uses `x86_64-<commit>` and `x86_64-<tag>`). The `imageTemplate` field supports Go template syntax to construct the final image reference from tag metadata.

```yaml
sources:
  - type: registry
    registry:
      url: https://registry.gitlab.com
      repositories:
        - gitlab-org/gitlab-runner/gitlab-runner-helper
      tagFilter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
      topX: 5
      imageTemplate: "registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:x86_64-{{ .Tag }}"
    secretRef:
      name: gitlab-registry-creds
```

Template variables available:
- `{{ .Tag }}` — the matched tag string
- `{{ .Repository }}` — the repository path
- `{{ .Registry }}` — the registry URL (without scheme)

If `imageTemplate` is not set, the default is `<url>/<repository>:<tag>`.

---

## Concrete example: Replacing the legacy GitLab helper script

**Legacy:** bash script curls GitLab API, extracts top 5 tags + commits, builds image refs with `x86_64-<commit>` and `x86_64-<tag>` suffixes, writes JSON.

**Operator equivalent:**

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: gitlab-runner-helpers
spec:
  sources:
    - type: registry
      registry:
        url: https://registry.gitlab.com
        repositories:
          - gitlab-org/gitlab-runner/gitlab-runner-helper
        tagFilter: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"   # only semver release tags
        topX: 5                                       # top 5 most recent
        imageTemplate: "registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:x86_64-{{ .Tag }}"
      secretRef:
        name: gitlab-registry-token                   # optional: token for private registry
  syncInterval: 1h
  maxImages: 5
---
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: gitlab-runner-helpers
spec:
  discoveryPolicyRef:
    name: gitlab-runner-helpers
  policyRef:
    name: build-pool-safe
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
  pullPolicy: Always             # helpers use moving tags
  repullPolicy: OnSchedule
```

**Result:** operator discovers the 5 latest release tags, constructs `x86_64-v17.0.0` style refs, creates 5 `CachedImage` children, pulls them onto build nodes with safe pacing. No bash, no cron, no manual YAML generation.

---

## Concrete example: Replacing the legacy Prometheus top-images script

**Legacy:** bash script curls Prometheus with basic auth, queries `container_memory_working_set_bytes`, parses with `jq`, sorts, takes top 30, generates DaemonSet YAML with `yq`.

**Operator equivalent:**

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: popular-build-images
spec:
  sources:
    - type: prometheus
      prometheus:
        endpoint: https://mimir.example.com/prometheus
        query: |
          topk(30,
            count by (image) (
              container_memory_working_set_bytes{
                container!="",
                container!="POD",
                namespace="build-stuff",
                cluster="mycluster",
                pod=~"runner-.*",
                image!~".+\\.ecr\\.eu-central-1\\.amazonaws\\.com.+"
              }
            )
          )
        interval: 6h                           # re-query every 6 hours
      secretRef:
        name: prometheus-creds                  # Secret: username=admin, password=<pass>
  syncInterval: 6h
  maxImages: 30
---
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: popular-build-images
spec:
  discoveryPolicyRef:
    name: popular-build-images
  policyRef:
    name: build-pool-safe
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
  pullPolicy: IfNotPresent
  repullPolicy: OnSchedule
```

**Result:** operator queries Prometheus every 6h, discovers top 30 images by usage, creates/updates 30 `CachedImage` children (GC'd when they drop out of top 30), pulls them onto build nodes. No bash, no jq, no yq, no DaemonSet templating.

---

## Design principles

1. **Declarative over imperative** — user declares _what_ to discover, operator handles _how_.
2. **Simple query contract** — Prometheus queries must return an `image` label. That's the only requirement.
3. **Score-based ranking** — all sources produce scored results; operator merges and ranks uniformly.
4. **Template-based ref construction** — handles complex tag-to-image-ref mappings (GitLab helper pattern) without custom code.
5. **Secret-based auth** — any auth scheme works via standard k8s Secrets. No operator changes needed for new auth patterns.
6. **Automatic lifecycle** — discovered images that drop out of results get their `CachedImage` garbage-collected via owner references.
7. **Multi-source merge** — a single `DiscoveryPolicy` can combine Prometheus + registry results, deduplicating by image ref.

---

## Status reporting

```yaml
status:
  lastSyncTime: "2026-05-22T09:00:00Z"
  discoveredImages:
    - image: "registry.example.com/team/runner:v17.0.0"
      score: 42
      source: prometheus
    - image: "registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:x86_64-v17.0.0"
      score: 0    # registry sources don't have usage scores, sorted by recency
      source: registry
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2026-05-22T09:00:00Z"
    - type: SourceHealthy
      status: "True"
      message: "All 2 sources responding"
```

---

## Error handling

| Failure | Behavior |
|---------|----------|
| Source endpoint unreachable | Retry with backoff, report condition `SourceHealthy=False` |
| Auth failure (401/403) | Report condition, don't clear previous results (stale-but-valid) |
| Query returns no results | Report condition `NoResults`, keep previous discovered set |
| Query returns invalid format (no `image` label) | Report condition `InvalidResponse`, keep previous set |
| Source timeout | Configurable via Secret or source config, default 30s |

**Key principle:** on transient failures, keep the last known good discovery set. Only update when a source returns valid results. This prevents cache thrashing during outages.

---

## Implementation phases

1. **Phase 1:** Prometheus source only (covers the main use case).
2. **Phase 2:** Registry source with tag listing + `imageTemplate`.
3. **Phase 3:** Additional source types as needed (webhook, etc.).

Each phase is independently useful and shippable.
