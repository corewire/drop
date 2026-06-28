<p align="center">
  <img src="docs/static/images/drop-logo.png" alt="Drop logo" width="500">
</p>

<p align="center">
  <a href="https://github.com/corewire/drop/releases/latest"><img src="https://img.shields.io/github/v/tag/corewire/drop" alt="Latest Release"></a>
  <a href="https://github.com/corewire/drop/actions"><img src="https://github.com/corewire/drop/workflows/CI/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/corewire/drop" alt="Go Version">
</p>


A Kubernetes operator that pre-pulls container images onto nodes — safely, with pacing, and with automatic discovery. 

## Why

When many CI jobs or workloads start simultaneously, Kubernetes nodes face a thundering herd of image pulls. We hit this running large-scale GitLab CI — concurrent pods on the same node all pulling the same large image would saturate bandwidth, stall containerd, and cascade into failures.

**The problems:**

- **Thundering herd** — a spike of pods on one node triggers parallel pulls of the same image, saturating node bandwidth and destabilizing containerd.
- **Registry overload** — sudden pull surges hit registry rate limits or cause outages.
- **Cold-start latency** — large images take minutes to pull, delaying workloads that need them immediately.

**Drop's approach:** pre-cache images on nodes *before* workloads need them, pace pulls to stay within safe limits, and automatically discover which images matter most.

## What it does

- **Pre-caches images** on selected nodes before workloads need them
- **Discovers images** automatically from Prometheus metrics or OCI registries based on your criteria (e.g. top-pulled images)
- **Paces pulls** to avoid saturating node bandwidth or registry rate limits
- **Reports errors** using standard Kubernetes status patterns (`ErrImagePull`, `ConnectionRefused`, etc.)

## Discovery of images from Prometheus and Registries

Drop Discovery is useful when image demand changes often and static image lists go stale. In fast-moving CI setups (for example with Renovate continuously landing new image versions), Prometheus-based discovery keeps your cache aligned with what jobs actually run. This is especially valuable when you rotate build nodes regularly (e.g. Cluster API MachineDeployments) — fresh nodes start with empty caches, and Discovery ensures the right images are pre-warmed immediately.

See full discovery docs and examples: **[Discovery guide](https://corewire.github.io/drop/docs/discovery/)**.

## Quickstart

Fetch the latest release, install it, then cache one image on your build nodes.

```bash
VERSION="$(curl -fsSL https://api.github.com/repos/corewire/drop/releases/latest | jq -r '.tag_name | sub("^v"; "")')"

# Install CRDs first so upgrades stay predictable
helm install drop-crds oci://ghcr.io/corewire/charts/drop-crds \
  --version "$VERSION"

# Install the operator
helm install drop oci://ghcr.io/corewire/charts/drop \
  --version "$VERSION" \
  --namespace drop-system \
  --create-namespace \
  --set crds.install=false

# Create one cached image
kubectl apply -f - <<'EOF'
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: alpine-3-20
spec:
  image: docker.io/library/alpine
  tag: "3.20"
EOF

# Watch the cache object
kubectl get cachedimage alpine-3-20 -w

# Check the pull Pod on the node(s)
kubectl get pods -l drop.corewire.io/cachedimage=alpine-3-20 -o wide

# Cleanup
kubectl delete cachedimage alpine-3-20
helm uninstall drop -n drop-system
helm uninstall drop-crds -n drop-system
```

## Examples

Each section is one use case. Apply the whole block for that use case.

### Use case: discover and pre-warm images on build nodes (including unready nodes)

This is the most common production pattern: automatically discover images from
Prometheus, pace the pulls safely, and cache them on build nodes — including
nodes that are not yet Ready (e.g. freshly provisioned Cluster API machines).

```yaml
# --- 1. PullPolicy: controls pacing and safety for image pulls ---
apiVersion: drop.corewire.io/v1alpha1
kind: PullPolicy
metadata:
  name: build-pool-safe
spec:
  # Pull on at most 2 nodes at the same time (default: 1)
  maxConcurrentNodes: 2
  # Wait at least 20s between starting pulls on different nodes (default: 10s)
  minDelayBetweenPulls: 20s
  # Exponential backoff on pull failures
  failureBackoff:
    initial: 30s
    max: 5m
---
# --- 2. DiscoveryPolicy: finds images from Prometheus metrics ---
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: ci-image-discovery
spec:
  # Re-query Prometheus every hour
  syncInterval: 1h
  # Keep at most 20 discovered images
  maxImages: 20
  # Only keep images from your internal registry (regex filter, optional)
  imageFilter: "registry.example.com/.*"
  queries:
    - name: runner-image-usage
      type: prometheus
      prometheus:
        # Any Prometheus-compatible API (Prometheus, Thanos, Mimir, VictoriaMetrics)
        endpoint: https://mimir.example.com
        # Aggregate over the last 7 days using query_range; counts container
        # instances per image across the window to produce a usage score
        queryType: range
        lookback: 168h
        # Resolution step for range queries (default: 5m)
        step: 5m
        # PromQL query — MUST return results with an "image" label.
        query: |
          count(
            container_memory_working_set_bytes{
              container!="", container!="POD",
              namespace="gitlab-runner", pod=~"runner-.*"
            }
          ) by (image)
      # Optional: Secret in the Drop pod namespace (default: drop-system)
      # Supported keys: token, username, password, ca.crt, tls.crt, tls.key
      secretRef:
        name: prometheus-creds
  signals:
    - name: total-usage
      queryRef: runner-image-usage
      type: aggregate
      aggregate:
        method: sum
  ranking:
    strategy: signal
    signal:
      signalRef: total-usage
---
# --- 3. CachedImageSet: ties discovery + policy together, targets nodes ---
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: ci-build-images
spec:
  # Reference the PullPolicy for pacing
  policyRef:
    name: build-pool-safe
  # Reference the DiscoveryPolicy for automatic image list
  discoveryPolicyRef:
    name: ci-image-discovery
  # Only pull if the image is not already present on the node
  imagePullPolicy: IfNotPresent
  # Only target nodes with this label
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  # Tolerations allow pull pods to be scheduled on tainted nodes.
  # This is critical for:
  #   - Build nodes tainted to repel regular workloads
  #   - Nodes that are NotReady (e.g. freshly joined nodes still initializing)
  tolerations:
    # Tolerate the build-node taint so pull pods land on build nodes
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
    # Tolerate NotReady nodes — allows pre-warming images on nodes that just
    # joined the cluster and are not yet fully Ready (common with Cluster API
    # node rotation, spot instance replacements, or scale-up events)
    - key: "node.kubernetes.io/not-ready"
      operator: "Exists"
      effect: "NoSchedule"
    # Tolerate unreachable nodes (network partition or kubelet restart)
    - key: "node.kubernetes.io/unreachable"
      operator: "Exists"
      effect: "NoSchedule"
```

> **Scheduling on unready nodes:** Kubernetes taints nodes with
> `node.kubernetes.io/not-ready:NoSchedule` when they are not yet Ready. By
> adding this toleration, Drop's pull pods can be scheduled on nodes as soon as
> they join the cluster — before the node is marked Ready. This lets you
> pre-warm images during the node initialization window so workloads start
> instantly once the node becomes Ready. The same pattern applies to the
> `node.kubernetes.io/unreachable` taint for nodes experiencing transient
> network issues.

### Use case: cache one public image on build nodes

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: golang-ci
spec:
  # Full image reference without tag
  image: docker.io/library/golang
  # Tag to pull (mutually exclusive with digest)
  tag: "1.22-bookworm"
  # Always (default): check registry for newer digest even if tag exists locally
  imagePullPolicy: Always
  # Only cache on nodes with this label
  nodeSelector:
    node-role.kubernetes.io/build: "true"
  # Allow scheduling pull pods on tainted build nodes
  tolerations:
    - key: "node-role.kubernetes.io/build"
      operator: "Exists"
      effect: "NoSchedule"
```

### Use case: pace a fixed CI toolchain cache

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: PullPolicy
metadata:
  name: ci-cache-conservative
spec:
  # Pull on at most 2 nodes simultaneously (default: 1)
  maxConcurrentNodes: 2
  # Wait at least 30s between starting pulls on different nodes (default: 10s)
  minDelayBetweenPulls: 30s
  failureBackoff:
    initial: 1m
    max: 10m
---
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: ci-tools
spec:
  # Apply the pacing policy above to every child CachedImage
  policyRef:
    name: ci-cache-conservative
  imagePullPolicy: IfNotPresent
  # Static list — each entry becomes a child CachedImage
  images:
    - image: docker.io/library/golang
      tag: "1.22-bookworm"
    - image: docker.io/library/node
      tag: "20-alpine"
    - image: docker.io/library/alpine
      tag: "3.19"
```

### Use case: cache private registry images

```yaml
apiVersion: v1
kind: Secret
metadata:
  # Drop reads imagePullSecrets from the namespace where it creates pull pods
  name: private-registry-pull
  namespace: drop-system
type: kubernetes.io/dockerconfigjson
stringData:
  .dockerconfigjson: |
    {
      "auths": {
        "registry.example.com": {
          "username": "REPLACE_ME",
          "password": "REPLACE_ME"
        }
      }
    }
---
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: private-ci-images
spec:
  imagePullSecrets:
    - name: private-registry-pull
  images:
    - image: registry.example.com/ci/builder
      tag: "v3.1.0"
    - image: registry.example.com/ci/test-runner
      tag: "v2.8.4"
```

### Use case: discover and cache GitLab runner images from Prometheus

```yaml
apiVersion: v1
kind: Secret
metadata:
  # Remove this Secret and source.secretRef if Prometheus does not require auth
  name: prometheus-creds
  namespace: drop-system
type: Opaque
stringData:
  token: REPLACE_WITH_PROMETHEUS_TOKEN
---
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: popular-build-images
spec:
  # Re-query Prometheus every hour (default: 30m)
  syncInterval: 1h
  # Keep at most 30 discovered images (default: 50)
  maxImages: 30
  # Only keep images matching this regex (optional)
  imageFilter: "registry.example.com/.*"
  queries:
    - name: runner-image-usage
      type: prometheus
      prometheus:
        # Any Prometheus-compatible API (Prometheus, Thanos, Mimir, VictoriaMetrics)
        endpoint: https://mimir.example.com
        # Aggregate over the last 7 days (uses query_range, sums values per image)
        # Omit for a point-in-time instant query instead
        queryType: range
        lookback: 168h
        # Resolution step for range queries (default: 5m)
        step: 5m
        # PromQL query — MUST return results with an "image" label.
        query: |
          count(
            container_memory_working_set_bytes{
              container!="", container!="POD",
              namespace="gitlab-runner", pod=~"runner-.*"
            }
          ) by (image)
      # Optional: Secret in the Drop pod namespace (default: drop-system)
      # Supported keys: token, username, password, ca.crt, tls.crt, tls.key, headers.<name>
      secretRef:
        name: prometheus-creds
  signals:
    - name: total-usage
      queryRef: runner-image-usage
      type: aggregate
      aggregate:
        method: sum
  ranking:
    strategy: signal
    signal:
      signalRef: total-usage
---
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: auto-cached-ci-images
spec:
  # Dynamically managed image list from the DiscoveryPolicy above
  discoveryPolicyRef:
    name: popular-build-images
  # Can also add static images alongside discovered ones
  images:
    - image: docker.io/library/alpine
      tag: "3.19"
```

### Use case: discover and cache application tags from a registry

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: registry-api-creds
  namespace: drop-system
type: Opaque
stringData:
  username: REPLACE_WITH_REGISTRY_USERNAME
  password: REPLACE_WITH_REGISTRY_PASSWORD
---
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: latest-app-tags
spec:
  syncInterval: 15m
  maxImages: 10
  queries:
    - name: registry-tags
      type: registry
      registry:
        # Registry base URL
        url: https://registry.example.com
        # Repositories to list tags from
        repositories:
          - team/frontend
          - team/backend
          - team/worker
        # Only discover semver tags (regex on tag name)
        tagFilter: "^v[0-9]+\\."
        # Keep only the last 3 matching tags returned by the registry
        topX: 3
      # Optional: Secret in the Drop pod namespace (default: drop-system)
      # Supported keys: token, username, password, ca.crt, tls.crt, tls.key, headers.<name>
      secretRef:
        name: registry-api-creds
  signals:
    - name: recent-tag-count
      queryRef: registry-tags
      type: aggregate
      aggregate:
        method: count
  ranking:
    strategy: signal
    signal:
      signalRef: recent-tag-count
---
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: registry-discovered-apps
spec:
  discoveryPolicyRef:
    name: latest-app-tags
```

## Quick Start

```bash
# Install CRDs and operator via Helm
helm install drop charts/drop -n drop-system --create-namespace

# Cache a single image
kubectl apply -f - <<YAML
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx
spec:
  image: docker.io/library/nginx
  tag: 1.25-alpine
YAML

# Check status
kubectl get cachedimage nginx -o wide
```

## CRDs

All resources are **cluster-scoped** under `drop.corewire.io/v1alpha1`.

| Kind | Purpose |
|------|---------|
| `CachedImage` | Cache a single image on target nodes |
| `CachedImageSet` | Manage a group of images (static or from discovery) |
| `PullPolicy` | Shared pacing/safety config (concurrency, backoff) |
| `DiscoveryPolicy` | Auto-discover images from Prometheus or registries |

```
kubectl get drop          # shows all drop resources
kubectl get drop -o wide  # includes error messages
```

## Status at a glance

```
$ kubectl get cachedimages
NAME         IMAGE              TAG           STATUS             READY   AGE
nginx        docker.io/nginx    1.25-alpine   Cached             2/2     5m
broken-img   registry.bad/x     latest        ErrImagePull       0/2     2m
auth-fail    private.io/app     v1            ImagePullBackOff   0/1     3m

$ kubectl get cachedimagesets
NAME       STATUS      READY   MANAGED   SOURCE         AGE
dev-set    AllReady    3/3     3         dev-registry   1h
web-apps   Degraded    1/3     3                        10m

$ kubectl get discoverypolicies
NAME             STATUS              IMAGES   LASTSYNC   AGE
dev-registry     Synced              3        30s        1h
broken-prom      ConnectionRefused   0                   5m
bad-auth         Unauthorized        0                   2m
```

## Development

```bash
# Prerequisites: Go 1.26+, Kind, Tilt, Helm
make generate      # deepcopy
make manifests     # CRDs + RBAC
go build ./...     # compile

# Local dev loop (Kind + Tilt)
tilt up
```

## Docs

Full documentation at **[corewire.github.io/drop/](https://corewire.github.io/drop/)** (GitHub Pages).
