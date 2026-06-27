# Feature 06: Status & observability output

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** 🟡 Partially done

## Summary

Expose enough status to explain every selected image: per-query results, per-signal
results, normalized values, ranking contributions, and the final selected list.

## Motivation

Multi-signal ranking is opaque without explainability. Operators need to debug query
failures, missing labels, missing signals, normalization values, and why an image was
selected.

## Current state

In `api/v1alpha1/discoverypolicy_types.go` / controller:

- ✅ `status.discoveredImages[]` with `{image, score, source}`.
- ✅ `status.imageCount`, `status.sourceCount`, `status.lastSyncTime`, `status.conditions`.
- ✅ Graceful "keep last good results" when sources fail.
- ⬜ Per-query results (series/samples/status), per-signal results, normalized values,
      and per-term ranking contributions.

## Scope

- `status.queryResults[]`: name, type, series/sample (or record) counts, success/failure.
- `status.signalResults[]`: name, image count, status.
- Extend `discoveredImages[]` with `rank`, `finalScore`, per-signal `rawValue` +
  `normalizedValue`, and ranking `terms[]` contributions.
- Surface query/signal failures and missing-label counts.

## Out of scope

- The scoring logic itself (Features 03–05, 07, 09, 10).

## Acceptance criteria

- [ ] Status reports per-query and per-signal results with success/failure.
- [ ] Each selected image shows rank, final score, and per-signal raw + normalized values.
- [ ] Weighted-sum contributions per term are visible.
- [ ] Query/signal failures are surfaced without dropping previously-good results.
