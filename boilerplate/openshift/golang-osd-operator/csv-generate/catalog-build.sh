#!/usr/bin/env bash

set -e

source `dirname $0`/common.sh

usage() { echo "Usage: $0 -o operator-name -c saas-repository-channel -r registry-image" 1>&2; exit 1; }

while getopts "o:c:r:" option; do
    case "${option}" in
        o)
            operator_name=${OPTARG}
            ;;
        c)
            operator_channel=${OPTARG}
            ;;
        r)
            # NOTE: This is the URL without the tag/digest
            registry_image=${OPTARG}
            ;;
        *)
            usage
    esac
done

# Detect the container engine to use, allowing override from the env
CONTAINER_ENGINE=${CONTAINER_ENGINE:-$(command -v podman || command -v docker || true)}
if [[ -z "$CONTAINER_ENGINE" ]]; then
    echo "WARNING: Couldn't find a container engine! Defaulting to docker."
    CONTAINER_ENGINE=docker
fi

# Checking parameters
check_mandatory_params operator_channel operator_name

# Parameters for the Dockerfile
SAAS_OPERATOR_DIR="saas-${operator_name}-bundle"
BUNDLE_DIR="${SAAS_OPERATOR_DIR}/${operator_name}"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"

# Checking SAAS_OPERATOR_DIR exist
if [ ! -d "${SAAS_OPERATOR_DIR}/.git" ] ; then
    echo "${SAAS_OPERATOR_DIR} should exist and be a git repository"
    exit 1
fi

# Calculate new operator version from bundles inside the saas directory
OPERATOR_NEW_VERSION=$(ls "${BUNDLE_DIR}" | sort -t . -k 3 -g | tail -n 1)

# Create package yaml
# This must be included in the registry build
# `currentCSV` must reference the latest bundle version included.
# Any version their after `currentCSV` loaded by the initalizer
# will be silently pruned as it's not reachable
PACKAGE_YAML_PATH="${BUNDLE_DIR}/${operator_name}.package.yaml"

cat <<EOF > "${PACKAGE_YAML_PATH}"
packageName: ${operator_name}
channels:
- name: ${operator_channel}
  currentCSV: ${operator_name}.v${OPERATOR_NEW_VERSION}
EOF

# Build registry
cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:4.10.0 AS builder
COPY $SAAS_OPERATOR_DIR manifests
RUN initializer --permissive

FROM registry.access.redhat.com/ubi8/ubi-micro:8.5-836

COPY --from=builder /bin/registry-server /bin/registry-server
COPY --from=builder /bin/grpc_health_probe /bin/grpc_health_probe
COPY --from=builder /bin/initializer /bin/initializer

WORKDIR /registry
RUN chgrp -R 0 /registry && chmod -R g+rwx /registry

USER 1001

COPY --from=builder /registry /registry

EXPOSE 50051

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

${CONTAINER_ENGINE} build --pull -f $DOCKERFILE_REGISTRY --tag "${registry_image}:${operator_channel}-latest" .

if [ $? -ne 0 ] ; then
    echo "docker build failed, exiting..."
    exit 1
fi

# TODO : Test the image and the version it contains
