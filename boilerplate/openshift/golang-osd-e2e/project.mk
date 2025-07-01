# Project specific values
OPERATOR_NAME?=$(shell sed -n 's/.*OperatorName .*"\([^"]*\)".*/\1/p' config/config.go)

E2E_IMAGE_REGISTRY?=quay.io
E2E_IMAGE_REPOSITORY?=app-sre
E2E_IMAGE_NAME?=$(OPERATOR_NAME)-e2e

 
REGISTRY_USER?=$(QUAY_USER)
REGISTRY_TOKEN?=$(QUAY_TOKEN)
