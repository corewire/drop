---
title: Testing
weight: 3
description: Unit tests with envtest, E2E with Chainsaw, and test patterns.
llmsDescription: |
  Testing guide for puller. Unit tests use controller-runtime envtest (real API server,
  no kubelet). E2E uses Kyverno Chainsaw on kind. Table-driven tests preferred.
  Discovery tests mock HTTP servers. Controller tests use real k8s client.
---

## Unit Tests (envtest)

```bash
make test
```

Uses controller-runtime's `envtest` — a real API server + etcd, no kubelet.
Coverage report lands in `cover.out`.

### Test Locations

| Path | What it tests |
|------|---------------|
| `internal/controller/*_test.go` | Controller reconciliation logic |
| `internal/pacing/*_test.go` | Pacing engine constraints |
| `internal/podbuilder/*_test.go` | Pod construction correctness |
| `internal/discovery/*_test.go` | Source implementations |

## E2E Tests (Chainsaw)

```bash
make test-e2e
```

Requires a running kind cluster with the operator deployed (Tilt handles this).
Tests live in `test/e2e/` and use [Kyverno Chainsaw](https://kyverno.github.io/chainsaw/).

Each test scenario is a directory with `chainsaw-test.yaml` defining steps:
1. Apply a resource
2. Assert expected state (status, child resources, events)
3. Cleanup

## Writing Tests

### Table-driven (preferred)

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name string
        // inputs
        // expected outputs
    }{
        {name: "happy path", ...},
        {name: "error case", ...},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // arrange, act, assert
        })
    }
}
```

### Controller tests (envtest)

```go
var k8sClient client.Client
var testEnv *envtest.Environment
// Setup in TestMain or BeforeSuite
```

Create resources with the real client, trigger reconciliation, assert status changes.

### Discovery tests (mock HTTP)

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Return mock Prometheus/Registry response
}))
defer srv.Close()

source := &PrometheusSource{Endpoint: srv.URL, ...}
results, err := source.Fetch(ctx)
```

## Adding a New Test

1. Create `*_test.go` next to the code being tested
2. Use table-driven format with descriptive case names
3. For controllers: create the CRD resource, reconcile, assert status
4. For discovery: mock the HTTP endpoint, call Fetch, assert results
5. Run `make test` to validate
