---
title: Documentation
weight: 1
---

# Puller Operator Documentation

Puller is a Kubernetes operator that caches container images on cluster nodes using declarative Custom Resources.

## Quick Start

```bash
helm install puller oci://ghcr.io/breee/charts/puller --version 0.1.0
```

## Core Concepts

- **CachedImage** — declares a single image to cache on target nodes
- **CachedImageSet** — manages multiple images with optional discovery
- **PullPolicy** — controls pacing (how fast images are pulled across nodes)
- **DiscoveryPolicy** — automatically discovers images from Prometheus or OCI registries

## How It Works

The operator creates short-lived Pods with `nodeName` placement and `command: ["true"]`. The kubelet pulls the image as part of Pod scheduling, then the Pod exits immediately. This approach:

- Requires no privileged access
- Never affects node schedulability
- Uses standard Kubernetes image pull mechanisms
- Works with all container runtimes
