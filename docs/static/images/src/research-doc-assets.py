#!/usr/bin/env python3
from __future__ import annotations

import shutil
from pathlib import Path

import matplotlib.image as mpimg
import matplotlib.pyplot as plt


RESEARCH_FIGURES = [
    "cold_exposure_by_policy_20runs.png",
    "minutes_saved_by_policy_20runs.png",
    "strategy_total_savings_top10.png",
    "strategy_dev_window_savings_top10.png",
    "oracle_gap_total_savings_top10.png",
    "oracle_gap_dev_window_savings_top10.png",
]

MODEL_FILES = [
    "strategy_summary_top10.csv",
    "strategy_comparison_all_runs.csv",
    "oracle_gap_strategy_summary_top10.csv",
    "oracle_gap_strategy_comparison_all_runs.csv",
    "policy_comparison_concise_20runs.csv",
]


# Keep docs assets deterministic and reproducible by regenerating from research/.
def main() -> None:
    src_dir = Path(__file__).resolve().parent
    repo_root = src_dir.parents[3]

    research_tex = repo_root / "research" / "tex"
    research_figures = research_tex / "figures"
    research_generated = research_tex / "generated"

    docs_images = repo_root / "docs" / "static" / "images"
    docs_research = repo_root / "docs" / "static" / "research"
    docs_research_paper = docs_research / "paper"
    docs_research_models = docs_research / "models"

    docs_research.mkdir(parents=True, exist_ok=True)
    docs_research_paper.mkdir(parents=True, exist_ok=True)
    docs_research_models.mkdir(parents=True, exist_ok=True)

    # Copy paper sources into docs static for direct download.
    for name in ["paper.pdf", "paper.tex"]:
        src = research_tex / name
        if src.exists():
            shutil.copy2(src, docs_research_paper / name)

    # Copy selected model summary CSVs.
    for name in MODEL_FILES:
        src = research_generated / name
        if src.exists():
            shutil.copy2(src, docs_research_models / name)

    # Reuse paper figures in docs as SVG assets.
    for name in RESEARCH_FIGURES:
        src = research_figures / name
        if not src.exists():
            continue

        out_svg = docs_images / f"research-{Path(name).stem}.svg"

        img = mpimg.imread(src)
        height, width = img.shape[:2]

        fig, ax = plt.subplots(figsize=(width / 100.0, height / 100.0), dpi=100)
        ax.imshow(img)
        ax.set_axis_off()
        fig.patch.set_facecolor("#fafafa")
        fig.savefig(out_svg, format="svg", bbox_inches="tight", pad_inches=0)
        plt.close(fig)


if __name__ == "__main__":
    main()
