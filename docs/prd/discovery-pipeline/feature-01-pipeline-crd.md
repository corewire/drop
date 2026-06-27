# Feature 01: Pipeline CRD — queries / signals / ranking

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** ⬜ Not started

## Summary

Introduce the `queries`, `signals`, and `ranking` API on `DiscoveryPolicy.spec`, replacing
the single-purpose `sources[]` model with a reusable pipeline:

```yaml
spec:
  syncInterval: 1h
  maxImages: 30
  queries: []   # named raw-data fetchers
  signals: []   # named per-image metrics derived from a queryRef
  ranking: {}   # strategy that combines signals into the final ordered list
```

## Motivation

The current `spec.sources[]` couples data collection with scoring, so one fetch cannot
feed multiple metrics and scoring cannot combine metrics. The pipeline model decouples
these concerns and is the foundation every other feature in this PRD builds on.

## Current state

- ✅ `spec.syncInterval`, `spec.maxImages`, `spec.imageFilter` exist and are reusable as-is.
- ⬜ `spec.queries[]`, `spec.signals[]`, `spec.ranking{}` — not implemented.
- Existing model: `spec.sources[]` with inline `prometheus`/`registry` config in
  `api/v1alpha1/discoverypolicy_types.go`.

## Scope

- Define typed Go structs for `Query`, `Signal`, and `Ranking` with discriminator fields
  (`type`/`strategy`) and per-type config sub-objects.
- Validation: `signal.queryRef` must reference a defined query; `ranking` signal refs must
  reference defined signals; query/signal names unique.
- Decide the relationship to the existing `sources[]` field (see Open questions).
- Regenerate deepcopy and CRD manifests (`make codegen`).

## Out of scope

- Executing queries, computing signals, or ranking (Features 02–10).

## Acceptance criteria

- [ ] `DiscoveryPolicy.spec` accepts `queries`, `signals`, `ranking`.
- [ ] Cross-reference validation rejects dangling `queryRef`/`signalRef`.
- [ ] Duplicate query/signal names are rejected.
- [ ] `make codegen` produces updated CRD + deepcopy with no manual edits.
- [ ] Round-trip apply/get of all three Complete Examples from #55 succeeds.

## Open questions

- Migration: keep `sources[]` for backward compatibility, or replace it outright?
  (No existing users assumed — confirm before choosing.)
- `missingSignal` default behaviour: `zero` vs. drop image from ranking.
