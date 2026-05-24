---
title: Installation
weight: 1
aliases:
  - /puller/docs/getting-started/
description: Install the puller operator.
llmsDescription: |
  Installation guide for the puller operator. Prerequisites: Kubernetes 1.28+,
  Helm 3.12+. Install via Helm chart from ghcr.io/breee/charts/puller.
  Optional: cert-manager for secure metrics, ServiceMonitor for Prometheus.
---

## Prerequisites

- Kubernetes 1.28+
- Helm 3.12+
- cert-manager (optional, for secure metrics)

## Helm Install

```bash
helm install puller oci://ghcr.io/breee/charts/puller \
  --namespace puller-system \
  --create-namespace
```

### With Prometheus ServiceMonitor

```bash
helm install puller oci://ghcr.io/breee/charts/puller \
  --namespace puller-system \
  --create-namespace \
  --set serviceMonitor.enabled=true \
  --set certManager.enabled=true
```

## Verify

```bash
kubectl -n puller-system get pods
```

The operator Pod should be running and ready.
