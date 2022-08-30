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
