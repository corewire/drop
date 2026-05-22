---
title: CRD Reference
weight: 2
---

# CRD Reference

All CRDs are cluster-scoped under `puller.corewire.io/v1alpha1`.

## CachedImage

Declares a single container image to cache on target nodes.

| Field | Type | Description |
|-------|------|-------------|
| `spec.image` | string | **Required.** Full image reference (e.g., `docker.io/library/nginx:1.25`) |
| `spec.nodeSelector` | map[string]string | Label selector for target nodes |
| `spec.tolerations` | []Toleration | Tolerations for tainted nodes |
| `spec.policyRef.name` | string | Reference to a PullPolicy for pacing |
| `spec.pullPolicy` | string | `Always` or `IfNotPresent` (default: `IfNotPresent`) |
| `spec.repullInterval` | duration | Re-pull interval for moving tags (e.g., `24h`) |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `status.phase` | string | `Pending`, `Pulling`, `Ready`, or `Degraded` |
| `status.nodesTargeted` | int32 | Number of nodes matching selector |
| `status.nodesReady` | int32 | Number of nodes with image cached |
| `status.lastPulledAt` | time | Timestamp of last successful pull |
| `status.conditions` | []Condition | Standard conditions (Ready) |

## CachedImageSet

Manages a collection of CachedImage resources.

| Field | Type | Description |
|-------|------|-------------|
| `spec.images` | []string | Static list of image references |
| `spec.discoveryPolicyRef.name` | string | Reference to a DiscoveryPolicy |
| `spec.nodeSelector` | map[string]string | Inherited by child CachedImages |
| `spec.tolerations` | []Toleration | Inherited by child CachedImages |
| `spec.policyRef.name` | string | Inherited by child CachedImages |

## PullPolicy

Controls pacing for image pulls across the cluster.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.maxConcurrentNodes` | int32 | `1` | Max nodes pulling simultaneously |
| `spec.minDelayBetweenPulls` | duration | `10s` | Minimum delay between starting new pulls |
| `spec.failureBackoff` | duration | `5m` | Wait time after failure before retry |

## DiscoveryPolicy

Discovers images from external sources.

| Field | Type | Description |
|-------|------|-------------|
| `spec.interval` | duration | How often to query sources (e.g., `1h`) |
| `spec.topX` | int32 | Maximum number of images to discover |
| `spec.imageFilter` | string | Regex filter applied to discovered images |
| `spec.sources` | []Source | List of discovery sources |

### Source Types

#### Prometheus

```yaml
sources:
  - type: prometheus
    prometheus:
      endpoint: https://prometheus.example.com
      query: 'count(container_image_pull_total) by (image)'
    secretRef:
      name: prometheus-creds
```

The query must return an `image` label. The metric value becomes the ranking score.

#### Registry

```yaml
sources:
  - type: registry
    registry:
      url: https://registry.example.com
      repositories:
        - my-org/my-app
      tagFilter: "^v\\d+\\.\\d+\\.\\d+$"
      topX: 5
      imageTemplate: "registry.example.com/{{ .Repository }}:{{ .Tag }}"
    secretRef:
      name: registry-creds
```

### Secret Format

Secrets referenced by `secretRef` support these well-known keys:

| Key | Description |
|-----|-------------|
| `token` | Bearer token for Authorization header |
| `username` | Username for basic auth |
| `password` | Password for basic auth |
| `ca.crt` | CA certificate for TLS verification |
| `tls.crt` | Client certificate for mTLS |
| `tls.key` | Client key for mTLS |
| `headers.<name>` | Custom HTTP header value |
