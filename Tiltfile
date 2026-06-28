# Tiltfile for local development with kind
# Usage: tilt up

# Ensure kind cluster exists (1 control-plane + 2 workers)
local('kind get clusters | grep -q drop-dev || kind create cluster --name drop-dev --config hack/kind-config.yaml --wait 5m')
# Export kubeconfig for the drop-dev cluster and switch context
local('kind export kubeconfig --name drop-dev --kubeconfig .kubeconfig')
os.putenv('KUBECONFIG', os.path.join(config.main_dir, '.kubeconfig'))
allow_k8s_contexts('kind-drop-dev')

# Build the operator binary and image, then load into kind
local_resource(
    'compile',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/manager cmd/main.go',
    deps=['cmd', 'internal', 'api'],
    labels=['operator'],
)

# Build and load the image into kind
custom_build(
    'controller',
    'docker build -t $EXPECTED_REF . && kind load docker-image $EXPECTED_REF --name drop-dev',
    deps=['cmd', 'internal', 'api', 'go.mod', 'go.sum'],
    ignore=['bin/'],
)

# --- cert-manager ---
# Install cert-manager for metrics TLS certificates
local('kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.2/cert-manager.yaml')
local('kubectl wait --for=condition=Available --timeout=120s deployment/cert-manager -n cert-manager')
local('kubectl wait --for=condition=Available --timeout=120s deployment/cert-manager-webhook -n cert-manager')
# Self-signed ClusterIssuer for dev/test
k8s_yaml('hack/e2e-infra/cert-manager-issuer.yaml')

# Ensure drop-system namespace exists
local('kubectl create namespace drop-system --dry-run=client -o yaml | kubectl apply -f -')

# Install CRDs
k8s_yaml(kustomize('config/crd'))

# Deploy operator via Helm
k8s_yaml(helm(
    'charts/drop',
    name='drop',
    namespace='drop-system',
    set=[
        'image.repository=controller',
        'image.tag=latest',
        'image.pullPolicy=IfNotPresent',
        'leaderElection.enabled=false',
        'metrics.enabled=true',
        'metrics.secureServing=true',
        'certManager.enabled=true',
        'certManager.issuerRef.name=selfsigned-issuer',
        'certManager.issuerRef.kind=ClusterIssuer',
        'crds.install=false',
    ],
))

# Port-forward metrics
k8s_resource('drop', port_forwards=['8443:8443', '8081:8081'],
    objects=[
        'drop:serviceaccount',
        'drop:clusterrole',
        'drop:clusterrolebinding',
        'drop-metrics-reader:clusterrole',
        'drop-metrics-cert:certificate',
        'selfsigned-issuer:clusterissuer',
        'cachedimages.drop.corewire.io:customresourcedefinition',
        'cachedimagesets.drop.corewire.io:customresourcedefinition',
        'discoverypolicies.drop.corewire.io:customresourcedefinition',
        'pullpolicies.drop.corewire.io:customresourcedefinition',
    ],
    labels=['operator'],
    resource_deps=['compile'],
)

# --- E2E Infrastructure: Prometheus + Registry ---
# Create namespace imperatively — Tilt must NOT manage it as an object,
# otherwise force-updates delete the NS and cascade-kill everything in it.
local('kubectl create namespace e2e-infra --dry-run=client -o yaml | kubectl apply -f -')
k8s_yaml('hack/e2e-infra/prometheus-config.yaml')
k8s_yaml('hack/e2e-infra/prometheus.yaml')
k8s_yaml('hack/e2e-infra/registry.yaml')
k8s_yaml('hack/e2e-infra/loki.yaml')

k8s_resource('prometheus', objects=['prometheus-config:configmap', 'prometheus:serviceaccount', 'prometheus-metrics-reader:clusterrolebinding'], port_forwards=['9090:9090'], labels=['infra'])
k8s_resource('registry', port_forwards=['5000:5000'], labels=['infra'])
k8s_resource('loki', objects=['loki-config:configmap'], port_forwards=['3100:3100'], labels=['infra'])

# Configure kind nodes to reach the in-cluster registry.
# Kubelet/containerd can't resolve cluster DNS, so we point them at the registry's ClusterIP.
local_resource(
    'registry-mirror',
    'hack/e2e-infra/setup-registry-mirror.sh',
    labels=['infra'],
    resource_deps=['registry'],
)

# Seed registry with test images
k8s_yaml('hack/e2e-infra/seed-registry-job.yaml')
k8s_resource('seed-registry', labels=['infra'], resource_deps=['registry-mirror'])

# Seed Loki with image-pull events
k8s_yaml('hack/e2e-infra/seed-loki-job.yaml')
k8s_resource('seed-loki', labels=['infra'], resource_deps=['loki'])

# --- Grafana with Drop dashboard ---
# Create dashboard ConfigMap from the shipped JSON, then apply grafana manifests.
dashboard_json = str(read_file('charts/drop/dashboards/drop-operator.json'))
# Indent each line for YAML embedding
indented = '\n'.join(['    ' + line for line in dashboard_json.split('\n')])
k8s_yaml(blob("""
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboards
  namespace: e2e-infra
data:
  drop-operator.json: |
""" + indented))

k8s_yaml('hack/e2e-infra/grafana.yaml')

# Keep ConfigMaps in a separate resource so force-updating grafana doesn't delete them
k8s_resource(
    new_name='grafana-config',
    objects=['grafana-datasources:configmap', 'grafana-dashboards-config:configmap', 'grafana-dashboards:configmap'],
    labels=['infra'],
)
k8s_resource('grafana',
    port_forwards=['3000:3000'],
    labels=['infra'],
    resource_deps=['grafana-config'],
)

# --- Documentation: Hugo Hextra (live reload) ---
local_resource(
    'docs',
    serve_cmd='cd docs && hugo server --buildDrafts --port 1314 --bind 0.0.0.0',
    deps=['docs/content', 'docs/hugo.yaml'],
    links=['http://localhost:1314/drop/'],
    labels=['docs'],
)

# --- Dev Sample Resources ---
# Deploy sample CRs to exercise the operator
k8s_yaml('hack/dev-samples.yaml')
k8s_resource(
    new_name='samples',
    objects=[
        'dev-conservative:pullpolicy',
        'dev-nginx:cachedimage',
        'dev-redis:cachedimage',
        'test-invalid-image:cachedimage',
        'dev-set:cachedimageset',
        'dev-set-discovered:cachedimageset',
        'dev-prometheus:discoverypolicy',
        'dev-prometheus-instant:discoverypolicy',
        'dev-hybrid:discoverypolicy',
        'dev-timeweighted:discoverypolicy',
        'dev-window:discoverypolicy',
        'dev-loki:discoverypolicy',
        'dev-registry:discoverypolicy',
        'dev-modelexposure:discoverypolicy',
        'test-broken-prom:discoverypolicy',
        'test-broken-registry:discoverypolicy',
        'test-notfound-repo:discoverypolicy',
    ],
    labels=['samples'],
    resource_deps=['drop'],
)

# Button to wipe cached images from nodes (triggers resync)
local_resource(
    'cleanup-node-images',
    'kubectl delete cachedimages --all && echo "Deleted all CachedImages — operator will resync"',
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    labels=['samples'],
)
