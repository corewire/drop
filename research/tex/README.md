# Math model docs

This folder contains the mathematical model and paper assets used to explain cold image-cache impact in CI workloads.

## Main files

- `paper.tex`: primary model/paper source
- `paper.pdf`: latest rendered paper output
- `generated/`: CSV and table artifacts used by the paper
- `figures/`: exported figures referenced by the paper

## Scope

- Per-image cold/warm exposure model
- Burst vs sequential scheduling effects
- Affected job-minute objective
- Strategy comparison and oracle-gap framing

## Update flow

- Edit model text/equations in `paper.tex`
- Refresh generated tables/figures from benchmark outputs
- Rebuild `paper.pdf` after updates