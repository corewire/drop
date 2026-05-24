# Feature: Operator Tooling (Go + modern framework)

## Decision
- Language: **Go**
- Framework: **Kubebuilder + controller-runtime** (current mainstream for Kubernetes operators)

## Why
- Strong compatibility with Kubernetes APIs and CRD workflows
- Mature scaffolding and testing patterns
- Clear migration path for future operator complexity

## Initial scaffold plan
1. Initialize project with Kubebuilder and Go modules.
2. Create API group/version: `puller.corewire.io/v1alpha1`.
3. Scaffold `CachedImage`, `CachedImageSet`, `PullPolicy`, and `DiscoveryPolicy` APIs/controllers.
4. Enable leader election and health probes by default.
