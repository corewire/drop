#!/usr/bin/env bash
set -euo pipefail

# Puller Operator Demo Script
# This script demonstrates the operator's end-to-end functionality using a kind cluster.
# Prerequisites: kind, kubectl, helm, docker

BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() { echo -e "${BLUE}[demo]${NC} $*"; }
success() { echo -e "${GREEN}[✓]${NC} $*"; }
section() { echo -e "\n${BOLD}${YELLOW}=== $* ===${NC}\n"; }

CLUSTER_NAME="puller-demo"
IMG="controller:demo"
NAMESPACE="puller-system"

cleanup() {
    log "Cleaning up..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
}

trap cleanup EXIT

section "1. Create Kind Cluster"
if kind get clusters 2>/dev/null | grep -q "$CLUSTER_NAME"; then
    log "Cluster $CLUSTER_NAME already exists, reusing."
else
    kind create cluster --name "$CLUSTER_NAME" --wait 60s
fi
success "Kind cluster ready"

section "2. Build and Load Operator Image"
docker build -t "$IMG" .
kind load docker-image "$IMG" --name "$CLUSTER_NAME"
success "Operator image loaded into kind"

section "3. Install CRDs"
make manifests
kubectl apply -f config/crd/bases/
success "CRDs installed"

section "4. Deploy Operator via Helm"
helm upgrade --install puller charts/puller \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --set image.repository=controller \
    --set image.tag=demo \
    --set image.pullPolicy=Never \
    --set leaderElection.enabled=false \
    --set metrics.enabled=true \
    --set metrics.secureServing=false \
    --wait --timeout 60s
success "Operator deployed"

kubectl -n "$NAMESPACE" get pods
echo ""

section "5. Create a PullPolicy (conservative pacing)"
cat <<EOF | kubectl apply -f -
apiVersion: puller.corewire.io/v1alpha1
kind: PullPolicy
metadata:
  name: demo-policy
spec:
  maxConcurrentNodes: 1
  minDelayBetweenPulls: 5s
  failureBackoff: 30s
EOF
success "PullPolicy created"

section "6. Create a CachedImage"
cat <<EOF | kubectl apply -f -
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImage
metadata:
  name: demo-nginx
spec:
  image: docker.io/library/nginx:1.25-alpine
  pullPolicy: IfNotPresent
  policyRef:
    name: demo-policy
EOF
success "CachedImage created"

section "7. Watch Operator Progress"
log "Waiting for image to be cached..."
for i in $(seq 1 30); do
    phase=$(kubectl get cachedimage demo-nginx -o jsonpath='{.status.phase}' 2>/dev/null || echo "Pending")
    if [ "$phase" = "Ready" ]; then
        success "Image cached successfully!"
        break
    fi
    echo "  Status: $phase (attempt $i/30)"
    sleep 2
done

section "8. Check Events"
kubectl get events --field-selector involvedObject.name=demo-nginx --sort-by='.lastTimestamp' 2>/dev/null || log "No events yet"

section "9. Check Final Status"
kubectl get cachedimage demo-nginx -o yaml | grep -A20 "^status:"

section "10. Create a CachedImageSet"
cat <<EOF | kubectl apply -f -
apiVersion: puller.corewire.io/v1alpha1
kind: CachedImageSet
metadata:
  name: demo-set
spec:
  images:
    - docker.io/library/alpine:3.19
    - docker.io/library/busybox:1.36
  policyRef:
    name: demo-policy
EOF
success "CachedImageSet created"

log "Waiting for child CachedImages..."
sleep 5
kubectl get cachedimages -l puller.corewire.io/imageset=demo-set

section "11. Verify Metrics"
OPERATOR_POD=$(kubectl -n "$NAMESPACE" get pods -l app.kubernetes.io/name=puller -o jsonpath='{.items[0].metadata.name}')
log "Operator pod: $OPERATOR_POD"
log "Port-forwarding metrics..."
kubectl -n "$NAMESPACE" port-forward "$OPERATOR_POD" 8080:8443 &
PF_PID=$!
sleep 2
echo ""
log "Custom metrics:"
curl -sk https://localhost:8080/metrics 2>/dev/null | grep "^puller_" || curl -s http://localhost:8080/metrics 2>/dev/null | grep "^puller_" || log "Could not reach metrics endpoint"
kill $PF_PID 2>/dev/null || true

section "Demo Complete!"
echo ""
echo "Resources created:"
kubectl get cachedimages
echo ""
kubectl get pullpolicies
echo ""
log "Run 'kind delete cluster --name $CLUSTER_NAME' to clean up."
