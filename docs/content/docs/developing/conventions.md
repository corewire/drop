---
title: Code Conventions
weight: 6
description: Naming, patterns, and rules for contributing.
llmsDescription: |
  Code conventions for drop. CRDs PascalCase, cluster-scoped. Status uses
  metav1.Condition type "Ready". Pod builder is pure function. Pacing in
  internal/pacing/ only. Table-driven tests. Import order: stdlib, k8s, project.
---

## Naming

- CRD kinds: PascalCase (`CachedImage`, not `Cached_Image`)
- API group: `drop.corewire.io/v1alpha1`
- Controller files: `<kind>_controller.go` (lowercase)
- Test files: `<kind>_controller_test.go`

## Status Patterns

Always use `metav1.Condition` with type `"Ready"`:

```go
meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    Reason:             "AllNodesCached",
    Message:            "Image cached on all target nodes",
    ObservedGeneration: obj.Generation,
})
```

Phase progression: `Pending` → `Pulling` → `Ready` (or `Degraded`).

## Error Classification

Controllers classify errors into condition reasons:
- `DNSError`, `ConnectionRefused`, `Timeout`, `AuthenticationFailed`, `NotFound`, `RateLimited`

## Pod Construction Rules

- Always use `podbuilder.BuildDropPod()` — never construct Pods inline
- Pods get labels: `app.kubernetes.io/managed-by=drop`, `drop.corewire.io/cachedimage=<name>`, `drop.corewire.io/node=<node>`
- `RestartPolicy: Never`
- `AutomountServiceAccountToken: false`
- `TerminationGracePeriodSeconds: 0`

## Import Order

```go
import (
    // stdlib
    "context"
    "fmt"

    // k8s / controller-runtime
    "sigs.k8s.io/controller-runtime/pkg/client"

    // project
    dropv1alpha1 "github.com/Breee/drop/api/v1alpha1"
    "github.com/Breee/drop/internal/pacing"
)
```

## Test Patterns

- Table-driven tests preferred
- envtest for controllers (real API server, no kubelet)
- `httptest.NewServer` for discovery source mocks
- No mocking the k8s client directly — use envtest

## Don'ts

- Don't add CRI socket access or privileged containers
- Don't put pacing logic outside `internal/pacing/`
- Don't create namespaced CRDs
- Don't manually edit generated files (`zz_generated.deepcopy.go`, `config/crd/bases/`)
- Don't manually edit `llms.txt`, `llms-full.txt`, `.cursorrules`, `AGENTS.md` — run `make docs-gen`
- Don't construct Pods outside of `podbuilder.BuildDropPod()`
- Don't use `client.Mock` — use envtest instead
