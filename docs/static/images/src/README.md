# Editable image sources

This directory is the editable source of truth for selected docs images.

## Practical workflow

- Diagram: `../discovery-pipeline.drawio.svg` is an **editable SVG** — it renders directly in the docs *and* opens in draw.io (app.diagrams.net or the VS Code "Draw.io Integration" extension). Edit and save; no separate export step.
- Data/benchmark charts: Python + pandas + seaborn (matplotlib backend)
- Keep images in `docs/static/images/` for Hugo pages

## Files

- `../discovery-pipeline.drawio.svg` -> editable SVG (render + draw.io source in one file)
- `data/eventpulltime_samples.csv` -> simple sample rows for the chart
- `signal-eventpulltime.py` -> reads CSV, writes `../signal-eventpulltime.svg`

## Export commands

```bash
# One-command static generation via Make (creates venv + installs deps)
make static
```

Open `../discovery-pipeline.drawio.svg` in draw.io, edit, and save in place.
Edit `data/eventpulltime_samples.csv` and rerun the script to regenerate its SVG.

## Why this setup

- The pipeline diagram is one editable SVG — no AI required
- Charts are reproducible from code and data
- No AI tooling is required to edit, tweak, or regenerate assets
