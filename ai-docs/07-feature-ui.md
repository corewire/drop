# UI Feature Specs

Design specs for a future DiscoveryPolicy UI. All previews use a dry-run API — never persisted in etcd.

## 1. Query Editor (Stage 1)

| Element | Purpose |
|---------|---------|
| PromQL/LogQL/registry query input with syntax highlighting | Fast query iteration |
| Live preview table: image ref, raw sample values, sample count | Shows query output before saving the CR |
| Query health badge: latency, series count, error message | Surface slow/broken endpoints |
| Registry: collapsible tag list per repo with tagFilter preview | Highlight matching/excluded tags so regex is visible |

## 2. Signal Inspector (Stage 2)

| Element | Purpose |
|---------|---------|
| Bar chart per signal: images on Y-axis sorted by value | "Which images score highest on this signal?" |
| Side-by-side signal comparison (pick 2+) | Reveals when signals disagree on ranking |
| timeWeightedAggregate: heatmap (hour-of-day × image) | Shows if business-hours window config shifts rankings |
| eventPullTime: histogram of pull durations with p50/p90/p95 lines | Debug why an image ranks high ("it takes 12s to pull") |

## 3. Ranking Playground (Stage 3)

| Element | Purpose |
|---------|---------|
| Ranked image list with stacked bar score breakdown | Shows *why* an image is ranked #1 vs #5 |
| Weight sliders (weightedSum): drag to reorder in real-time | Eliminates apply-wait-check loop |
| maxImages cutoff line: draggable line on ranked list | Simulate different maxImages values |
| Diff view: images entering/leaving top-N, score deltas | "Did my config change improve things?" |
| modelExposure: node exposure diagram with estimated pull cost | Makes the abstract formula concrete |

## 4. Cross-cutting Views

| Element | Purpose |
|---------|---------|
| Pipeline DAG: query → signal → ranking with health per node | Overview for complex multi-query setups |
| etcd budget meter: current status size vs max | Ops visibility |
| Sync timeline: imageCount sparkline with sync events | Detects flapping (oscillating image count) |
| CachedImageSet propagation: discovered → CachedImage → node pull status | Closes the loop: discovery → caching → readiness |

## Architecture

- Previews (query editor, weight sliders) computed via a `/dryrun` endpoint or CLI tool
- Dry-run takes a `DiscoveryPolicySpec`, runs the pipeline once, returns full result without writing status
- CR only stores the last committed sync result (slimmed status)
- UI richness comes from dry-run responses, not from bloating the stored status
