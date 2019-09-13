#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname "$0")

python "$CURRENT_DIR"/validate_yaml.py "$CURRENT_DIR"/../deploy/crds
if [ "$?" != "0" ]; then
    exit 1
fi

BASE_IMG="certman-operator"
IMG="${BASE_IMG}:latest"

BUILD_CMD="docker build" IMG="$IMG" make docker-build
