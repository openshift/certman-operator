SHELL := /usr/bin/env bash

OPERATOR_DOCKERFILE = ./build/Dockerfile

# Include shared Makefiles
include project.mk
include standard.mk

default: gobuild

# Extend Makefile after here

# Build the docker image
.PHONY: docker-build
docker-build: build

# Push the docker image
.PHONY: docker-push
docker-push: push
