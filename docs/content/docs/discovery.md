---
title: Discovery
weight: 3
aliases:
  - /drop/docs/discovery/
description: Automatic image discovery with DiscoveryPolicy.
llmsDescription: |
  DiscoveryPolicy CRD enables automatic image discovery using a three-stage pipeline:
  queries → signals → ranking. Referenced by CachedImageSet via discoveryPolicyRef.
  Discovered images are materialized as CachedImage resources. Supports filtering,
  time-weighted scoring, weighted ranking, and periodic re-discovery.
---

The DiscoveryPolicy CRD enables automatic image discovery from external sources. When referenced by a CachedImageSet, discovered images are automatically materialized as CachedImage resources.

## Why This Exists

Discovery came from operational pain:

- CI bursts created pull storms where many nodes pulled the same large images at once
- Registry rate limits and transient outages amplified cold-start latency
- Hand-maintained image lists became stale and missed newly hot images
- Node rotation (e.g. Cluster API MachineDeployments rolling new nodes daily or weekly) means fresh nodes start with empty image caches — every rotation triggers a full re-pull of all active images

With DiscoveryPolicy, image candidates are continuously sourced from real usage signals (metrics), ranked by configurable strategies, and consumed by CachedImageSet.

## Pipeline Overview

```
queries → signals → ranking → selected images
```

The pipeline has three stages:

1. **Queries** fetch raw observations from systems such as Prometheus or Loki.
2. **Signals** derive named per-image metrics from query results (e.g. `total-usage`, `peak-concurrency`).
3. **Ranking** combines one or more signals into the final ordered image list.

```
DiscoveryPolicy → runs pipeline → writes to status.discoveredImages
                                         ↓
CachedImageSet → reads discoveredImages → creates/deletes CachedImage children
```

## Stage 1 — Queries

A query fetches raw observations and is referenced by name from signals.

### Prometheus Query

```yaml
queries:
  - name: runner-image-usage
    type: prometheus
    prometheus:
      endpoint: https://mimir.example.com
      queryType: range        # range | instant (default: range)
      lookback: 168h          # time window for range queries
      step: 1m                # range resolution (default: 5m)
      query: |
        count(
          container_memory_working_set_bytes{
            container!="", container!="POD",
            namespace="gitlab-runner", pod=~"runner-.*"
          }
        ) by (image)
```

The PromQL result **must** carry an `image` label. That label value is the discovered image reference.

### Loki Query

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
        type: kubernetesEvents
        podField: involvedObject_name
        reasonField: reason
        messageField: message
        imageField: message
```

### Auth / TLS

Both query types support a `secretRef` for authentication and TLS:

```yaml
queries:
  - name: runner-image-usage
    type: prometheus
    prometheus:
      endpoint: https://mimir.example.com
      query: ...
    secretRef:
      name: prometheus-creds  # Secret in the drop-system namespace
```

Supported Secret keys: `token`, `username`, `password`, `ca.crt`, `tls.crt`, `tls.key`, `headers.<name>`.

## Stage 2 — Signals

A signal derives a named per-image value from exactly one query.

### `aggregate`

Aggregates all samples per image using a single method.

```yaml
signals:
  - name: total-usage
    queryRef: runner-image-usage
    type: aggregate
    aggregate:
      method: sum    # sum | max | avg | count | min

  - name: peak-concurrency
    queryRef: runner-image-usage
    type: aggregate
    aggregate:
      method: max
```

### `timeWeightedAggregate`

Multiplies each sample value by a per-hour window weight before aggregation.

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
        - startHour: 7
          endHour: 9
          weight: "300m"    # 0.3 (resource.Quantity format)
        - startHour: 9
          endHour: 17
          weight: "1"       # 1.0 — full weight during core hours
        - startHour: 17
          endHour: 20
          weight: "300m"
```

### `windowAggregate`

Aggregates only the samples within a specific time sub-window.

```yaml
signals:
  # Relative window (last N duration before now)
  - name: recent-usage
    queryRef: runner-image-usage
    type: windowAggregate
    windowAggregate:
      method: sum
      relativeWindow: 2h

  # Wall-clock window (specific hours of day)
  - name: pre-window-usage
    queryRef: runner-image-usage
    type: windowAggregate
    windowAggregate:
      method: sum
      timezone: Europe/Berlin
      window:
        start: "00:00"
        end: "09:00"
```

### `eventPullTime`

Derives image pull-time statistics from Loki event records.

```yaml
signals:
  - name: p50-cold-pull-time
    queryRef: image-pull-events
    type: eventPullTime
    eventPullTime:
      statistic: p50            # p50 | p90 | p95 | avg | max | count | failureCount | cacheHitCount
      includeCacheHits: false
      durationMode: eventPair   # eventPair | messageDuration
```

## Stage 3 — Ranking

Exactly one ranking strategy per policy.

### `signal`

Ranks images directly by the value of a single signal.

```yaml
ranking:
  strategy: signal
  signal:
    signalRef: total-usage
```

### `weightedSum`

Combines normalized signals using a weighted sum.

```yaml
ranking:
  strategy: weightedSum
  weightedSum:
    normalize: minMax      # only method available
    missingSignal: zero    # zero | drop
    terms:
      - signalRef: total-usage
        weight: "700m"     # 0.7 in resource.Quantity format
      - signalRef: peak-concurrency
        weight: "300m"     # 0.3
```

Score: `final_score(I) = Σ weight_k * normalize(signal_k(I))`

`minMax` normalization: `normalized(x) = (x - min) / (max - min)` — equals 1 when all values are equal.

### `modelExposure`

Ranks images by expected post-rotation cold-node exposure.

```yaml
ranking:
  strategy: modelExposure
  modelExposure:
    nodeCount: 100
    preWindowUsageSignalRef: pre-window-usage
    targetWindowUsageSignalRef: developer-window-usage
    pullTimeSignalRef: p50-cold-pull-time
```

Score: `score(I) = J_target(I) * (1 - 1/N)^J_pre(I) * p_hat(I)`

## Complete Examples

### Example 1: Total Usage (simplest)

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: total-usage
spec:
  syncInterval: 1h
  maxImages: 30

  queries:
    - name: runner-image-usage
      type: prometheus
      prometheus:
        endpoint: https://mimir.example.com
        queryType: range
        lookback: 168h
        step: 1m
        query: |
          count(
            container_memory_working_set_bytes{
              container!="", container!="POD",
              namespace="gitlab-runner", pod=~"runner-.*"
            }
          ) by (image)

  signals:
    - name: total-usage
      queryRef: runner-image-usage
      type: aggregate
      aggregate:
        method: sum

  ranking:
    strategy: signal
    signal:
      signalRef: total-usage
```

### Example 2: Hybrid Usage + Peak Concurrency

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: gitlab-hybrid-usage-concurrency
spec:
  syncInterval: 1h
  maxImages: 30

  queries:
    - name: runner-image-usage
      type: prometheus
      prometheus:
        endpoint: https://mimir.example.com
        queryType: range
        lookback: 168h
        step: 1m
        query: |
          count(
            container_memory_working_set_bytes{
              container!="", container!="POD",
              namespace="gitlab-runner", pod=~"runner-.*"
            }
          ) by (image)

  signals:
    - name: total-usage
      queryRef: runner-image-usage
      type: aggregate
      aggregate:
        method: sum

    - name: peak-concurrency
      queryRef: runner-image-usage
      type: aggregate
      aggregate:
        method: max

  ranking:
    strategy: weightedSum
    weightedSum:
      normalize: minMax
      missingSignal: zero
      terms:
        - signalRef: total-usage
          weight: "700m"
        - signalRef: peak-concurrency
          weight: "300m"
```

### Example 3: Developer-Time Weighted Usage

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: gitlab-developer-and-burst
spec:
  syncInterval: 1h
  maxImages: 30

  queries:
    - name: runner-image-usage
      type: prometheus
      prometheus:
        endpoint: https://mimir.example.com
        queryType: range
        lookback: 168h
        step: 1m
        query: |
          count(
            container_memory_working_set_bytes{
              container!="", container!="POD",
              namespace="gitlab-runner", pod=~"runner-.*"
            }
          ) by (image)

  signals:
    - name: developer-weighted-usage
      queryRef: runner-image-usage
      type: timeWeightedAggregate
      timeWeightedAggregate:
        method: sum
        timezone: Europe/Berlin
        defaultWeight: "0"
        windows:
          - startHour: 7
            endHour: 9
            weight: "300m"
          - startHour: 9
            endHour: 17
            weight: "1"
          - startHour: 17
            endHour: 20
            weight: "300m"

    - name: peak-concurrency
      queryRef: runner-image-usage
      type: aggregate
      aggregate:
        method: max

  ranking:
    strategy: weightedSum
    weightedSum:
      normalize: minMax
      missingSignal: zero
      terms:
        - signalRef: developer-weighted-usage
          weight: "700m"
        - signalRef: peak-concurrency
          weight: "300m"
```

## Status and Observability

The controller exposes per-query, per-signal, and per-image ranking detail in status:

```yaml
status:
  lastSyncTime: "2026-06-18T10:00:00Z"

  queryResults:
    - name: runner-image-usage
      type: prometheus
      series: 30
      samples: 60480
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

> **Note:** Pipeline execution is not yet implemented. The controller currently sets
> `Ready=False, reason=NotImplemented` and will populate status once execution is
> available in a future release (Issues 2–10 in the implementation sequence).

## Discovery Strategies Reference

| # | Strategy | Score formula | Signals needed |
|---|----------|---------------|----------------|
| 1 | Total usage | `Σ count_I(t)` over W | `total-usage` |
| 2 | Peak same-image concurrency | `max count_I(t)` over W | `peak-concurrency` |
| 3 | Developer-time weighted usage | `Σ weight(t)·count_I(t)` | `developer-weighted-usage` |
| 4 | Recent usage | `Σ count_I(t)` over recent window | `recent-usage` |
| 5 | Hybrid usage + peak | `α·norm(total) + (1-α)·norm(peak)` | `total-usage`, `peak-concurrency` |
| 6 | Hybrid dev-time + peak | `α·norm(dev) + (1-α)·norm(peak)` | `developer-weighted-usage`, `peak-concurrency` |
| 7 | Count × pull time | `total_usage(I) · p_hat(I)` | `total-usage`, `p50-cold-pull-time` |
| 9 | Model-aware exposure | `J_target · (1-1/N)^J_pre · p_hat` | `pre-window-usage`, `target-window-usage`, `p50-cold-pull-time` |

## Error Handling

- On transient failures, the operator keeps the **last known good** discovery results
- Source health is tracked via conditions on the DiscoveryPolicy status
- Each query is executed independently — one failing query does not block others
