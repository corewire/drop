package main

// ─── llms.txt (USE agents — short onboarding) ───────────────────────────────

var llmsTxtTmpl = `# {{.Project.Name}} — {{.Project.Description}}

> API group: {{.Project.APIGroup}} | Go {{.Project.GoVersion}} | All CRDs cluster-scoped

## CRDs

| Kind | Purpose |
|------|---------|
{{- range .CRDs}}
| {{.Kind}} | {{.Doc}} |
{{- end}}

## Architecture

Short-lived Pods with ` + "`nodeName`" + ` + ` + "`command: [\"true\"]`" + ` trigger image pulls via kubelet. No privileged containers.

Reconcilers:
{{- range .CRDs}}{{if .Controller}}
- {{.Kind}} → {{.Controller}}
{{- end}}{{end}}

## Key Directories

| Path | Role |
|------|------|
{{- range .Packages}}
| {{.Path}} | {{.Role}} |
{{- end}}
| charts/drop/ | Helm chart |
| test/e2e/ | Chainsaw E2E tests |
| hack/gen-ai-docs/ | Documentation generator |

## Build & Test

` + "```" + `
{{- range .MakeTargets}}
  make {{.Name}}{{"\t"}}# {{.Desc}}
{{- end}}
` + "```" + `

## CRD Quick Reference
{{range .CRDs}}
### {{.Kind}}

{{.Doc}}

**Spec fields:** {{range .SpecFields}}` + "`{{.JSON}}`" + `{{if .Default}} (default: {{.Default}}){{end}}, {{end}}
{{- if .StatusFields}}
**Status fields:** {{range .StatusFields}}` + "`{{.JSON}}`" + `, {{end}}
{{- end}}
{{end}}

## Status Condition Reasons

| Reason | Controller | Meaning |
|--------|-----------|---------|
{{- range .Errors}}
| {{.Reason}} | {{.Controller}} | {{.Meaning}} |
{{- end}}

## Metrics

{{- range .Metrics}}
- ` + "`{{.Name}}`" + ` ({{.Type}}) — {{.Help}}
{{- end}}

## Full Reference

See [llms-full.txt](llms-full.txt) for complete field documentation with types and examples.

## Documentation Pages

| Page | llmsDescription |
|------|-----------------|
| [Installation](docs/install/) | Install via Helm. Requires K8s 1.28+. |
| [Usage](docs/usage/) | CachedImage, CachedImageSet, PullPolicy examples with YAML. |
| [Discovery](docs/discovery/) | DiscoveryPolicy for automatic image discovery from Prometheus/OCI registries. |
| [Monitoring](docs/monitoring/) | Prometheus metrics, Kubernetes events, and status conditions. |
| [CRD Reference](docs/reference/crds/) | Complete field reference for all drop CRDs with types, defaults, and validation. |
| [Status & Errors](docs/reference/errors/) | Every condition reason emitted by controllers. Diagnose why resources are not Ready. |
| [Metrics](docs/reference/metrics/) | Prometheus metrics: names, types, descriptions, and example PromQL queries. |
| [Architecture](docs/reference/architecture/) | Package dependency graph and CRD ownership relationships. |
| [Developing](docs/developing/) | Build, test, lint, project structure for contributors. |
`

// ─── llms-full.txt (USE agents — complete reference) ─────────────────────────

var llmsFullTxtTmpl = `# {{.Project.Name}} — Full Reference for AI Agents

## Project

- **Name**: {{.Project.Name}}
- **Language**: Go {{.Project.GoVersion}}
- **Module**: {{.Project.Module}}
- **API Group**: {{.Project.APIGroup}}
- **Scope**: All CRDs cluster-scoped
- **License**: {{.Project.License}}
- **Framework**: Kubebuilder / controller-runtime

## CRD Field Reference
{{range .CRDs}}
### {{.Kind}}

{{.Doc}}
{{if .Controller}}
Controller: {{.Controller}} | Test: {{.TestFile}}
{{end}}
#### Spec
| Field | JSON | Type | Required | Default | Description |
|-------|------|------|----------|---------|-------------|
{{- range .SpecFields}}
| {{.Name}} | ` + "`{{.JSON}}`" + ` | ` + "`{{.Type}}`" + ` | {{if .Required}}✓{{else}}—{{end}} | {{if .Default}}` + "`{{.Default}}`" + `{{end}} | {{.Doc}}{{if .Enum}} Enum: {{range $i, $e := .Enum}}{{if $i}},{{end}}` + "`{{$e}}`" + `{{end}}{{end}} |
{{- end}}
{{if .StatusFields}}
#### Status
| Field | JSON | Type | Description |
|-------|------|------|-------------|
{{- range .StatusFields}}
| {{.Name}} | ` + "`{{.JSON}}`" + ` | ` + "`{{.Type}}`" + ` | {{.Doc}} |
{{- end}}
{{end}}
{{end}}

## Helper Types
{{range .HelperTypes}}
### {{.Name}}

{{.Doc}}

| Field | JSON | Type | Required | Default | Description |
|-------|------|------|----------|---------|-------------|
{{- range .Fields}}
| {{.Name}} | ` + "`{{.JSON}}`" + ` | ` + "`{{.Type}}`" + ` | {{if .Required}}✓{{else}}—{{end}} | {{if .Default}}` + "`{{.Default}}`" + `{{end}} | {{.Doc}}{{if .Enum}} Enum: {{range $i, $e := .Enum}}{{if $i}},{{end}}` + "`{{$e}}`" + `{{end}}{{end}} |
{{- end}}
{{end}}

## Relationships

` + "```mermaid" + `
graph LR
{{- range .Relationships}}
  {{.From}} -->|{{.Type}}| {{.To}}
{{- end}}
` + "```" + `

## Status Conditions & Error Reasons

| Reason | Controller | Meaning | Troubleshooting |
|--------|-----------|---------|-----------------|
{{- range .Errors}}
| {{.Reason}} | {{.Controller}} | {{.Meaning}} | {{.Troubleshooting}} |
{{- end}}

## Metrics

| Name | Type | Description |
|------|------|-------------|
{{- range .Metrics}}
| ` + "`{{.Name}}`" + ` | {{.Type}} | {{.Help}} |
{{- end}}

## Sample CRs

` + "```yaml" + `
{{.Samples}}
` + "```" + `

## Build & Test

` + "```" + `
{{- range .MakeTargets}}
  make {{.Name}}{{"\t"}}# {{.Desc}}
{{- end}}
` + "```" + `
`

// ─── .github/copilot-instructions.md (CODE agents) ──────────────────────────

var copilotInstructionsTmpl = `# Copilot Instructions for {{.Project.Name}}

## Critical Rules

1. **ALWAYS read project files before acting.** Read the Tiltfile, Makefile, and relevant source before writing docs, suggesting workflows, or describing how things work. Never guess based on general knowledge.
2. **Documentation must be short and concise.** Focus on high-level overview and usage. Avoid volatile implementation details. Avoid information that will change frequently.
3. **Simplicity over complexity.** If a simple solution exists, use it. DRY is NOT always best. No premature optimization.
4. **Kubernetes: always verify.** Use ` + "`kubectl explain`" + ` or read the CRD types before suggesting field values or resource specs.
5. **Security-conscious.** Never expose secrets in code or docs. Follow secure coding practices.
6. **Tilt handles the dev loop.** ` + "`tilt up`" + ` does everything: cluster creation, build, deploy, port-forwards, Hugo docs, e2e infra, dev samples. Don't suggest manual commands for things Tilt automates.

## Project

Kubernetes operator (Go {{.Project.GoVersion}}, Kubebuilder, controller-runtime) that pre-caches container images on cluster nodes.
API group: ` + "`{{.Project.APIGroup}}`" + `. All CRDs are cluster-scoped.

## Build Commands

` + "```bash" + `
make generate      # regenerate deepcopy
make manifests     # regenerate CRD + RBAC YAML
make codegen       # both of the above
go build ./...     # compile
make test          # unit tests (envtest)
make test-e2e      # e2e tests (chainsaw, needs kind)
make lint          # golangci-lint
make docs-gen      # regenerate AI docs from source
` + "```" + `

## Code Conventions
{{range .Conventions}}{{if or (eq (index .Scope 0) "code") (eq (index .Scope 0) "both")}}
- {{.Rule}}
{{- end}}{{end}}

## Testing Patterns

- Controller tests use envtest (` + "`internal/controller/*_test.go`" + `)
- Table-driven tests preferred
- E2E uses Kyverno Chainsaw in ` + "`test/e2e/`" + `
- Test fixtures in ` + "`config/samples/`" + ` and ` + "`hack/dev-samples.yaml`" + `

## CRD Quick Reference

| Kind | Controller | Purpose |
|------|-----------|---------|
{{- range .CRDs}}
| {{.Kind}} | {{.Controller}} | {{.Doc}} |
{{- end}}

## Package Dependency Graph

` + "```" + `
{{- range .Packages}}
{{.Path}} — {{.Role}}{{if .Imports}}
  imports: {{join .Imports ", "}}{{end}}
{{- end}}
` + "```" + `

## Don'ts

- Don't add CRI socket access or privileged containers — we use kubelet image pulls only
- Don't put pacing logic outside ` + "`internal/pacing/`" + `
- Don't create namespaced CRDs — all resources are cluster-scoped
- Don't manually edit generated files (` + "`zz_generated.deepcopy.go`" + `, ` + "`config/crd/bases/`" + `)
- Don't manually edit ` + "`llms.txt`" + `, ` + "`llms-full.txt`" + `, ` + "`.cursorrules`" + `, ` + "`AGENTS.md`" + ` — run ` + "`make docs-gen`" + `
`

// ─── .cursorrules (CODE agents) ──────────────────────────────────────────────

var cursorRulesTmpl = `# Cursor Rules for {{.Project.Name}}

## Critical Rules

1. ALWAYS read project files (Tiltfile, Makefile, source) before acting. Never guess.
2. Documentation: short, concise, high-level. No volatile details.
3. Simplicity over complexity. DRY is NOT always best. No premature optimization.
4. Kubernetes: use kubectl explain or read CRD types before suggesting specs.
5. Security: never expose secrets in code or docs.
6. Tilt handles the dev loop. tilt up does everything. Don't suggest manual commands for automated steps.

## Project Context
Kubernetes operator (Go {{.Project.GoVersion}}, Kubebuilder, controller-runtime).
Module: {{.Project.Module}}
API group: {{.Project.APIGroup}}. All CRDs cluster-scoped.

## Key Commands
- Build: go build ./...
- Test: make test
- Lint: make lint
- CRD gen: make manifests
- Deepcopy gen: make generate
- All codegen: make codegen
- AI docs gen: make docs-gen

## Structure
{{- range .Packages}}
- {{.Path}} — {{.Role}}
{{- end}}
- charts/drop/ — Helm chart
- test/e2e/ — Chainsaw E2E tests
- hack/gen-ai-docs/ — generates all docs from source

## CRDs → Controllers
{{- range .CRDs}}
- {{.Kind}}{{if .Controller}} → {{.Controller}}{{else}} (config-only, no controller){{end}}
{{- end}}

## Conventions
{{- range .Conventions}}
- {{.Rule}}
{{- end}}

## Don't
- Edit generated files (zz_generated.deepcopy.go, config/crd/bases/, llms.txt, llms-full.txt, knowledge.yaml)
- Add privileged containers or CRI socket mounts
- Create namespaced CRDs
- Put pacing logic outside internal/pacing/
`

// ─── AGENTS.md (CODE agents — generic) ──────────────────────────────────────

var agentsMdTmpl = `# Agent Instructions

## Critical Rules

1. ALWAYS read project files (Tiltfile, Makefile, source) before acting. Never guess.
2. Documentation: short, concise, high-level. No volatile details.
3. Simplicity over complexity. DRY is NOT always best. No premature optimization.
4. Kubernetes: use kubectl explain or read CRD types before suggesting specs.
5. Security: never expose secrets in code or docs.
6. Tilt handles the dev loop. ` + "`tilt up`" + ` does everything. Don't suggest manual commands for automated steps.

## Project: {{.Project.Name}}

Kubernetes operator (Go {{.Project.GoVersion}}) that pre-caches container images on cluster nodes.

## Quick Start

` + "```bash" + `
make codegen       # generate deepcopy + CRD manifests
go build ./...     # compile
make test          # unit tests
make docs-gen      # regenerate AI docs
` + "```" + `

## Architecture

- API group: ` + "`{{.Project.APIGroup}}`" + ` (cluster-scoped)
- Framework: Kubebuilder + controller-runtime
- Pull mechanism: short-lived Pods with ` + "`nodeName`" + ` + ` + "`command: [\"true\"]`" + `

## CRDs

| Kind | Purpose |
|------|---------|
{{- range .CRDs}}
| {{.Kind}} | {{.Doc}} |
{{- end}}

## Key Directories

| Path | Contents |
|------|----------|
{{- range .Packages}}
| {{.Path}} | {{.Role}} |
{{- end}}
| charts/drop/ | Helm chart |
| test/e2e/ | Chainsaw E2E tests |
| hack/gen-ai-docs/ | This doc generator |

## Rules

1. Run ` + "`make codegen`" + ` after changing api/v1alpha1/ types
2. Run ` + "`make docs-gen`" + ` after changing types or Makefile (regenerates this file)
3. Never edit generated files directly
4. All CRDs are cluster-scoped — no namespaced resources
5. No privileged containers — kubelet-based image pulls only
6. Status uses ` + "`metav1.Condition`" + ` with type "Ready"

## Full Reference

See [llms-full.txt](llms-full.txt) for complete CRD field documentation.
`

// ─── Hugo: CRD Reference ────────────────────────────────────────────────────

var hugoCRDsTmpl = `---
# Generated by make docs-gen — DO NOT EDIT
title: CRD Reference
weight: 1
aliases:
  - /drop/docs/reference/crds/
description: Custom Resource Definition reference for the drop operator.
llmsDescription: |
  Complete CRD field reference for drop.corewire.io/v1alpha1. All resources
  are cluster-scoped. Covers CachedImage, CachedImageSet, PullPolicy, and
  DiscoveryPolicy with every spec/status field, types, defaults, and validation.
---

All resources are cluster-scoped under ` + "`{{.Project.APIGroup}}`" + `.

## Quick Example

` + "```yaml" + `
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx
spec:
  image: docker.io/library/nginx
  tag: latest
  nodeSelector:
    kubernetes.io/arch: amd64
` + "```" + `
{{range .CRDs}}
## {{.Kind}}

{{.Doc}}
{{if .Controller}}
**Controller:** ` + "`{{.Controller}}`" + `
{{end}}
### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
{{- range .SpecFields}}
| ` + "`{{.JSON}}`" + ` | ` + "`{{.Type}}`" + ` | {{if .Required}}Yes{{else}}No{{end}} | {{if .Default}}{{.Default}}{{else}}—{{end}} | {{.Doc}}{{if .Enum}} ({{range $i, $e := .Enum}}{{if $i}} &#124; {{end}}` + "`{{$e}}`" + `{{end}}){{end}} |
{{- end}}
{{if .StatusFields}}
### Status

| Field | Type | Description |
|-------|------|-------------|
{{- range .StatusFields}}
| ` + "`{{.JSON}}`" + ` | ` + "`{{.Type}}`" + ` | {{.Doc}} |
{{- end}}
{{end}}
---
{{end}}

## Helper Types
{{range .HelperTypes}}
### {{.Name}}

{{.Doc}}

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
{{- range .Fields}}
| ` + "`{{.JSON}}`" + ` | ` + "`{{.Type}}`" + ` | {{if .Required}}Yes{{else}}No{{end}} | {{if .Default}}{{.Default}}{{else}}—{{end}} | {{.Doc}} |
{{- end}}
{{end}}
`

// ─── Hugo: Error Catalog ─────────────────────────────────────────────────────

var hugoErrorsTmpl = `---
# Generated by make docs-gen — DO NOT EDIT
title: Status & Errors
weight: 2
aliases:
  - /drop/docs/reference/errors/
description: Status conditions, reasons, and troubleshooting for drop CRDs.
llmsDescription: |
  Every metav1.Condition reason emitted by drop controllers. Lookup table
  maps reason codes to controller, meaning, and fix. Use this to diagnose
  why a CachedImage, CachedImageSet, or DiscoveryPolicy is not Ready.
---

All drop CRDs use ` + "`metav1.Condition`" + ` with type **"Ready"**. The ` + "`.reason`" + ` field indicates the specific state.

## Quick Lookup

| Reason | Controller | Meaning | How to Fix |
|--------|-----------|---------|------------|
{{- range .Errors}}
| **{{.Reason}}** | {{.Controller}} | {{.Meaning}} | {{if .Troubleshooting}}{{.Troubleshooting}}{{else}}—{{end}} |
{{- end}}

## By Controller

### CachedImage

| Reason | Meaning |
|--------|---------|
{{- range .Errors}}{{if eq .Controller "CachedImage"}}
| **{{.Reason}}** | {{.Meaning}} |
{{- end}}{{end}}

### CachedImageSet

| Reason | Meaning |
|--------|---------|
{{- range .Errors}}{{if eq .Controller "CachedImageSet"}}
| **{{.Reason}}** | {{.Meaning}} |
{{- end}}{{end}}

### DiscoveryPolicy

| Reason | Meaning |
|--------|---------|
{{- range .Errors}}{{if eq .Controller "DiscoveryPolicy"}}
| **{{.Reason}}** | {{.Meaning}} |
{{- end}}{{end}}
`

// ─── Hugo: Metrics ───────────────────────────────────────────────────────────

var hugoMetricsTmpl = `---
# Generated by make docs-gen — DO NOT EDIT
title: Metrics
weight: 3
aliases:
  - /drop/docs/reference/metrics/
description: Prometheus metrics exposed by the drop operator.
llmsDescription: |
  All Prometheus metrics registered by the drop operator. Includes metric
  name, type (counter/gauge/histogram), and description. Also provides
  example PromQL queries for monitoring image cache coverage and pull errors.
---

The drop operator exposes the following metrics:

| Metric | Type | Description |
|--------|------|-------------|
{{- range .Metrics}}
| ` + "`{{.Name}}`" + ` | {{.Type}} | {{.Help}} |
{{- end}}

## Useful Queries

` + "```promql" + `
# Images cached per node
sum by (node) (drop_images_cached_total)

# Pull error rate
rate(drop_pull_errors_total[5m])

# Average pull duration
histogram_quantile(0.95, rate(drop_pull_duration_seconds_bucket[10m]))

# Discovery coverage
drop_discovery_images_found
` + "```" + `
`

// ─── Hugo: Architecture (Mermaid) ───────────────────────────────────────────

var hugoArchTmpl = `---
# Generated by make docs-gen — DO NOT EDIT
title: Architecture
weight: 4
aliases:
  - /drop/docs/reference/architecture/
description: Internal architecture and package dependency graph.
llmsDescription: |
  Package dependency graph and CRD ownership relationships for the drop
  operator. Shows how controllers, pacing engine, pod builder, and discovery
  packages relate. Useful for understanding code navigation and import paths.
---

## CRD Relationships

` + "```mermaid" + `
graph TD
{{- range .Relationships}}
  {{.From}} -->|{{.Type}}| {{.To}}
{{- end}}
` + "```" + `

## Package Dependencies

` + "```mermaid" + `
graph LR
  cmd/main.go --> internal/controller
{{- range $pkg := .Packages}}{{if $pkg.Imports}}{{range $pkg.Imports}}
  {{$pkg.Path}} --> {{.}}
{{- end}}{{end}}{{end}}
` + "```" + `

## Reconciler → CRD Mapping

| CRD | Controller | Dependencies |
|-----|-----------|--------------|
{{- range .CRDs}}
| {{.Kind}} | {{if .Controller}}` + "`{{.Controller}}`" + `{{else}}(config-only){{end}} | {{if .Controller}}podbuilder, pacing, metrics{{end}} |
{{- end}}

## Pull Mechanism

` + "```mermaid" + `
sequenceDiagram
  participant CR as CachedImage
  participant Ctrl as Controller
  participant Pace as Pacing Engine
  participant K8s as Kubernetes API
  participant Node as Kubelet

  CR->>Ctrl: Reconcile triggered
  Ctrl->>Pace: Request pull slot
  Pace-->>Ctrl: Slot granted
  Ctrl->>K8s: Create Pod (nodeName=target)
  K8s->>Node: Schedule Pod
  Node->>Node: Pull image (kubelet)
  Node-->>K8s: Pod succeeds
  K8s-->>Ctrl: Watch event
  Ctrl->>CR: Update status (Ready)
` + "```" + `
`

// ─── Doc Generation Flow Diagram ─────────────────────────────────────────────

var docGenDiagramTmpl = `# Documentation Generation

<!-- Generated by make docs-gen — DO NOT EDIT -->

## How It Works

All documentation is generated from source code via ` + "`make docs-gen`" + ` (which runs ` + "`go run ./hack/gen-ai-docs/`" + `).

` + "```mermaid" + `
flowchart TD
  subgraph Sources["Source of Truth"]
    TYPES["api/v1alpha1/*_types.go<br/>(CRD types + kubebuilder markers)"]
    CTRL["internal/controller/*.go<br/>(reconcilers, error reasons)"]
    METRICS["internal/metrics/metrics.go<br/>(Prometheus metrics)"]
    MAKEFILE["Makefile<br/>(build targets)"]
    GOMOD["go.mod<br/>(Go version, module)"]
    SAMPLES["hack/dev-samples.yaml<br/>(example CRs)"]
  end

  subgraph Generator["hack/gen-ai-docs/"]
    PARSE["Go AST Parser<br/>+ go list -json"]
    KNOWLEDGE["knowledge.yaml<br/>(structured intermediate)"]
    RENDER["Template Renderer"]
  end

  subgraph UseAgents["USE Agents"]
    LLMS["llms.txt<br/>(short onboarding)"]
    LLMSFULL["llms-full.txt<br/>(complete reference)"]
  end

  subgraph CodeAgents["CODE Agents"]
    COPILOT[".github/copilot-instructions.md"]
    CURSOR[".cursorrules"]
    AGENTS["AGENTS.md"]
  end

  subgraph Humans["Humans (Hugo)"]
    CRDS["reference/_generated_crds.md"]
    ERRORS["reference/_generated_errors.md"]
    METRICSH["reference/_generated_metrics.md"]
    ARCH["reference/_generated_architecture.md"]
  end

  TYPES --> PARSE
  CTRL --> PARSE
  METRICS --> PARSE
  MAKEFILE --> PARSE
  GOMOD --> PARSE
  SAMPLES --> PARSE

  PARSE --> KNOWLEDGE
  KNOWLEDGE --> RENDER

  RENDER --> LLMS
  RENDER --> LLMSFULL
  RENDER --> COPILOT
  RENDER --> CURSOR
  RENDER --> AGENTS
  RENDER --> CRDS
  RENDER --> ERRORS
  RENDER --> METRICSH
  RENDER --> ARCH
` + "```" + `

## Three Audiences

` + "```mermaid" + `
graph LR
  subgraph SoT["Single Source of Truth"]
    CODE["Go Source Code"]
  end

  CODE -->|schema, fields, examples| USE["USE Agents<br/>(GitOps, kubectl, IaC)"]
  CODE -->|architecture, conventions| DEV["CODE Agents<br/>(Copilot, Cursor, Codex)"]
  CODE -->|narrative + generated ref| HUMAN["Humans<br/>(Hugo docs site)"]
` + "```" + `

## Commands

| Command | Purpose |
|---------|---------|
| ` + "`make docs-gen`" + ` | Regenerate all docs from source |
| ` + "`make docs-gen-check`" + ` | CI gate — fails if docs are stale |
| ` + "`make codegen`" + ` | CRDs + deepcopy + docs (full pipeline) |
`
