REGISTRY ?= ghcr.io/ystkfujii/playground_supply-chain-guard
IMAGE_NAME ?= go-server
TAG ?= dev
IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(TAG)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
TRUST_STORE ?= notation-demo
ARGOCD_VERSION ?= v3.4.2

.PHONY: start
start: cluster install-argocd

.PHONY: cluster
cluster:
	kind create cluster --config=./kind-config.yaml
	kubectl cluster-info

.PHONY: port-forward-argocd
port-forward-argocd:
	kubectl get -n argocd secret argocd-initial-admin-secret -o yaml | yq .data.password | base64 -d;echo
	kubectl -n argocd port-forward svc/argocd-server 8080:80

.PHONY: stop
stop:
	kind delete cluster --config=./kind-config.yaml

.PHONY: install-argocd
install-argocd:
	kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -n argocd --server-side --force-conflicts \
		-f "https://raw.githubusercontent.com/argoproj/argo-cd/$(ARGOCD_VERSION)/manifests/install.yaml"
	kubectl -n argocd rollout status deployment/argocd-server --timeout=300s
	kubectl -n argocd rollout status deployment/argocd-repo-server --timeout=300s
	kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=300s
	kubectl apply -f manifests/apps.yaml

.PHONY: setup
setup:
	go install github.com/notaryproject/notation/cmd/notation@v1.3.2
	go install github.com/sigstore/cosign/v3/cmd/cosign@v3.0.6

.PHONY: build
build:
	docker build -t $(IMAGE) --build-arg VERSION=$(TAG) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) app/go-server

.PHONY: push
push:
	docker push $(IMAGE)

.PHONY: digest
digest:
	@docker inspect --format='{{index .RepoDigests 0}}' $(IMAGE)

.PHONY: cert
cert:
	notation cert generate-test --default $(TRUST_STORE)
	notation key list
	notation cert list

.PHONY: policy
policy:
	@sed "s|__REGISTRY_SCOPE__|$(REGISTRY)/$(IMAGE_NAME)|g; s|__TRUST_STORE__|$(TRUST_STORE)|g" trustpolicy.template.json > trustpolicy.json
	notation policy import trustpolicy.json
	notation policy show

.PHONY: sign
sign:
	$(eval IMAGE_DIGEST := $(shell docker inspect --format='{{index .RepoDigests 0}}' $(IMAGE)))
	notation sign --signature-format cose -m app=$(IMAGE_NAME) -m tag=$(TAG) $(IMAGE_DIGEST)

.PHONY: list
list:
	$(eval IMAGE_DIGEST := $(shell docker inspect --format='{{index .RepoDigests 0}}' $(IMAGE)))
	notation list $(IMAGE_DIGEST)

.PHONY: verify
verify:
	$(eval IMAGE_DIGEST := $(shell docker inspect --format='{{index .RepoDigests 0}}' $(IMAGE)))
	notation verify $(NOTATION_FLAGS) -m app=$(IMAGE_NAME) $(IMAGE_DIGEST)

tamper:
	docker build -t $(IMAGE) --build-arg VERSION=$(TAG)-tampered --build-arg COMMIT=tampered --build-arg BUILD_TIME=$(BUILD_TIME) app/go-server
	docker push $(IMAGE)
	@echo "New digest reference:"
	@docker inspect --format='{{index .RepoDigests 0}}' $(IMAGE)
	@echo "Now run: make verify"
