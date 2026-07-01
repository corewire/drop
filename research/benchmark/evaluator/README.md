# CI Image Cache Benchmark Evaluator

This directory contains a small pandas-based evaluator suite for the paper
"Measuring CI Feedback Delay from Cold OCI Image Caches".

It is designed for two use cases:

1. **Synthetic benchmark data** for the 25,000-job scenario used in the paper.
2. **Real-cluster replay** from GitLab Kubernetes executor Pods collected from Prometheus, kube-state-metrics, Kubernetes Events, and/or GitLab job exports.

The evaluator intentionally separates three concepts:

- image usage discovery: which images appear often enough to consider prewarming,
- node-local cache state: whether image `I` is available on node `n`,
- developer-facing impact: affected jobs, affected job-minutes, and pipeline critical-path delay.

## Quick start

```bash
python -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt

python generate_synthetic_day.py --out data --jobs 25000 --nodes 100 --images 30 --seed 20260621
python evaluate_replay.py --data data --out outputs
python evaluate_discovery_strategies.py --data data --out outputs/strategy_eval
python plot_pipeline_gantt.py --modeled-jobs outputs/modeled_jobs_no_prewarming.csv --out figures/example_gantt.png
```

The checked-in `data/` and `outputs/` directories are generated from this command sequence.

## Input schema for real clusters

The main input file for real data is `gitlab_runner_jobs.csv`. Required columns:

```text
job_id,pipeline_id,stage,pod,namespace,node,image_id,image,digest,
pod_created,pod_scheduled,container_started,job_script_started,job_finished,
p50_pull_seconds,useful_runtime_seconds
```

For real clusters:

- `pod_scheduled` can come from kube-state-metrics or Kubernetes Pod status.
- `container_started` can come from kube-state-metrics if available.
- `Pulling` / `Pulled` events can be exported through a Kubernetes event exporter.
- `image`, `image_id`, and `digest` can come from `kube_pod_container_info`.
- `node` can come from `kube_pod_info`.
- `job_id` and `pipeline_id` can come from GitLab job metadata or runner labels if available.

If exact pull duration is not available, use:

```text
startup_delay = container_started - pod_scheduled
```

as a conservative CI startup-delay proxy. It includes image pull/unpack plus container creation overhead.

## Fetch straight from a live cluster

`fetch_cluster_data.py` builds every input CSV for you from a cluster's
Prometheus and Loki. You only supply the two endpoints; sensible query defaults
cover GitLab Kubernetes-executor pods (`pod=~"runner-.*"`).

It assumes:

- Kubernetes events are shipped to Loki by **Grafana Alloy**
  (`loki.source.kubernetes_events`), so each log line is a JSON event with
  `reason`, `name`, and `msg` fields. Pull durations and image sizes are parsed
  from kubelet "Successfully pulled image ..." messages.
- Per-pod placement and lifecycle come from **kube-state-metrics**
  (`kube_pod_info`, `kube_pod_container_info`, `kube_pod_created`,
  `kube_pod_start_time`, `kube_pod_completion_time`).

```bash
# Port-forward Prometheus and Loki first, then:
python fetch_cluster_data.py \
  --prometheus-url http://localhost:9090 \
  --loki-url       http://localhost:3100 \
  --lookback 24h \
  --out data

python evaluate_replay.py --data data --out outputs
python evaluate_discovery_strategies.py --data data --out outputs/strategy_eval
```

It writes `images.csv`, `gitlab_runner_jobs.csv`,
`prometheus_image_samples_5m.csv`, and `kubernetes_events.csv` in the exact
schema the evaluator expects. Override the queries when your labels differ:

- `--pod-selector` — kube-state-metrics selector for your runner pods.
- `--loki-query` — LogQL selector for your Alloy event stream.
- `--usage-query` — PromQL for running-container usage grouped by `image`.
- `--start` / `--end` — RFC3339 window (defaults to the last `--lookback`).
- `--token` / `FETCH_TOKEN` — bearer token if the APIs require auth.

The observed pull time each job experienced (`observed_image_wait_seconds`) and
whether it was a cold hit (`observed_cold_hit`) are derived by joining real Loki
pull events to each pod, so the observed-impact ranking uses live data.

## Replay semantics

The replay follows the rolling-concurrency model from the paper:

```text
For each scheduled job j using image I on node n:
  if image I is warm on n at S_j:
      W_j = 0
  elif image I is already being pulled/prepared on n:
      W_j = T_n(I) - S_j
  else:
      T_n(I) = S_j + p_I
      W_j = p_I
```

A job can be affected even if it does not trigger a separate image pull. Multiple jobs can wait on the same cold node while the first image availability operation is still in progress.

## Policies evaluated

`evaluate_replay.py` currently evaluates:

- `no_prewarming`
- `prewarm_top10_by_usage`
- `prewarm_top30_by_usage`
- `prewarm_top10_by_observed_impact`
- `spegel_only_40pct_faster_pulls`
- `spegel_plus_top10_prewarm`

The Spegel scenarios model a reduced image availability time `p_I`, not prewarmed node-local state. This matches the paper framing: mirroring can reduce the cost of remaining cold pulls, while prewarming reduces cold-node hits.


## Discovery strategy evaluation

`evaluate_discovery_strategies.py` treats discovery rankings as prewarming policy inputs and compares them by replaying the same workload. It evaluates strategies that can be computed from historical CI observations without image-size metadata:

- `count`: total observed image usage before prewarming.
- `dev_weighted_count`: image usage weighted by the developer-time function `f(t)`.
- `recent_count`: image usage in the latest interval before prewarming.
- `peak_concurrency`: maximum concurrently active jobs requiring the same image.
- `hybrid_count_concurrency`: normalized blend of total usage and peak same-image concurrency.
- `oracle_impact_upper_bound`: after-the-fact impact ranking; not deployable, only an upper bound.

The script writes:

- `outputs/strategy_eval/discovery_rankings.csv`
- `outputs/strategy_eval/strategy_comparison.csv`


## Outputs

- `outputs/policy_summary.csv`: aggregate comparison of policies.
- `outputs/image_impact_no_prewarm.csv`: image ranking by affected job-minutes.
- `outputs/modeled_jobs_<policy>.csv`: per-job replay output for each policy.
- `outputs/pipeline_critical_path_delta_top10.csv`: pipeline-level delta for top-10 prewarming.
- `figures/example_gantt.png`: example pipeline Gantt chart with image wait segments.

## Notes

The synthetic dataset is deliberately randomized and not tuned to favor prewarming. It includes a long-tail image portfolio, varied image sizes, different pull times, bursty traffic, uneven job runtimes, and randomized node placement.
