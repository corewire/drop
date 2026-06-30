#!/usr/bin/env python3
"""Render eventPullTime signal charts as light infographics (CSV-driven).

signal-eventpulltime-events.svg  — Gantt chart: each pull as a horizontal bar
                                    (bar width = pull duration, x = wall-clock time)
signal-eventpulltime-{stat}.svg  — Boxplot per image with the chosen statistic
                                    highlighted (p50/p90/p95/avg/max/count)

Edit data/eventpulltime_samples.csv and rerun.
CSV columns: image, t_start_s, pull_s
"""

from pathlib import Path

import matplotlib.patches as mpatches
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

# Row order (top → bottom): redis row 1, nginx row 0
ROWS = [("redis:7", 1, B), ("nginx:1.25", 0, A)]
DISPLAY = {"redis:7": "img-B", "nginx:1.25": "img-A"}

STATS = [
    ("p50",   "signal-eventpulltime-p50.svg",
     "p50 = median — half of all pulls were faster"),
    ("p90",   "signal-eventpulltime-p90.svg",
     "p90 = 90th percentile — only 1 in 10 pulls was slower"),
    ("p95",   "signal-eventpulltime-p95.svg",
     "p95 = 95th percentile — strict worst-case tail"),
    ("avg",   "signal-eventpulltime-avg.svg",
     "avg = mean — pulled up by the slow outlier"),
    ("max",   "signal-eventpulltime-max.svg",
     "max = the slowest single pull per image"),
    ("count", "signal-eventpulltime-count.svg",
     "count = number of observed pull events"),
]


def load():
    return pd.read_csv(DATA)


def stat_value(vals, stat):
    if stat == "count":  return float(vals.size)
    if stat == "avg":    return float(vals.mean())
    if stat == "max":    return float(vals.max())
    return float(np.percentile(vals, {"p50": 50, "p90": 90, "p95": 95}[stat]))


def base_style(fig, ax):
    fig.patch.set_facecolor(BG)
    ax.set_facecolor(BG)
    for s in ["top", "right"]:
        ax.spines[s].set_visible(False)
    ax.spines["bottom"].set_color("#ccd4e2")
    ax.spines["left"].set_color("#ccd4e2")
    ax.set_axisbelow(True)


def framed_legend(ax, handles, labels, title):
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


def save(fig, name):
    out = IMG / name
    fig.savefig(out, format="svg", bbox_inches="tight", pad_inches=0.2)
    plt.close(fig)
    print(f"wrote {out}")


def chart_events(df):
    """Gantt: each pull as a horizontal bar. bar width = pull duration."""
    xmax = (df["t_start_s"] + df["pull_s"]).max()
    sns.set_theme(style="white", context="notebook")
    fig, ax = plt.subplots(figsize=(10.0, 2.8), dpi=130)
    base_style(fig, ax)
    ax.set_title("eventPullTime · observed pull events", loc="left",
                 fontsize=13, fontweight="bold", color=INK, pad=18)
    ax.text(0.0, 1.06, "each bar = one Pulled event · bar width = pull duration",
            transform=ax.transAxes, fontsize=9, color=SUB)
    for img, y, c in ROWS:
        rows = df[df.image == img]
        bars = list(zip(rows["t_start_s"], rows["pull_s"]))
        ax.broken_barh(bars, (y - 0.32, 0.64), facecolors=c, alpha=0.85, zorder=3)
    ax.set_xlim(-5, xmax + 5)
    ax.set_ylim(-0.7, 1.7)
    ax.set_yticks([y for _, y, _ in ROWS])
    ax.set_yticklabels([DISPLAY[img] for img, _, _ in ROWS])
    ax.set_xlabel("time (s)", fontsize=9, color="#7b7d80")
    ax.tick_params(axis="y", length=0)
    ax.grid(axis="x", color="#eceff4")
    patches = [mpatches.Patch(color=c, label=DISPLAY[img]) for img, _, c in ROWS]
    framed_legend(ax, patches, [DISPLAY[img] for img, _, _ in ROWS], "image")
    save(fig, "signal-eventpulltime-events.svg")


def chart_stat(df, stat, name, subtitle):
    """Horizontal boxplot per image + jittered points + statistic emphasis."""
    sns.set_theme(style="white", context="notebook")
    fig, ax = plt.subplots(figsize=(8.8, 3.5), dpi=130)
    base_style(fig, ax)
    ax.set_title(f"eventPullTime · statistic={stat}", loc="left",
                 fontsize=13, fontweight="bold", color=INK, pad=18)
    ax.text(0.0, 1.06, subtitle, transform=ax.transAxes, fontsize=9, color=SUB)

    # ROWS define fixed y positions: redis=1 (top), nginx=0 (bottom).
    positions = {img: y for img, y, _ in ROWS}
    colors    = {img: c for img, _, c in ROWS}
    data      = {img: df[df.image == img]["pull_s"].to_numpy(dtype=float)
                 for img, _, _ in ROWS}
    order     = [img for img, _, _ in ROWS]

    bp = ax.boxplot(
        [data[img] for img in order],
        positions=[positions[img] for img in order],
        vert=False,
        widths=0.52,
        patch_artist=True,
        showfliers=False,
        medianprops=dict(color=INK, lw=1.5, zorder=4),
        whiskerprops=dict(color="#a9b2c3", lw=1.3),
        capprops=dict(color="#a9b2c3", lw=1.3),
    )
    for patch, img in zip(bp["boxes"], order):
        patch.set_facecolor(colors[img])
        patch.set_alpha(0.22)
        patch.set_edgecolor(colors[img])
        patch.set_linewidth(1.3)

    # Jittered individual events to keep samples visible without overwhelming.
    rng = np.random.default_rng(42)
    for img in order:
        vals = data[img]
        jitter = rng.uniform(-0.085, 0.085, size=len(vals))
        ax.scatter(vals, positions[img] + jitter,
                   s=30, color=colors[img], alpha=0.65, zorder=5,
                   edgecolors="#ffffff", linewidths=0.6)

    legend_entries = []
    xmax = max(v.max() for v in data.values())
    for img in order:
        vals = data[img]
        val = stat_value(vals, stat)
        pos = positions[img]
        c = colors[img]

        if stat == "count":
            ax.text(xmax * 1.03, pos, f"n={int(val)}",
                    ha="left", va="center", fontsize=10.5,
                    fontweight="bold", color=c)
            legend_entries.append((c, f"{DISPLAY[img]} · count = {int(val)}"))
        elif stat == "max":
            ax.scatter([val], [pos], s=250, facecolors="none", edgecolors=c,
                       linewidths=2.4, zorder=7)
            ax.scatter([val], [pos], s=42, color=c, zorder=8)
            legend_entries.append((c, f"{DISPLAY[img]} · max = {val:.0f} s"))
        else:
            ax.vlines(val, pos - 0.27, pos + 0.27, colors=c, lw=2.8,
                      capstyle="round", zorder=7)
            legend_entries.append((c, f"{DISPLAY[img]} · {stat} = {val:.0f} s"))

    ax.set_ylim(-0.7, 1.7)
    ax.set_xlim(10, xmax * 1.14)
    ax.set_yticks([positions[img] for img in order])
    ax.set_yticklabels([DISPLAY[img] for img in order], fontsize=10)
    ax.set_xlabel("pull duration (s)", fontsize=9, color="#7b7d80")
    ax.tick_params(axis="y", length=0)
    ax.grid(axis="x", color="#eceff4")

    handles = [plt.Line2D([0], [0], color=c, lw=3) for c, _ in legend_entries]
    framed_legend(ax, handles, [l for _, l in legend_entries], "signal value")
    save(fig, name)


def main():
    df = load()
    chart_events(df)
    for stat, name, subtitle in STATS:
        chart_stat(df, stat, name, subtitle)


if __name__ == "__main__":
    main()


