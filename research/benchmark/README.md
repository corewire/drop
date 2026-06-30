# Benchmark docs

This folder contains replay and strategy-evaluation benchmarks for discovery and prewarming policies.

## Structure

- `evaluator/`: runnable benchmark scripts and baseline inputs/outputs
- `results/`: captured multi-run result snapshots and figures

## What is measured

- Per-job startup wait from cold image availability
- Affected job-minutes by policy
- Pipeline critical-path deltas
- Discovery strategy quality over repeated runs

## Primary entry points

- `evaluator/README.md`: setup and command reference
- `evaluator/evaluate_replay.py`: policy replay engine
- `evaluator/evaluate_discovery_strategies.py`: discovery ranking evaluation