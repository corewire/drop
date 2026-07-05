#!/usr/bin/env python3
"""Generate a synthetic GitLab Kubernetes executor day for OCI image cache replay.

The data is intentionally realistic-but-synthetic. It is not tuned to make
prewarming look good: image sizes, pull times, node placements, background
traffic, and stage runtimes are randomized with a fixed seed.
"""
from __future__ import annotations

import argparse
from pathlib import Path
import numpy as np
import pandas as pd

STAGES = ["prepare", "build", "test", "integration", "package", "deploy-check"]


def hourly_weights() -> np.ndarray:
    # 24 hourly traffic shape: low night, ramp, developer peak, evening tail.
    w = np.array([
        0.20, 0.16, 0.13, 0.12, 0.12, 0.16,
        0.28, 0.45, 0.75, 1.45, 1.55, 1.50,
        1.30, 1.25, 1.28, 1.38, 1.45, 1.05,
        0.85, 0.70, 0.52, 0.40, 0.30, 0.23,
    ], dtype=float)
    return w / w.sum()


def generate_images(rng: np.random.Generator, n_images: int) -> pd.DataFrame:
    # A mixed CI image portfolio: a few very common images and a long tail.
    names = [
        "registry.example.com/ci/python-test:3.12",
        "registry.example.com/ci/node-build:22",
        "registry.example.com/ci/gitlab-runner-helper:x86_64-v17",
        "registry.example.com/ci/docker-buildx:latest",
        "registry.example.com/ci/java-gradle:21",
        "registry.example.com/ci/go-test:1.23",
        "registry.example.com/ci/php-unit:8.3",
        "registry.example.com/ci/ruby-test:3.3",
        "registry.example.com/ci/terraform-tools:1.9",
        "registry.example.com/ci/kubectl-helm:latest",
    ]
    while len(names) < n_images:
        names.append(f"registry.example.com/project/app-image-{len(names)+1:02d}:ci")

    ranks = np.arange(1, n_images + 1)
    usage_weight = 1 / np.power(ranks, 0.78)
    usage_weight = usage_weight / usage_weight.sum()
    rng.shuffle(usage_weight[10:])

    size_mb = rng.lognormal(mean=5.25, sigma=0.55, size=n_images)  # around 190 MB median
    size_mb = np.clip(size_mb, 70, 1200).round(1)
    layers = rng.integers(6, 24, size=n_images)
    shared_layer_ratio = np.clip(rng.beta(2.5, 4.0, size=n_images), 0.05, 0.85)

    # Pull/prepare time: influenced by size, layer count, registry variation, unpack cost.
    base = 12 + size_mb / rng.uniform(6.5, 12.0, size=n_images) + layers * rng.uniform(0.45, 1.25, size=n_images)
    tail = rng.lognormal(mean=0.0, sigma=0.25, size=n_images)
    p50_pull_seconds = np.clip(base * tail, 18, 180).round(1)

    return pd.DataFrame({
        "image_id": [f"img-{i+1:02d}" for i in range(n_images)],
        "image": names[:n_images],
        "digest": [f"sha256:{rng.bytes(12).hex()}{i:04x}" for i in range(n_images)],
        "usage_weight": usage_weight,
        "size_mb": size_mb,
        "layers": layers,
        "shared_layer_ratio": shared_layer_ratio.round(3),
        "p50_pull_seconds": p50_pull_seconds,
    })


def generate_jobs(rng: np.random.Generator, images: pd.DataFrame, n_jobs: int, n_nodes: int) -> pd.DataFrame:
    start = pd.Timestamp("2026-06-18T00:00:00Z")
    weights = hourly_weights()
    hourly_counts = rng.multinomial(n_jobs, weights)

    times = []
    for hour, count in enumerate(hourly_counts):
        # Within each hour, use a beta distribution for some burstiness.
        # Developer hours have more clustering around the middle of the hour.
        if 9 <= hour <= 17:
            offsets = rng.beta(2.0, 2.0, size=count) * 3600
        else:
            offsets = rng.uniform(0, 3600, size=count)
        times.extend([start + pd.Timedelta(hours=hour, seconds=float(x)) for x in offsets])
    times = pd.Series(times).sort_values(ignore_index=True)

    image_choices = rng.choice(images["image_id"].to_numpy(), size=n_jobs, p=images["usage_weight"].to_numpy())
    nodes = rng.choice([f"ci-node-{i:03d}" for i in range(n_nodes)], size=n_jobs)

    pipeline_ids = np.arange(100000, 100000 + int(n_jobs / 4) + 100)
    # 2-8 jobs per pipeline, then assign to reach n_jobs
    pids = []
    for pid in pipeline_ids:
        pids.extend([pid] * int(rng.integers(2, 9)))
        if len(pids) >= n_jobs:
            break
    pids = np.array(pids[:n_jobs])
    rng.shuffle(pids)

    stage_probs = np.array([0.08, 0.20, 0.45, 0.16, 0.08, 0.03])
    stages = rng.choice(STAGES, size=n_jobs, p=stage_probs)
    runtime_by_stage = {
        "prepare": (35, 15), "build": (210, 120), "test": (150, 90),
        "integration": (360, 180), "package": (120, 60), "deploy-check": (75, 30),
    }
    runtimes = []
    for s in stages:
        mean, sd = runtime_by_stage[s]
        runtimes.append(float(max(10, rng.lognormal(np.log(mean), 0.45) + rng.normal(0, sd/6))))

    df = pd.DataFrame({
        "job_id": np.arange(1, n_jobs + 1),
        "pipeline_id": pids,
        "stage": stages,
        "pod": [f"runner-project-{rng.integers(100,999)}-concurrent-{i%100}-{i:05d}" for i in range(n_jobs)],
        "namespace": "build-stuff",
        "node": nodes,
        "image_id": image_choices,
        "pod_created": times - pd.to_timedelta(rng.uniform(1, 8, n_jobs), unit="s"),
        "pod_scheduled": times,
        "useful_runtime_seconds": np.round(runtimes, 1),
    })
    df = df.merge(images[["image_id", "image", "digest", "p50_pull_seconds", "size_mb"]], on="image_id", how="left")
    return df.sort_values("pod_scheduled").reset_index(drop=True)


def simulate_observed_times(rng: np.random.Generator, jobs: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame]:
    availability: dict[tuple[str, str], pd.Timestamp] = {}
    rows = []
    events = []
    for r in jobs.itertuples(index=False):
        key = (r.node, r.image_id)
        scheduled = pd.Timestamp(r.pod_scheduled)
        p_i = float(r.p50_pull_seconds) * float(rng.lognormal(mean=0.0, sigma=0.18))
        p_i = max(5.0, p_i)
        base_overhead = max(2.0, rng.normal(6.0, 2.0))
        if key not in availability:
            pull_start = scheduled + pd.Timedelta(seconds=float(rng.uniform(0.3, 2.5)))
            available = scheduled + pd.Timedelta(seconds=p_i)
            availability[key] = available
            image_wait = p_i
            cold_hit = True
            events.extend([
                {"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "Scheduled", "timestamp": scheduled},
                {"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "Pulling", "timestamp": pull_start},
                {"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "Pulled", "timestamp": available},
            ])
        else:
            available = availability[key]
            if scheduled < available:
                image_wait = (available - scheduled).total_seconds()
                cold_hit = True
                events.append({"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "PullShared", "timestamp": scheduled})
            else:
                image_wait = 0.0
                cold_hit = False
                available = scheduled
            events.append({"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "Scheduled", "timestamp": scheduled})
        container_started = scheduled + pd.Timedelta(seconds=image_wait + base_overhead)
        job_script_started = container_started + pd.Timedelta(seconds=float(rng.uniform(1.0, 4.0)))
        job_finished = job_script_started + pd.Timedelta(seconds=float(r.useful_runtime_seconds))
        events.extend([
            {"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "Created", "timestamp": container_started - pd.Timedelta(seconds=0.5)},
            {"pod": r.pod, "node": r.node, "image_id": r.image_id, "reason": "Started", "timestamp": container_started},
        ])
        rows.append({
            "pod": r.pod,
            "container_started": container_started,
            "job_script_started": job_script_started,
            "job_finished": job_finished,
            "observed_image_wait_seconds": round(image_wait, 2),
            "observed_startup_delay_seconds": round((container_started - scheduled).total_seconds(), 2),
            "observed_cold_hit": cold_hit,
        })
    observed = jobs.merge(pd.DataFrame(rows), on="pod", how="left")
    events_df = pd.DataFrame(events).sort_values("timestamp").reset_index(drop=True)
    return observed, events_df


def generate_prometheus_samples(jobs: pd.DataFrame, out_dir: Path) -> pd.DataFrame:
    # 5-minute image usage samples from running containers. This imitates a range query result.
    samples = []
    start = pd.Timestamp("2026-06-18T00:00:00Z")
    for ts in pd.date_range(start, start + pd.Timedelta(hours=24), freq="5min", inclusive="left"):
        running = jobs[(jobs["job_script_started"] <= ts) & (jobs["job_finished"] > ts)]
        counts = running.groupby(["image_id", "image"]).size().reset_index(name="running_containers")
        counts["timestamp"] = ts
        samples.append(counts)
    df = pd.concat(samples, ignore_index=True) if samples else pd.DataFrame()
    return df[["timestamp", "image_id", "image", "running_containers"]]


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="data", help="Output directory")
    ap.add_argument("--jobs", type=int, default=25000)
    ap.add_argument("--nodes", type=int, default=100)
    ap.add_argument("--images", type=int, default=30)
    ap.add_argument("--seed", type=int, default=20260621)
    args = ap.parse_args()
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    rng = np.random.default_rng(args.seed)

    images = generate_images(rng, args.images)
    jobs = generate_jobs(rng, images, args.jobs, args.nodes)
    jobs, events = simulate_observed_times(rng, jobs)
    samples = generate_prometheus_samples(jobs, out)
    rotations = pd.DataFrame({
        "rotation_id": ["midnight-nodepool-rotation"],
        "rotation_time": [pd.Timestamp("2026-06-18T00:00:00Z")],
        "node_selector": ["ci-node-*"],
        "nodes_replaced": [args.nodes],
    })

    images.to_csv(out / "images.csv", index=False)
    jobs.to_csv(out / "gitlab_runner_jobs.csv", index=False)
    events.to_csv(out / "kubernetes_events.csv", index=False)
    samples.to_csv(out / "prometheus_image_samples_5m.csv", index=False)
    rotations.to_csv(out / "node_rotations.csv", index=False)
    print(f"Wrote synthetic day with {len(jobs)} jobs, {args.nodes} nodes and {args.images} images to {out}")


if __name__ == "__main__":
    main()
