CHART_REPO ?= https://aquasecurity.github.io/helm-charts/
CHART ?= trivy-operator
CHART_VERSION ?= 0.32.1
RELEASE_NAME ?= trivy-operator
NAMESPACE ?= trivy-system
VALUES ?= value.yaml
OUTPUT ?= upstream/install.yaml

.PHONY: generate
generate:
	mkdir -p $(dir $(OUTPUT))
	helm template $(RELEASE_NAME) $(CHART) \
		--repo $(CHART_REPO) \
		--version $(CHART_VERSION) \
		--namespace $(NAMESPACE) \
		--values $(VALUES) \
		--include-crds | yq 'select(.kind == "CustomResourceDefinition")' \
		> generated/crds.yaml
	helm template $(RELEASE_NAME) $(CHART) \
		--repo $(CHART_REPO) \
		--version $(CHART_VERSION) \
		--namespace $(NAMESPACE) \
		--values $(VALUES) \
		> $(OUTPUT)
	kustomize build . > generated/install.yaml
