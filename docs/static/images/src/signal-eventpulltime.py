#!/usr/bin/env python3
"""Render an editable eventPullTime SVG chart.

Usage:
  python docs/static/images/src/signal-eventpulltime.py
"""

from pathlib import Path

import matplotlib.pyplot as plt
import pandas as pd
import seaborn as sns


def main() -> None:
    src_dir = Path(__file__).resolve().parent
    out_path = Path(__file__).resolve().parents[1] / "signal-eventpulltime.svg"
    data_path = src_dir / "data" / "eventpulltime_samples.csv"

    samples = pd.read_csv(data_path)
    required = {"image", "pull_ms"}
    missing = required - set(samples.columns)
    if missing:
        raise ValueError(f"missing required columns in {data_path}: {sorted(missing)}")

    sns.set_theme(style="whitegrid", context="notebook")
    order = list(samples.groupby("image")["pull_ms"].median().sort_values().index)

    fig, ax = plt.subplots(figsize=(8.8, 4.8), dpi=120)
    ax.set_facecolor("#ffffff")

    sns.stripplot(
        data=samples,
        x="image",
        y="pull_ms",
        order=order,
        hue="image",
        palette="Set2",
        dodge=False,
        jitter=0.12,
        size=8,
        alpha=0.95,
        ax=ax,
    )

    stats = samples.groupby("image", as_index=False).agg(p50=("pull_ms", "median"), max=("pull_ms", "max"))
    x_map = {img: i for i, img in enumerate(order)}

    for _, row in stats.iterrows():
        x = x_map[row["image"]]
        ax.hlines(row["p50"], x - 0.24, x + 0.24, colors="#7a3eb1", linestyles=(0, (5, 3)), linewidth=1.7)
        ax.hlines(row["max"], x - 0.24, x + 0.24, colors="#d06c00", linestyles=(0, (5, 3)), linewidth=1.7)
        ax.text(x + 0.26, row["p50"], f"p50 {int(row['p50'])}", color="#7a3eb1", fontsize=8, va="center")
        ax.text(x + 0.26, row["max"], f"max {int(row['max'])}", color="#d06c00", fontsize=8, va="center")

    ax.set_title("eventPullTime signal", loc="left", fontsize=13, fontweight="bold")
    ax.text(
        0.0,
        1.01,
        "Sample pull durations per image (CSV-driven). p50 resists outliers, max tracks worst-case events.",
        transform=ax.transAxes,
        fontsize=8.8,
        color="#4f5d75",
    )
    ax.set_ylabel("pull duration (ms)")
    ax.set_xlabel("image")
    ax.set_ylim(0, max(samples["pull_ms"].max() * 1.18, 1000))
    ax.grid(axis="y", color="#e9edf4")
    ax.grid(axis="x", visible=False)
    ax.set_axisbelow(True)
    for side in ["top", "right"]:
        ax.spines[side].set_visible(False)
    ax.spines["left"].set_color("#ccd4e2")
    ax.spines["bottom"].set_color("#ccd4e2")

    # Keep the legend concise and non-duplicated.
    handles, labels = ax.get_legend_handles_labels()
    unique = []
    seen = set()
    for h, l in zip(handles, labels):
        if l not in seen:
            seen.add(l)
            unique.append((h, l))
    if unique:
        ax.legend(
            [h for h, _ in unique],
            [l for _, l in unique],
            title="samples",
            loc="upper left",
            frameon=False,
            fontsize=8,
            title_fontsize=8,
        )

    fig.tight_layout()

    fig.savefig(out_path, format="svg")
    plt.close(fig)
    print(f"wrote {out_path} from {data_path}")


if __name__ == "__main__":
    main()
