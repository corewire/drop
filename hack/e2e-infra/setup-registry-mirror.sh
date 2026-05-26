#!/usr/bin/env bash
# Configure containerd on kind nodes to reach the in-cluster registry.
# Kubelet can't resolve cluster DNS — this creates a containerd mirror
# that routes registry.e2e-infra.svc.cluster.local:5000 → ClusterIP.
set -euo pipefail

NAMESPACE="e2e-infra"
REGISTRY_HOST="registry.e2e-infra.svc.cluster.local:5000"

# Wait for registry to be ready
kubectl -n "$NAMESPACE" wait --for=condition=available deployment/registry --timeout=90s

REGISTRY_IP=$(kubectl -n "$NAMESPACE" get svc registry -o jsonpath='{.spec.clusterIP}')
echo "[registry-mirror] Configuring containerd: $REGISTRY_HOST → $REGISTRY_IP"

for node in $(kind get nodes --name drop-dev 2>/dev/null); do
  docker exec "$node" mkdir -p "/etc/containerd/certs.d/$REGISTRY_HOST"
  cat <<EOF | docker exec -i "$node" tee "/etc/containerd/certs.d/$REGISTRY_HOST/hosts.toml" > /dev/null
[host."http://$REGISTRY_IP:5000"]
  capabilities = ["pull", "resolve"]
  skip_verify = true
EOF
  echo "  ✓ $node"
done

echo "[registry-mirror] Done. Nodes can now pull from $REGISTRY_HOST"
