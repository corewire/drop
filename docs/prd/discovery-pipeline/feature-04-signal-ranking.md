# Feature 04: Basic `signal` ranking

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** 🟡 Partially done

## Summary

Implement the `signal` ranking strategy that orders images directly by one signal:

```yaml
ranking:
  strategy: signal
  signal:
    signalRef: total-usage
```

## Motivation

The simplest ranking: pick a single signal and sort by it. This reproduces today's
behaviour inside the new typed API and is the baseline the weighted/model strategies
extend.

## Current state

In `internal/controller/discoverypolicy_controller.go`:

- ✅ Sort by score descending, deterministic tie-break by image name.
- ✅ Multi-source merge + dedup (highest score per image).
- ✅ Truncate to `maxImages`.
- ⬜ No explicit `ranking.strategy: signal` typed API — ranking is implicit on the raw
  source score.

## Scope

- Implement a `signal` ranking strategy that selects and sorts by one referenced signal.
- Apply `imageFilter` and `maxImages` to the ranked output (reuse existing logic).
- Keep deterministic tie-breaking.

## Out of scope

- Combining multiple signals or normalization (Feature 05).

## Acceptance criteria

- [ ] `ranking.strategy: signal` orders images by the referenced signal, descending.
- [ ] Ties broken deterministically (by image reference).
- [ ] `imageFilter` and `maxImages` applied to the final list.
- [ ] Unit tests assert deterministic ordering and truncation.
