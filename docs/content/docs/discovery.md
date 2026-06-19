---
title: Discovery
weight: 3
aliases:
  - /drop/docs/discovery/
description: Automatic image discovery with DiscoveryPolicy.
llmsDescription: |
  DiscoveryPolicy CRD enables automatic image discovery from Prometheus metrics
  or OCI registries. Referenced by CachedImageSet via discoveryPolicyRef.
  Discovered images are materialized as CachedImage resources. Supports
  filtering, deduplication, and periodic re-discovery.
---

The DiscoveryPolicy CRD enables automatic image discovery from external sources. When referenced by a CachedImageSet, discovered images are automatically materialized as CachedImage resources.

## Why This Exists

Discovery came from operational pain:

- CI bursts created pull storms where many nodes pulled the same large images at once
- Registry rate limits and transient outages amplified cold-start latency
- Hand-maintained image lists became stale and missed newly hot images
- Node rotation (e.g. Cluster API MachineDeployments rolling new nodes daily or weekly) means fresh nodes start with empty image caches — every rotation triggers a full re-pull of all active images

This last point is especially painful in CI clusters: if your build nodes are managed by Cluster API and regularly replaced (scaling events, OS upgrades, spot instance recycling), every new node must pull the same large build images from scratch. Discovery combined with pre-caching ensures that the most relevant images are warmed immediately after a node joins, eliminating the cold-start penalty from node rotation.

With DiscoveryPolicy, image candidates are continuously sourced from real usage signals (metrics) or registry data, then consumed by CachedImageSet.

## How It Works

```
DiscoveryPolicy → queries sources → writes to status.discoveredImages
                                          ↓
CachedImageSet → reads discoveredImages → creates/deletes CachedImage children
```

1. The DiscoveryPolicy reconciler queries all configured sources at the specified interval
2. Results are normalized to `{image, score}` pairs, merged, deduplicated, filtered, and sorted by score
3. Top results (capped by `maxImages`) are written to `status.discoveredImages`
4. The CachedImageSet reconciler watches DiscoveryPolicy status changes
5. It diffs the desired images against existing CachedImage children
6. New CachedImages are created; orphaned ones are deleted via ownerReference GC

## Prometheus Source

### Query Contract

Your Prometheus query **must** return an `image` label. The metric value becomes the ranking score (higher = more important).

In practice this means each result series should look like:

- Labels include `image="<registry>/<repo>:<tag>"` (or equivalent image ref like `registry.example.com/team/app@sha256:...`)
- Value is numeric and used for ranking

**Example:** Find the 30 most-used images in a namespace:

```promql
count(container_memory_working_set_bytes{
  container!="",
  container!="POD",
  namespace="build-stuff"
}) by (image)
```

### War Story Example: Top GitLab Runner Images (last 7 days)

Hand-maintained image lists do not keep up in environments where automation (for example Renovate) ships new image versions every day. A practical pattern is to rank images by observed CI usage over a rolling window.

The `queryType` field controls whether Drop sends an instant or range query. When set to `range`, the `lookback` field defines the time window and `aggregationMethod` controls how the returned data points are combined into a single score per image:

| Method | Behavior | Use when |
|--------|----------|----------|
| `sum` (default) | Adds all data-point values over the window | Total cumulative usage matters (e.g. total memory consumed) |
| `count` | Counts the number of data points returned | You want to rank by how frequently an image appears |
| `avg` | Arithmetic mean of all data-point values | Average magnitude matters regardless of sample count |
| `max` | Highest single data-point value | Peak usage is more relevant than cumulative |

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: popular-build-images
spec:
  syncInterval: 1h
  maxImages: 30
  sources:
    - type: prometheus
      prometheus:
        endpoint: https://mimir.example.com
        queryType: range   # use query_range API
        lookback: 168h   # 7 days
        step: 5m
        aggregationMethod: sum   # default — rank by total usage over 7 days
        query: |
          count(
            container_memory_working_set_bytes{
              container!="",container!="POD",
              namespace="gitlab-runner",pod=~"runner-.*"
            }
          ) by (image)
```

Use this when you want DiscoveryPolicy to continuously follow what your GitLab runner jobs really pulled in the last week.

#### Field-by-field explanation

- `queryType: range` — tells Drop to use the Prometheus `query_range` API instead of an instant query. Valid values: `range`, `instant` (default).
- `lookback: 168h` — defines the time window for range queries (start=now-7d, end=now). Required when `queryType` is `range`.
- `aggregationMethod: sum` — sums all data-point values to rank by total usage. Use `count` to rank by number of appearances, `avg` for average magnitude, or `max` for peak value.
- `step: 5m` — resolution step for the range query (controls how many data points Prometheus returns).
- `count(...) by (image)` — counts the number of running containers per image to rank by popularity.
- `container_memory_working_set_bytes{...}` — source metric used to observe running containers.
- `container!=""` — ignore empty image labels.
- `container!="POD"` — ignore sandbox/pause container noise.
- `namespace="gitlab-runner"` — scope discovery to CI jobs in that namespace.
- `pod=~"runner-.*"` — further scope to runner pods only.

#### How score is calculated

For each unique `image` label, Drop uses the Prometheus query result value as the score.

When `queryType` is `instant` (the default), Drop sends an instant query (`/api/v1/query`) and uses the returned value directly. When `queryType` is `range`, Drop uses a range query (`/api/v1/query_range`) over the `lookback` window and aggregates data points using the `aggregationMethod`:

- `sum` (default): adds all data-point values — images with higher cumulative usage score higher
- `count`: counts the number of data points — images that appear more frequently score higher
- `avg`: averages data-point values — images with higher average value score higher
- `max`: takes the peak value — images with the highest single observation score higher

The example above uses `queryType: range` with `lookback: 168h` so Drop handles the 7-day windowing via the API — no need to embed `[7d]` in PromQL.

If Prometheus returns:

| image | value returned by query | meaning |
|---|---:|---|
| `registry.example.com/ci/build:1.0.3` | 4200 | seen most frequently in the 7-day window |
| `registry.example.com/ci/test:2.4.1` | 2500 | medium usage |
| `registry.example.com/ci/lint:1.8.0` | 900 | lower usage |

Drop stores the returned values as `{image, score}` pairs in memory and then applies `spec.maxImages` as the final cap when writing `status.discoveredImages`.

So the flow is:

1. Prometheus query returns per-image counts to Drop.
2. Drop ranks by score and applies `spec.maxImages` as the final list size.

```
score
4200 | build ██████████████████████████
2500 | test  ████████████████
900  | lint  ██████
      (bar length indicates score)
```

### Production Patterns

- Use `maxImages` to cap churn and focus on the highest-impact images
- Use `imageFilter` to exclude mirrors or registries you do not want to pre-cache
- Start with one high-traffic namespace/team first, then expand source scope

### Full Example

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: popular-build-images
spec:
  syncInterval: 1h
  maxImages: 30
  imageFilter: "^(?!.*ecr\\..*amazonaws\\.com).*$"  # Exclude ECR images
  sources:
    - type: prometheus
      prometheus:
        endpoint: https://mimir.example.com
        queryType: instant
        query: |
          count(container_memory_working_set_bytes{
            container!="", container!="POD",
            namespace="build-stuff", cluster="mycluster"
          }) by (image)
      secretRef:
        name: prometheus-creds
---
apiVersion: v1
kind: Secret
metadata:
  name: prometheus-creds
  namespace: drop-system
type: Opaque
stringData:
  username: admin
  password: my-prometheus-password
```

## Registry Source

### Use Case: GitLab Runner Helper Images

The registry source uses OCI Distribution API tag listing. Combined with `imageTemplate`, it handles complex tag patterns like GitLab Runner helpers:

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: gitlab-helpers
spec:
  syncInterval: 6h
  maxImages: 10
  sources:
    - type: registry
      registry:
        url: https://registry.gitlab.com
        repositories:
          - gitlab-org/gitlab-runner/gitlab-runner-helper
        tagFilter: "^v\\d+\\.\\d+\\.\\d+$"
        topX: 5
        imageTemplate: "registry.gitlab.com/{{ .Repository }}:x86_64-{{ .Tag }}"
```

This replaces the legacy bash script that curled the GitLab API and constructed image refs manually.

### Additional Example: Stable App Tags from Private Registry

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: platform-apps
spec:
  syncInterval: 2h
  maxImages: 20
  imageFilter: "^registry\\.example\\.com/platform/.*$"
  sources:
    - type: registry
      registry:
        url: https://registry.example.com
        repositories:
          - platform/api
          - platform/web
        tagFilter: "^v\\d+\\.\\d+\\.\\d+$"
        topX: 10
```

## Error Handling

- On transient failures, the operator keeps the **last known good** discovery results
- Source health is tracked via conditions on the DiscoveryPolicy status
- Each source is queried independently — one failing source doesn't block others
