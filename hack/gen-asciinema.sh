#!/usr/bin/env bash
# hack/gen-asciinema.sh — Generate asciinema .cast files for docs landing page.
# Requires: asciinema, kubectl, a running cluster with drop installed.
# Output: docs/static/casts/{apply,pods,events}.cast — displayed as tabs on site.
#
# Each recording is fully independent: clean state → apply → watch one perspective.
set -euo pipefail

CAST_DIR="$(git rev-parse --show-toplevel)/docs/static/casts"
mkdir -p "$CAST_DIR"

TMPFILE="/tmp/drop-demo-cachedimage.yaml"
cat > "$TMPFILE" <<'EOF'
apiVersion: drop.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: nginx-demo
spec:
  image: docker.io/library/nginx
  tag: "1.27"
  nodeSelector:
    kubernetes.io/os: linux
EOF

cleanup() {
  kubectl delete cachedimage nginx-demo --ignore-not-found >/dev/null 2>&1 || true
  kubectl delete pods -l app.kubernetes.io/managed-by=drop --ignore-not-found >/dev/null 2>&1 || true
  sleep 5
}

# ─── Recording 1: Apply manifest + watch CachedImage status ───────────────────
cleanup
echo "Recording 1/3: apply + status"
asciinema rec "$CAST_DIR/apply.cast" --overwrite --cols 80 --rows 22 --env "" -c "bash --norc --noprofile <<'REC'
echo '$ cat cachedimage.yaml'
sleep 1
cat $TMPFILE
sleep 3
echo ''
echo '$ kubectl apply -f cachedimage.yaml'
kubectl apply -f $TMPFILE
sleep 2
echo ''
echo '$ kubectl get cachedimages nginx-demo -w'
kubectl get cachedimages nginx-demo -w &
PID=\$!
sleep 12
kill \$PID 2>/dev/null || true
sleep 3
printf '\n'
REC"

# ─── Recording 2: Watch pods with node placement ─────────────────────────────
cleanup
echo "Recording 2/3: pods + nodes"
asciinema rec "$CAST_DIR/pods.cast" --overwrite --cols 80 --rows 22 --env "" -c "bash --norc --noprofile <<'REC'
echo '$ kubectl get pods -l app.kubernetes.io/managed-by=drop -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,NODE:.spec.nodeName -w'
sleep 1
kubectl get pods -l app.kubernetes.io/managed-by=drop -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,NODE:.spec.nodeName -w &
PID=\$!
sleep 2
kubectl apply -f $TMPFILE >/dev/null 2>&1
sleep 12
kill \$PID 2>/dev/null || true
sleep 3
printf '\n'
REC"

# ─── Recording 3: Watch Kubernetes events ────────────────────────────────────
cleanup
echo "Recording 3/3: events"
asciinema rec "$CAST_DIR/events.cast" --overwrite --cols 120 --rows 22 --env "" -c "bash --norc --noprofile <<'REC'
echo '$ kubectl get events --field-selector reason!=LeaderElection --watch-only'
sleep 1
kubectl get events --field-selector reason!=LeaderElection --watch-only &
PID=\$!
sleep 2
kubectl apply -f $TMPFILE >/dev/null 2>&1
sleep 12
kill \$PID 2>/dev/null || true
sleep 3
printf '\n'
REC"

rm -f "$TMPFILE"
echo "✓ Generated: $CAST_DIR/{apply,pods,events}.cast"
