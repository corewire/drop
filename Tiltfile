# Tiltfile for local development with kind
# Usage: tilt up

load('ext://restart_process', 'docker_build_with_restart')

# Build the operator binary
local_resource(
    'compile',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/manager cmd/main.go',
    deps=['cmd', 'internal', 'api'],
)

# Build container image and deploy
docker_build_with_restart(
    'controller:latest',
    '.',
    dockerfile='Dockerfile',
    entrypoint=['/manager'],
    live_update=[
        sync('./bin/manager', '/manager'),
    ],
)

# Install CRDs
k8s_yaml(kustomize('config/crd'))

# Deploy operator via Helm
k8s_yaml(helm(
    'charts/puller',
    name='puller',
    namespace='puller-system',
    set=[
        'image.repository=controller',
        'image.tag=latest',
        'image.pullPolicy=Never',
        'leaderElection.enabled=false',
        'metrics.enabled=true',
        'metrics.secureServing=false',
    ],
))

# Port-forward metrics
k8s_resource('puller', port_forwards=['8443:8443', '8081:8081'])

# --- E2E Infrastructure: Prometheus + Registry ---
# Deploy local Prometheus with seeded image metrics
k8s_yaml('hack/e2e-infra/prometheus-config.yaml')
k8s_yaml('hack/e2e-infra/prometheus.yaml')
k8s_yaml('hack/e2e-infra/registry.yaml')

k8s_resource('prometheus', port_forwards=['9090:9090'], labels=['infra'])
k8s_resource('registry', port_forwards=['5000:5000'], labels=['infra'])

# --- Documentation: Hugo Hextra (live reload) ---
local_resource(
    'docs',
    serve_cmd='cd docs && hugo server --buildDrafts --port 1313 --bind 0.0.0.0',
    deps=['docs/content', 'docs/hugo.yaml'],
    links=['http://localhost:1313'],
    labels=['docs'],
)
