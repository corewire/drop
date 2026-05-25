# puller

A Kubernetes operator that pre-pulls container images onto nodes — safely, with pacing, and with automatic discovery. 

## Why

When many CI jobs or workloads start simultaneously, Kubernetes nodes face a thundering herd of image pulls. We hit this running large-scale GitLab CI — concurrent pods on the same node all pulling the same large image would saturate bandwidth, stall containerd, and cascade into failures.

**The problems:**

- **Thundering herd** — a spike of pods on one node triggers parallel pulls of the same image, saturating node bandwidth and destabilizing containerd.
- **Registry overload** — sudden pull surges hit registry rate limits or cause outages.
- **Cold-start latency** — large images take minutes to pull, delaying workloads that need them immediately.

**Puller's approach:** pre-cache images on nodes *before* workloads need them, pace pulls to stay within safe limits, and automatically discover which images matter most.

## What it does

- **Pre-caches images** on selected nodes before workloads need them
- **Discovers images** automatically from Prometheus metrics or OCI registries based on your criteria (e.g. top-pulled images)
- **Paces pulls** to avoid saturating node bandwidth or registry rate limits
- **Reports errors** using standard Kubernetes status patterns (`ErrImagePull`, `ConnectionRefused`, etc.)

## Quick Start

```bash
# Install CRDs and operator via Helm
helm install puller charts/puller -n puller-system --create-namespace

# Cache a single image
kubectl apply -f - <<YAML
apiVersion: puller.corewire.io/v1alpha1
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

All resources are **cluster-scoped** under `puller.corewire.io/v1alpha1`.

| Kind | Purpose |
|------|---------|
| `CachedImage` | Cache a single image on target nodes |
| `CachedImageSet` | Manage a group of images (static or from discovery) |
| `PullPolicy` | Shared pacing/safety config (concurrency, backoff) |
| `DiscoveryPolicy` | Auto-discover images from Prometheus or registries |

```
kubectl get puller          # shows all puller resources
kubectl get puller -o wide  # includes error messages
```

## Status at a glance

The STATUS column shows what's happening — using the same terminology you see in `kubectl describe pod`:

```
NAME               IMAGE                TAG      STATUS             CACHED  TARGET  AGE
nginx              docker.io/nginx      1.25     Cached             2       2       5m
broken-img         registry.bad/x       latest   ErrImagePull       0       2       2m
auth-fail          private.io/app       v1       ImagePullBackOff   0       1       3m

NAME               STATUS              SOURCES  IMAGES  LASTSYNC  AGE
dev-registry       Synced              1        3       30s       1h
broken-prom        ConnectionRefused   1        0                 5m
bad-auth           Unauthorized        1        0                 2m
```

## Development

```bash
# Prerequisites: Go 1.23+, Kind, Tilt, Helm
make generate      # deepcopy
make manifests     # CRDs + RBAC
go build ./...     # compile

# Local dev loop (Kind + Tilt)
tilt up
```

## Docs

Full documentation at **[corewire.io/puller](https://corewire.io/puller)** (GitHub Pages).
