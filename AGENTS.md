# Agent Instructions

## Critical Rules

1. ALWAYS read project files (Tiltfile, Makefile, source) before acting. Never guess.
2. Documentation: short, concise, high-level. No volatile details.
3. Simplicity over complexity. DRY is NOT always best. No premature optimization.
4. Kubernetes: use kubectl explain or read CRD types before suggesting specs.
5. Security: never expose secrets in code or docs.
6. Tilt handles the dev loop. `tilt up` does everything. Don't suggest manual commands for automated steps.

## Project: Drop

Kubernetes operator (Go 1.23.0) that pre-caches container images on cluster nodes.

## Quick Start

```bash
make codegen       # generate deepcopy + CRD manifests
go build ./...     # compile
make test          # unit tests
make docs-gen      # regenerate AI docs
```

## Architecture

- API group: `drop.corewire.io/v1alpha1` (cluster-scoped)
- Framework: Kubebuilder + controller-runtime
- Pull mechanism: short-lived Pods with `nodeName` + `command: ["true"]`

## CRDs

| Kind | Purpose |
|------|---------|
| CachedImage | CachedImage is the Schema for the cachedimages API. |
| CachedImageSet | CachedImageSet is the Schema for the cachedimagesets API. |
| PullPolicy | PullPolicy is the Schema for the pullpolicies API. It is a configuration-only resource with no status. |
| DiscoveryPolicy | DiscoveryPolicy is the Schema for the discoverypolicies API. |

## Key Directories

| Path | Contents |
|------|----------|
| api/v1alpha1 | Package v1alpha1 contains API Schema definitions for the drop v1alpha1 API group. |
| internal/controller | Reconciler implementations (one per CRD) |
| internal/discovery | Discovery source interface + implementations |
| internal/metrics | Prometheus metrics registration |
| internal/pacing | Shared pacing engine for rate-limited pulls |
| internal/podbuilder | Pure Pod construction function (no k8s client) |
| charts/drop/ | Helm chart |
| test/e2e/ | Chainsaw E2E tests |
| hack/gen-ai-docs/ | This doc generator |

## Rules

1. Run `make codegen` after changing api/v1alpha1/ types
2. Run `make docs-gen` after changing types or Makefile (regenerates this file)
3. Never edit generated files directly
4. All CRDs are cluster-scoped — no namespaced resources
5. No privileged containers — kubelet-based image pulls only
6. Status uses `metav1.Condition` with type "Ready"

## Full Reference

See [llms-full.txt](llms-full.txt) for complete CRD field documentation.
