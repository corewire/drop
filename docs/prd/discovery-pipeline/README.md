# PRD: Query → Signal → Ranking Pipeline for DiscoveryPolicy

Source proposal: [corewire/drop#55](https://github.com/corewire/drop/issues/55)

## Goal

Extend Drop's `DiscoveryPolicy` so image discovery can rank images with more than a
single usage-count score. The model is:

```text
queries -> signals -> ranking -> selected images
```

This separates data collection from scoring:

- **Queries** fetch raw observations from systems such as Prometheus or Loki.
- **Signals** derive named per-image metrics from query results.
- **Ranking strategies** combine one or more signals into the final ordered image list.

The objective is to support practical image-prewarming strategies for Kubernetes
CI/CD workloads, especially GitLab Kubernetes-executor node pools.

## Why

A count-only discovery strategy answers *"which images appeared most often?"* — useful
but incomplete. CI workloads have different shapes: steady all-day images, developer
feedback-hour images, short high-concurrency bursts, nightly validation jobs, rarely
used but expensive-when-cold images, and images that matter mainly because node
rotation leaves many nodes cold. Supporting these needs named input data, reusable
derived signals, and explicit ranking logic.

## Current state (as implemented in `main`)

`DiscoveryPolicy` today uses a flatter `spec.sources[]` model. Each source
(`prometheus` or `registry`) directly emits `{image, score}`; the controller dedups
(highest score wins), applies `imageFilter`, sorts by score, and truncates to
`maxImages`. Aggregation (`sum`/`count`/`avg`/`max`) lives **inside** a Prometheus
source, not as a reusable `signal`.

Verified against:

- `api/v1alpha1/discoverypolicy_types.go`
- `internal/discovery/prometheus.go`, `internal/discovery/registry.go`, `internal/discovery/source.go`
- `internal/controller/discoverypolicy_controller.go`

This means the count-based baseline strategies from the proposal (Total Usage, Peak
Concurrency) are **already achievable today** via a Prometheus source with
`aggregationMethod: sum` or `max`. The pipeline reshaping and the advanced strategies
are net-new.

## Feature slices

Each feature below is an issue-ready slice. Open one GitHub issue per file.

| # | Feature | Status | Depends on |
|---|---------|--------|-----------|
| 01 | [Pipeline CRD: queries / signals / ranking](feature-01-pipeline-crd.md) | ⬜ Not started | — |
| 02 | [Prometheus query execution](feature-02-prometheus-query.md) | 🟡 Mostly done | 01 |
| 03 | [Aggregate signals](feature-03-aggregate-signals.md) | 🟡 Mostly done | 01, 02 |
| 04 | [Basic `signal` ranking](feature-04-signal-ranking.md) | 🟡 Partially done | 01, 03 |
| 05 | [Weighted ranking (`weightedSum` + normalization)](feature-05-weighted-ranking.md) | ⬜ Not started | 01, 03, 04 |
| 06 | [Status & observability output](feature-06-status-observability.md) | 🟡 Partially done | 01, 02, 03, 04 |
| 07 | [Time-based signals](feature-07-time-based-signals.md) | ⬜ Not started | 01, 02, 03 |
| 08 | [Loki query source](feature-08-loki-query.md) | ⬜ Not started | 01 |
| 09 | [Event pull-time signal](feature-09-event-pull-time-signal.md) | ⬜ Not started | 01, 08 |
| 10 | [Model-aware exposure ranking](feature-10-model-exposure-ranking.md) | ⬜ Not started | 01, 05, 07, 09 |
| 11 | [Documentation](feature-11-documentation.md) | 🟡 Partially done | 01–10 |

Legend: ✅ done · 🟡 partial · ⬜ not started.

## Suggested sequencing

1. **Land the pipeline CRD (01)** — every other slice depends on the
   `queries`/`signals`/`ranking` shape.
2. **Migrate existing behaviour (02, 03, 04, 06)** — these already exist in the
   `sources[]` model and mostly need re-homing into the new API plus the small gaps
   (e.g. `aggregate.min`).
3. **Add scoring depth (05, 07)** — weighted sums, normalization, and time windows.
4. **Add new data sources (08, 09)** — Loki and event-derived pull-time.
5. **Add the model (10)** and **finish docs (11)**.

## Quick wins

- Add `min` to the aggregation enum (only gap in Feature 03).
- The Total-Usage and Peak-Concurrency strategies can be documented as supported today
  (Feature 11) without waiting on the pipeline redesign.
