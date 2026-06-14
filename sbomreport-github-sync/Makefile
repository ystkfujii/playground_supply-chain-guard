REGISTRY ?= ghcr.io/ystkfujii/playground_supply-chain-guard
IMAGE_NAME ?= sbomreport-github-sync
TAG ?= dev
IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(TAG)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build
build:
	docker build -t $(IMAGE) --build-arg VERSION=$(TAG) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) .

.PHONY: kind-load
kind-load:
	kind load docker-image $(IMAGE) --name kind-cluster

