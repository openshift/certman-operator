#!/usr/bin/env bash

set -e

source `dirname $0`/common.sh

usage() { echo "Usage: $0 -o operator-name -c saas-repository-channel -r registry-image -H operator-commit-hash -n operator-commit-number [-p]" 1>&2; exit 1; }

while getopts "o:c:n:H:pr:" option; do
    case "${option}" in
        c)
            operator_channel=${OPTARG}
            ;;
        H)
            operator_commit_hash=${OPTARG}
            ;;
        n)
            operator_commit_number=${OPTARG}
            ;;
        o)
            operator_name=${OPTARG}
            ;;
        p)
            push_catalog=true
            ;;
        r)
            # NOTE: This is the URL without the tag/digest
            registry_image=${OPTARG}
            ;;
        *)
            usage
    esac
done

# Checking parameters
check_mandatory_params operator_channel operator_name operator_commit_hash operator_commit_number registry_image

# Calculate previous version
SAAS_OPERATOR_DIR="saas-${operator_name}-bundle"
BUNDLE_DIR="${SAAS_OPERATOR_DIR}/${operator_name}"
OPERATOR_NEW_VERSION=$(ls "${BUNDLE_DIR}" | sort -t . -k 3 -g | tail -n 1)
OPERATOR_PREV_VERSION=$(ls "${BUNDLE_DIR}" | sort -t . -k 3 -g | tail -n 2 | head -n 1)

# Get container engine
CONTAINER_ENGINE=$(command -v podman || command -v docker || true)
[[ -n "$CONTAINER_ENGINE" ]] || echo "WARNING: Couldn't find a container engine. Assuming you already in a container, running unit tests." >&2

# Set SRC container transport based on container engine
if [[ "${CONTAINER_ENGINE##*/}" == "podman" ]]; then
    SRC_CONTAINER_TRANSPORT="containers-storage"
else
    SRC_CONTAINER_TRANSPORT="docker-daemon"
fi

# Checking SAAS_OPERATOR_DIR exist
if [ ! -d "${SAAS_OPERATOR_DIR}/.git" ] ; then
    echo "${SAAS_OPERATOR_DIR} should exist and be a git repository"
    exit 1
fi

# Read the bundle version we're attempting to publish
# in the OLM catalog from the package yaml
PACKAGE_YAML_PATH="${BUNDLE_DIR}/${operator_name}.package.yaml"
PACKAGE_YAML_VERSION=$(awk '$1 == "currentCSV:" {print $2}' ${PACKAGE_YAML_PATH})

# Ensure we're commiting and pushing the version we think we are pushing
# Since we build the bundle in catalog-build.sh this script could be run
# independently and push a version we're not expecting.
# if ! [ "${operator_name}.v${OPERATOR_NEW_VERSION}" = "${PACKAGE_YAML_VERSION}" ]; then
#     echo "You are attemping to push a bundle that's pointing to a version of this catalog you are not building"
#     echo "You are building version: ${operator_name}.v${OPERATOR_NEW_VERSION}"
#     echo "Your local package yaml version is: ${PACKAGE_YAML_VERSION}"
#     exit 1
# fi

# add, commit & push
pushd "${SAAS_OPERATOR_DIR}"

git add .

MESSAGE="add version ${operator_commit_number}-${operator_commit_hash}

replaces ${OPERATOR_PREV_VERSION}
removed versions: ${REMOVED_VERSIONS}"

git commit -m "${MESSAGE}"
git push origin HEAD

if [ $? -ne 0 ] ; then
    echo "git push failed, exiting..."
    exit 1
fi

popd

if [ "$push_catalog" = true ] ; then
    # push image
    if [[ "${RELEASE_BRANCHED_BUILDS}" ]]; then
      skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
          "${SRC_CONTAINER_TRANSPORT}:${registry_image}:v${OPERATOR_NEW_VERSION}" \
          "docker://${registry_image}:v${OPERATOR_NEW_VERSION}"

      if [ $? -ne 0 ] ; then
          echo "skopeo push of ${registry_image}:v${OPERATOR_NEW_VERSION}-latest failed, exiting..."
          exit 1
      fi

      exit 0
    fi

    skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
        "${SRC_CONTAINER_TRANSPORT}:${registry_image}:${operator_channel}-latest" \
        "docker://${registry_image}:${operator_channel}-latest"

    if [ $? -ne 0 ] ; then
        echo "skopeo push of ${registry_image}:${operator_channel}-latest failed, exiting..."
        exit 1
    fi

    skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
        "${SRC_CONTAINER_TRANSPORT}:${registry_image}:${operator_channel}-latest" \
        "docker://${registry_image}:${operator_channel}-${operator_commit_hash}"

    if [ $? -ne 0 ] ; then
        echo "skopeo push of ${registry_image}:${operator_channel}-${operator_commit_hash} failed, exiting..."
        exit 1
    fi
fi
