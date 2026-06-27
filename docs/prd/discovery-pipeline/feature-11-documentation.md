# Feature 11: Documentation

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** 🟡 Partially done

## Summary

Document the discovery pipeline and the supported ranking strategies for users.

## Motivation

The pipeline is only useful if operators understand which strategy to pick and how to
configure queries, signals, and ranking.

## Current state

- 🟡 `docs/content/docs/discovery.md` documents the existing `sources`/prometheus/registry
  model.
- ⬜ Nothing for the queries/signals/ranking pipeline or the new strategies.

## Scope

Document each strategy with config, use-cases, and limitations:

- Total usage
- Peak concurrency
- Developer-time weighted usage
- Recent usage
- Hybrid usage / peak concurrency
- Hybrid developer-time / peak concurrency
- Count × pull time
- Developer-weighted count × pull time
- Model-aware exposure

Also document query types (Prometheus, Loki), signal types (aggregate,
timeWeightedAggregate, windowAggregate, eventPullTime), ranking strategies (signal,
weightedSum, modelExposure), and the explainable status output.

## Conventions

- Keep docs short and high-level per repo guidance; avoid volatile implementation detail.
- Verify all examples against a real cluster before merging.
- Regenerate AI docs with `make docs-gen` (do not hand-edit generated files).

## Acceptance criteria

- [ ] Each ranking strategy documented with config + use-case + limitation.
- [ ] Query/signal/ranking reference documented.
- [ ] Examples verified against a real cluster.
- [ ] `make docs-gen` run; generated artifacts updated.
