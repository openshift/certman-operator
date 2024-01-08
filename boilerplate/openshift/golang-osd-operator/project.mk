# Project specific values
OPERATOR_NAME?=$(shell sed -n 's/.*OperatorName .*"\([^"]*\)".*/\1/p' config/config.go)
OPERATOR_NAMESPACE?=$(shell sed -n 's/.*OperatorNamespace .*"\([^"]*\)".*/\1/p' config/config.go)

IMAGE_REGISTRY?=quay.io
IMAGE_REPOSITORY?=app-sre
IMAGE_NAME?=$(OPERATOR_NAME)

# Optional additional deployment image
SUPPLEMENTARY_IMAGE_NAME?=$(shell sed -n 's/.*SupplementaryImage .*"\([^"]*\)".*/\1/p' config/config.go)

# Optional: Enable OLM skip-range
# https://v0-18-z.olm.operatorframework.io/docs/concepts/olm-architecture/operator-catalog/creating-an-update-graph/#skiprange
EnableOLMSkipRange?=$(shell sed -n 's/.*EnableOLMSkipRange .*"\([^"]*\)".*/\1/p' config/config.go)

VERSION_MAJOR?=0
VERSION_MINOR?=1

REGISTRY_USER?=$(QUAY_USER)
REGISTRY_TOKEN?=$(QUAY_TOKEN)
