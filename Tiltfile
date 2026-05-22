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
