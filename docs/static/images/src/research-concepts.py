#!/usr/bin/env python3
from __future__ import annotations

from pathlib import Path

import matplotlib.pyplot as plt
from matplotlib.patches import Circle, Rectangle


BG = "#fafafa"
INK = "#1a1a2e"
SUB = "#666"
COLD = "#4361ee"
WARM = "#2ec4b6"
ACCENT = "#d81159"


def out_dir() -> Path:
    return Path(__file__).resolve().parents[1]


def save(fig: plt.Figure, name: str) -> None:
    target = out_dir() / name
    fig.patch.set_facecolor(BG)
    fig.savefig(target, format="svg", facecolor=BG, bbox_inches="tight", pad_inches=0.1)
    plt.close(fig)
    print(f"wrote {target}")


def fig_cold_warm_nodes() -> None:
    fig, ax = plt.subplots(figsize=(8.6, 3.0))
    ax.set_facecolor(BG)
    ax.set_xlim(-0.5, 10.2)
    ax.set_ylim(-1.2, 1.8)
    ax.axis("off")

    cold_nodes = [0, 1, 2, 3, 4, 5]
    warm_nodes = [6, 7, 8, 9]

    for i in cold_nodes:
        ax.add_patch(Circle((i, 0.6), 0.32, facecolor=COLD, edgecolor=INK, lw=1.2, alpha=0.25))
        ax.text(i, 0.6, f"N{i}", ha="center", va="center", fontsize=8, color=INK)

    for i in warm_nodes:
        ax.add_patch(Circle((i, 0.6), 0.32, facecolor=WARM, edgecolor=INK, lw=1.2, alpha=0.25))
        ax.text(i, 0.6, f"N{i}", ha="center", va="center", fontsize=8, color=INK)

    ax.text(0, 1.35, "Cold nodes for image I", color=COLD, fontsize=10, weight="bold")
    ax.text(6, 1.35, "Warm nodes for image I", color=WARM, fontsize=10, weight="bold")
    ax.text(
        0,
        -0.5,
        "Same cluster, one image I: nodes can be warm or cold for that image.",
        fontsize=10,
        color=SUB,
    )
    ax.text(0, -0.85, "Warm => job can start quickly. Cold => job waits for image availability.", fontsize=9, color=SUB)

    save(fig, "research-concept-cold-warm.svg")


def fig_arrival_modes() -> None:
    fig, ax = plt.subplots(figsize=(9.2, 3.2))
    ax.set_facecolor(BG)
    ax.set_xlim(0, 100)
    ax.set_ylim(0, 4)
    ax.axis("off")

    # time axis
    ax.plot([5, 95], [0.6, 0.6], color=INK, lw=1)
    ax.text(95.5, 0.6, "time", va="center", fontsize=8, color=SUB)

    # sequential
    ax.text(7, 3.25, "Sequential arrival", color=INK, fontsize=10, weight="bold")
    for x in [12, 26, 40, 54]:
        ax.add_patch(Rectangle((x, 2.65), 6, 0.35, facecolor=COLD, alpha=0.28, edgecolor=INK, lw=1))

    # burst
    ax.text(37, 3.25, "Burst arrival", color=INK, fontsize=10, weight="bold")
    for x in [42, 45.5, 49, 52.5]:
        ax.add_patch(Rectangle((x, 1.95), 5, 0.35, facecolor=ACCENT, alpha=0.28, edgecolor=INK, lw=1))

    # rolling
    ax.text(67, 3.25, "Rolling concurrency", color=INK, fontsize=10, weight="bold")
    for x in [70, 76, 79.5, 84, 88.5]:
        ax.add_patch(Rectangle((x, 1.25), 7, 0.35, facecolor=WARM, alpha=0.28, edgecolor=INK, lw=1))

    ax.text(8, 0.1, "Sequential: spaced jobs, warm-up can help later arrivals.", fontsize=9, color=SUB)
    ax.text(8, -0.2, "Burst: many jobs start together and see the same initial cache state.", fontsize=9, color=SUB)
    ax.text(8, -0.5, "Rolling: continuous overlap, closest to real CI traffic.", fontsize=9, color=SUB)

    save(fig, "research-concept-arrival-modes.svg")


def fig_affected_vs_warmed() -> None:
    fig, ax = plt.subplots(figsize=(8.6, 3.1))
    ax.set_facecolor(BG)
    ax.set_xlim(-0.5, 10.2)
    ax.set_ylim(-1.2, 2.0)
    ax.axis("off")

    # nodes
    for i in range(10):
        face = COLD if i < 6 else WARM
        alpha = 0.25
        ax.add_patch(Circle((i, 0.4), 0.32, facecolor=face, edgecolor=INK, lw=1.1, alpha=alpha))
        ax.text(i, 0.4, f"N{i}", ha="center", va="center", fontsize=8, color=INK)

    # jobs waiting on same cold node
    jobs = [(0, 1.45, "J1"), (0.55, 1.75, "J2"), (2, 1.45, "J3")]
    for x, y, label in jobs:
        ax.add_patch(Rectangle((x - 0.18, y - 0.12), 0.36, 0.24, facecolor=ACCENT, alpha=0.25, edgecolor=INK, lw=1))
        ax.text(x, y, label, ha="center", va="center", fontsize=8, color=INK)

    ax.annotate("", xy=(0, 0.72), xytext=(0, 1.28), arrowprops=dict(arrowstyle="->", lw=1, color=INK))
    ax.annotate("", xy=(0, 0.72), xytext=(0.55, 1.58), arrowprops=dict(arrowstyle="->", lw=1, color=INK))
    ax.annotate("", xy=(2, 0.72), xytext=(2, 1.28), arrowprops=dict(arrowstyle="->", lw=1, color=INK))

    ax.text(0, -0.45, "Affected jobs = 3 (J1, J2, J3 waited)", fontsize=10, color=ACCENT, weight="bold")
    ax.text(0, -0.82, "Newly warmed nodes = 2 (N0 and N2 warmed once)", fontsize=10, color=WARM, weight="bold")
    ax.text(0, -1.1, "Many jobs can wait on one node warm-up. These counts are different.", fontsize=9, color=SUB)

    save(fig, "research-concept-affected-vs-warmed.svg")


def main() -> None:
    fig_cold_warm_nodes()
    fig_arrival_modes()
    fig_affected_vs_warmed()


if __name__ == "__main__":
    main()
