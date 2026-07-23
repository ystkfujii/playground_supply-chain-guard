CHART_REPO ?= https://kubescape.github.io/helm-charts/
CHART ?= kubescape-operator
CHART_VERSION ?= v1.40.2
RELEASE_NAME ?= kubescape-operator
NAMESPACE ?= kubescape
VALUES ?= value.yaml
OUTPUT ?= upstream/install.yaml

.PHONY: generate
generate:
	mkdir -p $(dir $(OUTPUT))
	#helm template $(RELEASE_NAME) $(CHART) \
	#	--repo $(CHART_REPO) \
	#	--version $(CHART_VERSION) \
	#	--namespace $(NAMESPACE) \
	#	--values $(VALUES) \
	#	--include-crds | yq 'select(.kind == "CustomResourceDefinition")' \
	#	> generated/crds.yaml
	helm template $(RELEASE_NAME) $(CHART) \
		--repo $(CHART_REPO) \
		--version $(CHART_VERSION) \
		--namespace $(NAMESPACE) \
		--values $(VALUES) \
		> $(OUTPUT)
	kustomize build . > generated/install.yaml
