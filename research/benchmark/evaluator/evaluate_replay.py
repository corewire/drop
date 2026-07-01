#!/usr/bin/env python3
"""Replay GitLab Kubernetes executor jobs and evaluate cache-warming policies.

The evaluator consumes a CSV with at least:
  job_id,pipeline_id,stage,pod,node,image_id,image,digest,pod_scheduled,p50_pull_seconds,useful_runtime_seconds

It models the rolling-concurrency image availability semantics used in the paper:
  - if image I is warm on node n at scheduling time S_j: W_j = 0
  - if no pull/prewarm is in progress: set T_n(I) = S_j + p_I and W_j = p_I
  - if availability work is in progress: W_j = T_n(I) - S_j
"""
from __future__ import annotations

import argparse
from pathlib import Path
import pandas as pd
import numpy as np


def load_jobs(path: Path) -> pd.DataFrame:
    df = pd.read_csv(path, parse_dates=["pod_scheduled", "pod_created", "container_started", "job_script_started", "job_finished"])
    return df.sort_values("pod_scheduled").reset_index(drop=True)


def discovery_rank_from_jobs(jobs: pd.DataFrame, until: pd.Timestamp) -> list[str]:
    before = jobs[jobs["pod_scheduled"] < until]
    counts = before.groupby("image_id").size().sort_values(ascending=False)
    return list(counts.index)


def impact_rank_from_full_day(jobs: pd.DataFrame) -> list[str]:
    # Neutral ranking: uses observed wait from the synthetic day; real clusters would compute this after the fact.
    impact = jobs.groupby("image_id")["observed_image_wait_seconds"].sum().sort_values(ascending=False)
    return list(impact.index)


def replay_policy(
    jobs: pd.DataFrame,
    images: pd.DataFrame,
    policy_name: str,
    prewarm_images: set[str] | None = None,
    prewarm_at: pd.Timestamp | None = None,
    spegel_factor: float = 1.0,
    developer_start: pd.Timestamp | None = None,
) -> tuple[pd.DataFrame, pd.DataFrame]:
    if prewarm_images is None:
        prewarm_images = set()
    p_map = dict(zip(images["image_id"], images["p50_pull_seconds"]))
    availability: dict[tuple[str, str], pd.Timestamp] = {}

    rows = []
    newly_warmed = set()
    for r in jobs.sort_values("pod_scheduled").itertuples(index=False):
        S = pd.Timestamp(r.pod_scheduled)
        key = (r.node, r.image_id)
        p_i = float(p_map[r.image_id]) * spegel_factor
        if key not in availability:
            if prewarm_at is not None and r.image_id in prewarm_images and S >= prewarm_at:
                # Counterfactual: image was successfully prewarmed before this job arrived.
                availability[key] = prewarm_at
                W = 0.0
                cold_hit = False
            else:
                T = S + pd.Timedelta(seconds=p_i)
                availability[key] = T
                W = p_i
                cold_hit = True
                newly_warmed.add(key)
        else:
            T = availability[key]
            if S < T:
                W = (T - S).total_seconds()
                cold_hit = True
            else:
                W = 0.0
                cold_hit = False
        modeled_start = S + pd.Timedelta(seconds=W + 6.0)  # small scheduler/container overhead proxy
        modeled_finish = modeled_start + pd.Timedelta(seconds=float(r.useful_runtime_seconds))
        rows.append({
            "job_id": r.job_id,
            "pipeline_id": r.pipeline_id,
            "stage": r.stage,
            "pod": r.pod,
            "node": r.node,
            "image_id": r.image_id,
            "image": r.image,
            "pod_scheduled": S,
            "modeled_image_wait_seconds": round(W, 3),
            "modeled_cold_hit": cold_hit,
            "modeled_start": modeled_start,
            "modeled_finish": modeled_finish,
        })
    out = pd.DataFrame(rows)
    if developer_start is not None:
        dev = out[out["pod_scheduled"] >= developer_start]
    else:
        dev = out
    summary = pd.DataFrame([{
        "policy": policy_name,
        "jobs_total": len(out),
        "developer_window_jobs": len(dev),
        "affected_jobs_total": int(out["modeled_cold_hit"].sum()),
        "affected_jobs_developer_window": int(dev["modeled_cold_hit"].sum()),
        "affected_job_minutes_total": out["modeled_image_wait_seconds"].sum() / 60.0,
        "affected_job_minutes_developer_window": dev["modeled_image_wait_seconds"].sum() / 60.0,
        "p95_wait_seconds_developer_window": dev["modeled_image_wait_seconds"].quantile(0.95) if len(dev) else 0,
        "p99_wait_seconds_developer_window": dev["modeled_image_wait_seconds"].quantile(0.99) if len(dev) else 0,
        "newly_warmed_node_image_pairs": len(newly_warmed),
    }])
    return out, summary


def pipeline_critical_path(jobs: pd.DataFrame, baseline: pd.DataFrame, candidate: pd.DataFrame) -> pd.DataFrame:
    b = baseline.groupby("pipeline_id").agg(
        no_prewarm_finish=("modeled_finish", "max"),
        no_prewarm_start=("modeled_start", "min"),
    )
    c = candidate.groupby("pipeline_id").agg(
        candidate_finish=("modeled_finish", "max"),
        candidate_start=("modeled_start", "min"),
    )
    joined = b.join(c, how="inner")
    joined["critical_path_delta_seconds"] = (joined["no_prewarm_finish"] - joined["candidate_finish"]).dt.total_seconds()
    return joined.reset_index()


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="data", help="Directory containing generated CSV files")
    ap.add_argument("--out", default="outputs", help="Output directory")
    ap.add_argument("--developer-start", default="2026-06-18T09:00:00Z")
    ap.add_argument("--prewarm-at", default="2026-06-18T08:45:00Z")
    args = ap.parse_args()
    data = Path(args.data)
    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    jobs = load_jobs(data / "gitlab_runner_jobs.csv")
    images = pd.read_csv(data / "images.csv")
    developer_start = pd.Timestamp(args.developer_start)
    prewarm_at = pd.Timestamp(args.prewarm_at)

    usage_rank = discovery_rank_from_jobs(jobs, developer_start)
    impact_rank = impact_rank_from_full_day(jobs)

    policies = []
    policies.append(("no_prewarming", set(), None, 1.0))
    policies.append(("prewarm_top10_by_usage", set(usage_rank[:10]), prewarm_at, 1.0))
    policies.append(("prewarm_top30_by_usage", set(usage_rank[:30]), prewarm_at, 1.0))
    policies.append(("prewarm_top10_by_observed_impact", set(impact_rank[:10]), prewarm_at, 1.0))
    policies.append(("spegel_only_40pct_faster_pulls", set(), None, 0.60))
    policies.append(("spegel_plus_top10_prewarm", set(usage_rank[:10]), prewarm_at, 0.60))

    all_summaries = []
    modeled_by_policy = {}
    for name, prewarm_set, prewarm_time, factor in policies:
        modeled, summary = replay_policy(
            jobs, images, name, prewarm_images=prewarm_set, prewarm_at=prewarm_time,
            spegel_factor=factor, developer_start=developer_start,
        )
        modeled.to_csv(out_dir / f"modeled_jobs_{name}.csv", index=False)
        all_summaries.append(summary)
        modeled_by_policy[name] = modeled

    summary = pd.concat(all_summaries, ignore_index=True)
    base_minutes = float(summary.loc[summary["policy"] == "no_prewarming", "affected_job_minutes_developer_window"].iloc[0])
    summary["developer_window_minutes_avoided_vs_no_prewarm"] = base_minutes - summary["affected_job_minutes_developer_window"]
    summary.to_csv(out_dir / "policy_summary.csv", index=False)

    image_summary = modeled_by_policy["no_prewarming"].groupby(["image_id", "image"]).agg(
        jobs=("job_id", "count"),
        affected_jobs=("modeled_cold_hit", "sum"),
        affected_job_minutes=("modeled_image_wait_seconds", lambda s: s.sum() / 60.0),
    ).reset_index().sort_values("affected_job_minutes", ascending=False)
    image_summary.to_csv(out_dir / "image_impact_no_prewarm.csv", index=False)

    cp = pipeline_critical_path(jobs, modeled_by_policy["no_prewarming"], modeled_by_policy["prewarm_top10_by_usage"])
    cp.to_csv(out_dir / "pipeline_critical_path_delta_top10.csv", index=False)

    print(summary.to_string(index=False))


if __name__ == "__main__":
    main()
