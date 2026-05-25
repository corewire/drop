# Architecture Plan

## Overview

The **drop** operator caches container images onto Kubernetes nodes declaratively. It replaces manual DaemonSet/script-based pre-pulling with a controller-driven reconciliation loop that is safe, paced, and observable.

**Design principles:**
- Simple over clever — no over-abstraction, no premature optimization.
- Follow Go and Kubernetes operator best practices (Kubebuilder conventions, idempotent reconciliation, status subresource, owner references).
- Single-concern resources — each CRD does one thing well.
- Declarative intent — users declare *what* to cache; operator handles *how*.

---

## System Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  Kubernetes API Server                                                        │
│                                                                              │
│  CRDs (drop.corewire.io/v1alpha1, all cluster-scoped):                     │
│  ┌──────────────┐  ┌────────────────┐  ┌────────────┐  ┌─────────────────┐  │
│  │ CachedImage  │  │ CachedImageSet │  │ PullPolicy │  │ DiscoveryPolicy │  │
│  └──────────────┘  └────────────────┘  └────────────┘  └─────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
        ▲                    ▲                                      │
        │ owns               │ reads status                        │
        │ (ownerRef)         │                                     ▼
┌───────┴────────────────────┴─────────────────────────────────────────────────┐
│  drop-controller-manager (single Deployment, leader-elected)                │
│                                                                              │
│  ┌─────────────────────┐  ┌─────────────────────────┐  ┌──────────────────┐ │
│  │ CachedImage         │  │ CachedImageSet          │  │ DiscoveryPolicy  │ │
│  │ Reconciler          │  │ Reconciler              │  │ Reconciler       │ │
│  │                     │  │                         │  │                  │ │
│  │ • create drop Pod │  │ • diff spec vs children │  │ • query sources  │ │
│  │ • track completion  │  │ • create/delete children│  │ • write status   │ │
│  │ • update status     │  │ • propagate defaults    │  │ • requeue        │ │
│  └─────────────────────┘  └─────────────────────────┘  └──────────────────┘ │
│                                                                              │
│  Shared components:                                                          │
│  • PullPolicy cache (in-memory read of PullPolicy resources)                 │
│  • Rate limiter / pacing engine (enforces maxConcurrentNodes + delays)        │
│  • Metrics exporter (Prometheus /metrics endpoint)                            │
└──────────────────────────────────────────────────────────────────────────────┘
        │
        │ creates Pods (drop jobs)
        ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│  Kubernetes Nodes                                                             │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────┐        │
│  │ Drop Pod (short-lived, one per image×node)                      │        │
│  │   spec:                                                           │        │
│  │     nodeName: <target-node>                                       │        │
│  │     containers:                                                   │        │
│  │       - name: pull                                                │        │
│  │         image: <target-image>                                     │        │
│  │         command: ["true"]   # exits immediately after pull        │        │
│  │     restartPolicy: Never                                          │        │
│  └──────────────────────────────────────────────────────────────────┘        │
│                                                                              │
│  containerd/CRI pulls the image layers (parallel layer downloads built-in)   │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Pull Mechanism

### Approach: Short-lived Pods with `nodeName`

The operator creates a short-lived Pod per (image, node) pair. The Pod's container uses the target image with `command: ["true"]` and `restartPolicy: Never`. The kubelet pulls the image onto the node as part of normal Pod scheduling, then the container exits immediately.

**Why this approach (not DaemonSet, not crictl):**

| Approach | Pros | Cons |
|----------|------|------|
| DaemonSet with initContainers | Simple, native k8s | Hard to manage lifecycle, can't target individual nodes easily, restarts on change |
| Job per node with `crictl` | Direct CRI control | Requires privileged access, mounts runtime socket, security concern |
| **Pod with `nodeName` + `command: ["true"]`** | No privilege needed, uses standard kubelet image pull, easy cleanup, per-node targeting | Slightly more Pods to manage |

The chosen approach:
- **No elevated privileges** — works with standard RBAC.
- **Uses native kubelet image pull** — respects node-level pull secrets, mirrors, and runtime configuration.
- **Simple lifecycle** — Pod completes → operator observes `.status.phase == Succeeded` → marks node as ready in `CachedImage` status.
- **Easy cleanup** — completed Pods are deleted by the operator after status is recorded.
- **Per-node control** — `nodeName` field pins the Pod to a specific node; operator controls which nodes get Pods and when.

### Pod Spec (template)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: drop-<cachedimage-name>-<node-short>
  labels:
    app.kubernetes.io/managed-by: drop
    drop.corewire.io/cachedimage: <cachedimage-name>
    drop.corewire.io/node: <node-name>
  ownerReferences:
    - apiVersion: drop.corewire.io/v1alpha1
      kind: CachedImage
      name: <cachedimage-name>
      uid: <cachedimage-uid>
      controller: true
spec:
  nodeName: <target-node>
  containers:
    - name: pull
      image: <spec.image>:<spec.tag or @spec.digest>
      command: ["true"]
      resources:
        requests:
          cpu: "0"
          memory: "0"
  restartPolicy: Never
  terminationGracePeriodSeconds: 0
  automountServiceAccountToken: false
  enableServiceLinks: false
  tolerations: <from CachedImage spec>
```

### imagePullPolicy on the Pod

- When `CachedImage.spec.pullPolicy: IfNotPresent` → Pod container `imagePullPolicy: IfNotPresent` (skip if already on node).
- When `CachedImage.spec.pullPolicy: Always` → Pod container `imagePullPolicy: Always` (always check registry).

---

## Reconcilers

### CachedImage Reconciler

**Watches:** `CachedImage`, owned `Pod` resources.

**Reconcile loop (idempotent):**

```
1. Fetch CachedImage CR
2. If being deleted → clean up any active drop Pods → remove finalizer → done
3. Resolve target nodes:
   a. List nodes matching CachedImage.spec.nodeSelector
   b. Filter by tolerations (node must have matching taints)
   c. Result: set of target node names
4. Resolve PullPolicy (from spec.policyRef, or use built-in defaults)
5. For each target node:
   a. Check if drop Pod already exists (label selector)
   b. If Pod exists and Succeeded → record node as ready in status
   c. If Pod exists and Failed → record failure, apply backoff
   d. If Pod does not exist and node not yet ready:
      - Check pacing constraints (maxConcurrentNodes, minDelayBetweenPulls)
      - If within budget → create drop Pod
      - If over budget → skip, requeue
6. Update CachedImage.status:
   - nodesTargeted, nodesReady, phase, conditions, lastPulledAt
7. Clean up completed/failed Pods (after recording status)
8. If all nodes ready → set phase=Ready, done
9. If work remaining → requeue (with delay based on pacing)
```

**Key design points:**
- Idempotent: calling Reconcile multiple times produces the same result.
- Rate limiting is per-CachedImage and global (via PullPolicy pacing check).
- The reconciler does NOT watch all Pods in the cluster — only Pods it owns (via `.Owns(&corev1.Pod{})`).
- Uses `GenerationChangedPredicate` to avoid reconciling on status-only updates.

### CachedImageSet Reconciler

**Watches:** `CachedImageSet`, owned `CachedImage` resources, referenced `DiscoveryPolicy` (via watch with handler).

**Reconcile loop:**

```
1. Fetch CachedImageSet CR
2. Determine desired image list:
   a. If spec.images set → use static list
   b. If spec.discoveryPolicyRef set → read DiscoveryPolicy.status.discoveredImages
   c. Merge (static takes precedence for same image ref)
3. List existing child CachedImage resources (ownerReference filter)
4. Diff desired vs existing:
   a. New images → create CachedImage with ownerRef pointing to this set
   b. Removed images → delete child CachedImage (GC via ownerRef also works)
   c. Changed images → update child CachedImage spec
5. Propagate shared config to children:
   - policyRef, nodeSelector, tolerations, pullPolicy, repullPolicy
6. Update CachedImageSet.status:
   - imagesManaged, imagesReady (aggregate from children), phase, conditions
```

**Key design points:**
- Child `CachedImage` resources have `ownerReferences` → Kubernetes GC handles cleanup if the set is deleted.
- The reconciler watches `DiscoveryPolicy` changes via an explicit watch with `handler.EnqueueRequestsFromMapFunc` to trigger reconciliation when discovery results change.

### DiscoveryPolicy Reconciler

**Watches:** `DiscoveryPolicy`, referenced `Secret` resources (for auth credential rotation).

**Reconcile loop:**

```
1. Fetch DiscoveryPolicy CR
2. For each source in spec.sources:
   a. Build HTTP client:
      - Read secretRef → populate auth headers/TLS config
      - Set timeout (default 30s)
   b. Execute source-specific query:
      - Prometheus: GET /api/v1/query with query string
      - Registry: GET /v2/<repo>/tags/list
   c. Parse response into []ImageResult{Image, Score}
   d. On failure: log error, set condition, keep previous results, continue
3. Merge results from all sources (deduplicate by image ref, keep highest score)
4. Apply imageFilter regex (exclude non-matching)
5. Sort by score descending, truncate to maxImages
6. Write to status.discoveredImages
7. Update conditions (Ready, SourceHealthy)
8. Requeue after syncInterval
```

**Key design points:**
- On transient failures, preserve last known good results (no cache thrashing).
- Each source is independent — one failing source doesn't block others.
- The reconciler is purely a data producer; it does NOT create CachedImage resources directly. That responsibility belongs to `CachedImageSet`.

---

## Pacing Engine

The pacing engine is NOT a separate controller. It is shared logic called by the `CachedImage` reconciler before creating a drop Pod.

```go
// PacingDecision determines if a new pull can be started right now.
type PacingDecision struct {
    Allowed    bool
    RequeueIn time.Duration // if not allowed, when to retry
}

func (p *PacingEngine) CanPull(ctx context.Context, policy *v1alpha1.PullPolicy) PacingDecision {
    // 1. Count currently active drop Pods matching this policy's scope
    // 2. If active >= policy.Spec.MaxConcurrentNodes → deny, requeue
    // 3. Check time since last pull start for this policy
    // 4. If elapsed < policy.Spec.MinDelayBetweenPulls → deny, requeue with remaining delay
    // 5. Allow
}
```

**Implementation:** Query active Pods via label selectors (cached by informer). No external state store needed — all state is derived from the cluster.

**Defaults (when no PullPolicy is referenced):**
- `maxConcurrentNodes: 1` — sequential, safest default.
- `minDelayBetweenPulls: 10s` — gentle pacing.
- `failureBackoff: initial=30s, max=5m` — exponential with cap.

---

## Resource Relationships

```
PullPolicy ◄──── policyRef ─────── CachedImage
                                       ▲
                                       │ ownerRef
PullPolicy ◄──── policyRef ─────── CachedImageSet ──── discoveryPolicyRef ───► DiscoveryPolicy
                                       │
                                       │ creates (ownerRef)
                                       ▼
                                   CachedImage (child)
```

- `PullPolicy` is referenced but never owns or is owned.
- `DiscoveryPolicy` is referenced by `CachedImageSet`; never owns or is owned.
- `CachedImageSet` owns child `CachedImage` resources.
- `CachedImage` owns drop `Pod` resources.

---

## Project Structure (Go)

Following standard Kubebuilder layout:

```
drop/
├── api/
│   └── v1alpha1/
│       ├── cachedimage_types.go
│       ├── cachedimageset_types.go
│       ├── pullpolicy_types.go
│       ├── discoverypolicy_types.go
│       ├── groupversion_info.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── main.go                    # manager entrypoint
├── internal/
│   ├── controller/
│   │   ├── cachedimage_controller.go
│   │   ├── cachedimageset_controller.go
│   │   └── discoverypolicy_controller.go
│   ├── pacing/
│   │   └── engine.go              # pacing logic (shared)
│   ├── discovery/
│   │   ├── source.go              # Source interface
│   │   ├── prometheus.go          # Prometheus source implementation
│   │   └── registry.go           # Registry source implementation
│   └── podbuilder/
│       └── builder.go             # constructs drop Pod specs
├── config/
│   ├── crd/                       # generated CRD manifests
│   ├── rbac/                      # generated RBAC
│   ├── manager/                   # manager Deployment
│   └── samples/                   # example CRs
├── charts/
│   └── drop/                    # Helm chart
├── test/
│   └── e2e/                       # Kyverno Chainsaw test scenarios
├── docs/                          # Hugo Hextra source
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

---

## Key Interfaces

### Source Interface (Discovery)

```go
// Source is the interface every discovery backend implements.
type Source interface {
    // Fetch queries the backend and returns discovered images.
    Fetch(ctx context.Context) ([]ImageResult, error)
}

type ImageResult struct {
    Image string
    Score float64
}
```

Each source type (`prometheus`, `registry`) implements this interface. Adding a new source = one new file implementing `Source`. No other changes needed.

### Pod Builder

```go
// BuildDropPod creates a Pod spec for pulling an image onto a specific node.
func BuildDropPod(ci *v1alpha1.CachedImage, nodeName string) *corev1.Pod
```

Single function, tested in isolation. No abstraction layers.

---

## Controller Registration

```go
func main() {
    mgr, _ := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        LeaderElection:   true,
        LeaderElectionID: "drop.corewire.io",
        // ...
    })

    // CachedImage controller - owns Pods
    ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.CachedImage{}).
        Owns(&corev1.Pod{}).
        WithEventFilter(predicate.GenerationChangedPredicate{}).
        Complete(&controller.CachedImageReconciler{})

    // CachedImageSet controller - owns CachedImages, watches DiscoveryPolicy
    ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.CachedImageSet{}).
        Owns(&v1alpha1.CachedImage{}).
        Watches(&v1alpha1.DiscoveryPolicy{}, handler.EnqueueRequestsFromMapFunc(mapDiscoveryToSets)).
        WithEventFilter(predicate.GenerationChangedPredicate{}).
        Complete(&controller.CachedImageSetReconciler{})

    // DiscoveryPolicy controller
    ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.DiscoveryPolicy{}).
        Complete(&controller.DiscoveryPolicyReconciler{})

    mgr.Start(ctrl.SetupSignalHandler())
}
```

---

## RBAC (Least Privilege)

```yaml
# Core operations
- apiGroups: ["drop.corewire.io"]
  resources: ["cachedimages", "cachedimagesets", "pullpolicies", "discoverypolicies"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["drop.corewire.io"]
  resources: ["cachedimages/status", "cachedimagesets/status", "discoverypolicies/status"]
  verbs: ["get", "update", "patch"]

# Drop Pods
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch", "create", "delete"]

# Node listing (read-only)
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch"]

# Secrets for discovery auth (read-only)
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]

# Events
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]

# Leader election
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

---

## Status Conditions (standard k8s convention)

All status types use `metav1.Condition` for consistency:

**CachedImage conditions:**
- `Ready` — all target nodes have the image cached.
- `Progressing` — pulls are in progress.
- `Degraded` — some nodes have failed pulls (with message).

**CachedImageSet conditions:**
- `Ready` — all child CachedImages are ready.
- `Progressing` — children are being created/reconciled.

**DiscoveryPolicy conditions:**
- `Ready` — last sync was successful.
- `SourceHealthy` — all configured sources are responding.

---

## Observability

**Prometheus metrics (exposed on /metrics):**

| Metric | Type | Description |
|--------|------|-------------|
| `drop_cachedimage_nodes_ready` | Gauge | Nodes with image cached per CachedImage |
| `drop_cachedimage_nodes_targeted` | Gauge | Target nodes per CachedImage |
| `drop_pull_duration_seconds` | Histogram | Time to pull an image onto a node |
| `drop_pull_failures_total` | Counter | Failed pull attempts |
| `drop_discovery_sync_duration_seconds` | Histogram | Discovery query duration |
| `drop_discovery_images_found` | Gauge | Number of images discovered per DiscoveryPolicy |
| `drop_active_pulls` | Gauge | Currently active drop Pods |

**Kubernetes Events:**
- `PullSucceeded` — image successfully cached on node.
- `PullFailed` — image pull failed (with error message).
- `DiscoverySyncFailed` — discovery source query failed.
- `PolicyViolation` — pull rate exceeded (informational).

---

## Error Handling and Resilience

| Scenario | Behavior |
|----------|----------|
| Drop Pod fails | Record failure in CachedImage status, apply exponential backoff from PullPolicy, retry |
| Node removed from cluster | CachedImage status updated on next reconcile (node drops from targeted set) |
| Node added to cluster | Reconciler picks up new node on next cycle, creates drop Pod if within pacing budget |
| Discovery source down | Keep last known good results, set SourceHealthy=False condition, retry on next syncInterval |
| PullPolicy deleted while referenced | CachedImage reconciler falls back to built-in defaults, emits warning event |
| CachedImageSet deleted | Kubernetes GC cascades deletion to child CachedImage resources (ownerRef) |
| Controller restart | Reconcilers rebuild state from existing CRs and Pods — no external state store needed |

---

## Constraints and Non-Goals

**Constraints:**
- All resources are cluster-scoped (nodes are cluster-scoped).
- Pulls must never affect node schedulability (non-disruptive guarantee).
- No CRI socket mounting, no privileged containers.
- Single binary, single Deployment, leader-elected.

**Non-goals (explicitly out of scope):**
- Image garbage collection / cleanup (use Eraser or kubelet GC for that).
- Registry mirroring / caching proxy (use Spegel or registry mirrors).
- Pod scheduling decisions (this operator only pre-caches; it does not influence the scheduler).
- Multi-cluster support (single-cluster operator; run one instance per cluster).

---

## Implementation Phases

| Phase | Scope | Outcome |
|-------|-------|---------|
| 1 | Project bootstrap + CRDs + `CachedImage` reconciler (static, single node) | Can declare an image and have it pulled onto a specific node |
| 2 | Multi-node targeting + `PullPolicy` pacing | Safe, throttled pulls across multiple nodes |
| 3 | `CachedImageSet` with static image lists | Group images, shared config, ownerRef GC |
| 4 | `DiscoveryPolicy` with Prometheus source | Auto-discover top images from metrics |
| 5 | Registry source + imageTemplate | Discover images from OCI registries |
| 6 | Helm chart, CI/CD, multi-arch images, docs | Production-ready distribution |

Each phase is independently useful and deployable. No phase depends on later phases.

---

## Validation Summary

**Does this architecture follow Go best practices?**
- ✅ Standard project layout (Kubebuilder conventions).
- ✅ Interfaces for extensibility (`Source` interface).
- ✅ No globals — dependency injection via reconciler struct fields.
- ✅ Table-driven tests for Pod building, pacing logic.
- ✅ Packages grouped by domain responsibility, not by layer.

**Does this follow Kubernetes operator best practices?**
- ✅ Idempotent reconciliation — safe to call multiple times.
- ✅ Status subresource for observed state.
- ✅ OwnerReferences for garbage collection.
- ✅ Leader election for single-writer safety.
- ✅ Event predicates to avoid unnecessary reconciliations.
- ✅ Least-privilege RBAC.
- ✅ Standard conditions pattern (`metav1.Condition`).
- ✅ Finalizers only where external cleanup is needed (none needed here — all resources are k8s-native).
- ✅ No watch on all Pods — only owned Pods via `.Owns()`.

**Is it simple?**
- ✅ Three reconcilers, each with a single clear responsibility.
- ✅ No custom schedulers, no webhooks (for v1), no conversion webhooks.
- ✅ Pacing is shared utility code, not a separate controller.
- ✅ Discovery sources implement one interface with one method.
- ✅ Pull mechanism is a standard Pod — no DaemonSet lifecycle complexity.

**Is it powerful?**
- ✅ Handles static and dynamic image lists.
- ✅ Extensible discovery (any backend that implements `Source`).
- ✅ Per-pool pacing via nodeSelector on PullPolicy.
- ✅ Automatic cleanup via ownerReferences.
- ✅ Observable via Prometheus metrics and k8s events.
