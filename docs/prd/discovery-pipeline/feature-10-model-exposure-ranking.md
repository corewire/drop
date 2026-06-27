# Feature 10: Model-aware exposure ranking

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** ⬜ Not started

## Summary

Implement the typed `modelExposure` ranking strategy that ranks images by estimated
post-rotation cold-node exposure:

```yaml
ranking:
  strategy: modelExposure
  modelExposure:
    nodeCount: 100
    preWindowUsageSignalRef: pre-window-usage
    targetWindowUsageSignalRef: developer-window-usage
    pullTimeSignalRef: p50-cold-pull-time
```

Score: `score(I) = J_target(I) · (1 − 1/N)^J_pre(I) · p̂(I)`, where `N` is eligible CI
nodes, `J_pre`/`J_target` are pre/target-window usage, and `p̂` is estimated pull time.

## Motivation

Node rotation leaves many nodes cold for an image even if it is not globally frequent.
This strategy approximates affected job-minutes, prioritizing images whose cold exposure
after rotation is highest.

## Current state

- ⬜ Not implemented.

## Scope

- A typed `modelExposure` ranking referencing three signals (pre-window, target-window,
  pull-time) plus `nodeCount`.
- Compute the cold-fraction term `(1 − 1/N)^J_pre` and combine with target usage and
  pull time.
- Surface contributions in status (Feature 06).

## Dependencies

Requires time-based signals (Feature 07) and the event pull-time signal (Feature 09).

## Acceptance criteria

- [ ] `modelExposure` computes the documented formula deterministically.
- [ ] Missing referenced signals handled per the policy's `missingSignal` behaviour.
- [ ] Unit tests cover the cold-fraction term across node counts and usage values.
- [ ] Ranking contributions visible in status.
