#!/usr/bin/env python3
"""Render the discovery signal charts as light infographics (CSV-driven).

One SVG per method, consistent style:
  - prometheus-sampling : raw samples that feed every signal
  - signal-aggregate    : sum all samples -> one score per image
  - signal-timeweighted : scale each hour by a weight band, then sum
  - signal-windowaggregate : keep only samples inside one daily window

Edit data/discovery_series.csv (one representative day per image) and rerun.
"""

from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
import seaborn as sns

SRC = Path(__file__).resolve().parent
IMG = SRC.parent
DATA = SRC / "data" / "discovery_series.csv"

A = "#4361ee"   # img-A
B = "#2ec4b6"   # img-B
INK = "#1a1a2e"
SUB = "#666"
BG = "#fafafa"

HOURS = np.arange(0, 49)            # 0..48 inclusive, hourly
SAMPLE_HOURS = [0, 6, 12, 18]       # rows present in the CSV


def daily_curve(values):
    """Interpolate the 4 sample points into a smooth 24h curve (wraps at 24)."""
    anchor_x = SAMPLE_HOURS + [24]
    anchor_y = values + [values[0]]
    hx = np.arange(0, 24)
    return np.interp(hx, anchor_x, anchor_y)


def load():
    day = pd.read_csv(DATA)
    series = {}
    samples = {}
    for image in ["img-A", "img-B"]:
        vals = [int(day[(day.image == image) & (day.hour == h)]["count"].iloc[0]) for h in SAMPLE_HOURS]
        d = daily_curve(vals)
        full = np.concatenate([d, d, [d[0]]])      # 2 days + closing point -> len 49
        series[image] = full
        samples[image] = vals
    return series, samples


def new_fig(title, subtitle):
    sns.set_theme(style="white", context="notebook")
    fig, ax = plt.subplots(figsize=(8.8, 3.4), dpi=130)
    fig.patch.set_facecolor(BG)
    ax.set_facecolor(BG)
    ax.set_title(title, loc="left", fontsize=13, fontweight="bold", color=INK, pad=18)
    ax.text(0.0, 1.04, subtitle, transform=ax.transAxes, fontsize=9, color=SUB)
    ax.set_xticks(np.arange(0, 49, 6))
    ax.set_xticklabels(["00", "06", "12", "18", "00", "06", "12", "18", "24"])
    ax.set_xlim(0, 48)
    ax.set_ylim(-0.6, 9)
    ax.set_xlabel("hour of day", fontsize=9, color="#7b7d80")
    ax.axvline(24, color="#c9cdd6", ls=(0, (3, 3)), lw=1)
    ax.text(12, 8.6, "day 1", ha="center", fontsize=8, color="#9aa0ab")
    ax.text(36, 8.6, "day 2", ha="center", fontsize=8, color="#9aa0ab")
    for s in ["top", "right"]:
        ax.spines[s].set_visible(False)
    for s in ["left", "bottom"]:
        ax.spines[s].set_color("#ccd4e2")
    ax.grid(axis="y", color="#eceff4")
    ax.set_axisbelow(True)
    return fig, ax


def legend(ax, entries, title):
    """entries: list of (color, label). Placed OUTSIDE the axes (right) so it
    never overlaps the data."""
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
    fig.subplots_adjust(left=0.07, right=0.74, top=0.82, bottom=0.16)
    out = IMG / name
    # bbox_inches="tight" grows the canvas to include the outside legend so
    # nothing is ever clipped, regardless of label length.
    fig.savefig(out, format="svg", bbox_inches="tight", pad_inches=0.2)
    plt.close(fig)
    print(f"wrote {out}")


def markers(ax, series, color):
    sx = SAMPLE_HOURS + [h + 24 for h in SAMPLE_HOURS]
    ax.plot(sx, [series[h] for h in sx], "o", color=color, ms=5, zorder=5)


# ---------------------------------------------------------------- prometheus
def chart_sampling(series, samples):
    fig, ax = new_fig("count(...) by (image)",
                      "last 48h · step 1h — raw samples that feed every signal")
    for img, c in [("img-A", A), ("img-B", B)]:
        ax.fill_between(HOURS, series[img], color=c, alpha=0.12)
        ax.plot(HOURS, series[img], color=c, lw=2)
        markers(ax, series[img], c)
    legend(ax, [(A, "img-A"), (B, "img-B")], "series")
    save(fig, "prometheus-sampling.svg")


# ----------------------------------------------------------------- aggregate
# One line graph per aggregation Method enum value (sum | count | avg | max | min).
# Each chart shows the SAME 48h series; the dashed reference line marks the score
# that this method reduces each image's samples to.
AGG_METHODS = [
    ("sum", "signal-aggregate-sum.svg",
     "add up every sample in the lookback window"),
    ("count", "signal-aggregate-count.svg",
     "count how many samples landed in the window"),
    ("avg", "signal-aggregate-avg.svg",
     "average value across all samples"),
    ("max", "signal-aggregate-max.svg",
     "the single highest sample"),
    ("min", "signal-aggregate-min.svg",
     "the single lowest sample"),
]


def agg_score(vals, method):
    arr = np.array(vals * 2, dtype=float)   # 2 days -> 8 samples
    return {
        "sum": arr.sum(),
        "count": float(arr.size),
        "avg": arr.mean(),
        "max": arr.max(),
        "min": arr.min(),
    }[method]


def _fmt(v):
    return f"{v:.0f}" if v == int(v) else f"{v:.1f}"


def chart_aggregate_method(series, samples, method, name, subtitle):
    fig, ax = new_fig(f"aggregate · method={method}", subtitle)
    sample_x = SAMPLE_HOURS + [h + 24 for h in SAMPLE_HOURS]
    entries = []
    for img, c in [("img-A", A), ("img-B", B)]:
        s = series[img]
        score = agg_score(samples[img], method)

        if method == "sum":
            # accumulation: shade the area, drop a line from every sample
            ax.fill_between(HOURS, s, color=c, alpha=0.15)
            for h in sample_x:
                ax.vlines(h, 0, s[h], color=c, lw=1, ls=(0, (2, 2)), alpha=0.45)
            ax.plot(HOURS, s, color=c, lw=2)
            markers(ax, s, c)

        elif method == "count":
            # counting points: faded line, ring every sample
            ax.plot(HOURS, s, color=c, lw=1.4, alpha=0.45)
            for h in sample_x:
                ax.plot(h, s[h], "o", ms=11, mfc="white", mec=c, mew=1.8, zorder=5)
                ax.plot(h, s[h], "o", ms=3, color=c, zorder=6)

        elif method == "avg":
            # the mean IS the result: one prominent horizontal line per series
            ax.plot(HOURS, s, color=c, lw=1.6, alpha=0.55)
            markers(ax, s, c)
            ax.axhline(score, color=c, lw=2.2, ls=(0, (6, 3)), zorder=4)

        elif method in ("max", "min"):
            # one extreme sample is the result: keep the series in colour but
            # faint, then ring the single point that wins.
            ax.plot(HOURS, s, color=c, lw=1.6, alpha=0.4)
            for h in sample_x:
                ax.plot(h, s[h], "o", ms=4, color=c, alpha=0.4, zorder=3)
            pick = max if method == "max" else min
            ext_h = pick(sample_x, key=lambda h: s[h])
            ax.plot(ext_h, s[ext_h], "o", ms=16, mfc="none", mec=c,
                    mew=2.6, zorder=6, clip_on=False)
            ax.plot(ext_h, s[ext_h], "o", ms=6, color=c, zorder=7,
                    clip_on=False)

        entries.append((c, f"{img} · {method} = {_fmt(score)}"))
    legend(ax, entries, "score")
    save(fig, name)


def chart_aggregate(series, samples):
    for method, name, subtitle in AGG_METHODS:
        chart_aggregate_method(series, samples, method, name, subtitle)


# ------------------------------------------------------------- timeWeighted
def chart_timeweighted(series, samples):
    fig, ax = new_fig("timeWeightedAggregate",
                      "scale each hour by its weight band, then sum")
    bands = [(7, 9, 0.3), (9, 17, 1.0), (17, 20, 0.3)]
    for d in (0, 24):
        for lo, hi, w in bands:
            ax.axvspan(lo + d, hi + d, color="#7209b7",
                       alpha=0.05 + 0.13 * w, lw=0)
    for img, c in [("img-A", A), ("img-B", B)]:
        ax.plot(HOURS, series[img], color=c, lw=2)
        markers(ax, series[img], c)
    legend(ax, [(A, "img-A"), (B, "img-B"),
                ("#cdb4e6", "×0.3  off-hours"),
                ("#7209b7", "×1.0  core 09–17")],
           "hour weights")
    save(fig, "signal-timeweighted.svg")


# -------------------------------------------------------------- windowAggregate
def chart_window(series, samples):
    fig, ax = new_fig("windowAggregate",
                      "keep only samples inside one daily window (09:00–17:00)")
    sel = np.zeros_like(HOURS, dtype=bool)
    for d in (0, 24):
        for h in range(9 + d, 18 + d):
            if h <= 48:
                sel[h] = True
        ax.axvspan(9 + d, 17 + d, color="#4361ee", alpha=0.12, lw=0)
    for img, c in [("img-A", A), ("img-B", B)]:
        ax.plot(HOURS, series[img], color=c, lw=1.2, alpha=0.35)
        ax.plot(np.where(sel, HOURS, np.nan), np.where(sel, series[img], np.nan),
                color=c, lw=2.4)
        for h in SAMPLE_HOURS + [h + 24 for h in SAMPLE_HOURS]:
            inside = (9 <= (h % 24) < 17)
            ax.plot(h, series[img][h], "o", color=c, ms=6 if inside else 4,
                    alpha=1.0 if inside else 0.3, zorder=5)
    legend(ax, [(A, "img-A"), (B, "img-B"), ("#9dbff5", "selected window 09–17")],
           "windowAggregate")
    save(fig, "signal-windowaggregate.svg")


def main():
    series, samples = load()
    chart_sampling(series, samples)
    chart_aggregate(series, samples)
    chart_timeweighted(series, samples)
    chart_window(series, samples)


if __name__ == "__main__":
    main()
