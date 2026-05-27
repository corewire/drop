# Audience → Information Matrix

## Four Audiences

| Audience | Format Need | Current Output |
|----------|------------|----------------|
| Human USE (operator users) | Narrative guides, navigable site | Hugo docs (install, usage, discovery, monitoring) |
| AI USE (agents deploying/configuring) | Flat, complete, single-request | `llms.txt`, `llms-full.txt` |
| Human DEV (contributors) | Explanatory prose, how-tos | Hugo docs (developing/*) |
| AI DEV (coding assistants) | Terse rules, file mappings | `.github/copilot-instructions.md`, `.cursorrules`, `AGENTS.md` |

## What Each Audience Needs

| Information | Human USE | AI USE | Human DEV | AI DEV |
|-------------|:---------:|:------:|:---------:|:------:|
| Install (Helm values, prereqs) | ✓ | ✓ | — | — |
| CRD field reference (types, defaults, validation) | ✓ | ✓ | — | — |
| Working YAML examples | ✓ | ✓ | — | — |
| Error reasons + troubleshooting | ✓ | ✓ | — | — |
| Metrics + PromQL queries | ✓ | ✓ | — | — |
| Discovery guide (narrative how-to) | ✓ | — | — | — |
| Monitoring dashboards / events | ✓ | — | — | — |
| API group + resource names | — | ✓ | — | ✓ |
| Constraints (cluster-scoped, no privileged) | — | ✓ | — | ✓ |
| Architecture (package graph, reconciler design) | — | — | ✓ | ✓ |
| Conventions (naming, patterns, don'ts) | — | — | ✓ | ✓ |
| Build/test commands | — | — | ✓ | ✓ |
| Package imports + dependency graph | — | — | ✓ | ✓ |
| How to extend (add CRD, new source) | — | — | ✓ | ✓ |
| Debugging (Delve, logs, common issues) | — | — | ✓ | — |
| Release process | — | — | ✓ | — |
| Controller file ↔ CRD mapping | — | — | — | ✓ |
| Don'ts (pacing outside pkg, namespaced CRDs) | — | — | ✓ | ✓ |

## Key Insights

1. AI audiences need the same *facts* as their human counterparts, but in a different *format*:
   - AI USE: schema + examples in a single flat file — no navigation, no narrative
   - AI DEV: conventions + architecture as terse rules — not explanatory prose

2. Truly shared content:
   - CRD fields/types → USE (both human & AI)
   - Conventions/don'ts → DEV (both human & AI)

3. Audience-exclusive content:
   - Narrative guides (discovery, monitoring) → Human USE only
   - Debugging/release → Human DEV only
   - File-to-kind mapping, import paths → AI DEV only

## Implication for Generated Docs

Generated outputs should only produce content that is:
- Derivable from source code (CRD fields, errors, metrics, package graph)
- Needed by at least one audience in that exact format

Manual docs should only exist for:
- Narrative that can't be extracted (guides, how-tos, explanations)
- Stable content that won't go stale (install steps, design decisions)

## Generation Feasibility

### Easily generated (already doing or trivial)

| Information | Source | Ease |
|-------------|--------|------|
| CRD fields (names, types, descriptions, defaults, enums, required) | CRD YAML or Go AST | Trivial |
| Error reasons + messages | Controller source (`.Reason = "..."`) | Easy (regex) |
| Metrics (name, type, help) | `internal/metrics/metrics.go` | Easy (regex) |
| Package dependency graph | `go list -json ./...` | Trivial (stdlib) |
| Build/test commands | Makefile `##` annotations | Easy (regex) |
| API group + resource names | CRD YAML or go.mod + types | Trivial |
| Sample CRs | Embed `hack/dev-samples.yaml` as-is | Trivial |

### Could generate (marginal value)

| Information | Source | Notes |
|-------------|--------|-------|
| Helm values reference | `charts/drop/values.yaml` | Parse YAML + comments → table |
| `scope: Cluster` constraint | CRD YAML `spec.scope` | Replace hardcoded convention |

### Cannot generate (human knowledge)

| Information | Why |
|-------------|-----|
| Conventions, don'ts, architectural decisions | Domain knowledge, not in code |
| Narrative guides (discovery, monitoring, debugging, extending) | Explanatory prose |
| PromQL queries | Domain expertise |
| Troubleshooting advice ("How to Fix") | Operational experience |
| Release process | Workflow knowledge |

---

## Framework Design: `gendocs` for Go Projects

A lightweight Go tool that extracts documentation from code and produces all AI/human outputs with **one command**. Designed for maximum impact with minimal ceremony.

### Core Principle

**The code IS the documentation source.** If you write good Go doc comments, struct tags, and use idiomatic patterns (Cobra commands, kubebuilder markers, OpenAPI annotations), the tool generates everything else. Zero separate doc files to maintain for reference content.

### Three Archetypes

| Archetype | Primary Source | What's "Free" |
|-----------|---------------|---------------|
| **Operator** (Kubebuilder) | `api/` types + markers, controllers | CRD fields, defaults, validation, enums, error reasons, metrics |
| **CLI** (Cobra/Viper) | Command tree + flags | Commands, flags, types, defaults, examples, usage |
| **Cloud-native service** | Go doc comments + OpenAPI/struct tags | Endpoints, request/response types, config options |

### Design: What the Tool Does

```
go run ./gendocs
```

That's it. One command, run from project root. Outputs:

| Output | Audience | Content |
|--------|----------|---------|
| `llms.txt` | AI USE | Project overview + page index |
| `llms-full.txt` | AI USE | Complete reference (all fields, flags, endpoints) |
| `AGENTS.md` | AI DEV | Build commands, conventions, structure |
| `.github/copilot-instructions.md` | Copilot | Same as AGENTS.md + Copilot-specific framing |
| `docs/*.md` | Human | Markdown reference pages (Hugo/GitHub-compatible) |
| `knowledge.yaml` | Internal | Intermediate model (optional, for debugging/extension) |

### Architecture

```
gendocs/
├── main.go        # Entry point: find root, call extractors, render
├── config.go      # User-editable: project metadata, conventions, don'ts
├── extract.go     # Source parsing (shared + archetype-specific)
└── render.go      # Templates → output files
```

**Key design decisions:**
- Single `main.go` entry point — no framework, no plugins, no config files
- `config.go` is the only file users edit (project name, conventions list, output toggles)
- Extractors are pure functions: `func extractX(path string) []Thing`
- Templates are embedded strings (Go `text/template`) — easy to read and modify

### Per-Archetype Extraction

#### Operator (Kubebuilder/controller-runtime)

Source already contains everything via markers and Go AST:

```go
// api/v1alpha1/widget_types.go

// Widget ensures a widget is properly configured on all nodes.
// +kubebuilder:resource:scope=Cluster
type Widget struct { ... }

type WidgetSpec struct {
    // Image is the container image to configure.
    // +kubebuilder:validation:Required
    Image string `json:"image"`

    // Replicas is how many instances to run.
    // +kubebuilder:default=1
    // +kubebuilder:validation:Minimum=0
    Replicas int `json:"replicas,omitempty"`
}
```

**Extracts:** kind, doc, scope, fields (name, type, default, validation, description), status conditions, error reasons from controllers, metrics from registration.

**Zero extra work** if you already write kubebuilder markers and doc comments (which you should).

#### CLI (Cobra/Viper)

Cobra already has the full command tree in memory:

```go
// cmd/serve.go
var serveCmd = &cobra.Command{
    Use:   "serve",
    Short: "Start the HTTP server",
    Long:  `serve starts the API server on the configured port. Use --port to override.`,
    Example: `  myapp serve
  myapp serve --port 9090`,
    Run: runServe,
}

func init() {
    serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
    serveCmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logging")
    rootCmd.AddCommand(serveCmd)
}
```

**Extracts:** command name, short/long description, examples, flags (name, shorthand, type, default, usage), subcommand tree.

**Zero extra work** — Cobra already stores all this. The generator just walks the command tree at build time (see `cobra/doc` package for prior art). The `gendocs` approach adds `llms.txt` + `AGENTS.md` output on top.

#### Cloud-Native Service (HTTP/gRPC)

For services without Cobra or CRDs, use struct tags and Go doc comments:

```go
// internal/api/types.go

// CreateOrderRequest is the payload for POST /v1/orders.
type CreateOrderRequest struct {
    // Item is the product identifier to order.
    Item string `json:"item" validate:"required"`

    // Quantity is how many to order. Defaults to 1.
    Quantity int `json:"quantity,omitempty" default:"1" validate:"min=1,max=100"`
}
```

**Extracts:** type name, doc comment, fields (json name, type, validation, default from tags).

Pairs well with OpenAPI annotations if you already have them — but works without.

### What Users Write (config.go)

The only manual input — project identity and human-knowledge that can't be extracted:

```go
// config.go — edit this file for your project

func projectConfig() ProjectMeta {
    return ProjectMeta{
        Name:        "myapp",
        Description: "A cloud-native widget service.",
        Language:    "Go 1.22",
        Module:      "github.com/org/myapp",
        Repository:  "https://github.com/org/myapp",
    }
}

func conventions() []string {
    return []string{
        "All resources are cluster-scoped",
        "Table-driven tests preferred",
        "No privileged containers",
    }
}

func donts() []string {
    return []string{
        "Don't put business logic in handlers — use service layer",
        "Don't manually edit generated files — run make docs",
    }
}
```

Everything else is extracted from code. Conventions and don'ts are the only things that require human knowledge.

### Makefile Integration

```makefile
docs: ## Regenerate all docs (AI + human)
	go run ./gendocs

check-docs: docs ## CI: fail if docs are stale
	@git diff --exit-code llms.txt llms-full.txt AGENTS.md .github/copilot-instructions.md docs/ || \
		(echo "error: docs are stale — run 'make docs' and commit" && exit 1)
```

### Why This Works (Maximum Impact, Minimum Effort)

| Effort | Impact |
|--------|--------|
| Write good doc comments on types/commands (you should anyway) | All reference docs generated for free |
| Add `gendocs/` directory (~200 lines for simple projects) | 5 output files serving 4 audiences |
| `make docs` in CI | Docs never go stale |
| Edit `config.go` when conventions change | Agent instructions stay current |

### What We Explicitly Skip

- **MCP server** — overkill for most projects; add later if needed
- **SKILL.md** — only relevant if your project is an SDK others consume
- **Dual descriptions / llmsDescription frontmatter** — only for web-hosted docs sites
- **Hugo/web rendering** — optional; GitHub Markdown rendering is enough for many projects
- **Plugin system** — YAGNI; just edit the extractor functions directly

### Comparison: Existing Examples

| Project | Archetype | `gendocs` approach |
|---------|-----------|-------------------|
| `drop` (this repo) | Operator | Parses `api/v1alpha1/*_types.go` + controllers + metrics → full pipeline |
| `dice` (ai-friendly-docs example) | CLI | Walks Cobra command tree → llms.txt + AGENTS.md + Markdown |
| Generic service | Service | Parses handler types + struct tags → endpoint reference |

The `dice` example in `ai-friendly-docs/examples/gen-ai-docs/` is a working ~180-line implementation for the CLI archetype. The `drop` implementation in `hack/gen-ai-docs/` is the operator archetype. A service archetype follows the same pattern with different extractors.

---

## The Dream: Self-Documenting Code (Operators + Go CLIs)

### What Every Audience Actually Wants

| Audience | The dream | Gap today |
|----------|-----------|-----------|
| **AI agent deploying** | Read one file → produce correct YAML on first try, zero follow-ups | Missing: "when to use", field relationships, validated examples |
| **AI agent coding** | Open any file → know what to do, what NOT to do, where new code goes | Missing: pitfalls, cross-references between packages |
| **Human user** | Every error says how to fix it, every field says when I'd use it | Missing: intent, troubleshooting in context |
| **Human dev** | Code is the doc — no separate file to forget updating | Missing: narrative context collocated with code |

### Key Insight: Use What Already Exists

We don't need a new documentation framework. Go already has all the building blocks:

| Building block | Already exists | What it gives us |
|----------------|---------------|------------------|
| Go doc comments | Since Go 1.0 | Type/field descriptions |
| Go 1.19 doc headings | `// # Heading` | Structured sections within a doc comment |
| Kubebuilder markers | `+kubebuilder:` | Defaults, validation, enums, scope |
| Cobra fields | `Short`, `Long`, `Example` | Command docs, flag docs |
| `config/samples/` | Convention | Working YAML examples |
| Makefile `##` annotations | Convention | Build commands |

**The only thing missing is ~3 lightweight markers for human intent.**

### The Minimal Marker Set

For operators and CLIs, we need exactly **3 custom markers**. Everything else is already in the code:

```
+docs:when=<text>       — When/why to use this (intent)
+docs:see=<ref>         — Related resources/commands
+docs:pitfall=<text>    — Common mistake (auto-generates "Don't" lists)
```

That's it. Three markers. No framework, no plugins, no config files.

Optional (add only when you hit the need):
```
+docs:fix:<Reason>=<text>  — How to fix an error (collocated with error constants)
+docs:prereq=<text>        — What must exist before using this
```

---

### Archetype 1: Kubernetes Operator

#### What you already write (and what it gives you for free)

```go
// CachedImage ensures a single container image is pre-cached on cluster nodes.
//
// The controller creates one pull Pod per targeted node and tracks completion
// via status conditions. Use NodeSelector to limit which nodes get the image.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type CachedImage struct { ... }
```

**Free extraction from standard code:**
- Kind name, doc comment → description
- `+kubebuilder:resource:scope` → cluster/namespaced
- `+kubebuilder:printcolumn` → what matters at a glance
- Every field's doc comment → field reference table
- `+kubebuilder:default`, `+kubebuilder:validation:Enum` → defaults, allowed values
- `+optional` / `+kubebuilder:validation:Required` → required vs optional
- Struct tags (`json:"..."`) → YAML field names
- `config/samples/*.yaml` → working examples (already exist!)

**What's missing (and costs 1 line each to add):**

```go
// CachedImage ensures a single container image is pre-cached on cluster nodes.
//
// +docs:when=You need a specific image available on nodes before pods schedule (large ML models, critical system images, air-gapped deploys).
// +docs:see=CachedImageSet (manage groups), PullPolicy (control pacing), DiscoveryPolicy (auto-discover from registry)
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type CachedImage struct { ... }
```

And per-field, where it matters:

```go
type CachedImageSpec struct {
    // ImagePullPolicy controls when kubelet pulls the image.
    // +kubebuilder:validation:Enum=Always;IfNotPresent;Never
    // +kubebuilder:default=Always
    // +docs:when=Set IfNotPresent for immutable tags (saves bandwidth). Use Always for mutable tags.
    // +docs:pitfall=IfNotPresent with mutable tags (like "latest") means nodes never get updates.
    ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

    // Priority is a pull ordering hint (lower = pulled first).
    // +docs:when=Multiple CachedImages compete for bandwidth. Set critical images to 0, nice-to-have to 100.
    // +optional
    Priority *int32 `json:"priority,omitempty"`

    // NodeSelector restricts which nodes to cache the image on.
    // +docs:when=You only need the image on a subset (e.g. GPU nodes, specific zone).
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}
```

#### Error troubleshooting — collocated with the condition

```go
// Status conditions set by the controller.
const (
    // +docs:fix:ErrImagePull=Verify imagePullSecrets exist and have valid credentials. Check: kubectl get events --field-selector reason=ErrImagePull
    // +docs:fix:ImagePullBackOff=Image does not exist or tag is wrong. Verify with: crane manifest <image>
    conditionTypeReady = "Ready"
)
```

#### What the generator produces (operator)

From the above (which is ~10 extra comment lines total across the whole CRD), the generator outputs:

**For AI (llms-full.txt) — complete, flat, one-shot:**

```markdown
## CachedImage

Ensures a single container image is pre-cached on cluster nodes.

**When to use:** You need a specific image available on nodes before pods schedule
(large ML models, critical system images, air-gapped deploys).

**Related:** CachedImageSet (manage groups), PullPolicy (control pacing),
DiscoveryPolicy (auto-discover from registry)

### Spec Fields

| Field | Type | Default | Required | When to use |
|-------|------|---------|----------|-------------|
| image | string | — | yes | Always — the image reference |
| tag | string | — | no | Use for mutable references |
| digest | string | — | no | Use for immutable references (preferred in prod) |
| imagePullPolicy | Always / IfNotPresent / Never | Always | no | IfNotPresent for immutable tags |
| nodeSelector | map[string]string | — | no | Only need image on subset of nodes |
| priority | int | — | no | Multiple images competing for bandwidth |

### Example

apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: ml-model-gpu
spec:
  image: registry.example.com/ml-model
  tag: v2.1
  nodeSelector:
    gpu: "true"
  priority: 0

### Errors

| Condition | Reason | How to Fix |
|-----------|--------|-----------|
| Ready=False | ErrImagePull | Verify imagePullSecrets exist and have valid credentials |
| Ready=False | ImagePullBackOff | Image/tag doesn't exist. Verify with: crane manifest <image> |

### Pitfalls

- IfNotPresent with mutable tags means nodes never get updates
```

**For devs (AGENTS.md) — pitfalls auto-become Don'ts:**

```markdown
## Don'ts
- Don't use IfNotPresent with mutable tags (like "latest") — nodes never get updates
```

---

### Archetype 2: Go CLI (Cobra)

#### What you already write (and what it gives you for free)

```go
var deployCmd = &cobra.Command{
    Use:   "deploy [flags] <environment>",
    Short: "Deploy the application to a target environment",
    Long: `deploy pushes the current build to the specified environment.
It runs health checks after deployment and rolls back on failure.`,
    Example: `  myapp deploy staging
  myapp deploy production --canary --canary-percent 10
  myapp deploy production --dry-run`,
    Args: cobra.ExactArgs(1),
    RunE: runDeploy,
}

func init() {
    deployCmd.Flags().BoolVar(&canary, "canary", false, "Use canary deployment strategy")
    deployCmd.Flags().IntVar(&canaryPercent, "canary-percent", 5, "Percentage of traffic for canary")
    deployCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would happen without executing")
    deployCmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "How long to wait for health checks")
    rootCmd.AddCommand(deployCmd)
}
```

**Free extraction from standard Cobra:**
- `Use` → command signature (including args)
- `Short` → one-line description
- `Long` → full description
- `Example` → copy-paste usage
- `Args` → argument validation
- All flags: name, shorthand, type, default, usage text
- Subcommand tree structure

**This is already 90% of what anyone needs.** Cobra is inherently self-documenting.

**What's missing (1-3 lines):**

```go
// +docs:when=You have a new build ready and want to ship it. Always deploy to staging first.
// +docs:prereq=Authenticated to the cluster (run: myapp auth login). Build must exist (run: myapp build).
// +docs:pitfall=Don't deploy to production without --canary first. Rollbacks take 5+ minutes.
var deployCmd = &cobra.Command{
    Use:   "deploy [flags] <environment>",
    ...
}
```

Per-flag markers (only where there's a non-obvious pitfall):

```go
func init() {
    // +docs:pitfall=Values above 25% can cause cascading failures if the canary is broken.
    deployCmd.Flags().IntVar(&canaryPercent, "canary-percent", 5, "Percentage of traffic for canary")
    ...
}
```

#### What the generator produces (CLI)

**For AI (llms-full.txt):**

```markdown
## myapp deploy

Deploy the application to a target environment.

**When to use:** You have a new build ready and want to ship it.
Always deploy to staging first.

**Prerequisites:** Authenticated (run: myapp auth login).
Build must exist (run: myapp build).

### Usage

  myapp deploy [flags] <environment>

### Examples

  myapp deploy staging
  myapp deploy production --canary --canary-percent 10
  myapp deploy production --dry-run

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| --canary | — | bool | false | Use canary deployment strategy |
| --canary-percent | — | int | 5 | Percentage of traffic for canary |
| --dry-run | — | bool | false | Print what would happen without executing |
| --timeout | — | duration | 5m | How long to wait for health checks |

### Pitfalls

- Don't deploy to production without --canary first. Rollbacks take 5+ minutes.
- --canary-percent above 25% can cause cascading failures if the canary is broken.
```

**For devs (AGENTS.md) — pitfalls auto-generate Don'ts.**

---

### Why This Works: The "Already Writing It" Test

Every marker we propose passes this test: **"Is the developer already thinking this when they write the code?"**

| Marker | Developer thought at write-time |
|--------|-------------------------------|
| `+docs:when=` | "Someone will ask why this exists. I know the answer right now." |
| `+docs:see=` | "This relates to X. I just imported it." |
| `+docs:pitfall=` | "I just fixed a bug caused by this. Don't repeat it." |
| `+docs:fix:Reason=` | "I just debugged this error. Here's how." |
| `+docs:prereq=` | "This won't work unless you did Y first." |

If you capture these at **write-time** (1 comment line), you never need to write a separate troubleshooting guide, pitfalls page, or "when to use" section. They're generated.

### Total Developer Cost

**For a new CRD (operator):**

| What | Lines | When |
|------|-------|------|
| Doc comment on type (you already write this) | 1-2 | At creation |
| `+docs:when=` on the CRD type | 1 | At creation |
| `+docs:see=` if related to other CRDs | 1 | At creation |
| `+docs:when=` on non-obvious fields | 0-3 | At creation |
| `+docs:pitfall=` on footgun fields | 0-2 | When you discover one |
| `+docs:fix:` on error constants | 0-3 | When you debug one |
| **Total** | **3-12 lines** | |

**For a new Cobra command (CLI):**

| What | Lines | When |
|------|-------|------|
| `Short`, `Long`, `Example` (you already write these) | 3-5 | At creation |
| Flag descriptions (you already write these) | 1 per flag | At creation |
| `+docs:when=` on the command | 1 | At creation |
| `+docs:pitfall=` on dangerous flags | 0-2 | When you discover one |
| `+docs:prereq=` if dependencies exist | 0-1 | At creation |
| **Total** | **1-4 extra lines** | |

### What We DON'T Do

- **No `+docs:example=` with inline JSON** — ugly, unreadable, untestable. Keep examples in `config/samples/` (operators) or Cobra's `Example` field (CLIs). The generator reads them from there.
- **No multi-line marker syntax** — if it doesn't fit in one line, it belongs in the doc comment body or a separate guide.
- **No marker on every field** — only on fields where the "when to use" isn't obvious from the description alone.
- **No custom tooling to install** — it's a `go run ./gendocs` in your repo. That's it.
- **No generated code you can't read** — the generator is 200-400 lines of Go. Any dev can understand and modify it.

### Generator Implementation: How Little Code This Is

The marker parser for both archetypes:

```go
// extractDocsMarkers parses +docs: markers from a comment group.
// Returns map like {"when": "text", "see": "text", "pitfall": ["text"]}
func extractDocsMarkers(comments string) DocsMarkers {
    var m DocsMarkers
    for _, line := range strings.Split(comments, "\n") {
        line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
        switch {
        case strings.HasPrefix(line, "+docs:when="):
            m.When = strings.TrimPrefix(line, "+docs:when=")
        case strings.HasPrefix(line, "+docs:see="):
            m.See = strings.TrimPrefix(line, "+docs:see=")
        case strings.HasPrefix(line, "+docs:pitfall="):
            m.Pitfalls = append(m.Pitfalls, strings.TrimPrefix(line, "+docs:pitfall="))
        case strings.HasPrefix(line, "+docs:prereq="):
            m.Prereqs = append(m.Prereqs, strings.TrimPrefix(line, "+docs:prereq="))
        case strings.HasPrefix(line, "+docs:fix:"):
            // +docs:fix:ReasonName=how to fix text
            rest := strings.TrimPrefix(line, "+docs:fix:")
            if k, v, ok := strings.Cut(rest, "="); ok {
                m.Fixes = append(m.Fixes, Fix{Reason: k, HowToFix: v})
            }
        }
    }
    return m
}
```

That's ~25 lines. The rest is template changes to include `When`, `See`, `Pitfalls` in the output.

### Comparison: Effort vs. Coverage

| Approach | Developer effort per new type/command | Docs coverage |
|----------|--------------------------------------|--------------|
| Write separate Markdown docs | 30-60 min | High initially, stale within weeks |
| Only kubebuilder markers + Cobra fields | 0 extra | ~70% (fields, types, defaults — but no intent/troubleshooting) |
| **+ 3 markers** (`when`, `see`, `pitfall`) | 1-4 extra lines | ~95% (everything except narrative guides) |
| Full annotation set (+ fix, prereq) | 3-12 extra lines | ~98% |

### Adoption: How to Start Tomorrow

**Operators:**
1. Add `+docs:when=` to each CRD type's doc comment (4 types × 1 line = 4 lines total for `drop`)
2. Add `+docs:see=` where CRDs reference each other (3 lines for `drop`)
3. Add marker parser to `hack/gen-ai-docs/extract.go` (~25 lines)
4. Add `When`, `See`, `Pitfalls` fields to knowledge model types (~5 lines)
5. Update templates to render them (~20 lines)
6. Done. `make docs-gen` now produces richer output.

**CLIs:**
1. Ensure every command has `Short`, `Long`, `Example` (Cobra basics — you should already)
2. Add `+docs:when=` above commands that aren't self-explanatory
3. Add `+docs:pitfall=` above dangerous flags as you find them
4. The `gendocs/` tool walks the command tree + parses markers → done

No big-bang migration. Each marker is independently useful. Start with `+docs:when=` on your top-level types — that's the single highest-value addition.
