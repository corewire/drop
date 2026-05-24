# AI Docs

Living design documents for the puller operator. Historical planning docs have been archived to `docs/decisions/`.

## Current Files

- `progress.md` — implementation tracking checklist
- `05-ai-friendly-docs.md` — documentation generation strategy and conventions
- `13-discovery-architecture.md` — discovery reconciliation flow, query contract, source types
- `14-architecture.md` — system architecture: reconcilers, pull mechanism, pacing
- `15-implementation-plan.md` — tasks, acceptance criteria, dependencies

## Generated Docs (DO NOT EDIT)

All generated documentation lives at the repo root and in `docs/content/docs/reference/`:
- `knowledge.yaml` — structured intermediate (full project model)
- `llms.txt` / `llms-full.txt` — for USE agents
- `.github/copilot-instructions.md` / `.cursorrules` / `AGENTS.md` — for CODE agents
- `docs/content/docs/reference/_generated_*.md` — for humans (Hugo)
- `docs/doc-generation.md` — Mermaid diagram of the generation flow

Regenerate with: `make docs-gen`
