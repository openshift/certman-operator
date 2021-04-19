#!/bin/bash

set -ev

usage() {
    echo "$0 OPERATOR_IMAGE_URI REGISTRY_IMAGE CURRENT_COMMIT"
    exit -1
}

REPO_ROOT=$(git rev-parse --show-toplevel)
source $REPO_ROOT/boilerplate/_lib/common.sh

[[ $# -eq 3 ]] || usage

OPERATOR_IMAGE_URI=$1
REGISTRY_IMAGE=$2
CURRENT_COMMIT=$3

# Don't rebuild the image if it already exists in the repository
if image_exists_in_repo "${OPERATOR_IMAGE_URI}"; then
    echo "Skipping operator image build/push"
else
    # build and push the operator image
    make docker-push
fi

for channel in staging production; do
    # If the catalog image already exists, short out
    if image_exists_in_repo "${REGISTRY_IMAGE}:${channel}-${CURRENT_COMMIT}"; then
        echo "Catalog image ${REGISTRY_IMAGE}:${channel}-${CURRENT_COMMIT} already "
        echo "exists. Assuming this means the saas bundle work has also been done "
        echo "properly. Nothing to do!"
    else
        # build the CSV and create & push image catalog for the appropriate channel
        make ${channel}-common-csv-build ${channel}-catalog-build ${channel}-catalog-publish
    fi
done
