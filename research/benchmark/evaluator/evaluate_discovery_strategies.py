#!/usr/bin/env python3
"""Evaluate discovery strategies as inputs to OCI image prewarming replay.

This script intentionally avoids image-size metadata. It compares strategies
that can be computed from historical GitLab Kubernetes executor job data:

- count: total image usage in the discovery window
- dev_weighted: image usage weighted by a developer-time function f(t)
- recent: image usage in the latest interval before prewarming
- peak_concurrency: maximum concurrently active jobs requiring the same image
- hybrid_count_concurrency: normalized blend of total usage and peak concurrency
- oracle_impact: after-the-fact upper-bound ranking by no-prewarm impact

The oracle is not deployable; it is an analysis upper bound.
"""
from __future__ import annotations

import argparse
from pathlib import Path
import numpy as np
import pandas as pd

from evaluate_replay import load_jobs, replay_policy


def developer_weight(ts: pd.Timestamp) -> float:
    hour = ts.hour + ts.minute / 60.0 + ts.second / 3600.0
    if 9 <= hour < 17:
        return 1.0
    if 7 <= hour < 9 or 17 <= hour < 20:
        return 0.3
    return 0.0


def normalize(s: pd.Series) -> pd.Series:
    s = s.astype(float)
    if len(s) == 0:
        return s
    lo = float(s.min())
    hi = float(s.max())
    if hi == lo:
        return pd.Series(1.0, index=s.index)
    return (s - lo) / (hi - lo)


def discovery_subset(jobs: pd.DataFrame, start: pd.Timestamp | None, end: pd.Timestamp) -> pd.DataFrame:
    mask = jobs["pod_scheduled"] < end
    if start is not None:
        mask &= jobs["pod_scheduled"] >= start
    return jobs.loc[mask].copy()


def total_count_rank(jobs: pd.DataFrame) -> pd.Series:
    return jobs.groupby("image_id").size().astype(float).sort_values(ascending=False)


def dev_weighted_rank(jobs: pd.DataFrame) -> pd.Series:
    if jobs.empty:
        return pd.Series(dtype=float)
    df = jobs.copy()
    df["weight"] = df["pod_scheduled"].map(developer_weight)
    return df.groupby("image_id")["weight"].sum().sort_values(ascending=False)


def recent_rank(jobs: pd.DataFrame, end: pd.Timestamp, window: pd.Timedelta) -> pd.Series:
    df = jobs[(jobs["pod_scheduled"] >= end - window) & (jobs["pod_scheduled"] < end)]
    return total_count_rank(df)


def peak_concurrency_rank(jobs: pd.DataFrame, window_end: pd.Timestamp | None = None) -> pd.Series:
    """Maximum active jobs per image using a sweep-line over start/end events."""
    if jobs.empty:
        return pd.Series(dtype=float)
    scores: dict[str, float] = {}
    end_limit = pd.Timestamp(window_end).value if window_end is not None else None
    for image_id, g in jobs.groupby("image_id"):
        events: list[tuple[int, int]] = []
        for row in g.itertuples(index=False):
            start = pd.Timestamp(row.pod_scheduled).value
            finish = pd.Timestamp(row.job_finished).value
            if end_limit is not None:
                finish = min(finish, end_limit)
            if finish <= start:
                continue
            # End events before start events at the same timestamp avoid overcounting handoff.
            events.append((start, 1))
            events.append((finish, -1))
        active = 0
        peak = 0
        for _ts, delta in sorted(events, key=lambda x: (x[0], x[1])):
            active += delta
            if active > peak:
                peak = active
        scores[image_id] = float(peak)
    return pd.Series(scores).sort_values(ascending=False)


def hybrid_rank(count_scores: pd.Series, conc_scores: pd.Series, alpha: float) -> pd.Series:
    all_images = count_scores.index.union(conc_scores.index)
    c = count_scores.reindex(all_images).fillna(0.0)
    k = conc_scores.reindex(all_images).fillna(0.0)
    score = alpha * normalize(c) + (1.0 - alpha) * normalize(k)
    return score.sort_values(ascending=False)


def impact_rank(no_prewarm: pd.DataFrame, start: pd.Timestamp, end: pd.Timestamp | None = None) -> pd.Series:
    df = no_prewarm[no_prewarm["pod_scheduled"] >= start]
    if end is not None:
        df = df[df["pod_scheduled"] < end]
    return (df.groupby("image_id")["modeled_image_wait_seconds"].sum() / 60.0).sort_values(ascending=False)


def evaluate_strategies(
    jobs: pd.DataFrame,
    images: pd.DataFrame,
    out_dir: Path,
    developer_start: pd.Timestamp,
    developer_end: pd.Timestamp,
    prewarm_at: pd.Timestamp,
    recent_window: pd.Timedelta,
    topk_values: list[int],
    hybrid_alpha: float,
) -> pd.DataFrame:
    out_dir.mkdir(parents=True, exist_ok=True)
    discovery = discovery_subset(jobs, None, prewarm_at)

    count_scores = total_count_rank(discovery)
    dev_scores = dev_weighted_rank(discovery)
    recent_scores = recent_rank(jobs, prewarm_at, recent_window)
    conc_scores = peak_concurrency_rank(discovery, prewarm_at)
    hybrid_scores = hybrid_rank(count_scores, conc_scores, hybrid_alpha)

    baseline_jobs, _baseline_summary = replay_policy(
        jobs, images, "no_prewarming", set(), None, 1.0, developer_start
    )
    oracle_scores = impact_rank(baseline_jobs, developer_start, developer_end)

    ranking_map = {
        "count": count_scores,
        "dev_weighted_count": dev_scores,
        "recent_count": recent_scores,
        "peak_concurrency": conc_scores,
        f"hybrid_count_concurrency_a{hybrid_alpha:g}": hybrid_scores,
        "oracle_impact_upper_bound": oracle_scores,
    }

    rank_rows = []
    for strategy, scores in ranking_map.items():
        for rank, (image_id, score) in enumerate(scores.items(), start=1):
            rank_rows.append({"strategy": strategy, "rank": rank, "image_id": image_id, "score": float(score)})
    pd.DataFrame(rank_rows).to_csv(out_dir / "discovery_rankings.csv", index=False)

    baseline_dev = baseline_jobs[(baseline_jobs["pod_scheduled"] >= developer_start) & (baseline_jobs["pod_scheduled"] < developer_end)]
    base_minutes = float(baseline_dev["modeled_image_wait_seconds"].sum() / 60.0)
    base_jobs = int(baseline_dev["modeled_cold_hit"].sum())

    rows = []
    for strategy, scores in ranking_map.items():
        ids = list(scores.index)
        for k in topk_values:
            selected = set(ids[:k])
            modeled, _summary = replay_policy(
                jobs, images, f"{strategy}_top{k}", selected, prewarm_at, 1.0, developer_start
            )
            dev = modeled[(modeled["pod_scheduled"] >= developer_start) & (modeled["pod_scheduled"] < developer_end)]
            minutes = float(dev["modeled_image_wait_seconds"].sum() / 60.0)
            affected = int(dev["modeled_cold_hit"].sum())
            rows.append({
                "strategy": strategy,
                "top_k": k,
                "selected_images": len(selected),
                "affected_jobs_developer_window": affected,
                "affected_job_minutes_developer_window": minutes,
                "minutes_avoided_vs_no_prewarm": base_minutes - minutes,
                "percent_minutes_saved": 100.0 * (base_minutes - minutes) / base_minutes if base_minutes else 0.0,
                "affected_jobs_avoided_vs_no_prewarm": base_jobs - affected,
                "percent_affected_jobs_saved": 100.0 * (base_jobs - affected) / base_jobs if base_jobs else 0.0,
                "p95_wait_seconds_developer_window": float(dev["modeled_image_wait_seconds"].quantile(0.95)) if len(dev) else 0.0,
                "p99_wait_seconds_developer_window": float(dev["modeled_image_wait_seconds"].quantile(0.99)) if len(dev) else 0.0,
            })
    result = pd.DataFrame(rows)
    result.to_csv(out_dir / "strategy_comparison.csv", index=False)
    return result


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="data")
    ap.add_argument("--out", default="outputs")
    ap.add_argument("--developer-start", default="2026-06-18T09:00:00Z")
    ap.add_argument("--developer-end", default="2026-06-18T17:00:00Z")
    ap.add_argument("--prewarm-at", default="2026-06-18T08:45:00Z")
    ap.add_argument("--recent-window-minutes", type=int, default=120)
    ap.add_argument("--topk", default="5,10,20,30")
    ap.add_argument("--hybrid-alpha", type=float, default=0.5)
    args = ap.parse_args()

    data = Path(args.data)
    out = Path(args.out)
    jobs = load_jobs(data / "gitlab_runner_jobs.csv")
    images = pd.read_csv(data / "images.csv")
    developer_start = pd.Timestamp(args.developer_start)
    developer_end = pd.Timestamp(args.developer_end)
    prewarm_at = pd.Timestamp(args.prewarm_at)
    topk_values = [int(x) for x in args.topk.split(",") if x.strip()]

    result = evaluate_strategies(
        jobs, images, out, developer_start, developer_end, prewarm_at,
        pd.Timedelta(minutes=args.recent_window_minutes), topk_values, args.hybrid_alpha,
    )
    print(result.to_string(index=False))


if __name__ == "__main__":
    main()
