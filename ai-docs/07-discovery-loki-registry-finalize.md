# Discovery Docs Finalization — Loki & Registry

Baseline: discovery page at `docs/content/docs/discovery.md`. Prometheus is fully
documented (query → "How it's used" → "What happens"). Loki and Registry are stub
examples without explanatory prose. This plan finalizes them to match the Prometheus
depth, then verifies against the running cluster.

Scope: docs only. The engine already implements all three sources
(`internal/discovery/{prometheus,loki,registry}.go`); no code changes expected unless
verification exposes a doc/runtime mismatch.

## Goal

- Loki Query section explains the `kubernetesEvents` parser, field mappings, event
  reasons, and the two duration modes — not just a YAML blob.
- Registry Query section explains `imageTemplate`, `tagFilter`, `topX` semantics, and
  the per-image score it produces.
- Add a signal × source compatibility note so `eventPullTime` (Loki-only) is explicit.
- Every example verified against tilt loki + registry e2e infra; no unverified claims.

## Current State (line anchors)

- Loki Query: discovery.md L130–171 — example only, no prose.
- Registry Query: discovery.md L172–204 — example only, no prose.
- Auth/TLS: L206–237 — fine, leave as is but also document registry authentication. it's also manadatory to explain the secret content.
- eventPullTime signal: L439–507 — outlier example fixed; statistic table correct.

## Plan

1. Loki Query prose (after the example, mirror Prometheus structure):
   - "How it's used": kubernetesEvents parser maps log fields → structured event records
     (`podField`, `reasonField`, `messageField`, `imageField`); defaults
     `involvedObject_name` / `reason` / `message` / `message`.
   - Reasons consumed: `Pulled` only.
   - Duration and size are parsed from the Pulled message (`in 42.3s`, `Image size: N bytes`).
   - Note: only `eventPullTime` consumes this structure; aggregate-family signals see
     generic per-image samples.
   - **Alloy ingestion setup** (how pull events reach Loki). Add a short "Shipping
     Kubernetes events to Loki" block with the Grafana Alloy approach:
     - Component `loki.source.kubernetes_events` watches Events, forwards to
       `loki.write`. Set `log_format = "json"` so reason/name/message are parsable.
       Ref: https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.kubernetes_events/
       and https://grafana.com/docs/alloy/latest/reference/components/loki/loki.write/
     - Alloy adds labels `namespace`/`job`/`instance` only; event fields land in the
       JSON body. Cluster-scope watch needs a ClusterRoleBinding for events/events.k8s.io.
     - **Field-mapping caveat (verified):** Alloy json keys are `name` (pod) and `msg`
       (message), NOT `involvedObject_name`/`message`. So set `podField: name`,
       `messageField: msg`, `imageField: msg`; raw event-exporter json keeps the
       `involvedObject_name`/`message` defaults. Document both. Working example:
       `hack/e2e-infra/alloy.yaml`.
     - Mention non-Alloy alternative: event-exporter pushing raw event JSON.
2. Registry Query prose:
   - Lists `/v2/{repo}/tags/list`, filters by `tagFilter` regex, keeps last `topX`
     returned (registry order — not true recency; cross-link existing topX caveat).
   - `imageTemplate` vars `{{.Registry}}` `{{.Repository}}` `{{.Tag}}`, default
     `{{.Registry}}/{{.Repository}}:{{.Tag}}`.
   - Pairs with `aggregate` (count/sum); not compatible with `eventPullTime`.
3. Compatibility matrix: aggregate/timeWeighted/windowAggregate = any source;
   eventPullTime = Loki kubernetesEvents only.
4. Graphics readability pass (`docs/static/images/signal-*.svg`):
   - Legend always anchored top-right, in a single consistent box; show
     image color↔name (img-A/img-B) plus statistic markers (min/max/avg dots).
   - Drop per-dot value labels (e.g. "A max 6", "B min 0"); a colored dot + the
     legend is enough. Keep only the summary Σ/p50/max readout.
   - signal-aggregate.svg: remove the 4 min/max value texts; legend top-right.
   - signal-timeweighted.svg + signal-windowaggregate.svg: move img-A/img-B legend
     to top-right, align weight/window key with it.
   - signal-eventpulltime.svg: keep p50/max markers, ensure 4.1s bar + labels
     don't collide; legend top-right.
   - Verify every SVG renders inside viewBox (no clipped text, no overlap).
5. Regenerate AI docs: `make docs-gen`.
6. Rebuild Hugo: `cd docs && hugo`.
7. Verify on cluster: apply `dev-loki` + `dev-registry` from `hack/dev-samples.yaml`,
   confirm policy status; verify Alloy is shipping real pull events to Loki
   (`hack/e2e-infra/alloy.yaml`); run `make test-e2e` (discovery-loki, discovery-registry).

## Verify

- discovery.md sections parity: each source has example + "how used" + caveats.
- `grep -n queryRef\|signalRef` clean.
- All signal-*.svg: legend top-right, no per-dot value clutter, no clipped/overlapping text.
- `make docs-gen` no diff drift; hugo builds.
- e2e discovery-loki + discovery-registry green.

## Recovery

Docs-only; revert `git restore -- docs/ ai-docs/` if needed. Never hand-edit generated
files (`llms*.txt`, `knowledge.yaml`, charts CRDs) — rerun `make docs-gen`.
