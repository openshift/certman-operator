SHELL := /usr/bin/env bash

OPERATOR_DOCKERFILE = ./build/Dockerfile

# Include shared Makefiles
include project.mk
include standard.mk

default: gobuild

# Extend Makefile after here

BUILD_CMD ?= docker build

# Image URL to use all building/pushing image targets
IMG ?= certman-operator:latest

# Build the docker image
.PHONY: docker-build
docker-build:
	$(BUILD_CMD) -t ${IMG} ./build/

# Push the docker image
.PHONY: docker-push
docker-push:
	$(BUILD_CMD) -t ${IMG} -f ./build/Dockerfile .
