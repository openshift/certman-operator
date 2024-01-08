#!/usr/bin/env bash

set -e

REPO_ROOT=$(git rev-parse --show-toplevel)
CONVENTION_DIR="$REPO_ROOT/boilerplate/openshift/golang-osd-operator"
PRE_V1_SDK_MANAGER_DIR="$REPO_ROOT/cmd/manager"

if [[ -d "$PRE_V1_SDK_MANAGER_DIR" ]]
then
  MAIN_DIR=$PRE_V1_SDK_MANAGER_DIR
else
  MAIN_DIR=$REPO_ROOT
fi

echo "Writing fips file at $MAIN_DIR/fips.go"

cp $CONVENTION_DIR/fips.go.tmplt "$MAIN_DIR/fips.go"
