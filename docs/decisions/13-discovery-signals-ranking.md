# Feature: Discovery Query/Signal/Ranking Pipeline

Status: **Accepted (not yet implemented)** — ready to start when prioritized.
Supersedes the `DiscoveryPolicy.spec.sources[]` shape described in
`12-naming-structure-proposals.md`.

## Goal

Extend `DiscoveryPolicy` so image discovery can rank images with more than a
single usage-count score. Replace the current single-axis `sources[]` model with
an explicit three-stage pipeline:

```text
queries -> signals -> ranking -> selected images
```

- **Queries** fetch raw observations from systems such as Prometheus or Loki.
- **Signals** derive named per-image metrics from one query's results.
- **Ranking** combines one or more signals into the final ordered image list.

This separates *data collection* from *scoring* so one query can feed multiple
signals and one ranking can combine multiple signals.

## Decision (locked)

- This is a **breaking change**. `spec.sources[]` is **removed**, not kept
  alongside the new shape. `DiscoveryPolicy` is `v1alpha1`, which carries no
  backward-compatibility guarantee, and no `DiscoveryPolicy` is in production
  use, so a clean break is cheaper than maintaining dual shapes plus a
  conversion layer. (See "Open question resolution" in
  `10-policy-redesign-proposals.md` — no migration path needed pre-stability.)
- The old `registry` discovery source type (`RegistrySource`:
  `url`/`repositories`/`tagFilter`/`topX`/`imageTemplate`) is **dropped** in this
  redesign. The new model only defines `prometheus` and `loki` query types.
  Registry tag discovery can be reintroduced later as a `registry` query type if
  needed; it is explicitly out of scope for the first implementation.
- Implementation is split into 11 sequential issues (see "Implementation split").
  Issue 1 (the CRD types) is the foundation every later issue depends on and must
  land first.

## Why the single-count model is insufficient

A count-based strategy answers only "which images appeared most often?" CI/CD
workloads (especially GitLab Kubernetes executor node pools) have varied shapes:

- steady all-day usage,
- usage concentrated in developer feedback hours,
- short high-concurrency bursts (fan-out / nightly validation),
- infrequent but expensive-when-cold images,
- node rotation leaving many nodes cold for specific images.

Supporting these requires named input data, reusable derived signals, and
explicit ranking logic.

---

## Target CRD shape

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: gitlab-runner-discovery
spec:
  syncInterval: 1h        # retained from current API
  maxImages: 30           # retained from current API
  imageFilter: ""         # retained from current API (regex on discovered refs)

  queries: []
  signals: []
  ranking: {}
status:
  # see "Status and observability"
```

`syncInterval`, `maxImages`, and `imageFilter` keep their current semantics and
defaults (`syncInterval: 30m`, `maxImages: 50`). `sources` and
`status.sourceCount` are removed.

---

## Stage 1 — Queries

A query fetches raw observations and is referenced by name from signals.

### Common query fields

| Field        | Type                  | Notes |
|--------------|-----------------------|-------|
| `name`       | string (required)     | Unique within the policy; referenced by `signal.queryRef`. |
| `type`       | enum `prometheus`/`loki` (required) | Selects which typed config block is read. |
| `prometheus` | object                | Required when `type=prometheus`. |
| `loki`       | object                | Required when `type=loki`. |
| `secretRef`  | LocalObjectReference  | Optional auth, same well-known Secret keys as today (`token`, `username`, `password`, `ca.crt`, `tls.crt`, `tls.key`, `headers.<name>`). Secret lives in the pod namespace (default `drop-system`). |

### Prometheus query

```yaml
queries:
  - name: runner-image-usage
    type: prometheus
    prometheus:
      endpoint: https://mimir.example.com   # Prometheus-compatible API (Prometheus/Thanos/Mimir/VictoriaMetrics)
      queryType: range                       # range | instant (default range)
      lookback: 168h                         # required for range
      step: 1m                               # range resolution (default 5m)
      query: |
        count(
          container_memory_working_set_bytes{
            container!="", container!="POD",
            namespace="gitlab-runner", pod=~"runner-.*"
          }
        ) by (image)
```

- The PromQL result MUST carry an `image` label; that label value is the
  discovered image reference.
- Normalized output: `timestamp,image,value`.
- The `endpoint`/`queryType`/`lookback`/`step`/`query` fields carry over directly
  from today's `PrometheusSource`, so the existing Prometheus client logic is
  reusable.

### Loki query

```yaml
queries:
  - name: image-pull-events
    type: loki
    loki:
      endpoint: https://loki.example.com
      queryType: range
      lookback: 168h
      query: |
        {job="kubernetes-events", namespace="gitlab-runner"}
        | json
        | involvedObject_name =~ "runner-.*"
        | reason =~ "Pulling|Pulled|Failed|BackOff"
      parser:
        type: kubernetesEvents          # only parser type for v1
        podField: involvedObject_name
        reasonField: reason
        messageField: message
        imageField: message             # image is extracted from the message text
```

- Normalized output: `timestamp,pod,image,reason,message`.
- Expected event messages to parse:
  - `Pulling image "<ref>"`
  - `Successfully pulled image "<ref>" in 42.3s`
  - `Container image "<ref>" already present on machine` (cache hit)
  - `Failed to pull image "<ref>"`
  - `Back-off pulling image "<ref>"`

### Pull-cost profile (future, not in v1 CRD)

Optional external alternative to Loki, producing
`image,p50ColdPullSeconds,p95ColdPullSeconds,sampleCount`. Generated by an
external analyzer. Deferred; see "Open decisions".

---

## Stage 2 — Signals

A signal derives a named per-image value from exactly one query
(`queryRef`). `type` selects the typed config block.

| Signal `type`           | Purpose                                   | Example signal names |
|-------------------------|-------------------------------------------|----------------------|
| `aggregate`             | Aggregate all samples per image           | `total-usage`, `peak-concurrency` |
| `timeWeightedAggregate` | Apply per-time-window weights, then aggregate | `developer-weighted-usage` |
| `windowAggregate`       | Aggregate only a specific sub-window       | `recent-usage`, `pre-window-usage`, `target-window-usage` |
| `eventPullTime`         | Derive pull-time stats from Loki events    | `p50-cold-pull-time`, `p95-cold-pull-time` |

### Common signal fields

| Field      | Type              | Notes |
|------------|-------------------|-------|
| `name`     | string (required) | Unique within the policy; referenced by ranking. |
| `queryRef` | string (required) | Must match a `queries[].name`. |
| `type`     | enum (required)   | One of the four above. |

### `aggregate`

```yaml
signals:
  - name: total-usage
    queryRef: runner-image-usage
    type: aggregate
    aggregate:
      method: sum        # sum | max | avg | count | min
  - name: peak-concurrency
    queryRef: runner-image-usage
    type: aggregate
    aggregate:
      method: max
```

Note: `min` is a new method on the existing `AggregationMethod` enum (currently
`sum;count;avg;max`).

### `timeWeightedAggregate`

```yaml
signals:
  - name: developer-weighted-usage
    queryRef: runner-image-usage
    type: timeWeightedAggregate
    timeWeightedAggregate:
      method: sum
      timezone: Europe/Berlin     # IANA tz; required because windows are wall-clock
      defaultWeight: "0"          # weight for hours not covered by any window
      windows:
        - startHour: 7
          endHour: 9
          weight: "0.3"
        - startHour: 9
          endHour: 17
          weight: "1.0"
        - startHour: 17
          endHour: 20
          weight: "0.3"
```

- `startHour`/`endHour` are integers 0-24 (local hour-of-day).
- Each sample's value is multiplied by the matching window weight (or
  `defaultWeight`) before aggregation.

### `windowAggregate`

Two mutually exclusive window forms:

```yaml
# relative-to-now form
signals:
  - name: recent-usage
    queryRef: runner-image-usage
    type: windowAggregate
    windowAggregate:
      method: sum
      relativeWindow: 2h          # aggregate only the last 2h of samples

# wall-clock-of-day form
  - name: pre-window-usage
    queryRef: runner-image-usage
    type: windowAggregate
    windowAggregate:
      method: sum
      timezone: Europe/Berlin
      window: { start: "00:00", end: "09:00" }

  - name: developer-window-usage
    queryRef: runner-image-usage
    type: windowAggregate
    windowAggregate:
      method: sum
      timezone: Europe/Berlin
      window: { start: "09:00", end: "17:00" }
```

- Exactly one of `relativeWindow` or (`window` + `timezone`) must be set.

### `eventPullTime`

```yaml
signals:
  - name: p50-cold-pull-time
    queryRef: image-pull-events     # must reference a loki query
    type: eventPullTime
    eventPullTime:
      statistic: p50                # p50 | p90 | p95 | avg | max | count | failureCount | cacheHitCount
      includeCacheHits: false       # exclude "already present" events from cold-pull duration
      durationMode: eventPair       # eventPair | messageDuration
```

| `durationMode`    | Meaning |
|-------------------|---------|
| `eventPair`       | `Pulled.timestamp - Pulling.timestamp` for the same Pod/image. |
| `messageDuration` | Parse the duration directly from a `Pulled` event message (e.g. `... in 42.3s`). |

When `includeCacheHits: false`, "already present on machine" events are detected
and excluded from cold-pull duration statistics.

---

## Stage 3 — Ranking

Exactly one strategy per policy.

| `strategy`      | Purpose |
|-----------------|---------|
| `signal`        | Rank directly by one signal. |
| `weightedSum`   | Combine normalized signals into a weighted score. |
| `modelExposure` | Rank by expected post-rotation cold-node exposure. |

### `signal`

```yaml
ranking:
  strategy: signal
  signal:
    signalRef: total-usage
```

### `weightedSum`

```yaml
ranking:
  strategy: weightedSum
  weightedSum:
    normalize: minMax          # only method for v1
    missingSignal: zero        # zero | drop  (default zero — see open decisions)
    terms:
      - signalRef: total-usage
        weight: "0.7"
      - signalRef: peak-concurrency
        weight: "0.3"
```

Score:

```text
final_score(I) = Σ weight_k * normalize(signal_k(I))
```

`minMax` normalization:

```text
normalized(x) = (x - min) / (max - min)
normalized(x) = 1   if all values are equal
```

### `modelExposure`

```yaml
ranking:
  strategy: modelExposure
  modelExposure:
    nodeCount: 100
    preWindowUsageSignalRef: pre-window-usage
    targetWindowUsageSignalRef: developer-window-usage
    pullTimeSignalRef: p50-cold-pull-time
```

Score:

```text
score(I) = J_target(I) * (1 - 1/N)^J_pre(I) * p_hat(I)
```

where `N = nodeCount`, `J_pre`/`J_target` are the pre/target window usage
signals, and `p_hat` is the pull-time signal. `cold_fraction_hat(I) =
(1 - 1/N)^J_pre(I)` is the probability a node is still cold for image `I` at the
start of the target window.

---

## Discovery strategy catalog

The pipeline must express these strategies (build/test against all of them):

| # | Strategy | Score | Signals | Queries |
|---|----------|-------|---------|---------|
| 1 | Total usage | `Σ count_I(t)` over W | `total-usage` | Prometheus |
| 2 | Peak same-image concurrency | `max count_I(t)` over W | `peak-concurrency` | Prometheus |
| 3 | Developer-time weighted usage | `Σ weight(t)·count_I(t)` | `developer-weighted-usage` | Prometheus |
| 4 | Recent usage | `Σ count_I(t)` over recent window | `recent-usage` | Prometheus |
| 5 | Hybrid usage + peak | `α·norm(total) + (1-α)·norm(peak)` | `total-usage`, `peak-concurrency` | Prometheus |
| 6 | Hybrid dev-time + peak | `α·norm(dev) + (1-α)·norm(peak)` | `developer-weighted-usage`, `peak-concurrency` | Prometheus |
| 7 | Count × pull time | `total_usage(I)·p_hat(I)` | `total-usage`, `p50/p95-cold-pull-time` | Prometheus + Loki |
| 8 | Dev-weighted count × pull time | `dev_usage(I)·p_hat(I)` | `developer-weighted-usage`, `p50-cold-pull-time` | Prometheus + Loki |
| 9 | Model-aware exposure | see `modelExposure` | `pre-window-usage`, `target-window-usage`, `p50-cold-pull-time` | Prometheus + Loki |

First production-ready strategies (deliver first): 1, 2, 5, 3, 6 — i.e.
`signal(total-usage)`, `signal(peak-concurrency)`,
`weightedSum(total-usage, peak-concurrency)`,
`signal(developer-weighted-usage)`,
`weightedSum(developer-weighted-usage, peak-concurrency)`.

Advanced strategy (deliver last):
`modelExposure(pre-window-usage, target-window-usage, p50-cold-pull-time)`.

---

## Status and observability

Status must explain every selected image. Replaces the current
`sourceCount`/flat `discoveredImages` status.

```yaml
status:
  lastRunTime: "2026-06-18T10:00:00Z"
  observedGeneration: 4

  queryResults:
    - name: runner-image-usage
      type: prometheus
      series: 30
      samples: 60480
      status: success           # success | failed (with message)
    - name: image-pull-events
      type: loki
      records: 1820
      status: success

  signalResults:
    - name: total-usage
      images: 30
      status: success
    - name: peak-concurrency
      images: 30
      status: success

  discoveredImages:
    - image: registry.example.com/ci/java-gradle:21
      rank: 1
      finalScore: "0.8768"
      selected: true
      signals:
        - name: total-usage
          rawValue: "8210"
          normalizedValue: "0.824"
        - name: peak-concurrency
          rawValue: "96"
          normalizedValue: "1.0"
      ranking:
        strategy: weightedSum
        terms:
          - signal: total-usage
            weight: "0.7"
            contribution: "0.5768"
          - signal: peak-concurrency
            weight: "0.3"
            contribution: "0.3"
```

Status must support debugging: query failures, missing `image` labels, missing
signals, normalization values, ranking contributions, and the final selected
set. `CachedImageSet` only consumes `status.discoveredImages[].image`, so it
keeps working as long as that field is present.

Print columns to update: drop the `Sources` column; add a `Queries` count column
alongside the existing `Images`/`LastSync` columns.

---

## Required pipeline capabilities (acceptance criteria)

- One query can feed multiple signals.
- Multiple signals can feed one ranking.
- Prometheus output normalizes to `timestamp,image,value`.
- Loki output normalizes to `timestamp,pod,image,reason,message`.
- Missing `image` labels are handled per a defined rule (reject or ignore).
- Query/signal failures surface in status without failing the whole reconcile
  where partial results are usable.
- Selected image order is deterministic, including tie-breaking.

---

## Validation plan

**Query tests** — Prometheus normalization; Loki normalization; missing-label
handling; failure surfaced in status.

**Signal tests** — `aggregate.sum/max/avg/count/min`; `timeWeightedAggregate`;
`windowAggregate` (both window forms); `eventPullTime` (each statistic, both
duration modes, cache-hit exclusion).

**Ranking tests** — `signal`; `weightedSum` (normalization, missing-signal
behavior, deterministic ties); `modelExposure`.

**Integration tests** — fake Prometheus and Loki responses verifying: one query →
many signals; many signals → one ranking; deterministic selected order; status
contains query, signal, and ranking detail.

Follow existing patterns: table-driven unit tests, envtest for the controller
(`internal/controller/*_test.go`), Chainsaw e2e in `test/e2e/`.

---

## Package placement

- API types: `api/v1alpha1/discoverypolicy_types.go` (+ regenerate
  `zz_generated.deepcopy.go` and `config/crd/bases/` via `make codegen`).
- Query execution: `internal/discovery/` (existing Prometheus client logic moves
  under the `prometheus` query type; add a `loki` client).
- Signal derivation and ranking: new pure-function packages under
  `internal/discovery/` (e.g. `signal`, `ranking`) so they are unit-testable
  without a Kubernetes client, mirroring the `internal/podbuilder` convention.
- Reconcile wiring and status: `internal/controller/discoverypolicy_controller.go`.
- Regenerate AI docs with `make docs-gen` after API changes.

---

## Implementation split

Land in order; each is a self-contained PR.

1. **CRD for query/signal/ranking pipeline** — define `queries`, `signals`,
   `ranking` types and new status; remove `sources[]`/`sourceCount`/registry
   types; regenerate deepcopy + CRD YAML; update samples and AI docs; make the
   tree compile (stub execution to a clear "not implemented" condition).
2. **Prometheus query execution** — named range/instant queries → normalized
   `timestamp,image,value`.
3. **Aggregate signals** — `sum`, `max`, `avg`, `count`, `min`.
4. **Basic ranking** — `signal` strategy.
5. **Weighted ranking** — `weightedSum` with `minMax` normalization.
6. **Status output** — query results, signal results, ranking contributions,
   selected images.
7. **Time-based signals** — `timeWeightedAggregate`, `windowAggregate`.
8. **Loki query source** — Loki range queries + `kubernetesEvents` parser.
9. **Event pull-time signal** — `eventPullTime` (statistics, duration modes,
   cache-hit exclusion).
10. **Model-aware exposure ranking** — typed `modelExposure`.
11. **Documentation** — total usage, peak concurrency, developer-time usage,
    hybrid usage/concurrency, pull-time-aware ranking, model-aware exposure.

Issues 1-6 deliver the first production-ready strategies; 7-10 deliver the
advanced ones.

---

## Open decisions to resolve before/while implementing

1. **Missing-signal behavior** — `missingSignal: zero` (treat absent signal as 0)
   vs. `drop` (remove the image from ranking if a required signal is missing).
   Recommendation: default `zero`, make it configurable per `weightedSum`.
2. **Pull-time statistic default** — `p50` vs. `p95` cold-pull time.
   Recommendation: configurable via `eventPullTime.statistic`; no hard default
   baked into ranking.
3. **Pull-time source** — native Loki + `eventPullTime` vs. an external
   `ImagePullCostProfile` produced by a separate analyzer. Recommendation: ship
   the Loki types in the CRD now (cheap), implement under Issues 8-9; keep the
   external-profile path as a documented future query type.
4. **Decimal field type** — weights/`defaultWeight`/window weights as quoted
   `string` vs. `resource.Quantity`. Recommendation: `resource.Quantity` for
   built-in validation and exact decimal arithmetic. (Status `finalScore`,
   `normalizedValue`, `contribution` likewise stored as strings.)
5. **Status object size** — full per-image signal/contribution breakdown can grow
   the object near the etcd size limit at large `maxImages`. Recommendation: cap
   the detailed breakdown to the selected top-N (`maxImages`) and keep
   non-selected images minimal.
6. **Registry discovery** — confirmed dropped for v1; reintroduce later as a
   `registry` query type only if demand appears. The existing
   `test/e2e/discovery-aggregation/` fixtures and registry client code are
   removed/retired as part of Issue 1.

---

## Consequences

- Clean, extensible discovery model: new query types, signal types, and ranking
  strategies can be added without reshaping the spec.
- One-time breaking change to `DiscoveryPolicy`; no installed resources to
  migrate.
- Loss of registry-tag discovery until/unless reintroduced as a query type.
- Larger, richer status object — mitigated by the top-N cap above.
