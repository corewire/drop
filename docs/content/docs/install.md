---
title: Installation
weight: 1
aliases:
  - /drop/docs/getting-started/
description: Install the drop operator.
llmsDescription: |
  Installation guide for the drop operator. Prerequisites: Kubernetes 1.28+,
  Helm 3.12+. Install via Helm chart from ghcr.io/breee/charts/drop.
  Optional: cert-manager for secure metrics, ServiceMonitor for Prometheus.
---

## Prerequisites

- Kubernetes 1.28+
- Helm 3.12+
- cert-manager (optional, for secure metrics)

## Helm Install

```bash
helm install drop oci://ghcr.io/breee/charts/drop \
  --namespace drop-system \
  --create-namespace
```

### With Prometheus ServiceMonitor

```bash
helm install drop oci://ghcr.io/breee/charts/drop \
  --namespace drop-system \
  --create-namespace \
  --set serviceMonitor.enabled=true \
  --set certManager.enabled=true
```

## Verify

```bash
kubectl -n drop-system get pods
```

The operator Pod should be running and ready.
