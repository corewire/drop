#!/usr/bin/env python3
"""Render eventPullTime signal charts as light infographics (CSV-driven).

One "events" SVG shows all raw pull events as a time scatter (x=when, y=how long).
Six per-statistic SVGs show the same dots with an emphasis mark for the statistic.

Edit data/eventpulltime_samples.csv and rerun.
"""

from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
import seaborn as sns

SRC = Path(__file__).resolve().parent
IMG = SRC.parent
DATA = SRC / "data" / "eventpulltime_samples.csv"

A = "#4361ee"   # nginx (blue)
B = "#2ec4b6"   # redis (teal)
INK = "#1a1a2e"
SUB = "#666"
BG = "#fafafa"

IMAGES = [("nginx:1.25", A), ("redis:7", B)]

STATS = [
    ("p50", "signal-eventpulltime-p50.svg",
     "dashed line = median — half of all pulls were faster"),
    ("p90", "signal-eventpulltime-p90.svg",
     "dashed line = 90th percentile — only 1 in 10 pulls was slower"),
    ("p95", "signal-eventpulltime-p95.svg",
     "dashed line = 95th percentile — strict worst-case tail"),
    ("avg", "signal-eventpulltime-avg.svg",
     "dashed line = mean — dragged up by the one slow outlier"),
    ("max", "signal-eventpulltime-max.svg",
     "ringed dot = the slowest single pull per image"),
    ("count", "signal-eventpulltime-count.svg",
     "ringed dots = all observed pull events; count = number of rings"),
]


def load():
    return pd.read_csv(DATA)


def stat_value(vals, stat):
    if stat == "count":
        return float(vals.size)
    if stat == "avg":
        return float(vals.mean())
    if stat == "max":
        return float(vals.max())
    return float(np.percentile(vals, {"p50": 50, "p90": 90, "p95": 95}[stat]))


def new_fig(title, subtitle, xmax, ymax):
    sns.set_theme(style="white", context="notebook")
    fig, ax = plt.subplots(figsize=(9.0, 3.8), dpi=130)
    fig.patch.set_facecolor(BG)
    ax.set_facecolor(BG)
    ax.set_title(title, loc="left", fontsize=13, fontweight="bold", color=INK, pad=18)
    ax.text(0.0, 1.06, subtitle, transform=ax.transAxes, fontsize=9, color=SUB)
    ax.set_xlim(-0.8, xmax + 0.8)
    ax.set_ylim(-200, ymax * 1.18)
    ax.set_xlabel("time within lookback window (hours)", fontsize=9, color="#7b7d80")
    ax.set_ylabel("pull duration (ms)", fontsize=9, color="#7b7d80")
    for s in ["top", "right"]:
        ax.spines[s].set_visible(False)
    ax.spines["bottom"].set_color("#ccd4e2")
    ax.spines["left"].set_color("#ccd4e2")
    ax.grid(axis="y", color="#eceff4", zorder=0)
    ax.set_axisbelow(True)
    return fig, ax


def legend(ax, entries, title):
    handles = [plt.Line2D([0], [0], color=c, lw=3) for c, _ in entries]
    labels = [l for _, l in entries]
    leg = ax.legend(
        handles, labels, title=title,
        loc="upper left", bbox_to_anchor=(1.02, 1.0),
        frameon=True, fontsize=9, title_fontsize=9, handlelength=1.4,
        borderpad=0.8, labelspacing=0.6,
    )
    leg.get_frame().set_facecolor("#ffffff")
    leg.get_frame().set_edgecolor("#e0e4ec")
    leg.get_title().set_color(INK)
    leg.get_title().set_fontweight("bold")
    leg._legend_box.align = "left"
    return leg


def save(fig, name):
    out = IMG / name
    fig.savefig(out, format="svg", bbox_inches="tight", pad_inches=0.2)
    plt.close(fig)
    print(f"wrote {out}")


def chart_events(df):
    """All raw pull events — no statistic emphasis."""
    xmax = df["pull_time_h"].max()
    ymax = df["pull_ms"].max()
    fig, ax = new_fig(
        "eventPullTime · raw events",
        "each dot = one Pulled event; x = when it happened, y = how long it took",
        xmax=xmax, ymax=ymax,
    )
    entries = []
    for img, c in IMAGES:
        rows = df[df.image == img]
        ax.scatter(rows["pull_time_h"], rows["pull_ms"],
                   s=72, color=c, alpha=0.88, edgecolors="white", linewidths=0.9, zorder=4)
        entries.append((c, img))
    legend(ax, entries, "image")
    save(fig, "signal-eventpulltime-events.svg")


def chart_stat(df, stat, name, subtitle):
    xmax = df["pull_time_h"].max()
    ymax = df["pull_ms"].max()
    fig, ax = new_fig(
        f"eventPullTime · statistic={stat}",
        subtitle,
        xmax=xmax, ymax=ymax,
    )
    entries = []
    for img, c in IMAGES:
        rows = df[df.image == img]
        vals = rows["pull_ms"].to_numpy(dtype=float)
        val = stat_value(vals, stat)

        # faded dots (all events visible but de-emphasised)
        ax.scatter(rows["pull_time_h"], rows["pull_ms"],
                   s=60, color=c, alpha=0.30, edgecolors="none", zorder=3)

        if stat == "count":
            # ring every dot — count = number of rings
            ax.scatter(rows["pull_time_h"], rows["pull_ms"],
                       s=200, facecolors="none", edgecolors=c, linewidths=1.8, zorder=5)
            entries.append((c, f"{img} · count = {int(val)}"))
        elif stat == "max":
            # ring the slowest dot
            peak = rows.loc[rows["pull_ms"].idxmax()]
            ax.scatter([peak["pull_time_h"]], [peak["pull_ms"]],
                       s=260, facecolors="none", edgecolors=c, linewidths=2.4, zorder=6)
            ax.scatter([peak["pull_time_h"]], [peak["pull_ms"]],
                       s=52, color=c, zorder=7)
            entries.append((c, f"{img} · max = {val:.0f} ms"))
        else:
            # horizontal line spanning the full x range at the stat value
            ax.axhline(val, color=c, lw=2.0, ls=(0, (6, 3)), alpha=0.9, zorder=5)
            entries.append((c, f"{img} · {stat} = {val:.0f} ms"))

    legend(ax, entries, "signal value")
    save(fig, name)


def main():
    df = load()
    chart_events(df)
    for stat, name, subtitle in STATS:
        chart_stat(df, stat, name, subtitle)


if __name__ == "__main__":
    main()


from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
import seaborn as sns

SRC = Path(__file__).resolve().parent
IMG = SRC.parent
DATA = SRC / "data" / "eventpulltime_samples.csv"

A = "#4361ee"   # redis (slow-tail image, top row)
B = "#2ec4b6"   # nginx (consistent image, bottom row)
INK = "#1a1a2e"
SUB = "#666"
BG = "#fafafa"

# fixed row order so switching tabs only changes the emphasis, never the layout
ROWS = [("redis:7", 1, A), ("nginx:1.25", 0, B)]

STATS = [
    ("p50", "signal-eventpulltime-p50.svg",
     "median pull — typical latency, ignores the slow outlier"),
    ("p90", "signal-eventpulltime-p90.svg",
     "90th percentile — the slow tail starts to show"),
    ("p95", "signal-eventpulltime-p95.svg",
     "95th percentile — strict worst-case tail for SLOs"),
    ("avg", "signal-eventpulltime-avg.svg",
     "mean pull — dragged upward by the one slow outlier"),
    ("max", "signal-eventpulltime-max.svg",
     "slowest single pull — the worst cold node"),
    ("count", "signal-eventpulltime-count.svg",
     "number of cold-pull events, regardless of duration"),
]


def load():
    df = pd.read_csv(DATA)
    return {img: df[df.image == img]["pull_ms"].to_numpy(dtype=float)
            for img, _, _ in ROWS}


def stat_value(vals, stat):
    if stat == "count":
        return float(vals.size)
    if stat == "avg":
        return float(vals.mean())
    if stat == "max":
        return float(vals.max())
    return float(np.percentile(vals, {"p50": 50, "p90": 90, "p95": 95}[stat]))


def new_fig(stat, subtitle, xmax):
    sns.set_theme(style="white", context="notebook")
    fig, ax = plt.subplots(figsize=(8.8, 3.0), dpi=130)
    fig.patch.set_facecolor(BG)
    ax.set_facecolor(BG)
    ax.set_title(f"eventPullTime · statistic={stat}", loc="left",
                 fontsize=13, fontweight="bold", color=INK, pad=18)
    ax.text(0.0, 1.06, subtitle, transform=ax.transAxes, fontsize=9, color=SUB)
    ax.set_xlim(0, xmax)
    ax.set_ylim(-0.7, 1.7)
    ax.set_yticks([y for _, y, _ in ROWS])
    ax.set_yticklabels([img for img, _, _ in ROWS])
    ax.set_xlabel("pull duration (ms)", fontsize=9, color="#7b7d80")
    for s in ["top", "right", "left"]:
        ax.spines[s].set_visible(False)
    ax.spines["bottom"].set_color("#ccd4e2")
    ax.grid(axis="x", color="#eceff4")
    ax.set_axisbelow(True)
    ax.tick_params(axis="y", length=0)
    return fig, ax


def legend(ax, entries, title):
    handles = [plt.Line2D([0], [0], color=c, lw=3) for c, _ in entries]
    labels = [l for _, l in entries]
    leg = ax.legend(
        handles, labels, title=title,
        loc="upper left", bbox_to_anchor=(1.02, 1.0),
        frameon=True, fontsize=9, title_fontsize=9, handlelength=1.4,
        borderpad=0.8, labelspacing=0.6,
    )
    leg.get_frame().set_facecolor("#ffffff")
    leg.get_frame().set_edgecolor("#e0e4ec")
    leg.get_title().set_color(INK)
    leg.get_title().set_fontweight("bold")
    leg._legend_box.align = "left"
    return leg


def save(fig, name):
    out = IMG / name
    fig.savefig(out, format="svg", bbox_inches="tight", pad_inches=0.2)
    plt.close(fig)
    print(f"wrote {out}")


def chart(samples, stat, name, subtitle, xmax):
    fig, ax = new_fig(stat, subtitle, xmax)
    entries = []
    for img, y, c in ROWS:
        vals = samples[img]
        val = stat_value(vals, stat)

        # distribution: range line min→max, then the individual sample dots
        if vals.size > 1:
            ax.hlines(y, vals.min(), vals.max(), color=c, lw=2, alpha=0.25,
                      zorder=2)
        ax.scatter(vals, np.full_like(vals, y), s=60, color=c, alpha=0.4,
                   edgecolors="none", zorder=3)

        if stat == "count":
            # the count IS the result: ring every sample
            ax.scatter(vals, np.full_like(vals, y), s=190, facecolors="none",
                       edgecolors=c, linewidths=1.8, zorder=5)
        elif stat == "max":
            # one extreme sample: ring the slowest pull
            ax.scatter([val], [y], s=230, facecolors="none", edgecolors=c,
                       linewidths=2.4, zorder=6)
            ax.scatter([val], [y], s=44, color=c, zorder=7)
        else:
            # p50/p90/p95/avg land on the duration axis (often interpolated
            # into the tail, where there is no dot) → a short vertical tick
            ax.vlines(val, y - 0.26, y + 0.26, color=c, lw=2.6, zorder=6)

        unit = "" if stat == "count" else " ms"
        entries.append((c, f"{img} · {stat} = {val:.0f}{unit}"))
    legend(ax, entries, "signal value")
    save(fig, name)


def main():
    samples = load()
    xmax = max(v.max() for v in samples.values()) * 1.12
    for stat, name, subtitle in STATS:
        chart(samples, stat, name, subtitle, xmax)


if __name__ == "__main__":
    main()
