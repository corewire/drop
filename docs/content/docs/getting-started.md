---
title: Getting Started
weight: 2
description: Install and configure the puller operator.
llmsDescription: |
  Installation guide for the puller operator. Prerequisites: Kubernetes 1.28+.
  Install via Helm chart (charts/puller/). Create CachedImage or CachedImageSet
  resources to start caching images. Operator watches for these resources and
  creates short-lived Pods on target nodes to pull images via kubelet.
---

## Prerequisites

- Kubernetes 1.28+
- Helm 3.12+
- cert-manager (optional, for secure metrics)

## Installation

### Via Helm (recommended)

```bash
helm install puller oci://ghcr.io/breee/charts/puller \
  --namespace puller-system \
  --create-namespace
```

### With ServiceMonitor enabled

```bash
helm install puller oci://ghcr.io/breee/charts/puller \
  --namespace puller-system \
  --create-namespace \
  --set serviceMonitor.enabled=true \
  --set certManager.enabled=true
```

## Your First CachedImage

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx-latest
spec:
  image: docker.io/library/nginx:latest
  pullPolicy: Always
```

Apply it:

```bash
kubectl apply -f cachedimage.yaml
kubectl get cachedimages
```

## Adding Pacing

Create a PullPolicy to control how fast images are distributed:

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: PullPolicy
metadata:
  name: conservative
spec:
  maxConcurrentNodes: 2
  minDelayBetweenPulls: 30s
  failureBackoff: 5m
```

Reference it from your CachedImage:

```yaml
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx-latest
spec:
  image: docker.io/library/nginx:latest
  policyRef:
    name: conservative
```

## Next Steps

- [CRD Reference](../reference/_generated_crds/) — full field documentation
- [Discovery](../discovery/) — automatic image discovery
- [Observability](../observability/) — metrics and monitoring
