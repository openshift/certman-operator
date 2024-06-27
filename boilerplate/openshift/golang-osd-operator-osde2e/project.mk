# Project specific values
OPERATOR_NAME?=$(shell sed -n 's/.*OperatorName .*"\([^"]*\)".*/\1/p' config/config.go)

HARNESS_IMAGE_REGISTRY?=quay.io
HARNESS_IMAGE_REPOSITORY?=app-sre
HARNESS_IMAGE_NAME?=$(OPERATOR_NAME)-test-harness

 
REGISTRY_USER?=$(QUAY_USER)
REGISTRY_TOKEN?=$(QUAY_TOKEN)
