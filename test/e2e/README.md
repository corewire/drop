# Chainsaw E2E Tests

This directory contains scenario-based E2E tests using [Kyverno Chainsaw](https://kyverno.github.io/chainsaw/).

## Prerequisites

- A running Kind cluster with the operator deployed
- `chainsaw` binary installed (`make chainsaw`)

## Running

```bash
# From repo root
make test-e2e-chainsaw
```

## Test Scenarios

| Directory | Description |
|-----------|-------------|
| `cachedimage-basic/` | Basic CachedImage creation and pod scheduling |
| `cachedimage-pacing/` | PullPolicy pacing enforcement |
| `cachedimageset/` | CachedImageSet managing child resources |
| `discovery-prometheus/` | DiscoveryPolicy with mock Prometheus |
| `pull-policy-backoff/` | Failure backoff behavior |
