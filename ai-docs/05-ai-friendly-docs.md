# Feature: AI-Friendly Documentation

## Goal
Adopt patterns from `Breee/ai-friendly-docs` so agents need fewer context calls.

## Conventions
- Small focused docs (one feature per file)
- Stable headings and predictable section order
- "Current State / Decision / Next Steps" blocks
- Explicit assumptions and non-goals
- Cross-links to canonical docs instead of duplicating long context

## CI checks
- Validate presence of required sections in critical docs
- Optionally fail CI if progress tracker and feature docs diverge
