# Progress Tracker

- [x] Create AI docs structure and feature-sliced plan files
- [ ] Bootstrap Go operator project using Kubebuilder (controller-runtime)
- [ ] Define CRDs (`PrePullImage`, `ImageDiscoveryPolicy`) in `puller.corewire.io/v1alpha1`
- [ ] Implement `PrePullImage` reconciliation with pull throttling and status
- [ ] Implement discovery reconciliation (Prometheus + registry top-X)
- [ ] Add e2e tests with kind and Kyverno Chainsaw
- [ ] Add automated release pipeline (tags, changelog, artifacts)
- [ ] Add Helm chart packaging and publishing
- [ ] Add multi-arch container builds (`linux/amd64`, `linux/arm64`) to GHCR
- [ ] Add Hugo Hextra docs generation and publishing
- [ ] Add AI-friendly docs lint/checks in CI
- [ ] Evaluate Kamera simulation workflows for controller verification
