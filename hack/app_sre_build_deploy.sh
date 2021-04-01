#!/bin/bash

# AppSRE team CD

set -exv

_OPERATOR_NAME="certman-operator"

CURRENT_DIR=$(dirname "$0")

BASE_IMG="${_OPERATOR_NAME}"
QUAY_IMAGE="quay.io/app-sre/${BASE_IMG}"
# FIXME: Don't override this. The default (set in standard.mk) is fine.
IMG="${BASE_IMG}:latest"

GIT_HASH=$(git rev-parse --short=7 HEAD)

# build the image
BUILD_CMD="docker build" IMG="$IMG" make docker-build

# push the image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:${GIT_HASH}"

# create and push staging image catalog
"$CURRENT_DIR"/app_sre_create_image_catalog.sh staging "$QUAY_IMAGE"

# create and push production image catalog
REMOVE_UNDEPLOYED=true "$CURRENT_DIR"/app_sre_create_image_catalog.sh production "$QUAY_IMAGE"
