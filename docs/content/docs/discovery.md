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

With DiscoveryPolicy, image candidates are continuously sourced from real usage signals (metrics) or registry data, then consumed by CachedImageSet.

## How It Works

```
DiscoveryPolicy → queries sources → writes to status.discoveredImages
                                          ↓
CachedImageSet → reads discoveredImages → creates/deletes CachedImage children
```

1. The DiscoveryPolicy reconciler queries all configured sources at the specified interval
2. Results are normalized to `{image, score}` pairs, merged, deduplicated, filtered, and sorted by score
3. Top-X results are written to `status.discoveredImages`
4. The CachedImageSet reconciler watches DiscoveryPolicy status changes
5. It diffs the desired images against existing CachedImage children
6. New CachedImages are created; orphaned ones are deleted via ownerReference GC

## Prometheus Source

### Query Contract

Your Prometheus query **must** return an `image` label. The metric value becomes the ranking score (higher = more important).

In practice this means each result series should look like:

- Labels include `image="<registry>/<repo>:<tag>"` (or equivalent image ref)
- Value is numeric and used for ranking

**Example:** Find the 30 most-used images in a namespace:

```promql
count(container_memory_working_set_bytes{
  container!="",
  container!="POD",
  namespace="build-stuff"
}) by (image)
```

### Production Patterns

- Use `topX` to cap churn and focus on the highest-impact images
- Use `imageFilter` to exclude mirrors or registries you do not want to pre-cache
- Start with one noisy namespace/team first, then expand source scope

### Full Example

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: popular-build-images
spec:
  interval: 1h
  topX: 30
  imageFilter: "^(?!.*ecr\\..*amazonaws\\.com).*$"  # Exclude ECR images
  sources:
    - type: prometheus
      prometheus:
        endpoint: https://mimir.example.com
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
  interval: 6h
  topX: 10
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
  interval: 2h
  topX: 20
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
