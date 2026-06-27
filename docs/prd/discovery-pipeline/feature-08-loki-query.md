# Feature 08: Loki query source

**Parent PRD:** [Query → Signal → Ranking Pipeline](README.md) · **Status:** ⬜ Not started

## Summary

Add a `type: loki` query that runs a Loki range query and emits a normalized event
stream:

```text
timestamp,pod,image,reason,message
```

```yaml
queries:
  - name: image-pull-events
    type: loki
    loki:
      endpoint: https://loki.example.com
      queryType: range
      lookback: 168h
      query: |
        {job="kubernetes-events", namespace="gitlab-runner"}
        | json
        | involvedObject_name =~ "runner-.*"
        | reason =~ "Pulling|Pulled|Failed|BackOff"
      parser:
        type: kubernetesEvents
        podField: involvedObject_name
        reasonField: reason
        messageField: message
        imageField: message
```

## Motivation

Prometheus does not expose useful per-image pull durations. Kubernetes image-pull events
in Loki provide the raw data needed for the pull-time signal (Feature 09).

## Current state

- ⬜ Not implemented. Only `prometheus` and `registry` sources exist; there is no Loki
  client.

## Scope

- Loki range-query client with `endpoint`, `lookback`, `query`, and secret-based auth
  (reuse the existing secret pattern).
- `kubernetesEvents` parser extracting `pod`, `image`, `reason`, `message` from event
  log lines.
- Emit normalized `timestamp,pod,image,reason,message` records.

## Out of scope

- Pull-time statistics derived from the events (Feature 09).

## Acceptance criteria

- [ ] `type: loki` query returns normalized event records.
- [ ] `kubernetesEvents` parser extracts image refs from event messages
      (Pulling/Pulled/Failed/BackOff/already-present).
- [ ] Auth (token/basic/TLS/headers) works via `secretRef`.
- [ ] Query failures surfaced in status (Feature 06).
