# Implementation Plan

Detailed, step-by-step implementation plan for the drop operator. Each task includes exact commands, files to create/modify, acceptance criteria, and estimated effort. Tasks are ordered by dependency — later tasks depend on earlier ones completing.

---

## Phase 1: Project Bootstrap

### Task 1.1: Initialize Kubebuilder Project

**Goal:** Scaffold Go project with Kubebuilder, establish module and project structure.

**Commands:**
```bash
# Prerequisites: Go 1.22+, Kubebuilder 4.x
kubebuilder init --domain corewire.io --repo github.com/Breee/drop
```

**Files created (by scaffolding):**
- `go.mod` (module `github.com/Breee/drop`)
- `go.sum`
- `Makefile` (Kubebuilder-generated, with controller-gen, envtest, kustomize targets)
- `cmd/main.go` (manager entrypoint with leader election, health probes)
- `config/` (manager, RBAC, CRD kustomize bases)
- `Dockerfile`
- `PROJECT` (Kubebuilder project metadata)
- `.golangci.yml` (add manually — standard strict config)

**Manual additions after scaffold:**
- Add `.golangci.yml` with `gofmt`, `govet`, `errcheck`, `staticcheck`, `unused`, `gosec` linters.
- Add `Taskfile.yml` (go-task) mirroring Make targets for developer preference.
- Add `.editorconfig` for consistent formatting.
- Add `.gitignore` for Go binaries, `bin/`, `testbin/`, `vendor/`, coverage files.

**Acceptance criteria:**
- [ ] `make build` succeeds (empty operator binary compiles).
- [ ] `make test` succeeds (no tests yet, but envtest setup works).
- [ ] `go vet ./...` passes.
- [ ] `golangci-lint run` passes.

---

### Task 1.2: Scaffold CRD APIs

**Goal:** Create the four API types with all spec/status fields.

**Commands:**
```bash
kubebuilder create api --group drop --version v1alpha1 --kind CachedImage --resource --controller
kubebuilder create api --group drop --version v1alpha1 --kind CachedImageSet --resource --controller
kubebuilder create api --group drop --version v1alpha1 --kind PullPolicy --resource --controller=false
kubebuilder create api --group drop --version v1alpha1 --kind DiscoveryPolicy --resource --controller
```

**Files to implement (after scaffold, fill in types):**

#### `api/v1alpha1/cachedimage_types.go`
```go
type CachedImageSpec struct {
    // Image is the fully qualified image reference (without tag/digest).
    Image string `json:"image"`
    // Tag to pull. Mutually exclusive with Digest.
    // +optional
    Tag string `json:"tag,omitempty"`
    // Digest to pull (immutable reference). Mutually exclusive with Tag.
    // +optional
    Digest string `json:"digest,omitempty"`
    // PullPolicy controls whether to pull if image exists on node.
    // +kubebuilder:default=IfNotPresent
    // +kubebuilder:validation:Enum=IfNotPresent;Always
    PullPolicy string `json:"pullPolicy,omitempty"`
    // RepullPolicy controls refresh behavior for cached images.
    // +kubebuilder:default=Never
    // +kubebuilder:validation:Enum=Never;OnSchedule;Always
    RepullPolicy string `json:"repullPolicy,omitempty"`
    // NodeSelector restricts which nodes to cache the image on.
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`
    // Tolerations allow targeting tainted nodes.
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
    // Priority is a pull ordering hint (lower values pulled first).
    // +optional
    Priority *int32 `json:"priority,omitempty"`
    // PolicyRef references a PullPolicy for pacing controls.
    // +optional
    PolicyRef *PolicyReference `json:"policyRef,omitempty"`
}

type CachedImageStatus struct {
    // ObservedGeneration is the last generation reconciled.
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
    // Phase summarizes the overall state.
    // +kubebuilder:validation:Enum=Pending;Pulling;Ready;Degraded
    Phase string `json:"phase,omitempty"`
    // NodesTargeted is the number of nodes that should have this image.
    NodesTargeted int32 `json:"nodesTargeted,omitempty"`
    // NodesReady is the number of nodes that have successfully pulled the image.
    NodesReady int32 `json:"nodesReady,omitempty"`
    // LastPulledAt is the timestamp of the most recent successful pull.
    // +optional
    LastPulledAt *metav1.Time `json:"lastPulledAt,omitempty"`
    // Conditions represent the latest available observations.
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type PolicyReference struct {
    Name string `json:"name"`
}
```

#### `api/v1alpha1/cachedimageset_types.go`
```go
type CachedImageSetSpec struct {
    // PolicyRef references a PullPolicy for pacing controls.
    // +optional
    PolicyRef *PolicyReference `json:"policyRef,omitempty"`
    // DiscoveryPolicyRef references a DiscoveryPolicy for dynamic image lists.
    // +optional
    DiscoveryPolicyRef *DiscoveryPolicyReference `json:"discoveryPolicyRef,omitempty"`
    // NodeSelector restricts which nodes to cache images on (propagated to children).
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`
    // Tolerations allow targeting tainted nodes (propagated to children).
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
    // Images is a static list of images to cache.
    // +optional
    Images []ImageEntry `json:"images,omitempty"`
    // PullPolicy default for child CachedImage resources.
    // +kubebuilder:default=IfNotPresent
    // +kubebuilder:validation:Enum=IfNotPresent;Always
    // +optional
    PullPolicy string `json:"pullPolicy,omitempty"`
    // RepullPolicy default for child CachedImage resources.
    // +kubebuilder:default=Never
    // +kubebuilder:validation:Enum=Never;OnSchedule;Always
    // +optional
    RepullPolicy string `json:"repullPolicy,omitempty"`
}

type ImageEntry struct {
    Image  string `json:"image"`
    Tag    string `json:"tag,omitempty"`
    Digest string `json:"digest,omitempty"`
}

type DiscoveryPolicyReference struct {
    Name string `json:"name"`
}

type CachedImageSetStatus struct {
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
    Phase              string             `json:"phase,omitempty"`
    ImagesManaged      int32              `json:"imagesManaged,omitempty"`
    ImagesReady        int32              `json:"imagesReady,omitempty"`
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
}
```

#### `api/v1alpha1/pullpolicy_types.go`
```go
type PullPolicySpec struct {
    // MaxConcurrentNodes is the max nodes pulling simultaneously for this policy.
    // +kubebuilder:default=1
    // +kubebuilder:validation:Minimum=1
    MaxConcurrentNodes int32 `json:"maxConcurrentNodes,omitempty"`
    // MinDelayBetweenPulls is the minimum time between starting pulls on different nodes.
    // +kubebuilder:default="10s"
    MinDelayBetweenPulls metav1.Duration `json:"minDelayBetweenPulls,omitempty"`
    // FailureBackoff configures retry delays on pull failures.
    // +optional
    FailureBackoff *BackoffConfig `json:"failureBackoff,omitempty"`
    // RepullPolicyDefault is the default repull behavior for images referencing this policy.
    // +kubebuilder:default=Never
    // +kubebuilder:validation:Enum=Never;OnSchedule;Always
    RepullPolicyDefault string `json:"repullPolicyDefault,omitempty"`
    // NodeSelector scopes this policy to a specific node pool.
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`
    // Tolerations match tainted nodes in the pool.
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

type BackoffConfig struct {
    // Initial delay before first retry.
    // +kubebuilder:default="30s"
    Initial metav1.Duration `json:"initial,omitempty"`
    // Max delay cap for exponential backoff.
    // +kubebuilder:default="5m"
    Max metav1.Duration `json:"max,omitempty"`
}

// PullPolicy has no status — it is a configuration-only resource.
type PullPolicyStatus struct{}
```

#### `api/v1alpha1/discoverypolicy_types.go`
```go
type DiscoveryPolicySpec struct {
    // Sources is the list of discovery backends to query.
    Sources []DiscoverySource `json:"sources"`
    // ImageFilter is a regex to filter discovered images.
    // +optional
    ImageFilter string `json:"imageFilter,omitempty"`
    // SyncInterval is how often to re-query sources.
    // +kubebuilder:default="30m"
    SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
    // MaxImages caps the number of discovered images.
    // +kubebuilder:default=50
    // +kubebuilder:validation:Minimum=1
    MaxImages int32 `json:"maxImages,omitempty"`
}

type DiscoverySource struct {
    // Type identifies the backend (prometheus, registry).
    // +kubebuilder:validation:Enum=prometheus;registry
    Type string `json:"type"`
    // Prometheus config (when type=prometheus).
    // +optional
    Prometheus *PrometheusSource `json:"prometheus,omitempty"`
    // Registry config (when type=registry).
    // +optional
    Registry *RegistrySource `json:"registry,omitempty"`
    // SecretRef references a Secret for auth/TLS for this source.
    // +optional
    SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

type PrometheusSource struct {
    Endpoint string `json:"endpoint"`
    Query    string `json:"query"`
}

type RegistrySource struct {
    URL           string   `json:"url"`
    Repositories  []string `json:"repositories"`
    TagFilter     string   `json:"tagFilter,omitempty"`
    TopX          int32    `json:"topX,omitempty"`
    ImageTemplate string   `json:"imageTemplate,omitempty"`
}

type DiscoveryPolicyStatus struct {
    LastSyncTime     *metav1.Time       `json:"lastSyncTime,omitempty"`
    DiscoveredImages []DiscoveredImage  `json:"discoveredImages,omitempty"`
    Conditions       []metav1.Condition `json:"conditions,omitempty"`
}

type DiscoveredImage struct {
    Image  string  `json:"image"`
    Score  float64 `json:"score"`
    Source string  `json:"source"`
}
```

**Post-scaffold steps:**
```bash
make generate   # deepcopy generators
make manifests  # CRD YAML generation
```

**Acceptance criteria:**
- [ ] `make generate` succeeds.
- [ ] `make manifests` produces CRD YAML files in `config/crd/bases/`.
- [ ] `make build` compiles with all types defined.
- [ ] CRD YAMLs contain all fields with correct validation markers.
- [ ] `kubectl apply -f config/crd/bases/` succeeds against a kind cluster.

---

### Task 1.3: Implement Pod Builder

**Goal:** Build drop Pod specs in isolation from controller logic.

**File:** `internal/podbuilder/builder.go`

```go
package podbuilder

// BuildDropPod creates a Pod spec for pulling an image onto a specific node.
func BuildDropPod(ci *v1alpha1.CachedImage, nodeName string, scheme *runtime.Scheme) (*corev1.Pod, error)
```

**Implementation details:**
- Set `pod.Spec.NodeName = nodeName`.
- Set container image to `ci.Spec.Image:ci.Spec.Tag` (or `@ci.Spec.Digest`).
- Set `command: ["true"]`, `restartPolicy: Never`.
- Set `imagePullPolicy` from `ci.Spec.PullPolicy`.
- Copy `tolerations` from `ci.Spec.Tolerations`.
- Set `ownerReference` to the CachedImage (via `controllerutil.SetControllerReference`).
- Set labels: `app.kubernetes.io/managed-by=drop`, `drop.corewire.io/cachedimage=<name>`, `drop.corewire.io/node=<node>`.
- Set `automountServiceAccountToken: false`, `enableServiceLinks: false`, `terminationGracePeriodSeconds: 0`.
- Set resource requests to zero (pull-only Pod).

**File:** `internal/podbuilder/builder_test.go`

**Tests (table-driven):**
- Pod has correct nodeName.
- Pod has correct image ref (tag variant).
- Pod has correct image ref (digest variant).
- Pod has correct imagePullPolicy mapping.
- Pod has ownerReference set.
- Pod has expected labels.
- Pod tolerations match CachedImage tolerations.
- Pod has no resource requests/limits (other than zero).

**Acceptance criteria:**
- [ ] `go test ./internal/podbuilder/...` passes.
- [ ] 100% branch coverage on builder function.

---

### Task 1.4: Implement Pacing Engine

**Goal:** Shared pacing logic that CachedImage reconciler calls before creating Pods.

**File:** `internal/pacing/engine.go`

```go
package pacing

type Engine struct {
    client client.Client
}

type Decision struct {
    Allowed   bool
    RequeueIn time.Duration
}

// CanStartPull checks pacing constraints and returns whether a new pull can start.
func (e *Engine) CanStartPull(ctx context.Context, policy *v1alpha1.PullPolicy, cachedImageName string) (Decision, error)
```

**Implementation details:**
- List Pods with label `app.kubernetes.io/managed-by=drop` that are in Running/Pending phase.
- If policy has `nodeSelector`, filter active Pods to those on matching nodes.
- Count active pulls. If `>= policy.Spec.MaxConcurrentNodes` → deny.
- Find most recent Pod creation timestamp among active pulls for this policy scope.
- If `time.Since(lastCreated) < policy.Spec.MinDelayBetweenPulls` → deny with `RequeueIn` = remaining delay.
- Otherwise → allow.

**File:** `internal/pacing/engine_test.go`

**Tests:**
- Allows when no active pulls exist.
- Denies when maxConcurrentNodes reached, returns correct requeue duration.
- Denies when minDelayBetweenPulls not elapsed, returns remaining duration.
- Allows when exactly at boundary (maxConcurrentNodes - 1 active).
- Handles nil policy (use defaults).
- Scopes correctly when policy has nodeSelector.

**Acceptance criteria:**
- [ ] `go test ./internal/pacing/...` passes.
- [ ] Unit tests cover all decision paths.

---

### Task 1.5: Implement CachedImage Reconciler

**Goal:** Core reconciler that creates drop Pods and tracks node-level completion.

**File:** `internal/controller/cachedimage_controller.go`

**Reconcile loop implementation:**
1. Fetch CachedImage; handle not-found (deleted).
2. List nodes matching `spec.nodeSelector` (via `client.List` with label selector).
3. Filter nodes whose taints are tolerated by `spec.tolerations`.
4. Fetch referenced PullPolicy (or use defaults if none referenced / not found).
5. List owned Pods (label selector `drop.corewire.io/cachedimage=<name>`).
6. Build per-node state map: `{node → podStatus}`.
7. For nodes with Succeeded Pod → mark ready, delete Pod (cleanup).
8. For nodes with Failed Pod → record failure, calculate backoff, delete Pod.
9. For nodes with no Pod and not yet ready → check pacing via `pacing.Engine.CanStartPull()`.
10. If allowed → call `podbuilder.BuildDropPod()` → `client.Create()`.
11. Update `CachedImage.Status` (nodesTargeted, nodesReady, phase, conditions).
12. Return `ctrl.Result{RequeueAfter: ...}` based on pacing needs.

**Controller setup:**
```go
func (r *CachedImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.CachedImage{}).
        Owns(&corev1.Pod{}).
        WithEventFilter(predicate.GenerationChangedPredicate{}).
        Complete(r)
}
```

**File:** `internal/controller/cachedimage_controller_test.go`

**Tests (envtest-based integration):**
- Creating a CachedImage with one matching node → drop Pod created.
- Drop Pod completes → CachedImage status shows nodesReady=1, phase=Ready.
- Drop Pod fails → CachedImage status shows Degraded condition.
- Two nodes match, PullPolicy maxConcurrentNodes=1 → only one Pod at a time.
- NodeSelector filters nodes correctly.
- Deleting CachedImage cleans up Pods.
- Updating CachedImage spec triggers new reconcile.

**Acceptance criteria:**
- [ ] `make test` passes (envtest integration tests).
- [ ] CachedImage reaches Ready phase when all target nodes complete.
- [ ] Pacing is respected (verified by checking Pod creation timing in tests).

---

## Phase 2: Multi-Node Pacing + PullPolicy

### Task 2.1: Complete Pacing Integration

**Goal:** End-to-end verification that PullPolicy controls multi-node rollout speed.

**Tests to add:**
- 5-node cluster, PullPolicy `maxConcurrentNodes: 2` → never more than 2 active drop Pods.
- PullPolicy `minDelayBetweenPulls: 5s` → Pods created at least 5s apart.
- Failure backoff: Pod fails → next retry respects exponential delay.
- PullPolicy update (e.g. increase maxConcurrentNodes) → immediate effect on next reconcile.

**Acceptance criteria:**
- [ ] Integration tests pass with timing assertions.
- [ ] No race conditions under `MaxConcurrentReconciles > 1`.

---

### Task 2.2: RepullPolicy (Moving Tags)

**Goal:** Support refreshing images on schedule for moving tags like `latest`.

**Implementation in CachedImage reconciler:**
- After a node is marked Ready, check `repullPolicy`:
  - `Never` → do nothing until spec changes.
  - `OnSchedule` → on next reconcile after syncInterval, create new drop Pod with `imagePullPolicy: Always`.
  - `Always` → every reconcile cycle, re-pull (only for specific use cases).
- Track `lastPulledAt` per node in status to determine if refresh is due.

**Acceptance criteria:**
- [ ] `OnSchedule` triggers re-pull after interval.
- [ ] `Never` does not re-pull.
- [ ] `Always` + `imagePullPolicy: Always` forces registry check on each cycle.

---

## Phase 3: CachedImageSet

### Task 3.1: Implement CachedImageSet Reconciler

**File:** `internal/controller/cachedimageset_controller.go`

**Reconcile loop:**
1. Fetch CachedImageSet CR.
2. Build desired image list from `spec.images` (static).
3. List existing child CachedImage resources (ownerReference match).
4. Diff: create new, delete removed, update changed.
5. For each child CachedImage, propagate: `policyRef`, `nodeSelector`, `tolerations`, `pullPolicy`, `repullPolicy`.
6. Set ownerReference on each child → parent CachedImageSet.
7. Update status: imagesManaged, imagesReady (count children with phase=Ready).

**Controller setup:**
```go
func (r *CachedImageSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.CachedImageSet{}).
        Owns(&v1alpha1.CachedImage{}).
        WithEventFilter(predicate.GenerationChangedPredicate{}).
        Complete(r)
}
```

**Tests:**
- CachedImageSet with 3 static images → 3 CachedImage children created.
- Remove one image from set → child CachedImage deleted.
- Delete CachedImageSet → all children garbage collected (ownerRef cascade).
- Config propagation: change nodeSelector on set → children updated.

**Acceptance criteria:**
- [ ] Static image list CRUD works correctly.
- [ ] OwnerReference cascade deletion works.
- [ ] Status aggregation reflects child states.

---

## Phase 4: DiscoveryPolicy (Prometheus)

### Task 4.1: Implement Source Interface + Prometheus Source

**File:** `internal/discovery/source.go`
```go
package discovery

type Source interface {
    Fetch(ctx context.Context) ([]ImageResult, error)
}

type ImageResult struct {
    Image string
    Score float64
}
```

**File:** `internal/discovery/prometheus.go`

**Implementation:**
- Build HTTP client with auth from Secret (basic auth or bearer token).
- Execute `GET /api/v1/query` with `query` parameter.
- Parse standard Prometheus JSON response.
- Extract `image` label from each result → `ImageResult.Image`.
- Extract metric value → `ImageResult.Score`.
- Return sorted results.

**Tests (unit with httptest):**
- Valid Prometheus response → correct ImageResult list.
- Missing `image` label → skip result, don't error.
- Auth headers applied from Secret data.
- HTTP error → return error (caller handles gracefully).
- Timeout respected.

**Acceptance criteria:**
- [ ] `go test ./internal/discovery/...` passes.
- [ ] Prometheus source handles real response format correctly.

---

### Task 4.2: Implement DiscoveryPolicy Reconciler

**File:** `internal/controller/discoverypolicy_controller.go`

**Reconcile loop:**
1. Fetch DiscoveryPolicy CR.
2. For each source in `spec.sources`:
   a. Resolve Secret (if secretRef set).
   b. Construct appropriate `Source` implementation.
   c. Call `source.Fetch(ctx)`.
   d. On error: set condition `SourceHealthy=False`, keep previous status, continue.
3. Merge all results (deduplicate by image, keep highest score).
4. Apply `imageFilter` regex.
5. Sort by score descending, truncate to `maxImages`.
6. Write `status.discoveredImages`.
7. Set conditions (`Ready`, `SourceHealthy`).
8. Return `ctrl.Result{RequeueAfter: syncInterval}`.

**Tests:**
- Single Prometheus source → discovered images appear in status.
- Source failure → condition set, previous results preserved.
- imageFilter excludes non-matching images.
- maxImages truncation works.
- syncInterval causes periodic requeue.

**Acceptance criteria:**
- [ ] Discovery results appear in status.
- [ ] Transient failure preserves last good results.
- [ ] Conditions reflect source health.

---

### Task 4.3: Connect CachedImageSet to DiscoveryPolicy

**Modification:** `internal/controller/cachedimageset_controller.go`

**Changes:**
- If `spec.discoveryPolicyRef` is set, read `DiscoveryPolicy.status.discoveredImages`.
- Convert discovered images to desired CachedImage list.
- Merge with static `spec.images` (static wins on conflict).
- Add watch: `Watches(&v1alpha1.DiscoveryPolicy{}, handler.EnqueueRequestsFromMapFunc(mapDiscoveryToSets))`.

**The map function:**
```go
func mapDiscoveryToSets(ctx context.Context, obj client.Object) []reconcile.Request {
    // List all CachedImageSets that reference this DiscoveryPolicy
    // Return reconcile.Request for each
}
```

**Tests:**
- DiscoveryPolicy updates status → CachedImageSet reconciles → children updated.
- Image drops from discovery → child CachedImage deleted.
- New image discovered → child CachedImage created.

**Acceptance criteria:**
- [ ] End-to-end: DiscoveryPolicy discovers images → CachedImageSet materializes children → CachedImage pulls onto nodes.
- [ ] GC works when images leave discovery results.

---

## Phase 5: Registry Source

### Task 5.1: Implement Registry Source

**File:** `internal/discovery/registry.go`

**Implementation:**
- HTTP client with auth from Secret (bearer token or basic auth).
- `GET /v2/<repository>/tags/list` (OCI Distribution API).
- Parse tag list response.
- Apply `tagFilter` regex.
- Sort by semver (if parseable) or lexicographic.
- Take top X.
- Apply `imageTemplate` (Go `text/template`) to construct full image refs.
- Return `[]ImageResult` (score = index-based ranking for recency).

**Tests:**
- Valid tag list → correct image refs constructed.
- tagFilter excludes non-matching tags.
- imageTemplate produces expected refs (GitLab helper pattern).
- Semver sorting works correctly.
- Auth headers applied.
- Pagination handling (if registry returns `Link` header).

**Acceptance criteria:**
- [ ] `go test ./internal/discovery/...` passes.
- [ ] GitLab helper image pattern works with `imageTemplate`.

---

## Phase 6: Production Readiness

### Task 6.1: Helm Chart

**Directory:** `charts/drop/`

**Structure:**
```
charts/drop/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── deployment.yaml
│   ├── serviceaccount.yaml
│   ├── clusterrole.yaml
│   ├── clusterrolebinding.yaml
│   ├── _helpers.tpl
│   └── NOTES.txt
└── crds/
    └── (symlinked or copied from config/crd/bases/)
```

**values.yaml key settings:**
- `image.repository`, `image.tag`
- `replicaCount: 1` (leader election handles HA)
- `resources` (sensible defaults for controller)
- `leaderElection.enabled: true`
- `metrics.enabled: true`
- `serviceMonitor.enabled: false` (opt-in)

**Acceptance criteria:**
- [ ] `helm lint charts/drop` passes.
- [ ] `helm template drop charts/drop` produces valid YAML.
- [ ] `helm install` on kind cluster deploys working operator.

---

### Task 6.2: CI Pipeline (GitHub Actions)

**File:** `.github/workflows/ci.yml`

**Jobs:**
1. **lint** — `golangci-lint run`
2. **test** — `make test` (unit + envtest)
3. **build** — `make build` (compile binary)
4. **e2e** — Create kind cluster → install CRDs → run Kyverno Chainsaw tests
5. **docker** — Build multi-arch image (`linux/amd64`, `linux/arm64`) via `docker buildx`

**File:** `.github/workflows/release.yml`

**Trigger:** on tag `v*`

**Jobs:**
1. Run CI pipeline (lint, test, build, e2e).
2. Build + push multi-arch image to `ghcr.io/breee/drop:<tag>`.
3. Package Helm chart → push to GHCR OCI registry.
4. Create GitHub Release with changelog (generated from conventional commits via `git-cliff` or similar).

**Acceptance criteria:**
- [ ] CI passes on PRs.
- [ ] Release produces multi-arch image on GHCR.
- [ ] Helm chart is pullable from GHCR OCI.

---

### Task 6.3: E2E Tests (Kyverno Chainsaw)

**Directory:** `test/e2e/`

**Scenario files (Chainsaw YAML):**

1. `test/e2e/static-pull/chainsaw-test.yaml` — Create CachedImage → verify drop Pod created → verify status Ready.
2. `test/e2e/pull-policy/chainsaw-test.yaml` — Create PullPolicy + 2 CachedImages → verify sequential pulls.
3. `test/e2e/image-set/chainsaw-test.yaml` — Create CachedImageSet with static images → verify children created.
4. `test/e2e/discovery/chainsaw-test.yaml` — Create DiscoveryPolicy (mock Prometheus) → verify discovered images in status.
5. `test/e2e/cleanup/chainsaw-test.yaml` — Delete CachedImageSet → verify children and Pods cleaned up.

**Acceptance criteria:**
- [ ] All Chainsaw scenarios pass against kind cluster.
- [ ] Tests complete within 5 minutes.

---

### Task 6.4: Dockerfile (Multi-Arch)

**File:** `Dockerfile`

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.22 AS builder
ARG TARGETOS TARGETARCH
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o manager cmd/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
```

**Acceptance criteria:**
- [ ] Builds for `linux/amd64` and `linux/arm64`.
- [ ] Final image is < 50MB.
- [ ] Runs as non-root.

---

### Task 6.5: Documentation (Hugo Hextra)

**Directory:** `docs/`

**Pages:**
- `docs/content/_index.md` — Landing page.
- `docs/content/getting-started.md` — Quickstart with Helm.
- `docs/content/crds/cachedimage.md` — CRD reference.
- `docs/content/crds/cachedimageset.md` — CRD reference.
- `docs/content/crds/pullpolicy.md` — CRD reference.
- `docs/content/crds/discoverypolicy.md` — CRD reference.
- `docs/content/guides/static-images.md` — How to cache specific images.
- `docs/content/guides/discovery.md` — How to set up Prometheus discovery.
- `docs/content/architecture.md` — High-level architecture for users.

**Acceptance criteria:**
- [ ] `hugo serve` renders docs locally.
- [ ] CRD reference docs generated/synced from code comments.

---

## Dependency Graph

```
Task 1.1 (bootstrap)
  └─► Task 1.2 (CRD APIs)
       ├─► Task 1.3 (Pod builder)
       ├─► Task 1.4 (Pacing engine)
       └─► Task 1.5 (CachedImage reconciler) ◄── depends on 1.3 + 1.4
            └─► Task 2.1 (Pacing integration tests)
            └─► Task 2.2 (RepullPolicy)
                 └─► Task 3.1 (CachedImageSet reconciler)
                      └─► Task 4.1 (Source interface + Prometheus)
                           └─► Task 4.2 (DiscoveryPolicy reconciler)
                                └─► Task 4.3 (Connect Set ↔ Discovery)
                                     └─► Task 5.1 (Registry source)

Task 6.1 (Helm) ◄── depends on Task 1.5+ (needs working operator)
Task 6.2 (CI)   ◄── depends on Task 1.1 (needs compilable project)
Task 6.3 (E2E)  ◄── depends on Task 1.5+ (needs reconciler)
Task 6.4 (Dockerfile) ◄── depends on Task 1.1
Task 6.5 (Docs) ◄── can start anytime, references CRD types
```

---

## Effort Estimates

| Task | Effort | Complexity |
|------|--------|------------|
| 1.1 Bootstrap | Small | Low — scaffolding |
| 1.2 CRD APIs | Medium | Low — type definitions |
| 1.3 Pod builder | Small | Low — single function |
| 1.4 Pacing engine | Medium | Medium — timing logic |
| 1.5 CachedImage reconciler | Large | High — core reconciler |
| 2.1 Pacing integration | Medium | Medium — timing tests |
| 2.2 RepullPolicy | Small | Low — add condition |
| 3.1 CachedImageSet | Medium | Medium — child management |
| 4.1 Prometheus source | Medium | Medium — HTTP + parsing |
| 4.2 DiscoveryPolicy reconciler | Medium | Medium — multi-source |
| 4.3 Connect Set ↔ Discovery | Small | Low — wire existing |
| 5.1 Registry source | Medium | Medium — OCI API |
| 6.1 Helm | Small | Low — templating |
| 6.2 CI | Medium | Low — standard GHA |
| 6.3 E2E | Medium | Medium — scenario design |
| 6.4 Dockerfile | Small | Low — standard |
| 6.5 Docs | Medium | Low — content creation |

---

## Quality Gates (Per Task)

Every task must pass before moving to the next:

1. **Compiles** — `make build` succeeds.
2. **Lints** — `golangci-lint run` passes.
3. **Unit tests** — `make test` passes with new tests.
4. **No regressions** — all existing tests still pass.
5. **CRD validation** — `make manifests` produces valid CRDs.

For Phase 6 tasks additionally:
6. **E2E** — Chainsaw scenarios pass on kind.
7. **Helm** — `helm lint` + `helm template` pass.
8. **Image** — `docker build` succeeds for both architectures.

---

## Review Checklist

This plan meets the project's standards:

- ✅ **Simple architecture** — three reconcilers, each doing one thing. No webhooks, no custom schedulers, no abstraction layers beyond what's needed.
- ✅ **No premature optimization** — pacing uses Pod listing (informer-cached), no external databases or caches. Adds complexity only when proven necessary.
- ✅ **Go best practices** — interfaces for extensibility, table-driven tests, dependency injection, standard project layout, no globals.
- ✅ **Kubernetes operator best practices** — idempotent reconciliation, ownerRefs for GC, status subresource, leader election, least-privilege RBAC, event predicates.
- ✅ **Testable** — every component testable in isolation (pod builder, pacing, sources) and integrated (envtest, Chainsaw).
- ✅ **Incrementally shippable** — Phase 1 alone is useful (static image caching). Each phase adds value independently.
- ✅ **No guesses** — pull mechanism (nodeName Pod), pacing (informer-based counting), discovery (Source interface) are all patterns used by production Kubernetes operators (kube-fledged, eraser, etc.).
