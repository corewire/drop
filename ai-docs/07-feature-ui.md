# UI Feature Specs

Design specs for a future DiscoveryPolicy UI. All previews use a dry-run API — never persisted in etcd.

## 1. Query Editor (Stage 1)

| Element | Purpose |
|---------|---------|
| PromQL/LogQL/registry query input with syntax highlighting | Fast query iteration |
| Live preview table: image ref, raw sample values, sample count | Shows query output before saving the CR |
| Query health badge: latency, series count, error message | Surface slow/broken endpoints |
| Registry: collapsible tag list per repo with tagFilter preview | Highlight matching/excluded tags so regex is visible |

## 2. Signal Inspector (Stage 2)

| Element | Purpose |
|---------|---------|
| Bar chart per signal: images on Y-axis sorted by value | "Which images score highest on this signal?" |
| Side-by-side signal comparison (pick 2+) | Reveals when signals disagree on ranking |
| timeWeightedAggregate: heatmap (hour-of-day × image) | Shows if business-hours window config shifts rankings |
| eventPullTime: histogram of pull durations with p50/p90/p95 lines | Debug why an image ranks high ("it takes 12s to pull") |

## 3. Ranking Playground (Stage 3)

| Element | Purpose |
|---------|---------|
| Ranked image list with stacked bar score breakdown | Shows *why* an image is ranked #1 vs #5 |
| Weight sliders (weightedSum): drag to reorder in real-time | Eliminates apply-wait-check loop |
| maxImages cutoff line: draggable line on ranked list | Simulate different maxImages values |
| Diff view: images entering/leaving top-N, score deltas | "Did my config change improve things?" |
| modelExposure: node exposure diagram with estimated pull cost | Makes the abstract formula concrete |
| Live YAML output panel (editable) | Final manifest is visible, editable, and exportable at all times |

## 4. Cross-cutting Views

| Element | Purpose |
|---------|---------|
| Pipeline DAG: query → signal → ranking with health per node | Overview for complex multi-query setups |
| etcd budget meter: current status size vs max | Ops visibility |
| Sync timeline: imageCount sparkline with sync events | Detects flapping (oscillating image count) |
| CachedImageSet propagation: discovered → CachedImage → node pull status | Closes the loop: discovery → caching → readiness |

## Architecture

- Previews (query editor, weight sliders) computed via `/dry-runs` endpoints or CLI tooling
- Dry-run takes a `DiscoveryPolicySpec`, runs the pipeline once, returns full result without writing status
- CR only stores the last committed sync result (slimmed status)
- If `spec.ranking` is omitted, engine fallback ranks by per-image max raw sample value
- UI richness comes from dry-run responses, not from bloating the stored status

## 5. Interactive Policy Builder and Replay Lab

The UI should be an interactive DiscoveryPolicy builder, not a static form.

- Immediate query feedback: as users edit PromQL/LogQL/registry settings, preview results update instantly
- Ranking and signal explainability: show how each signal and weight changes top-N outcomes in real time
- Targeting assistant: collect taints/tolerations and node labels in the same flow, and suggest candidate targets from observed cluster state
- Autodiscovery hints: suggest likely image targets and policy seeds from historical usage and pull events
- Built-in benchmarking and evaluation: run day-replay experiments from the UI using Prometheus and Loki data sources
- Efficient replay engine: use vectorized/matrix-based computation so full-day evaluations are fast enough for iterative tuning
- YAML-first finish: the final output of the UI flow is a `DiscoveryPolicy` YAML manifest that can be copied, downloaded, or applied

## 6. API Specification (Discovery UI Preview API)

All endpoints are preview-only. They execute the discovery pipeline in memory and never persist status to etcd.

### Base Path

- `/api/v1/discovery-ui`

### Authentication

- Reuse Kubernetes authn/authz from the caller context
- Required capability: same permission level needed to create/update `DiscoveryPolicy` objects

### Endpoint: Validate Spec

- `POST /validate`
- Purpose: fast schema and semantic validation before preview or save

Request body:

```json
{
	"spec": {
		"queries": [],
		"signals": [],
		"ranking": {
			"strategy": "signal",
			"signal": "pull-time"
		},
		"imageFilter": "",
		"syncInterval": "30m",
		"maxImages": 50
	}
}
```

Response `200`:

```json
{
	"valid": true,
	"errors": [],
	"warnings": [
		{
			"path": "spec.queries[0].lookback",
			"code": "LARGE_LOOKBACK",
			"message": "Lookback is large and may increase preview latency"
		}
	]
}
```

### Endpoint: Start Dry-Run

- `POST /dry-runs`
- Purpose: run query -> signal -> ranking once and return a run id

Request body:

```json
{
	"spec": {
		"queries": [],
		"signals": [],
		"ranking": {
			"strategy": "weightedSum",
			"weightedSum": {
				"normalize": "minMax",
				"missingSignal": "zero",
				"terms": [
					{
						"signal": "pull-time-p95",
						"weight": "0.7"
					},
					{
						"signal": "image-size-avg",
						"weight": "0.3"
					}
				]
			}
		},
		"maxImages": 30
	},
	"options": {
		"includeSamples": true,
		"sampleLimitPerImage": 20,
		"timeoutSeconds": 20
	}
}
```

Response `202`:

```json
{
	"runId": "dr_01J7K7Q8M70E9H9T2G2QW0C5XK",
	"status": "queued"
}
```

### Endpoint: Get Dry-Run Result

- `GET /dry-runs/{runId}`
- Purpose: fetch full preview result for inspector and ranking views

Response `200` (completed):

```json
{
	"runId": "dr_01J7K7Q8M70E9H9T2G2QW0C5XK",
	"status": "succeeded",
	"timing": {
		"totalMs": 1342,
		"queryMs": 992,
		"signalMs": 201,
		"rankingMs": 149
	},
	"queryResults": [
		{
			"name": "loki-pulls",
			"type": "loki",
			"status": "success",
			"seriesCount": 421,
			"sampleCount": 2630
		}
	],
	"signals": [
		{
			"name": "pull-time-p95",
			"type": "eventPullTime",
			"statistic": "p95",
			"metric": "pullTime",
			"values": [
				{
					"image": "docker.io/library/nginx:1.25-alpine",
					"value": 12.9,
					"sampleCount": 44
				}
			]
		}
	],
	"ranking": {
		"strategy": "weightedSum",
		"images": [
			{
				"image": "docker.io/library/nginx:1.25-alpine",
				"rank": 1,
				"finalScore": "0.9123",
				"components": [
					{
						"signal": "pull-time-p95",
						"raw": 12.9,
						"normalized": 1.0,
						"weight": "0.7",
						"weighted": 0.7
					}
				]
			}
		]
	},
	"diff": {
		"baselineRunId": "dr_01J7K7Q4A5FJTZ5V3WE1Y5GR47",
		"enteredTopN": ["ghcr.io/acme/api:v2.4.1"],
		"leftTopN": ["ghcr.io/acme/worker:v2.3.9"]
	}
}
```

Response `200` (running):

```json
{
	"runId": "dr_01J7K7Q8M70E9H9T2G2QW0C5XK",
	"status": "running",
	"progress": {
		"stage": "signals",
		"percent": 68
	}
}
```

### Endpoint: Stream Run Events (Optional)

- `GET /dry-runs/{runId}/events`
- Purpose: server-sent events for live progress and partial preview updates

Event types:
- `progress`
- `query.partial`
- `signal.partial`
- `completed`
- `failed`

### Endpoint: Cancel Run

- `POST /dry-runs/{runId}:cancel`
- Purpose: cancel long previews when the user changes config mid-flight

Response `202`:

```json
{
	"runId": "dr_01J7K7Q8M70E9H9T2G2QW0C5XK",
	"status": "cancelled"
}
```

### YAML Handling (Client-side Only)

- YAML serialization and parsing happen fully in the browser (no dedicated YAML API endpoint)
- UI model -> YAML uses a deterministic serializer so output stays canonical
- YAML editor -> UI model parsing runs in-browser with line/column error feedback
- Backend endpoints remain focused on preview execution (`/dry-runs`) and optional semantic validation

### Error Contract

All non-2xx responses return:

```json
{
	"error": {
		"code": "QUERY_BACKEND_UNREACHABLE",
		"message": "Loki endpoint request failed",
		"details": {
			"query": "loki-pulls",
			"endpoint": "https://loki.example.com"
		}
	}
}
```

## 7. Technology Specification (Low-Dependency, Non-React)

### Goals

- No React dependency unless proven necessary
- Keep runtime and build dependencies small
- Avoid writing large amounts of custom UI plumbing
- Preserve fast iteration for interactive previews

### Frontend Stack

- `HTMX` for request/response flows (forms, partial updates, compare panel)
- `Alpine.js` for local state and lightweight interactivity (sliders, toggles, tabs)
- `uPlot` for charts (small bundle, fast rendering for ranking/signal views)
- `Shoelace` web components for accessible primitives (drawer, tabs, input, badge, dialog)
- `yaml` (eemeli) for in-browser parse/stringify with stable formatting and good diagnostics
- Plain CSS with CSS variables; no utility framework required

Why this mix:
- Far fewer moving parts than React + state library + router
- Less custom JS than hand-written DOM manipulation
- Works well with progressively enhanced HTML fragments from the API

### Backend Stack

- Go `net/http` + `chi` router for a small API surface
- Reuse existing discovery pipeline packages for query/signal/ranking execution
- `go-playground/validator` for request validation
- Optional SSE endpoint for long dry-runs
- No YAML render/parse endpoint required

### Packaging and Deployment

- Ship UI as static assets from the same binary that exposes preview endpoints
- No Node-based SSR required
- Optional: separate `drop-ui` service if security boundaries require isolation

### State Model

- Source of truth is the current editor state of a DiscoveryPolicy spec
- Every meaningful edit can trigger a debounced dry-run
- Runs are immutable; compare is based on `baselineRunId` and current `runId`
- UI controls and YAML editor are bi-directionally synced
- Last write wins with debounce (for example 250-400ms), then re-validate and re-render canonical YAML
- Invalid YAML is never silently discarded; parser errors are shown inline with line/column info

### YAML UX Requirements

- Always-visible YAML pane (desktop split view, mobile bottom sheet)
- One-click actions: copy YAML, download `.yaml`, apply via kubectl proxy endpoint (optional)
- Canonical formatting on render to prevent noisy diffs
- Structural edits in YAML immediately update form controls when parse succeeds
- If parse fails, keep previous valid model active for previews and show non-blocking YAML error state

### Performance Targets

- Validation API p95: < 150ms
- Dry-run API p95: < 2s for standard lookback windows
- UI update after slider change: < 300ms when cached query results are reusable

### Dependency Budget

- Frontend runtime dependencies: max 5 primary libraries (HTMX, Alpine.js, uPlot, Shoelace, yaml)
- Backend dependencies: router + validator + existing project modules
- No Redux-style state framework, no heavyweight SPA framework by default

### Exit Criteria for Introducing React (Not Default)

Introduce React only if all are true:
- UI state transitions become too complex for Alpine.js patterns
- Team productivity drops due to custom client-side composition code
- Measured bundle/perf and maintenance costs are acceptable

## 8. Generator-Ready UI Contract

This section is intentionally strict so a UI generator can build a coherent first version without product guesses.

### 8.1 Information Architecture

- Single page app shell with 3 columns on desktop
- Left: policy editor (queries, signals, ranking)
- Center: preview and diagnostics (query health, charts, ranked list)
- Right: live YAML editor and export actions
- Mobile: stacked tabs in this order: Builder, Preview, YAML

### 8.2 Canonical State Shape

```json
{
	"metadata": {
		"name": "",
		"labels": {}
	},
	"spec": {
		"queries": [],
		"signals": [],
		"ranking": null,
		"imageFilter": "",
		"syncInterval": "30m",
		"maxImages": 50
	},
	"ui": {
		"activeTab": "builder",
		"selectedQuery": null,
		"selectedSignal": null,
		"baselineRunId": null,
		"currentRunId": null,
		"runStatus": "idle",
		"yamlText": "",
		"yamlParseErrors": []
	}
}
```

### 8.3 Required Components

| Component ID | Type | Required Inputs | Required Outputs |
|---|---|---|---|
| `policy-meta-form` | Form | `metadata.*`, `spec.syncInterval`, `spec.maxImages`, `spec.imageFilter` | Updates canonical state paths directly |
| `queries-list` | Repeater + editor | `spec.queries[]` | Add/remove/edit query rows |
| `signals-list` | Repeater + editor | `spec.signals[]`, `spec.queries[].name` | Add/remove/edit signal rows |
| `ranking-editor` | Conditional form | `spec.ranking`, `spec.signals[].name` | Valid ranking object by selected strategy |
| `run-controls` | Action bar | full `spec`, `ui.currentRunId` | Start run, cancel run, set baseline |
| `query-health-panel` | Status list | `preview.queryResults[]` | Per-query status, latency, sample counts |
| `signal-inspector` | Chart + table | `preview.signals[]` | Selected signal detail view |
| `ranking-table` | Table | `preview.ranking.images[]` | Sorted list with component score breakdown |
| `diff-panel` | Delta list | `preview.diff` + `ui.baselineRunId` | enteredTopN/leftTopN rendering |
| `yaml-editor` | Text editor | canonical state -> YAML text | YAML text edits -> parsed state patch |
| `yaml-actions` | Action bar | `ui.yamlText` | copy, download, optional apply |
| `validation-banner` | Alert stack | validation and parse errors | grouped blocking/non-blocking errors |

### 8.4 Event and Data-Binding Rules

- Any edit in `policy-meta-form`, `queries-list`, `signals-list`, or `ranking-editor` must:
- update canonical state
- regenerate canonical YAML text in `yaml-editor`
- trigger debounced validation and optional debounced dry-run
- Any edit in `yaml-editor` must:
- parse YAML in browser
- if parse succeeds: patch canonical state and refresh form controls
- if parse fails: keep last valid canonical state, show line/column errors, keep YAML text untouched
- Dry-run requests are only built from canonical state, never from raw YAML text

### 8.5 Action Contract

| Action | Trigger | Backend Call | Success State | Failure State |
|---|---|---|---|---|
| `validatePolicy` | debounced edit or manual click | `POST /api/v1/discovery-ui/validate` | `validation-banner` clears blocking errors | show field/path errors |
| `startDryRun` | Run Preview button | `POST /api/v1/discovery-ui/dry-runs` | set `ui.currentRunId`, `ui.runStatus=queued` | show toast + keep previous preview |
| `pollDryRun` | after `startDryRun` until terminal | `GET /api/v1/discovery-ui/dry-runs/{runId}` | update preview panels | set `ui.runStatus=failed` with error |
| `cancelDryRun` | Cancel button | `POST /api/v1/discovery-ui/dry-runs/{runId}:cancel` | `ui.runStatus=cancelled` | show non-blocking error |
| `setBaseline` | Set Baseline button | none | copy `ui.currentRunId` -> `ui.baselineRunId` | no-op |
| `copyYaml` | Copy button | none | clipboard success toast | copy error toast |
| `downloadYaml` | Download button | none | file `discoverypolicy-<name>.yaml` | download error toast |

### 8.6 Generator Constraints

- Do not invent additional fields beyond DiscoveryPolicy metadata + spec
- Do not hide unknown YAML keys; preserve them round-trip if present
- Do not auto-delete invalid user YAML; preserve text and show errors
- Show ranking strategy-specific form sections only when selected
- All list rows must support deterministic reorder and stable item keys

### 8.7 Done Criteria (UI Generator Output)

- A user can complete full flow without manual YAML typing: build -> preview -> copy/download YAML
- A user can complete full flow from YAML-first editing: paste YAML -> parsed controls -> preview
- Dry-run failures surface with query-level context, not generic errors only
- YAML output is canonical and stable across no-op edits
- Mobile layout remains usable with Builder/Preview/YAML tab switch

## 9. JSON Schema (UI Generator Contract)

Use this schema as the generator input contract.

```json
{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"$id": "https://drop.corewire.io/schemas/discovery-ui-generator.schema.json",
	"title": "Discovery UI Generator Contract",
	"type": "object",
	"required": ["metadata", "spec", "ui"],
	"additionalProperties": false,
	"properties": {
		"metadata": {
			"type": "object",
			"required": ["name"],
			"additionalProperties": true,
			"properties": {
				"name": {
					"type": "string",
					"minLength": 1
				},
				"labels": {
					"type": "object",
					"additionalProperties": {
						"type": "string"
					}
				}
			}
		},
		"spec": {
			"$ref": "#/$defs/discoveryPolicySpec"
		},
		"ui": {
			"$ref": "#/$defs/uiState"
		}
	},
	"$defs": {
		"duration": {
			"type": "string",
			"pattern": "^[0-9]+(ms|s|m|h)$"
		},
		"queryType": {
			"type": "string",
			"enum": ["prometheus", "loki", "registry"]
		},
		"signalType": {
			"type": "string",
			"enum": ["aggregate", "timeWeightedAggregate", "windowAggregate", "eventPullTime"]
		},
		"rankingStrategy": {
			"type": "string",
			"enum": ["signal", "weightedSum", "modelExposure"]
		},
		"nodeCountConfig": {
			"type": "object",
			"additionalProperties": false,
			"properties": {
				"count": { "type": "integer", "minimum": 1 },
				"selector": {
					"type": "object",
					"additionalProperties": true
				}
			}
		},
		"prometheusQuery": {
			"type": "object",
			"required": ["endpoint", "query"],
			"additionalProperties": false,
			"properties": {
				"endpoint": { "type": "string", "minLength": 1 },
				"query": { "type": "string", "minLength": 1 },
				"queryType": { "type": "string", "enum": ["range", "instant"] },
				"lookback": { "$ref": "#/$defs/duration" },
				"step": { "$ref": "#/$defs/duration" }
			}
		},
		"lokiParser": {
			"type": "object",
			"required": ["type"],
			"additionalProperties": false,
			"properties": {
				"type": { "type": "string", "enum": ["kubernetesEvents"] },
				"podField": { "type": "string" },
				"reasonField": { "type": "string" },
				"messageField": { "type": "string" },
				"imageField": { "type": "string" }
			}
		},
		"lokiQuery": {
			"type": "object",
			"required": ["endpoint", "query"],
			"additionalProperties": false,
			"properties": {
				"endpoint": { "type": "string", "minLength": 1 },
				"query": { "type": "string", "minLength": 1 },
				"queryType": { "type": "string", "enum": ["range"] },
				"lookback": { "$ref": "#/$defs/duration" },
				"parser": { "$ref": "#/$defs/lokiParser" }
			}
		},
		"registryQuery": {
			"type": "object",
			"required": ["url", "repositories"],
			"additionalProperties": false,
			"properties": {
				"url": { "type": "string", "minLength": 1 },
				"repositories": {
					"type": "array",
					"minItems": 1,
					"items": { "type": "string", "minLength": 1 }
				},
				"tagFilter": { "type": "string" },
				"tagSeek": { "type": "string" },
				"topX": { "type": "integer", "minimum": 1 },
				"maxScan": { "type": "integer", "minimum": 1 },
				"versionPattern": { "type": "string" },
				"imageTemplate": { "type": "string" }
			}
		},
		"discoveryQuery": {
			"type": "object",
			"required": ["name", "type"],
			"additionalProperties": false,
			"properties": {
				"name": { "type": "string", "minLength": 1 },
				"type": { "$ref": "#/$defs/queryType" },
				"prometheus": { "$ref": "#/$defs/prometheusQuery" },
				"loki": { "$ref": "#/$defs/lokiQuery" },
				"registry": { "$ref": "#/$defs/registryQuery" },
				"secretRef": {
					"type": "object",
					"additionalProperties": false,
					"properties": { "name": { "type": "string", "minLength": 1 } }
				}
			},
			"allOf": [
				{
					"if": { "properties": { "type": { "const": "prometheus" } } },
					"then": { "required": ["prometheus"] }
				},
				{
					"if": { "properties": { "type": { "const": "loki" } } },
					"then": { "required": ["loki"] }
				},
				{
					"if": { "properties": { "type": { "const": "registry" } } },
					"then": { "required": ["registry"] }
				}
			]
		},
		"aggregateSignalConfig": {
			"type": "object",
			"required": ["method"],
			"additionalProperties": false,
			"properties": {
				"method": { "type": "string", "enum": ["sum", "count", "avg", "max", "min"] }
			}
		},
		"timeWeightedAggregateSignalConfig": {
			"type": "object",
			"required": ["method", "timezone", "defaultWeight", "windows"],
			"additionalProperties": false,
			"properties": {
				"method": { "type": "string", "enum": ["sum", "count", "avg", "max", "min"] },
				"timezone": { "type": "string", "minLength": 1 },
				"defaultWeight": { "type": "string", "minLength": 1 },
				"windows": {
					"type": "array",
					"minItems": 1,
					"items": {
						"type": "object",
						"required": ["startHour", "endHour", "weight"],
						"additionalProperties": false,
						"properties": {
							"startHour": { "type": "integer", "minimum": 0, "maximum": 23 },
							"endHour": { "type": "integer", "minimum": 1, "maximum": 24 },
							"weight": { "type": "string", "minLength": 1 }
						}
					}
				}
			}
		},
		"windowAggregateSignalConfig": {
			"type": "object",
			"required": ["method"],
			"additionalProperties": false,
			"properties": {
				"method": { "type": "string", "enum": ["sum", "count", "avg", "max", "min"] },
				"relativeWindow": { "$ref": "#/$defs/duration" },
				"timezone": { "type": "string" },
				"window": {
					"type": "object",
					"required": ["start", "end"],
					"additionalProperties": false,
					"properties": {
						"start": { "type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$" },
						"end": { "type": "string", "pattern": "^([01][0-9]|2[0-3]):[0-5][0-9]$" }
					}
				}
			}
		},
		"eventPullTimeSignalConfig": {
			"type": "object",
			"required": ["statistic"],
			"additionalProperties": false,
			"properties": {
				"metric": { "type": "string", "enum": ["pullTime", "imageSize"] },
				"statistic": { "type": "string", "enum": ["p50", "p90", "p95", "avg", "max", "count"] }
			}
		},
		"discoverySignal": {
			"type": "object",
			"required": ["name", "query", "type"],
			"additionalProperties": false,
			"properties": {
				"name": { "type": "string", "minLength": 1 },
				"query": { "type": "string", "minLength": 1 },
				"type": { "$ref": "#/$defs/signalType" },
				"aggregate": { "$ref": "#/$defs/aggregateSignalConfig" },
				"timeWeightedAggregate": { "$ref": "#/$defs/timeWeightedAggregateSignalConfig" },
				"windowAggregate": { "$ref": "#/$defs/windowAggregateSignalConfig" },
				"eventPullTime": { "$ref": "#/$defs/eventPullTimeSignalConfig" }
			},
			"allOf": [
				{
					"if": { "properties": { "type": { "const": "aggregate" } } },
					"then": { "required": ["aggregate"] }
				},
				{
					"if": { "properties": { "type": { "const": "timeWeightedAggregate" } } },
					"then": { "required": ["timeWeightedAggregate"] }
				},
				{
					"if": { "properties": { "type": { "const": "windowAggregate" } } },
					"then": { "required": ["windowAggregate"] }
				},
				{
					"if": { "properties": { "type": { "const": "eventPullTime" } } },
					"then": { "required": ["eventPullTime"] }
				}
			]
		},
		"weightedSumTerm": {
			"type": "object",
			"required": ["signal", "weight"],
			"additionalProperties": false,
			"properties": {
				"signal": { "type": "string", "minLength": 1 },
				"weight": { "type": "string", "minLength": 1 }
			}
		},
		"ranking": {
			"type": "object",
			"required": ["strategy"],
			"additionalProperties": false,
			"properties": {
				"strategy": { "$ref": "#/$defs/rankingStrategy" },
				"signal": { "type": "string", "minLength": 1 },
				"weightedSum": {
					"type": "object",
					"required": ["normalize", "missingSignal", "terms"],
					"additionalProperties": false,
					"properties": {
						"normalize": { "type": "string", "enum": ["minMax"] },
						"missingSignal": { "type": "string", "enum": ["zero", "drop"] },
						"terms": {
							"type": "array",
							"minItems": 1,
							"items": { "$ref": "#/$defs/weightedSumTerm" }
						}
					}
				},
				"modelExposure": {
					"type": "object",
					"required": ["preWindowUsageSignal", "targetWindowUsageSignal", "pullTimeSignal"],
					"additionalProperties": false,
					"properties": {
						"nodes": { "$ref": "#/$defs/nodeCountConfig" },
						"preWindowUsageSignal": { "type": "string", "minLength": 1 },
						"targetWindowUsageSignal": { "type": "string", "minLength": 1 },
						"pullTimeSignal": { "type": "string", "minLength": 1 }
					}
				}
			},
			"allOf": [
				{
					"if": { "properties": { "strategy": { "const": "signal" } } },
					"then": { "required": ["signal"] }
				},
				{
					"if": { "properties": { "strategy": { "const": "weightedSum" } } },
					"then": { "required": ["weightedSum"] }
				},
				{
					"if": { "properties": { "strategy": { "const": "modelExposure" } } },
					"then": { "required": ["modelExposure"] }
				}
			]
		},
		"discoveryPolicySpec": {
			"type": "object",
			"required": ["syncInterval", "maxImages"],
			"additionalProperties": true,
			"properties": {
				"queries": {
					"type": "array",
					"items": { "$ref": "#/$defs/discoveryQuery" }
				},
				"signals": {
					"type": "array",
					"items": { "$ref": "#/$defs/discoverySignal" }
				},
				"ranking": { "$ref": "#/$defs/ranking" },
				"imageFilter": { "type": "string" },
				"syncInterval": { "$ref": "#/$defs/duration" },
				"maxImages": { "type": "integer", "minimum": 1 }
			}
		},
		"uiState": {
			"type": "object",
			"required": [
				"activeTab",
				"selectedQuery",
				"selectedSignal",
				"baselineRunId",
				"currentRunId",
				"runStatus",
				"yamlText",
				"yamlParseErrors"
			],
			"additionalProperties": false,
			"properties": {
				"activeTab": { "type": "string", "enum": ["builder", "preview", "yaml"] },
				"selectedQuery": { "type": ["string", "null"] },
				"selectedSignal": { "type": ["string", "null"] },
				"baselineRunId": { "type": ["string", "null"] },
				"currentRunId": { "type": ["string", "null"] },
				"runStatus": {
					"type": "string",
					"enum": ["idle", "queued", "running", "succeeded", "failed", "cancelled"]
				},
				"yamlText": { "type": "string" },
				"yamlParseErrors": {
					"type": "array",
					"items": {
						"type": "object",
						"required": ["message"],
						"additionalProperties": false,
						"properties": {
							"line": { "type": "integer", "minimum": 1 },
							"column": { "type": "integer", "minimum": 1 },
							"message": { "type": "string", "minLength": 1 }
						}
					}
				}
			}
		}
	}
}
```

### 9.1 Notes for Generator Authors

- The schema intentionally allows unknown `spec` keys via `additionalProperties: true` to preserve round-trip YAML fields.
- `modelExposure.nodes.selector` is intentionally open-typed in this schema; pass through the Kubernetes `NodeSelector` object as-is.
- `modelExposure.nodes.selector` takes precedence over `modelExposure.nodes.count`; `count` is fallback/default `N`.
- Enforce cross-reference checks in generator/runtime logic:
	- `signals[].query` must reference an existing `queries[].name`
	- ranking signal references must point to existing `signals[].name`
- Use this schema for structure validation, then call `/validate` for semantic validation.

## 10. Sample Data

Use these payloads for UI generator integration tests and local demos.

### 10.1 Sample Generator Input State

```json
{
	"metadata": {
		"name": "dev-hybrid",
		"labels": {
			"app.kubernetes.io/managed-by": "drop-ui",
			"drop.corewire.io/env": "dev"
		}
	},
	"spec": {
		"queries": [
			{
				"name": "prom-usage",
				"type": "prometheus",
				"prometheus": {
					"endpoint": "http://prometheus.e2e-infra.svc.cluster.local:9090",
					"query": "sum(rate(container_cpu_usage_seconds_total{image!=""}[5m])) by (image)",
					"queryType": "range",
					"lookback": "24h",
					"step": "5m"
				}
			},
			{
				"name": "loki-pulls",
				"type": "loki",
				"loki": {
					"endpoint": "http://loki.e2e-infra.svc.cluster.local:3100",
					"query": "{job=\"kubelet\",drop_e2e=\"true\"} | json | reason=\"Pulled\"",
					"queryType": "range",
					"lookback": "7d",
					"parser": {
						"type": "kubernetesEvents",
						"podField": "name",
						"reasonField": "reason",
						"messageField": "msg",
						"imageField": "msg"
					}
				}
			}
		],
		"signals": [
			{
				"name": "usage-count",
				"query": "prom-usage",
				"type": "aggregate",
				"aggregate": {
					"method": "sum"
				}
			},
			{
				"name": "pull-time-p95",
				"query": "loki-pulls",
				"type": "eventPullTime",
				"eventPullTime": {
					"metric": "pullTime",
					"statistic": "p95"
				}
			}
		],
		"ranking": {
			"strategy": "weightedSum",
			"weightedSum": {
				"normalize": "minMax",
				"missingSignal": "zero",
				"terms": [
					{
						"signal": "usage-count",
						"weight": "0.6"
					},
					{
						"signal": "pull-time-p95",
						"weight": "0.4"
					}
				]
			}
		},
		"imageFilter": "^docker\\.io/.+",
		"syncInterval": "30m",
		"maxImages": 30
	},
	"ui": {
		"activeTab": "builder",
		"selectedQuery": "loki-pulls",
		"selectedSignal": "pull-time-p95",
		"baselineRunId": null,
		"currentRunId": null,
		"runStatus": "idle",
		"yamlText": "",
		"yamlParseErrors": []
	}
}
```

### 10.2 Sample Validate Response

```json
{
	"valid": true,
	"errors": [],
	"warnings": [
		{
			"path": "spec.queries[1].loki.lookback",
			"code": "LARGE_LOOKBACK",
			"message": "Lookback is large and may increase preview latency"
		}
	]
}
```

### 10.3 Sample Dry-Run Start Response

```json
{
	"runId": "dr_01J7M8P0D8WM7SPSAF2VY2H2PS",
	"status": "queued"
}
```

### 10.4 Sample Dry-Run Completed Response

```json
{
	"runId": "dr_01J7M8P0D8WM7SPSAF2VY2H2PS",
	"status": "succeeded",
	"timing": {
		"totalMs": 1188,
		"queryMs": 801,
		"signalMs": 201,
		"rankingMs": 186
	},
	"queryResults": [
		{
			"name": "prom-usage",
			"type": "prometheus",
			"status": "success",
			"seriesCount": 104,
			"sampleCount": 2864
		},
		{
			"name": "loki-pulls",
			"type": "loki",
			"status": "success",
			"seriesCount": 391,
			"sampleCount": 2140
		}
	],
	"signals": [
		{
			"name": "usage-count",
			"type": "aggregate",
			"values": [
				{
					"image": "docker.io/library/nginx:1.25-alpine",
					"value": 93.1,
					"sampleCount": 182
				}
			]
		},
		{
			"name": "pull-time-p95",
			"type": "eventPullTime",
			"metric": "pullTime",
			"statistic": "p95",
			"values": [
				{
					"image": "docker.io/library/nginx:1.25-alpine",
					"value": 11.7,
					"sampleCount": 39
				}
			]
		}
	],
	"ranking": {
		"strategy": "weightedSum",
		"images": [
			{
				"image": "docker.io/library/nginx:1.25-alpine",
				"rank": 1,
				"finalScore": "0.8942",
				"components": [
					{
						"signal": "usage-count",
						"raw": 93.1,
						"normalized": 1.0,
						"weight": "0.6",
						"weighted": 0.6
					},
					{
						"signal": "pull-time-p95",
						"raw": 11.7,
						"normalized": 0.7355,
						"weight": "0.4",
						"weighted": 0.2942
					}
				]
			}
		]
	},
	"diff": {
		"baselineRunId": "dr_01J7M7ZYTTZHAX11K9Q3Y8DCPX",
		"enteredTopN": [
			"ghcr.io/acme/api:v2.4.1"
		],
		"leftTopN": [
			"ghcr.io/acme/worker:v2.3.9"
		]
	}
}
```

### 10.5 Sample YAML Output

```yaml
apiVersion: drop.corewire.io/v1alpha1
kind: DiscoveryPolicy
metadata:
	name: dev-hybrid
	labels:
		app.kubernetes.io/managed-by: drop-ui
		drop.corewire.io/env: dev
spec:
	imageFilter: ^docker\.io/.+
	syncInterval: 30m
	maxImages: 30
	queries:
		- name: prom-usage
			type: prometheus
			prometheus:
				endpoint: http://prometheus.e2e-infra.svc.cluster.local:9090
				query: sum(rate(container_cpu_usage_seconds_total{image!=""}[5m])) by (image)
				queryType: range
				lookback: 24h
				step: 5m
		- name: loki-pulls
			type: loki
			loki:
				endpoint: http://loki.e2e-infra.svc.cluster.local:3100
				query: '{job="kubelet",drop_e2e="true"} | json | reason="Pulled"'
				queryType: range
				lookback: 7d
				parser:
					type: kubernetesEvents
					podField: name
					reasonField: reason
					messageField: msg
					imageField: msg
	signals:
		- name: usage-count
			query: prom-usage
			type: aggregate
			aggregate:
				method: sum
		- name: pull-time-p95
			query: loki-pulls
			type: eventPullTime
			eventPullTime:
				metric: pullTime
				statistic: p95
	ranking:
		strategy: weightedSum
		weightedSum:
			normalize: minMax
			missingSignal: zero
			terms:
				- signal: usage-count
					weight: "0.6"
				- signal: pull-time-p95
					weight: "0.4"
```

### 10.6 Sample YAML Parse Error Payload

```json
{
	"yamlParseErrors": [
		{
			"line": 21,
			"column": 7,
			"message": "mapping values are not allowed in this context"
		}
	]
}
```

### 10.7 Sample ModelExposure Snippet (Latest)

```yaml
ranking:
	strategy: modelExposure
	modelExposure:
		nodes:
			count: 8
			selector:
				nodeSelectorTerms:
					- matchExpressions:
							- key: node-role.kubernetes.io/worker
								operator: Exists
		preWindowUsageSignal: usage-pre
		targetWindowUsageSignal: usage-target
		pullTimeSignal: pull-time-p95
```

Behavior:
- If `nodes.selector` is set, node count is resolved dynamically from Ready nodes matching the selector.
- If selector resolution fails or selector is unset, `nodes.count` is used as fallback.
