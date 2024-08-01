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

ifdef RELEASE_BRANCHED_BUILDS
    # Make sure all called shell scripts know what's up
    export RELEASE_BRANCHED_BUILDS

    # RELEASE_BRANCH from env vars takes precedence; if not set, try to figure it out
    RELEASE_BRANCH:=${RELEASE_BRANCH}
    ifneq ($(RELEASE_BRANCH),)
        # Sanity check, just to be nice
        RELEASE_BRANCH_TEST := $(shell echo ${RELEASE_BRANCH} | grep -E '^release-[0-9]+\.[0-9]+$$')
        ifeq ($(RELEASE_BRANCH_TEST),)
            $(warning Provided RELEASE_BRANCH doesn't conform to "release-X.Y" pattern; you sure you didn't make a mistake?)
        endif
    endif

    ifeq ($(RELEASE_BRANCH),)
        # Check git repo's branch first
        RELEASE_BRANCH := $(shell git rev-parse --abbrev-ref HEAD | grep -E '^release-[0-9]+\.[0-9]+$$')
    endif

    ifeq ($(RELEASE_BRANCH),)
        # Try to parse it out of Jenkins' JOB_NAME
        RELEASE_BRANCH := $(shell echo ${JOB_NAME} | grep -E --only-matching 'release-[0-9]+\.[0-9]+')
    endif

    ifeq ($(RELEASE_BRANCH),)
        $(error RELEASE_BRANCHED_BUILDS is set, but couldn't detect a release branch and RELEASE_BRANCH is not set; giving up)
    else
        SEMVER := $(subst release-,,$(subst ., ,$(RELEASE_BRANCH)))
        VERSION_MAJOR := $(firstword $(SEMVER))
        VERSION_MINOR := $(lastword $(SEMVER))
    endif
endif

REGISTRY_USER?=$(QUAY_USER)
REGISTRY_TOKEN?=$(QUAY_TOKEN)
