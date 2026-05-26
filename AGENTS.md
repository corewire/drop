# Agent Instructions

## Critical Rules

1. ALWAYS read project files (Tiltfile, Makefile, source) before acting. Never guess.
2. Simplicity over complexity. DRY is NOT always best.
3. Kubernetes: use kubectl explain or read CRD types before suggesting specs.
4. Never expose secrets in code or docs.
5. `tilt up` handles the dev loop — don't suggest manual commands for automated steps.
6. Never edit generated files directly — run `make docs-gen`.

## Project: drop

Kubernetes operator (Go 1.26.0) that pre-caches container images on cluster nodes.
API group: `drop.corewire.io/v1alpha1` (cluster-scoped). Framework: Kubebuilder + controller-runtime.

## Quick Start

```bash
make codegen       # generate deepcopy + CRD manifests
go build ./...     # compile
make test          # unit tests
make docs-gen      # regenerate AI docs
```

## CRDs

| Kind | Purpose |
|------|---------|
| CachedImage | CachedImage ensures a single container image is pre-cached on cluster nodes. |
| CachedImageSet | CachedImageSet manages a group of images to cache, optionally backed by a DiscoveryPolicy. |
| DiscoveryPolicy | DiscoveryPolicy automatically discovers images from registries or Prometheus metrics. |
| PullPolicy | PullPolicy controls the pacing and retry behavior for image pulls across cluster nodes. It is a configuration-only resource with no status. |

## Key Directories

| Path | Contents |
|------|----------|
| api/v1alpha1 | Package v1alpha1 contains API Schema definitions for the drop v1alpha1 API group. |
| internal/controller | Package controller implements Kubernetes reconcilers for the drop CRDs (one per Kind). |
| internal/discovery | Package discovery implements image discovery from registries and Prometheus metrics. |
| internal/metrics | Package metrics registers Prometheus metrics for the drop operator. |
| internal/pacing | Package pacing implements the shared rate-limiting engine for image pull scheduling. |
| internal/podbuilder | Package podbuilder constructs pull Pods as a pure function (no Kubernetes client dependency). |
| charts/drop/ | Helm chart |
| test/e2e/ | Chainsaw E2E tests |
| hack/gen-ai-docs/ | This doc generator |

## References

- [llms-full.txt](llms-full.txt) — complete CRD fields, error reasons, metrics, samples
- [.github/copilot-instructions.md](.github/copilot-instructions.md) — conventions, testing patterns, package graph, don'ts
