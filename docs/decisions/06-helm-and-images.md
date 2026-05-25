# Feature: Helm Chart + Multi-Arch Images

## Helm plan
- Provide a simple chart with defaults for:
  - operator deployment
  - RBAC/service account
  - metrics endpoint/service monitor (optional)
- Package chart in CI and publish as release artifact.

## Image plan
- Build and push to GitHub Container Registry (GHCR).
- Target architectures:
  - `linux/amd64`
  - `linux/arm64`
- Publish multi-platform manifest tags per release.
