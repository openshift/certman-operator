#!/bin/bash

# AppSRE team CD

set -exv

BASE_IMG="certman-operator"
IMG="${BASE_IMG}:latest"

BUILD_CMD="docker build" IMG="$IMG" make docker-build
