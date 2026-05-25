---
title: Extending
weight: 5
description: Step-by-step guide to adding a new CRD.
llmsDescription: |
  How to add a new CRD to puller. Steps: define types in api/v1alpha1/, run make codegen,
  write controller in internal/controller/, register in cmd/main.go, add tests (envtest + e2e),
  create sample, run make docs-gen. All CRDs must be cluster-scoped.
---

## Adding a New CRD

### 1. Define the types

Create `api/v1alpha1/<name>_types.go`:

```go
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type MyCRD struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              MyCRDSpec   `json:"spec,omitempty"`
    Status            MyCRDStatus `json:"status,omitempty"`
}

type MyCRDSpec struct {
    // +kubebuilder:validation:Required
    SomeField string `json:"someField"`
}

type MyCRDStatus struct {
    Phase      string             `json:"phase,omitempty"`
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type MyCRDList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []MyCRD `json:"items"`
}

func init() {
    SchemeBuilder.Register(&MyCRD{}, &MyCRDList{})
}
```

**Rules:**
- Must be cluster-scoped (`+kubebuilder:resource:scope=Cluster`)
- Status must include `[]metav1.Condition`
- Register in `init()` via `SchemeBuilder`

### 2. Generate code

```bash
make codegen
```

This produces:
- `api/v1alpha1/zz_generated.deepcopy.go` (updated)
- `config/crd/bases/puller.corewire.io_mycrds.yaml`
- RBAC roles in `config/rbac/`

### 3. Write the controller

Create `internal/controller/<name>_controller.go`:

```go
package controller

import (
    "context"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    pullerv1alpha1 "github.com/Breee/puller/api/v1alpha1"
)

type MyCRDReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=puller.corewire.io,resources=mycrds,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=puller.corewire.io,resources=mycrds/status,verbs=get;update;patch

func (r *MyCRDReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    var obj pullerv1alpha1.MyCRD
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    log.Info("reconciling", "name", obj.Name)

    // Business logic here

    return ctrl.Result{}, nil
}

func (r *MyCRDReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&pullerv1alpha1.MyCRD{}).
        Complete(r)
}
```

### 4. Register in cmd/main.go

```go
if err = (&controller.MyCRDReconciler{
    Client: mgr.GetClient(),
    Scheme: mgr.GetScheme(),
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "MyCRD")
    os.Exit(1)
}
```

### 5. Add tests

**Unit test** — `internal/controller/<name>_controller_test.go`:
- Use envtest suite
- Create the resource, trigger reconciliation, assert status

**E2E test** — `test/e2e/<name>-basic/chainsaw-test.yaml`:
- Apply resource, assert expected status/children

**Sample** — `config/samples/puller_v1alpha1_<name>.yaml`:
- Minimal valid resource for testing

### 6. Regenerate docs

```bash
make docs-gen
```

This updates `llms.txt`, `AGENTS.md`, `.cursorrules`, `knowledge.yaml`, and the copilot instructions.
