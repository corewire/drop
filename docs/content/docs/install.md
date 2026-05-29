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
  CRDs can be installed separately via the drop-crds chart for reliable upgrades.
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

## CRD Management

Helm does not update CRDs on `helm upgrade`. For reliable CRD lifecycle
management, install CRDs separately using the **drop-crds** chart:

```bash
# Install CRDs independently
helm install drop-crds oci://ghcr.io/breee/charts/drop-crds

# Install the operator with CRD installation disabled
helm install drop oci://ghcr.io/breee/charts/drop \
  --namespace drop-system \
  --create-namespace \
  --set crds.install=false
```

To upgrade CRDs later:

```bash
helm upgrade drop-crds oci://ghcr.io/breee/charts/drop-crds
```

### ArgoCD

When using ArgoCD, deploy CRDs and the operator as separate Applications so
that CRD updates are applied independently. See
[`examples/argocd/`](https://github.com/Breee/drop/tree/main/examples/argocd)
for ready-to-use Application manifests.

Key points for ArgoCD CRD management:

- Use `ServerSideApply=true` and `Replace=true` sync options on the CRDs Application.
- Set a negative sync-wave (`argocd.argoproj.io/sync-wave: "-1"`) so CRDs are synced before the operator.
- Disable `crds.install` in the operator chart values.

### Renovate

The repository includes Renovate custom managers that automatically detect new
chart versions in the ArgoCD example manifests. Add similar regex managers to
your own `renovate.json` to keep chart references up to date:

```json
{
  "customManagers": [
    {
      "customType": "regex",
      "fileMatch": ["argocd/.*\\.yaml$"],
      "matchStrings": ["chart: drop-crds\\n\\s+repoURL: oci://ghcr\\.io/breee/charts\\n\\s+targetRevision: (?<currentValue>\\S+)"],
      "depNameTemplate": "drop-crds",
      "datasourceTemplate": "docker",
      "packageNameTemplate": "ghcr.io/breee/charts/drop-crds"
    }
  ]
}
```

## Verify

```bash
kubectl -n drop-system get pods
```

The operator Pod should be running and ready.
