---
title: Releasing
weight: 7
description: Tag-triggered CI, multi-arch builds, and Helm OCI publishing.
llmsDescription: |
  Release process for drop. Push a semver git tag to trigger CI: lint, test, e2e,
  multi-arch Docker build (amd64+arm64) to ghcr.io, Helm chart OCI push, GitHub Release.
---

## How to Release

```bash
git tag v0.1.0
git push origin v0.1.0
```

That's it. The CI pipeline handles the rest.

## What CI Does on Tag Push

1. **Lint** — golangci-lint
2. **Unit tests** — `make test` (envtest)
3. **E2E tests** — Chainsaw on kind
4. **Build multi-arch image** — `linux/amd64` + `linux/arm64` → `ghcr.io/breee/drop:<tag>`
5. **Package Helm chart** — push to OCI registry
6. **GitHub Release** — auto-generated release notes

## Versioning

| Format | Example | Use |
|--------|---------|-----|
| Stable | `v0.1.0` | Production release |
| Pre-release | `v0.1.0-rc.1` | Testing before stable |

Chart version in `charts/drop/Chart.yaml` tracks the app version.

## CI Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci.yml` | Push, PR | Lint + test + build + e2e |
| `release.yml` | Tag push | Multi-arch build + publish |
| `docs.yml` | docs/ changes | Hugo build + GitHub Pages deploy |
