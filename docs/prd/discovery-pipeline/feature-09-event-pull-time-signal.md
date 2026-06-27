# Feature 09: Event pull-time signal

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** ⬜ Not started

## Summary

Implement the `eventPullTime` signal deriving per-image pull-time statistics from a Loki
event query:

```yaml
signals:
  - name: p50-cold-pull-time
    queryRef: image-pull-events
    type: eventPullTime
    eventPullTime:
      statistic: p50          # p50 | p90 | p95 | avg | max | count | failureCount | cacheHitCount
      includeCacheHits: false
      durationMode: eventPair # eventPair | messageDuration
```

## Motivation

Pull-time-aware and model-aware strategies need per-image cold-pull cost. This signal
turns raw pull events into statistics like p50/p95 cold-pull seconds.

## Current state

- ⬜ Not implemented. No event parsing, percentile computation, or cache-hit detection.

## Scope

- `durationMode`:
  - `eventPair` — `Pulled.timestamp − Pulling.timestamp` for the same pod/image.
  - `messageDuration` — parse the duration embedded in a `Pulled` event message.
- Statistics: `p50`, `p90`, `p95`, `avg`, `max`, `count`, `failureCount`, `cacheHitCount`.
- Cache-hit detection (`already present on machine`); excluded from cold-pull durations
  when `includeCacheHits: false`.

## Out of scope

- Loki query execution (Feature 08).

## Acceptance criteria

- [ ] `eventPair` and `messageDuration` duration modes both implemented.
- [ ] All listed statistics computed correctly (percentiles via a defined method).
- [ ] Cache hits detected and excluded when `includeCacheHits: false`.
- [ ] Unit tests cover paired/unpaired events, failures, and cache hits.
