# Validate variables in project.mk exist
ifndef OPERATOR_NAME
$(error OPERATOR_NAME is not set; only operators should consume this convention; check project.mk file)
endif
ifndef E2E_IMAGE_REGISTRY
$(error E2E_IMAGE_REGISTRY is not set; check project.mk file)
endif
ifndef E2E_IMAGE_REPOSITORY
$(error E2E_IMAGE_REPOSITORY is not set; check project.mk file)
endif

# Use current commit as e2e image tag
CURRENT_COMMIT=$(shell git rev-parse --short=7 HEAD)
E2E_IMAGE_TAG=$(CURRENT_COMMIT)

### Accommodate docker or podman
#
# The docker/podman creds cache needs to be in a location unique to this
# invocation; otherwise it could collide across jenkins jobs. We'll use
# a .docker folder relative to pwd (the repo root).
CONTAINER_ENGINE_CONFIG_DIR = .docker
JENKINS_DOCKER_CONFIG_FILE = /var/lib/jenkins/.docker/config.json
export REGISTRY_AUTH_FILE = ${CONTAINER_ENGINE_CONFIG_DIR}/config.json

# If this configuration file doesn't exist, podman will error out. So
# we'll create it if it doesn't exist.
ifeq (,$(wildcard $(REGISTRY_AUTH_FILE)))
$(shell mkdir -p $(CONTAINER_ENGINE_CONFIG_DIR))
# Copy the node container auth file so that we get access to the registries the
# parent node has access to
$(shell if test -f $(JENKINS_DOCKER_CONFIG_FILE); then cp $(JENKINS_DOCKER_CONFIG_FILE) $(REGISTRY_AUTH_FILE); fi)
endif

# ==> Docker uses --config=PATH *before* (any) subcommand; so we'll glue
# that to the CONTAINER_ENGINE variable itself. (NOTE: I tried half a
# dozen other ways to do this. This was the least ugly one that actually
# works.)
ifndef CONTAINER_ENGINE
CONTAINER_ENGINE=$(shell command -v podman 2>/dev/null || echo docker --config=$(CONTAINER_ENGINE_CONFIG_DIR))
endif

REGISTRY_USER ?=
REGISTRY_TOKEN ?=

# TODO: Figure out how to discover this dynamically
OSDE2E_CONVENTION_DIR := boilerplate/openshift/golang-osd-operator-osde2e

# log into quay.io
.PHONY: container-engine-login
container-engine-login:
	@test "${REGISTRY_USER}" != "" && test "${REGISTRY_TOKEN}" != "" || (echo "REGISTRY_USER and REGISTRY_TOKEN must be defined" && exit 1)
	mkdir -p ${CONTAINER_ENGINE_CONFIG_DIR}
	@${CONTAINER_ENGINE} login -u="${REGISTRY_USER}" -p="${REGISTRY_TOKEN}" quay.io

######################
# Targets used by e2e test suite
######################

# create binary
.PHONY: e2e-binary-build
e2e-binary-build: GOFLAGS_MOD=-mod=mod
e2e-binary-build: GOENV=GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0 GOFLAGS="${GOFLAGS_MOD}"
e2e-binary-build:
	go mod tidy
	go test ./test/e2e -v -c --tags=osde2e -o e2e.test

# push e2e image tagged as latest and as repo commit hash
.PHONY: e2e-image-build-push
e2e-image-build-push: container-engine-login
	${CONTAINER_ENGINE} build --pull -f test/e2e/Dockerfile -t $(E2E_IMAGE_REGISTRY)/$(E2E_IMAGE_REPOSITORY)/$(E2E_IMAGE_NAME):$(E2E_IMAGE_TAG) .
	${CONTAINER_ENGINE} tag $(E2E_IMAGE_REGISTRY)/$(E2E_IMAGE_REPOSITORY)/$(E2E_IMAGE_NAME):$(E2E_IMAGE_TAG) $(E2E_IMAGE_REGISTRY)/$(E2E_IMAGE_REPOSITORY)/$(E2E_IMAGE_NAME):latest
	${CONTAINER_ENGINE} push $(E2E_IMAGE_REGISTRY)/$(E2E_IMAGE_REPOSITORY)/$(E2E_IMAGE_NAME):$(E2E_IMAGE_TAG)
	${CONTAINER_ENGINE} push $(E2E_IMAGE_REGISTRY)/$(E2E_IMAGE_REPOSITORY)/$(E2E_IMAGE_NAME):latest
