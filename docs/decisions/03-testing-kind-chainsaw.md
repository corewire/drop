# Feature: E2E Testing (kind + Kyverno Chainsaw)

## Goal
Run realistic operator scenarios in ephemeral Kubernetes clusters.

## Stack
- **kind** for ephemeral cluster lifecycle in CI
- **Kyverno Chainsaw** for scenario-based Kubernetes workflow tests

## Planned scenarios
- Static `CachedImage` reconciliation and status updates
- Pull policy/repull policy behavior for moving tags
- Node selector and toleration scheduling behavior
- `CachedImageSet` managing child `CachedImage` resources
- `DiscoveryPolicy` producing expected top-X discovered images
- Failure/backoff and condition reporting
- Cleanup/GC via ownerReference cascade
