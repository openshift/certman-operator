# Validate variables in project.mk exist
ifndef OPERATOR_NAME
$(error OPERATOR_NAME is not set; only operators should consume this convention; check project.mk file)
endif
ifndef HARNESS_IMAGE_REGISTRY
$(error HARNESS_IMAGE_REGISTRY is not set; check project.mk file)
endif
ifndef HARNESS_IMAGE_REPOSITORY
$(error HARNESS_IMAGE_REPOSITORY is not set; check project.mk file)
endif

# Use current commit as harness image tag
CURRENT_COMMIT=$(shell git rev-parse --short=7 HEAD)
HARNESS_IMAGE_TAG=$(CURRENT_COMMIT)

### Accommodate docker or podman
#
# The docker/podman creds cache needs to be in a location unique to this
# invocation; otherwise it could collide across jenkins jobs. We'll use
# a .docker folder relative to pwd (the repo root).
CONTAINER_ENGINE_CONFIG_DIR = .docker
export REGISTRY_AUTH_FILE = ${CONTAINER_ENGINE_CONFIG_DIR}/config.json

# If this configuration file doesn't exist, podman will error out. So
# we'll create it if it doesn't exist.
ifeq (,$(wildcard $(REGISTRY_AUTH_FILE)))
$(shell mkdir -p $(CONTAINER_ENGINE_CONFIG_DIR))
# Copy the node container auth file so that we get access to the registries the
# parent node has access to
$(shell cp /var/lib/jenkins/.docker/config.json $(REGISTRY_AUTH_FILE))
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

# TODO: figure out how to container-engine-login only once across multiple `make` calls
.PHONY: container-build-push-one
container-build-push-one: container-engine-login
	@(if [[ -z "${IMAGE_URI}" ]]; then echo "Must specify IMAGE_URI"; exit 1; fi)
	@(if [[ -z "${DOCKERFILE_PATH}" ]]; then echo "Must specify DOCKERFILE_PATH"; exit 1; fi)
	${CONTAINER_ENGINE} build --pull -f $(DOCKERFILE_PATH) -t $(IMAGE_URI) .
	${CONTAINER_ENGINE} push ${IMAGE_URI}

# log into quay.io
.PHONY: container-engine-login
container-engine-login:
	@test "${REGISTRY_USER}" != "" && test "${REGISTRY_TOKEN}" != "" || (echo "REGISTRY_USER and REGISTRY_TOKEN must be defined" && exit 1)
	mkdir -p ${CONTAINER_ENGINE_CONFIG_DIR}
	@${CONTAINER_ENGINE} login -u="${REGISTRY_USER}" -p="${REGISTRY_TOKEN}" quay.io

######################
# Targets used by e2e test harness
######################

# create e2e scaffolding
.PHONY: e2e-harness-generate
e2e-harness-generate:
	${OSDE2E_CONVENTION_DIR}/e2e-harness-generate.sh $(OPERATOR_NAME) $(OSDE2E_CONVENTION_DIR)

# create binary
.PHONY: e2e-harness-build
e2e-harness-build: GOFLAGS_MOD=-mod=mod
e2e-harness-build: GOENV=GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0 GOFLAGS="${GOFLAGS_MOD}"
e2e-harness-build:
	go mod tidy
	${GOENV} go test ./test/e2e -v -c --tags=osde2e -o harness.test

# TODO: Push to a known image tag and commit id
# push harness image
.PHONY: e2e-image-build-push
e2e-image-build-push:
	${OSDE2E_CONVENTION_DIR}/e2e-image-build-push.sh "./test/e2e/Dockerfile $(IMAGE_REGISTRY)/$(HARNESS_IMAGE_REPOSITORY)/$(HARNESS_IMAGE_NAME):$(HARNESS_IMAGE_TAG)"
	${OSDE2E_CONVENTION_DIR}/e2e-image-build-push.sh "./test/e2e/Dockerfile $(IMAGE_REGISTRY)/$(HARNESS_IMAGE_REPOSITORY)/$(HARNESS_IMAGE_NAME):latest"
