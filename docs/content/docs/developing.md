---
title: Developing
weight: 6
description: Build, test, and contribute to Puller.
llmsDescription: |
  Developer guide for puller. Build commands: make codegen, go build, make test,
  make lint. Project uses Kubebuilder + controller-runtime. CRDs in api/v1alpha1/,
  controllers in internal/controller/. E2E tests use Kyverno Chainsaw with kind.
---

## Prerequisites

- Go 1.23+
- Docker (for kind cluster)
- kind (for E2E tests)

## Build

```bash
make codegen       # regenerate deepcopy + CRD manifests
go build ./...     # compile
```

## Test

```bash
make test          # unit tests (envtest)
make test-e2e      # e2e tests (requires kind cluster)
make lint          # golangci-lint
```

## Project Structure

| Path | Purpose |
|------|---------|
| `api/v1alpha1/` | CRD type definitions |
| `internal/controller/` | Reconcilers (one per CRD) |
| `internal/pacing/` | Rate-limiting engine |
| `internal/podbuilder/` | Pure Pod construction (no k8s client) |
| `internal/discovery/` | Image discovery sources |
| `internal/metrics/` | Prometheus metrics registration |
| `charts/puller/` | Helm chart |
| `test/e2e/` | Chainsaw E2E tests |

## Dev Workflow

```bash
# After changing api/v1alpha1/ types:
make codegen

# After changing anything:
go build ./... && make test

# Regenerate documentation:
make docs-gen
```

## Local Cluster

```bash
kind create cluster --config hack/kind-config.yaml
tilt up
```

## Conventions

- All CRDs are cluster-scoped
- Status uses `metav1.Condition` with type "Ready"
- No privileged containers — kubelet-based image pulls only
- Pod builder is a pure function (no k8s client)
- Pacing logic lives exclusively in `internal/pacing/`
- Table-driven tests preferred
