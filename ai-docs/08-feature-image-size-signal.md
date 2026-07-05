# Feature: Image-Size Signal

## Overview

Kubernetes `Pulled` events from the kubelet report image size alongside pull duration. When Grafana Alloy ships these events to Loki (via `loki.source.kubernetes_events` with `log_format=json`), the parsed message includes both timing and byte count:

```
Successfully pulled image "docker.io/library/nginx:1.25-alpine" in 730ms. Image size: 20461242 bytes.
```

This size is a strong cold-start signal: large images dominate cache warm-up time and bandwidth on fresh node rotations.

## Ingestion Pipeline (Alloy → Loki)

### Setup

Deploy Grafana Alloy in your cluster with the `loki.source.kubernetes_events` component configured to:
- Watch all Kubernetes events (cluster-scoped)
- Set `log_format = "json"` so event fields are parsable
- Forward to Loki with external labels for filtering (e.g., `drop_e2e=true`)

Example Alloy config (from `hack/e2e-infra/alloy.yaml`):

```yaml
loki.source.kubernetes_events "events" {
  job_name   = "kubelet"
  log_format = "json"
  forward_to = [loki.write.local.receiver]
}

loki.write "local" {
  external_labels = { drop_e2e = "true" }
  endpoint {
    url = "http://loki.e2e-infra.svc.cluster.local:3100/loki/api/v1/push"
  }
}
```

### Field Mapping (Alloy JSON)

When Alloy ships events as JSON, the kubelet event fields map to:
- `name` → pod name (used by `podField`)
- `reason` → event reason (Pulled)
- `msg` → event message (contains both duration and image size)
- `type` → event type (Normal, Warning)
- `kind` → resource kind (Pod)

The DiscoveryPolicy parser config for Alloy events:

```yaml
parser:
  type: kubernetesEvents
  podField: name          # Alloy uses "name" for pod
  reasonField: reason
  messageField: msg       # Alloy uses "msg" for message
  imageField: msg         # Image extracted from message text
```

## Extracting Image Size

### Message Format

Successful pulls carry both duration and size:

```
Successfully pulled image "nginx:1.25" in 730ms. Image size: 20461242 bytes.
```

Only `Pulled` events are consumed for signal derivation.

### Parsing Logic

Image size is extracted from the pattern `Image size: N bytes` using regex:

```regex
Image\s+size:\s+(\d+)\s+bytes
```

- Match the literal text "Image size:"
- Capture the byte count (digits only, no decimals)
- Scale: bytes are used directly (no unit conversion needed)

### Per-Image Aggregation

Once parsed, image-size samples are aggregated per image using the same method as pull durations:
- **sum**: total bytes across all pulls (cumulative warm-up demand)
- **avg**: mean bytes per pull (representative pull size)
- **max**: largest single pull (worst-case node preload)
- **min**: smallest observed pull

For most use cases, `avg` (mean size) or `max` (worst-case) ranks images appropriately.

## DiscoveryPolicy Example

Query Loki for successful pulls, extract both duration and size, then rank by size:

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
  name: image-size-ranking
spec:
  syncInterval: 1h
  maxImages: 30

  queries:
    - name: pull-events
      type: loki
      loki:
        endpoint: https://loki.example.com
        queryType: range
        lookback: 7d
        query: |
          {job="kubelet", namespace="my-namespace"}
          | json
          | reason="Pulled"
      parser:
        type: kubernetesEvents
        podField: name           # Alloy field
        reasonField: reason
        messageField: msg        # Alloy field
        imageField: msg

  signals:
    # Extract average image size per image
    - name: avg-image-size
      queryRef: pull-events
      type: eventPullTime       # one parser extracts time + size; metric selects
      eventPullTime:
        metric: imageSize       # pullTime (default) | imageSize | failure | cacheHit
        statistic: avg          # p50 | p90 | p95 | avg | max | count

  ranking:
    strategy: signal
    signal:
      signalRef: avg-image-size
```

## Ranking Strategies

### Strategy 1: Size-Only (Largest Images First)

Rank images by their average or maximum size; largest images get pre-cached first:

```yaml
ranking:
  strategy: signal
  signal:
    signalRef: avg-image-size  # Rank by size
```

**Use when:** Node storage/bandwidth is the primary bottleneck.

### Strategy 2: Combined (Size + Usage)

Combine image size with usage frequency using `weightedSum`:

```yaml
signals:
  - name: avg-image-size
    queryRef: pull-events
    type: eventPullTime
    eventPullTime:
        metric: imageSize
  - name: usage-count
    queryRef: container-usage   # E.g., Prometheus metric
    type: aggregate
    aggregate:
      method: sum

ranking:
  strategy: weightedSum
  weightedSum:
    signalRefs:
      - name: avg-image-size
        weight: "0.4"           # 40% weight on size
      - name: usage-count
        weight: "0.6"           # 60% weight on frequency
```

**Use when:** Both frequently used *and* large images need priority.

### Strategy 3: Model-Exposure (Size × Pull-Time × Frequency)

Estimate total node warm-up cost using `modelExposure`:

```yaml
ranking:
  strategy: modelExposure
  modelExposure:
    exposureSignalRef: usage-count
    pullTimeSignalRef: avg-image-size
    nodeCount: 10                          # Your cluster node pool size
    targetJobCount: 50                     # Expected concurrent job starts
```

**Use when:** You want to optimize for job startup latency on node rotations.

## Implementation Notes

### Current Status

- **Parsing:** Image size extraction from Loki messages is implemented in `internal/discovery/loki.go` (the single `kubernetesEvents` parser extracts duration, failures, cache hits, and size).
- **Signal:** `metric: imageSize` on the `eventPullTime` signal ranks by bytes; `metric: pullTime` (default) ranks by duration. Both come from the same Pulled events.
- **Aggregation:** `statistic` (p50/p90/p95/avg/max/count) applies uniformly to whichever metric is selected.

### Verification

Test the full pipeline with the e2e test:

```bash
make test-e2e  # Includes discovery-loki-alloy test
```

Query Loki directly to inspect raw events:

```bash
# Replace endpoint and filters as needed
curl -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={job="kubelet"} | json | reason="Pulled"' \
  --data-urlencode 'start=<unix_ns>' \
  --data-urlencode 'end=<unix_ns>'
```

Look for messages like:
```
Successfully pulled image "nginx:1.25" in 730ms. Image size: 20461242 bytes.
```

### Future Extensions

- **Size + latency model:** Combine size and pull-time into a "node-warm-cost" signal (`bytes_transferred_seconds` proxy).
- **Layer granularity:** Extract per-layer sizes from manifest digests (requires registry API, not event-based).
