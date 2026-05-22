# Progress Tracker

- [x] Create AI docs structure and feature-sliced plan files
- [x] Decide CRD naming: `CachedImage`, `CachedImageSet`, `PullPolicy`, `DiscoveryPolicy` (cluster-scoped)
- [x] Consolidate all docs to use decided naming and structure
- [ ] Bootstrap Go operator project using Kubebuilder (controller-runtime)
- [ ] Define CRDs (`CachedImage`, `CachedImageSet`, `PullPolicy`, `DiscoveryPolicy`) in `puller.corewire.io/v1alpha1`
- [ ] Implement `CachedImage` reconciliation with pull throttling and status
- [ ] Implement `CachedImageSet` reconciliation (static image lists, child management)
- [ ] Implement `PullPolicy` controller for pacing enforcement
- [ ] Implement `DiscoveryPolicy` reconciliation (Prometheus + registry)
- [ ] Add e2e tests with kind and Kyverno Chainsaw
- [ ] Add automated release pipeline (tags, changelog, artifacts)
- [ ] Add Helm chart packaging and publishing
- [ ] Add multi-arch container builds (`linux/amd64`, `linux/arm64`) to GHCR
- [ ] Add Hugo Hextra docs generation and publishing
- [ ] Add AI-friendly docs lint/checks in CI
- [ ] Evaluate Kamera simulation workflows for controller verification
