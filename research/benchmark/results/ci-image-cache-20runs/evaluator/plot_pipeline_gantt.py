#!/usr/bin/env python3
"""Create a small pipeline Gantt chart for a selected pipeline ID."""
from __future__ import annotations
import argparse
from pathlib import Path
import pandas as pd
import matplotlib.pyplot as plt


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--modeled-jobs", default="outputs/modeled_jobs_no_prewarming.csv")
    ap.add_argument("--pipeline-id", type=int, default=None)
    ap.add_argument("--out", default="figures/pipeline_gantt.png")
    args = ap.parse_args()
    df = pd.read_csv(args.modeled_jobs, parse_dates=["pod_scheduled", "modeled_start", "modeled_finish"])
    if args.pipeline_id is None:
        # Pick a pipeline with multiple jobs and visible waits.
        candidates = df.groupby("pipeline_id").agg(jobs=("job_id", "count"), wait=("modeled_image_wait_seconds", "sum"))
        pipeline_id = int(candidates[(candidates.jobs >= 4)].sort_values("wait", ascending=False).index[0])
    else:
        pipeline_id = args.pipeline_id
    p = df[df["pipeline_id"] == pipeline_id].sort_values("pod_scheduled").copy()
    if p.empty:
        raise SystemExit(f"Pipeline {pipeline_id} not found")
    t0 = p["pod_scheduled"].min()
    p["wait_start_min"] = (p["pod_scheduled"] - t0).dt.total_seconds() / 60
    p["wait_min"] = p["modeled_image_wait_seconds"] / 60
    p["run_start_min"] = (p["modeled_start"] - t0).dt.total_seconds() / 60
    p["run_min"] = (p["modeled_finish"] - p["modeled_start"]).dt.total_seconds() / 60

    fig, ax = plt.subplots(figsize=(10, max(3, len(p) * 0.35)))
    y = range(len(p))
    ax.barh(y, p["wait_min"], left=p["wait_start_min"], label="modeled image wait")
    ax.barh(y, p["run_min"], left=p["run_start_min"], label="job runtime")
    ax.set_yticks(list(y))
    ax.set_yticklabels([f"{r.stage} #{r.job_id}" for r in p.itertuples()])
    ax.invert_yaxis()
    ax.set_xlabel("minutes since first pod scheduled")
    ax.set_title(f"Pipeline {pipeline_id}: modeled image wait and runtime")
    ax.legend(loc="best")
    fig.tight_layout()
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(out, dpi=160)
    print(f"Wrote {out}")


if __name__ == "__main__":
    main()
