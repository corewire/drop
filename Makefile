# Image URL to use all building/pushing image targets
IMG ?= controller:latest
IMG_UI ?= drop-ui:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTAINER_TOOL ?= docker
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: build
build: ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-ui
build-ui: ## Build Drop Control Center UI binary.
	go build -o bin/drop-ui ./cmd/ui/

.PHONY: run
run: ## Run controller from your host.
	go run ./cmd/main.go

.PHONY: run-ui
run-ui: ## Run Drop Control Center UI from your host (requires kubeconfig).
	go run ./cmd/ui/

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint with auto-fix.
	$(GOLANGCI_LINT) run --fix

##@ Code Generation

.PHONY: generate
generate: controller-gen ## Generate DeepCopy methods.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate CRD and RBAC manifests.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: codegen
codegen: generate manifests docs-gen ## Run all code generation (deepcopy + CRDs + docs).

##@ Testing

.PHONY: test
test: setup-envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: chainsaw ## Run Chainsaw E2E tests (requires kind cluster).
	$(CHAINSAW) test test/e2e/

##@ Cluster

.PHONY: kind-create
kind-create: ## Create kind cluster for development.
	$(KIND) create cluster --name drop-dev --config hack/kind-config.yaml --wait 5m

.PHONY: kind-delete
kind-delete: ## Delete the kind cluster.
	$(KIND) delete cluster --name drop-dev

.PHONY: install
install: manifests kustomize ## Install CRDs into cluster.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from cluster.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found -f -

.PHONY: e2e-infra
e2e-infra: ## Deploy Prometheus + Registry for E2E/dev.
	@chmod +x hack/e2e-infra/setup.sh && hack/e2e-infra/setup.sh

##@ Docker

.PHONY: docker-build
docker-build: ## Build docker image.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: kind-load
kind-load: docker-build ## Build and load image into kind.
	$(KIND) load docker-image ${IMG} --name drop-dev

##@ Helm & Docs

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	helm lint charts/drop

.PHONY: helm-template
helm-template: ## Render Helm templates locally.
	helm template drop charts/drop

.PHONY: docs-serve
docs-serve: ## Serve Hugo docs locally.
	cd docs && hugo server --buildDrafts --port 1313

.PHONY: docs-gen
docs-gen: ## Regenerate AI agent docs (llms.txt, instructions, etc.) from source.
	go run ./hack/gen-ai-docs/

.PHONY: docs-gen-check
docs-gen-check: docs-gen ## Verify generated AI docs are up to date.
	@git diff --exit-code knowledge.yaml llms.txt llms-full.txt .github/copilot-instructions.md .cursorrules AGENTS.md docs/doc-generation.md docs/content/docs/reference/_generated_*.md docs/content/docs/_generated_examples.md || \
		(echo "ERROR: generated docs are out of date — run 'make docs-gen'" && exit 1)

.PHONY: tools
tools: ## Install local tooling and check optional docs/chart binaries.
	@$(MAKE) kustomize controller-gen setup-envtest golangci-lint chainsaw
	@command -v hugo >/dev/null 2>&1 || echo "WARNING: hugo not found — needed for docs"
	@command -v helm >/dev/null 2>&1 || echo "WARNING: helm not found — needed for chart dev"

##@ Tool Dependencies

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
CHAINSAW ?= $(LOCALBIN)/chainsaw

KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.12.2
CHAINSAW_VERSION ?= v0.2.15

.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: $(ENVTEST)
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: chainsaw
chainsaw: $(CHAINSAW)
$(CHAINSAW): $(LOCALBIN)
	$(call go-install-tool,$(CHAINSAW),github.com/kyverno/chainsaw,$(CHAINSAW_VERSION))

define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) GOTOOLCHAIN=local go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
