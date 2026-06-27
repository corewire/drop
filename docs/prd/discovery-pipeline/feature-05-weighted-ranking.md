# Feature 05: Weighted ranking (`weightedSum` + normalization)

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** ⬜ Not started

## Summary

Implement the `weightedSum` ranking strategy that combines normalized signals:

```yaml
ranking:
  strategy: weightedSum
  weightedSum:
    normalize: minMax
    missingSignal: zero
    terms:
      - signalRef: total-usage
        weight: "0.7"
      - signalRef: peak-concurrency
        weight: "0.3"
```

Score: `final(I) = Σ weightₖ · normalize(signalₖ(I))`.

## Motivation

Single-signal ranking cannot balance competing concerns (e.g. steady hot images vs.
bursty images). Weighted sums over normalized signals enable the hybrid strategies in
the proposal (Total + Peak, Developer-Time + Peak).

## Current state

- ⬜ Not implemented. Scores are raw `int64`; there is no normalization and no
  multi-signal combination.

## Scope

- `minMax` normalization: `normalized(x) = (x − min) / (max − min)`; when all values are
  equal, `normalized(x) = 1`.
- `weightedSum` over terms `{signalRef, weight}` with decimal weights.
- `missingSignal` behaviour: `zero` (treat as 0) per initial proposal; consider the
  drop-image alternative.
- Switch internal score representation to support fractional scores (status reporting in
  Feature 06).

## Out of scope

- Time-based signals (Feature 07), model exposure (Feature 10).

## Acceptance criteria

- [ ] `minMax` normalization implemented, including the all-equal edge case.
- [ ] `weightedSum` computes the weighted combination deterministically.
- [ ] `missingSignal: zero` handled; behaviour documented.
- [ ] Unit tests cover normalization edges (single value, all equal, zero range) and
      weighting math.
