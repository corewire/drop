# Feature 07: Time-based signals

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** ⬜ Not started

## Summary

Implement two time-aware signal types over a query series:

- `timeWeightedAggregate` — apply per-time-window weights before aggregating
  (e.g. developer-feedback hours weighted higher).
- `windowAggregate` — aggregate a specific sub-window (recent, pre-window, target-window).

```yaml
signals:
  - name: developer-weighted-usage
    queryRef: runner-image-usage
    type: timeWeightedAggregate
    timeWeightedAggregate:
      method: sum
      timezone: Europe/Berlin
      defaultWeight: "0"
      windows:
        - { startHour: 9, endHour: 17, weight: "1.0" }
  - name: recent-usage
    queryRef: runner-image-usage
    type: windowAggregate
    windowAggregate:
      method: sum
      relativeWindow: 2h
```

## Motivation

Many prewarming strategies care about *when* usage happened: developer-time weighting,
recent-usage emphasis, and pre/target windows feeding the model-exposure strategy.

## Current state

- ⬜ Not implemented. No timezone, window, or weight handling exists.

## Scope

- `timeWeightedAggregate`: timezone-aware hour windows, per-window weight, `defaultWeight`,
  then aggregate by `method`.
- `windowAggregate`: either a `relativeWindow` (e.g. `2h`) or an absolute
  `start`/`end` clock window with timezone; then aggregate by `method`.
- Operate on the normalized query series (sample timestamps).

## Out of scope

- Plain whole-window aggregation (Feature 03).

## Acceptance criteria

- [ ] `timeWeightedAggregate` applies window weights and `defaultWeight` correctly across
      a timezone boundary.
- [ ] `windowAggregate` supports both relative and absolute (clock) windows.
- [ ] Unit tests cover timezone handling, window edges, and empty windows.
- [ ] Signals feed both `signal` and `weightedSum` rankings.
