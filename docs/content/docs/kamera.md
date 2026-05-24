---
title: Kamera Integration
weight: 5
description: Simulation-based controller verification with Kamera.
llmsDescription: |
  Kamera integration for simulation-based verification of puller controllers.
  Uses deterministic simulation to test controller behaviour without a real
  cluster. Catches race conditions and edge cases in reconciliation logic.
---

[Kamera](https://github.com/tgoodwin/Kamera) uses simulation to verify Kubernetes controller logic without running a real cluster.

## Evaluation Status

**Decision: Evaluate after MVP is stable.**

### Rationale

1. **Current coverage is sufficient for MVP**: Unit tests (pod builder, pacing, discovery) + envtest integration tests + Chainsaw E2E tests provide high confidence.
2. **Kamera adds value for complex state transitions**: Once we have production experience with edge cases (node churn during pulls, policy changes mid-rollout), Kamera can help verify invariants that are hard to test deterministically.
3. **Low priority vs. feature work**: The operator needs to be deployed and battle-tested first.

### Planned Use Cases (Post-MVP)

| Scenario | Invariant to Verify |
|----------|-------------------|
| Node removed during pull | No orphaned Pods, status eventually consistent |
| PullPolicy changed mid-rollout | New pacing applied without restarting in-flight pulls |
| DiscoveryPolicy source failure | Last known good set preserved, no cache thrashing |
| Concurrent CachedImage updates | No duplicate Pods per node |

### Integration Plan

1. Add `kamera` build tag to reconciler tests
2. Define state machine model for CachedImage lifecycle
3. Run simulation sweeps in CI nightly (not on every PR — too slow)
4. Compare failure modes found vs. existing test coverage

### References

- [Kamera GitHub](https://github.com/tgoodwin/Kamera)
- [The New Stack article](https://thenewstack.io/kamera-uses-simulation-to-verify-kubernetes-controller-logic/)
