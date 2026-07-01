---
title: Basic Usage
weight: 2
description: Create and manage cached images.
llmsDescription: |
  Usage guide for drop CRDs. Create CachedImage to cache a single image,
  CachedImageSet for multiple images, PullPolicy for rate limiting. Examples
  with YAML manifests for each resource type.
---

## Cache a Single Image

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx
spec:
  image: docker.io/library/nginx
  tag: latest
```

```bash
kubectl apply -f cachedimage.yaml
kubectl get cachedimages
```

## Target Specific Nodes

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx-amd64
spec:
  image: docker.io/library/nginx
  tag: latest
  nodeSelector:
    kubernetes.io/arch: amd64
```

## Add Pacing

Create a PullPolicy to control pull rate:

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: PullPolicy
metadata:
  name: conservative
spec:
  maxConcurrentNodes: 2
  minDelayBetweenPulls: 30s
  failureBackoff: 5m
```

Reference it:

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx
spec:
  image: docker.io/library/nginx
  tag: latest
  policyRef:
    name: conservative
```

## Cache Multiple Images

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: platform-images
spec:
  policyRef:
    name: conservative
  images:
    - image: docker.io/library/nginx
      tag: "1.27"
    - image: docker.io/library/redis
      tag: "7"
    - image: gcr.io/distroless/static-debian12
      tag: latest
```

## Check Status

```bash
# Overview
kubectl get cachedimages

# Detailed status
kubectl describe cachedimage nginx

# Watch progress
kubectl get cachedimages -w
```

A CachedImage is Ready when all targeted nodes have the image cached. 

This is the most basic usage of the drop operator. For more advanced usage, see the [Discovery](../discovery) section, which explains how to use the Loki and Registry sources to generate signals for image caching based on Kubernetes events, registry tags, and prometheus metrics. 
