#!/usr/bin/env bash

set -ev

usage() {
    cat <<EOF
    Usage: $0 "IMAGE_SPECS"
    IMAGE_SPECS is a multiline string where each line has the format:

dockerfile_path image_uri

    For example:

# This is the test harness image
./build/Dockerfile quay.io/app-sre/my-wizbang-operator-test-harness:latest


    The parameter is mandatory; if only building the catalog image,
    specify the empty string.
EOF
    exit -1
}

REPO_ROOT=$(git rev-parse --show-toplevel)
source $REPO_ROOT/boilerplate/_lib/common.sh

[[ $# -eq 1 ]] || usage


IMAGE_SPECS="$1"


while read dockerfile_path image_uri junk; do
    # Support comment lines
    if [[ "$dockerfile_path" == '#'* ]]; then
        continue
    fi
    # Support blank lines
    if [[ "$dockerfile_path" == "" ]]; then
        continue
    fi
    if [[ "$junk" != "" ]] && [[ "$junk" != '#'* ]]; then
        echo "Invalid image spec: found extra garbage: '$junk'"
        exit 1
    fi
    if ! [[ -f "$dockerfile_path" ]]; then
        echo "Invalid image spec: no such dockerfile: '$dockerfile_path'"
        exit 1
    fi

    make IMAGE_URI="${image_uri}" DOCKERFILE_PATH="${dockerfile_path}" container-build-push-one

done <<< "$1"
