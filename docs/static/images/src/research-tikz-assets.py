#!/usr/bin/env python3
from __future__ import annotations

import shutil
import subprocess
from pathlib import Path


FIGURES = [
    "research-concept-cold-warm",
    "research-concept-arrival-modes",
    "research-concept-affected-vs-warmed",
    "research-concept-rolling-timeline",
    "research-concept-developer-weight",
    "research-concept-exposure-curves",
    "research-concept-discovery-pipeline",
    "research-concept-mise-en-place",
    "research-concept-variance-intuition",
    "research-concept-night-warming",
    "research-concept-arrival-bounds",
    "research-concept-portfolio-cumulative",
    "research-concept-prewarm-vs-mirror",
    "research-concept-benchmark-pipeline",
    "research-concept-kitchen",
]


def run(cmd: list[str], cwd: Path) -> None:
    subprocess.run(cmd, cwd=cwd, check=True)


def main() -> None:
    src_dir = Path(__file__).resolve().parent
    repo_root = src_dir.parents[3]
    figures_src = repo_root / "research" / "tex" / "figures-src"
    docs_images = repo_root / "docs" / "static" / "images"

    if not shutil.which("pdflatex"):
        raise RuntimeError("pdflatex is required to build TikZ figures")
    if not shutil.which("dvisvgm"):
        raise RuntimeError("dvisvgm is required to convert PDF to SVG")

    for name in FIGURES:
        tex = figures_src / f"{name}.tex"
        pdf = figures_src / f"{name}.pdf"
        svg = figures_src / f"{name}.svg"
        if not tex.exists():
            raise FileNotFoundError(f"missing TikZ source: {tex}")

        run(["pdflatex", "-interaction=nonstopmode", "-halt-on-error", tex.name], cwd=figures_src)
        # --no-fonts renders glyphs as vector paths so word spacing is exact
        # (embedded fonts can collapse inter-word spaces in some viewers).
        # --bbox=papersize keeps the standalone page background (pagecolor).
        run(
            ["dvisvgm", "--pdf", "--no-fonts", "--bbox=papersize", f"--output={svg.name}", pdf.name],
            cwd=figures_src,
        )

        target = docs_images / f"{name}.svg"
        shutil.copy2(svg, target)
        print(f"wrote {target}")


if __name__ == "__main__":
    main()
