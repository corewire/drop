# AI Docs

This directory contains feature-sliced planning docs intended to reduce context size for AI agents working on `puller`.

## Structure
- `progress.md` — checklist and implementation tracking
- `01-operator-tooling.md` — Go and operator framework decisions
- `02-release-automation.md` — automated release plan
- `03-testing-kind-chainsaw.md` — e2e strategy with kind + Kyverno Chainsaw
- `04-docs-hugo-hextra.md` — docs generation with Hugo Hextra
- `05-ai-friendly-docs.md` — AI-friendly documentation conventions
- `06-helm-and-images.md` — Helm chart + multi-arch image publishing plan
- `07-dev-tooling.md` — local developer experience/tooling plan
- `08-advanced-debugging-kamera.md` — simulation/debugging plan with Kamera
- `09-crd-reference.md` — CRD field reference and slow-pull safety model
- `10-policy-redesign-proposals.md` — simplified PullPolicy design for cluster-wide pacing
- `11-example-scenarios.md` — concrete CR examples for real-world operator scenarios
- `12-naming-structure-proposals.md` — CRD naming decision (CachedImage/CachedImageSet/PullPolicy/DiscoveryPolicy)
- `13-discovery-architecture.md` — Discovery architecture: reconciliation flow, query contract, source types, legacy migration
- `14-architecture.md` — Overall system architecture plan: reconcilers, pull mechanism, pacing, project structure

## Decided CRD naming

| Kind | Scope | Purpose |
|------|-------|---------|
| `CachedImage` | Cluster | Single image to cache on target nodes |
| `CachedImageSet` | Cluster | Group of images with shared config/discovery |
| `PullPolicy` | Cluster | Pacing and safety controls |
| `DiscoveryPolicy` | Cluster | Dynamic image discovery (Prometheus, registry) |

API group: `puller.corewire.io/v1alpha1`
