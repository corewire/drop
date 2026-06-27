# Feature 03: Aggregate signals

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** 🟡 Mostly done

## Summary

Implement the `aggregate` signal type that reduces all samples per image to a single
value:

```yaml
signals:
  - name: total-usage
    queryRef: runner-image-usage
    type: aggregate
    aggregate:
      method: sum   # sum | max | avg | count | min
```

## Motivation

Aggregation is the simplest and most common way to turn a sample series into a
per-image metric (e.g. `total-usage` = sum, `peak-concurrency` = max).

## Current state

Implemented in `internal/discovery/prometheus.go` (`aggregateRangeValues`) and the
`AggregationMethod` enum in `api/v1alpha1/discoverypolicy_types.go`:

- ✅ `sum`, `max`, `avg`, `count`.
- ⬜ `min` — not in the enum.
- ⬜ Exposed as a standalone `aggregate` **signal** (currently a `prometheus` source field
  `aggregationMethod`).

## Scope

- Implement an `aggregate` signal type consuming a `queryRef` series.
- Support methods `sum`, `max`, `avg`, `count`, **`min`**.
- Reuse existing aggregation math; add the missing `min` reducer.

## Out of scope

- Time-weighted or windowed aggregation (Feature 07).

## Acceptance criteria

- [ ] `aggregate` signal supports `sum`, `max`, `avg`, `count`, `min`.
- [ ] Table-driven unit tests cover each method, including empty input.
- [ ] A signal references a query by `queryRef` and produces one value per image.
- [ ] `min` added to the validation enum and CRD manifest via `make codegen`.
