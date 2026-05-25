---
title: Local Dev Setup
weight: 2
description: Prerequisites, kind cluster, and Tilt workflow.
llmsDescription: |
  Local development setup for drop. Requires Go 1.23+, Docker, kind, Tilt, kubectl,
  Helm 3, golangci-lint, chainsaw. Run tilt up for full dev loop (compile, build,
  deploy, port-forward, Hugo docs, e2e infra, dev samples).
---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.23+ | Build the operator |
| Docker | any | Build images, run kind |
| kind | any | Local multi-node cluster |
| Tilt | any | Live-reload dev loop |
| kubectl | any | Cluster interaction |
| Helm | 3.x | Chart linting/deployment |
| golangci-lint | latest | Linting |
| chainsaw | latest | E2E tests |

## Quick Start

```bash
tilt up
```

That's it. Tilt handles everything:

- Creates kind cluster `drop-dev` (1 control-plane + 2 workers) if it doesn't exist
- Compiles the Go binary
- Builds + loads the Docker image into kind
- Installs CRDs
- Deploys the operator via Helm
- Deploys e2e infrastructure (Prometheus, Registry, Grafana)
- Applies dev samples from `hack/dev-samples.yaml`
- Serves Hugo docs with live-reload
- Sets up port-forwards:

| Port | Service |
|------|---------|
| 8443 | Operator metrics |
| 8081 | Health probes |
| 9090 | Prometheus |
| 5000 | OCI Registry |
| 3000 | Grafana |
| 1314 | Hugo docs |

## Build Commands

```bash
make codegen       # regenerate deepcopy + CRD manifests + docs
make generate      # deepcopy only
make manifests     # CRD + RBAC YAML only
go build ./...     # compile
make docker-build  # build container image
make docs-gen      # regenerate AI docs (llms.txt, AGENTS.md, etc.)
```

### When to run what

| Changed… | Run |
|----------|-----|
| `api/v1alpha1/*_types.go` | `make codegen` |
| Any Go code | `go build ./...` |
| Controller RBAC markers | `make manifests` |
| Makefile or types | `make docs-gen` |

## Useful Make Targets

```bash
make help          # list all targets
make kind-create   # create dev cluster (Tilt does this automatically)
make install       # apply CRDs to cluster
make e2e-infra     # deploy Prometheus + Registry for testing
make helm-lint     # lint the Helm chart
make lint          # golangci-lint
make codegen       # full code generation
make docs-gen      # regenerate AI-friendly docs
```

## Without Tilt

If you prefer not to use Tilt:

```bash
# Create cluster
make kind-create

# Install CRDs
make install

# Run operator locally (uses ~/.kube/config)
go run ./cmd/ --metrics-bind-address=:8443

# Apply dev samples
kubectl apply -f hack/dev-samples.yaml
```
