# Feature: Automated Releases

## Goal
Provide automated, repeatable releases similar to the `Breee/kubeswitch` release style.

## Plan
- Trigger release workflow on version tags.
- Generate changelog from conventional commits/PR metadata.
- Publish:
  - GitHub Release notes + assets
  - Helm chart artifacts
  - Container images to GHCR
- Sign/provenance support can be added as a hardening step.

## CI/CD checkpoints
- Validate tests and lint before release job starts.
- Block publish on failed e2e tests.
