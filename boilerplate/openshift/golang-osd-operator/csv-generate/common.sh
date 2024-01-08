#!/usr/bin/env bash

REPO_ROOT=$(git rev-parse --show-toplevel)
source $REPO_ROOT/boilerplate/_lib/common.sh

function check_mandatory_params() {
    local csv_missing_param_error
    local param_name
    local param_val
    for param_name in "$@"; do
        eval param_val=\$$param_name
        if [ -z "$param_val" ]; then
            echo "Missing $param_name parameter"
            csv_missing_param_error=true
        fi
    done
    if [ ! -z "$csv_missing_param_error" ]; then
        usage
    fi
}

# generateImageDigest returns the image URI as repo URL + image digest
function generateImageDigest() {
    local param_image
    local param_version
    local image_digest

    param_image="$1"
    param_version="$2"
    if [[ -z $param_image || -z $param_version ]]; then
        usage
    fi

    image_digest=$(skopeo inspect docker://${param_image}:v${param_version} | jq -r .Digest)
    if [[ -z "$image_digest" ]]; then
    echo "Couldn't discover IMAGE_DIGEST for docker://${param_image}:v${param_version}!"
    exit 1
    fi

    echo "${param_image}@${image_digest}"
}
