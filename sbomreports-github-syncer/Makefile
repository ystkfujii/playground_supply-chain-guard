REGISTRY ?= ghcr.io/ystkfujii/playground_supply-chain-guard
IMAGE_NAME ?= sbomreports-github-syncer
TAG ?= dev
IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(TAG)

.PHONY: build
build:
	docker build -t $(IMAGE) .

.PHONY: kind-load
kind-load:
	kind load docker-image $(IMAGE) --name kind-cluster

