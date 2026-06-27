# Feature 02: Prometheus query execution

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** 🟡 Mostly done

## Summary

Execute named Prometheus queries and emit a normalized per-image sample stream:

```text
timestamp,image,value
```

A query must return an `image` label; results without it are ignored. One query feeds
one or more signals.

## Motivation

Prometheus is the primary data source for usage, concurrency, and time-window signals.
The pipeline needs queries that are named and independent of scoring so a single fetch
can feed multiple signals.

## Current state

Implemented in `internal/discovery/prometheus.go`:

- ✅ Range (`/api/v1/query_range`) and instant (`/api/v1/query`) execution.
- ✅ `lookback`, `step` (default 5m), endpoint, secret-based auth.
- ✅ Per-image extraction keyed on the `image` label; missing-label results skipped.
- ✅ Status-success check and HTTP error surfacing.
- ⬜ Exposed as a named `query` decoupled from a source/signal (currently coupled to a
  `sources[]` entry that also scores).

## Scope

- Re-home the existing Prometheus client behind the new `query` API (`type: prometheus`).
- Preserve range/instant, lookback, step, and auth behaviour.
- Output the normalized `timestamp,image,value` series for consumption by signals
  (rather than directly producing a score).

## Out of scope

- Aggregation/scoring of the series (Feature 03+).

## Acceptance criteria

- [ ] A `type: prometheus` query produces a normalized series consumable by ≥2 signals.
- [ ] Range and instant query types both work.
- [ ] Missing `image` label handled per defined behaviour (ignore + surface in status).
- [ ] Query failures are surfaced in status (Feature 06).
- [ ] Existing Prometheus auth (token/basic/TLS/headers) continues to work.
