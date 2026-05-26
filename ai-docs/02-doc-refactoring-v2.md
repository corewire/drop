# Audience → Information Matrix

## Four Audiences

| Audience | Format Need | Current Output |
|----------|------------|----------------|
| Human USE (operator users) | Narrative guides, navigable site | Hugo docs (install, usage, discovery, monitoring) |
| AI USE (agents deploying/configuring) | Flat, complete, single-request | `llms.txt`, `llms-full.txt` |
| Human DEV (contributors) | Explanatory prose, how-tos | Hugo docs (developing/*) |
| AI DEV (coding assistants) | Terse rules, file mappings | `.github/copilot-instructions.md`, `.cursorrules`, `AGENTS.md` |

## What Each Audience Needs

| Information | Human USE | AI USE | Human DEV | AI DEV |
|-------------|:---------:|:------:|:---------:|:------:|
| Install (Helm values, prereqs) | ✓ | ✓ | — | — |
| CRD field reference (types, defaults, validation) | ✓ | ✓ | — | — |
| Working YAML examples | ✓ | ✓ | — | — |
| Error reasons + troubleshooting | ✓ | ✓ | — | — |
| Metrics + PromQL queries | ✓ | ✓ | — | — |
| Discovery guide (narrative how-to) | ✓ | — | — | — |
| Monitoring dashboards / events | ✓ | — | — | — |
| API group + resource names | — | ✓ | — | ✓ |
| Constraints (cluster-scoped, no privileged) | — | ✓ | — | ✓ |
| Architecture (package graph, reconciler design) | — | — | ✓ | ✓ |
| Conventions (naming, patterns, don'ts) | — | — | ✓ | ✓ |
| Build/test commands | — | — | ✓ | ✓ |
| Package imports + dependency graph | — | — | ✓ | ✓ |
| How to extend (add CRD, new source) | — | — | ✓ | ✓ |
| Debugging (Delve, logs, common issues) | — | — | ✓ | — |
| Release process | — | — | ✓ | — |
| Controller file ↔ CRD mapping | — | — | — | ✓ |
| Don'ts (pacing outside pkg, namespaced CRDs) | — | — | ✓ | ✓ |

## Key Insights

1. AI audiences need the same *facts* as their human counterparts, but in a different *format*:
   - AI USE: schema + examples in a single flat file — no navigation, no narrative
   - AI DEV: conventions + architecture as terse rules — not explanatory prose

2. Truly shared content:
   - CRD fields/types → USE (both human & AI)
   - Conventions/don'ts → DEV (both human & AI)

3. Audience-exclusive content:
   - Narrative guides (discovery, monitoring) → Human USE only
   - Debugging/release → Human DEV only
   - File-to-kind mapping, import paths → AI DEV only

## Implication for Generated Docs

Generated outputs should only produce content that is:
- Derivable from source code (CRD fields, errors, metrics, package graph)
- Needed by at least one audience in that exact format

Manual docs should only exist for:
- Narrative that can't be extracted (guides, how-tos, explanations)
- Stable content that won't go stale (install steps, design decisions)

## Generation Feasibility

### Easily generated (already doing or trivial)

| Information | Source | Ease |
|-------------|--------|------|
| CRD fields (names, types, descriptions, defaults, enums, required) | CRD YAML or Go AST | Trivial |
| Error reasons + messages | Controller source (`.Reason = "..."`) | Easy (regex) |
| Metrics (name, type, help) | `internal/metrics/metrics.go` | Easy (regex) |
| Package dependency graph | `go list -json ./...` | Trivial (stdlib) |
| Build/test commands | Makefile `##` annotations | Easy (regex) |
| API group + resource names | CRD YAML or go.mod + types | Trivial |
| Sample CRs | Embed `hack/dev-samples.yaml` as-is | Trivial |

### Could generate (marginal value)

| Information | Source | Notes |
|-------------|--------|-------|
| Helm values reference | `charts/drop/values.yaml` | Parse YAML + comments → table |
| `scope: Cluster` constraint | CRD YAML `spec.scope` | Replace hardcoded convention |

### Cannot generate (human knowledge)

| Information | Why |
|-------------|-----|
| Conventions, don'ts, architectural decisions | Domain knowledge, not in code |
| Narrative guides (discovery, monitoring, debugging, extending) | Explanatory prose |
| PromQL queries | Domain expertise |
| Troubleshooting advice ("How to Fix") | Operational experience |
| Release process | Workflow knowledge |
