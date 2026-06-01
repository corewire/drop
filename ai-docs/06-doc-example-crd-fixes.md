# Doc Example and CRD Description Fix Plan

Baseline commit: `96d972f` (`docs: add full examples and CRD descriptions`)

## Goal

Fix the issues found after adding full README examples and richer CRD descriptions:

- `kubectl explain` must show updated descriptions from regenerated CRD schemas.
- Discovery `secretRef` docs must match runtime behavior.
- Registry `topX` docs must not claim creation-date or true recency semantics.
- AI docs (`llms-full.txt`, `knowledge.yaml`, generated reference pages) must stay generated from source.

## Plan

1. Keep the current docs/example commit as the rollback point.
2. Wire `DiscoveryPolicyReconciler` to use the configured Drop pod namespace for `secretRef` lookup instead of hardcoded `kube-system`.
3. Update source comments and README examples:
   - Secrets live in the Drop pod namespace (default `drop-system`).
   - Supported Discovery secret keys include `token`, `username`, `password`, `ca.crt`, `tls.crt`, `tls.key`, and `headers.<name>`.
   - Registry `topX` keeps the last N matching tags returned by the registry API.
4. Regenerate CRDs and Helm CRD templates with `make manifests sync-crds`.
5. Regenerate AI docs with `make docs-gen`.
6. Verify with `go test ./...` or `make test` if envtest assets are available, plus `make docs-gen-check`.

## Recovery

If the fix goes sideways, first save any useful work on a branch, then restore back to the baseline docs commit:

```bash
git switch -c rescue-doc-fix
git switch main
git restore --source 96d972f -- .
```

Do not manually edit generated files. Regenerate them from source instead.