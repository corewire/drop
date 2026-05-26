# Copilot Instructions for drop

## Critical Rules

1. **ALWAYS read project files before acting.** Read the Tiltfile, Makefile, and relevant source before writing docs, suggesting workflows, or describing how things work. Never guess based on general knowledge.
2. **Documentation must be short and concise.** Focus on high-level overview and usage. Avoid volatile implementation details. Avoid information that will change frequently.
3. **Simplicity over complexity.** If a simple solution exists, use it. DRY is NOT always best. No premature optimization.
4. **Kubernetes: always verify.** Use `kubectl explain` or read the CRD types before suggesting field values or resource specs.
5. **Security-conscious.** Never expose secrets in code or docs. Follow secure coding practices.
6. **Tilt handles the dev loop.** `tilt up` does everything: cluster creation, build, deploy, port-forwards, Hugo docs, e2e infra, dev samples. Don't suggest manual commands for things Tilt automates.

## Project

Kubernetes operator (Go 1.26.0, Kubebuilder, controller-runtime) that pre-caches container images on cluster nodes.
API group: `drop.corewire.io/v1alpha1`. All CRDs are cluster-scoped.

## Build Commands

```bash
make generate      # regenerate deepcopy
make manifests     # regenerate CRD + RBAC YAML
make codegen       # both of the above
go build ./...     # compile
make test          # unit tests (envtest)
make test-e2e      # e2e tests (chainsaw, needs kind)
make lint          # golangci-lint
make docs-gen      # regenerate AI docs from source
```

## Code Conventions

- All CRDs are cluster-scoped
- Status uses metav1.Condition with type "Ready"
- No privileged containers — kubelet-based image pulls only
- Single responsibility reconcilers — one controller per CRD
- Pod builder is a pure function in internal/podbuilder/ (no k8s client)
- Pacing logic lives exclusively in internal/pacing/
- ownerReferences: CachedImageSet→CachedImage, controller→Pod
- Table-driven tests preferred; envtest for controllers
- Pods use nodeName placement + command: ["true"]
- Don't manually edit generated files — run make docs-gen

## Testing Patterns

- Controller tests use envtest (`internal/controller/*_test.go`)
- Table-driven tests preferred
- E2E uses Kyverno Chainsaw in `test/e2e/`
- Test fixtures in `config/samples/` and `hack/dev-samples.yaml`

## CRD Quick Reference

| Kind | Controller | Purpose |
|------|-----------|---------|
| CachedImage | internal/controller/cachedimage_controller.go | CachedImage ensures a single container image is pre-cached on cluster nodes. |
| CachedImageSet | internal/controller/cachedimageset_controller.go | CachedImageSet manages a group of images to cache, optionally backed by a DiscoveryPolicy. |
| DiscoveryPolicy | internal/controller/discoverypolicy_controller.go | DiscoveryPolicy automatically discovers images from registries or Prometheus metrics. |
| PullPolicy |  | PullPolicy controls the pacing and retry behavior for image pulls across cluster nodes. It is a configuration-only resource with no status. |

## Package Dependency Graph

```
api/v1alpha1 — Package v1alpha1 contains API Schema definitions for the drop v1alpha1 API group.
internal/controller — Package controller implements Kubernetes reconcilers for the drop CRDs (one per Kind).
  imports: api/v1alpha1, internal/discovery, internal/metrics, internal/pacing, internal/podbuilder
internal/discovery — Package discovery implements image discovery from registries and Prometheus metrics.
internal/metrics — Package metrics registers Prometheus metrics for the drop operator.
internal/pacing — Package pacing implements the shared rate-limiting engine for image pull scheduling.
  imports: api/v1alpha1, internal/podbuilder
internal/podbuilder — Package podbuilder constructs pull Pods as a pure function (no Kubernetes client dependency).
  imports: api/v1alpha1
```

## Don'ts

- Don't add CRI socket access or privileged containers — we use kubelet image pulls only
- Don't put pacing logic outside `internal/pacing/`
- Don't create namespaced CRDs — all resources are cluster-scoped
- Don't manually edit generated files (`zz_generated.deepcopy.go`, `config/crd/bases/`)
- Don't manually edit `llms.txt`, `llms-full.txt`, `.cursorrules`, `AGENTS.md` — run `make docs-gen`
