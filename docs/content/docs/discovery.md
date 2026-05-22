---
title: Discovery
weight: 3
---

# Image Discovery

The DiscoveryPolicy CRD enables automatic image discovery from external sources. When referenced by a CachedImageSet, discovered images are automatically materialized as CachedImage resources.

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

**Example:** Find the 30 most-used images in a namespace:

```promql
count(container_memory_working_set_bytes{
  container!="",
  container!="POD",
  namespace="build-stuff"
}) by (image)
```

### Full Example

```yaml
apiVersion: puller.corewire.io/v1alpha1
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
  namespace: puller-system
type: Opaque
stringData:
  username: admin
  password: my-prometheus-password
```

## Registry Source

### Use Case: GitLab Runner Helper Images

The registry source uses OCI Distribution API tag listing. Combined with `imageTemplate`, it handles complex tag patterns like GitLab Runner helpers:

```yaml
apiVersion: puller.corewire.io/v1alpha1
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

## Error Handling

- On transient failures, the operator keeps the **last known good** discovery results
- Source health is tracked via conditions on the DiscoveryPolicy status
- Each source is queried independently — one failing source doesn't block others
