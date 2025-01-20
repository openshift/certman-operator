#!/usr/bin/env bash

set -e

source `dirname $0`/common.sh

usage() { echo "Usage: $0 -o operator-name -c saas-repository-channel -H operator-commit-hash -n operator-commit-number -i operator-image -V operator-version -s supplementary-image -e skip-range-enabled" 1>&2; exit 1; }

# TODO : Add support of long-options
while getopts "c:dg:H:i:n:o:V:s:e:" option; do
    case "${option}" in
        c)
            operator_channel=${OPTARG}
            ;;
        H)
            operator_commit_hash=${OPTARG}
            ;;
        i)
            # This should be $OPERATOR_IMAGE from standard.mk
            # I.e. the URL to the image repository with *no* tag.
            operator_image=${OPTARG}
            ;;
        n)
            operator_commit_number=${OPTARG}
            ;;
        o)
            operator_name=${OPTARG}
            ;;
        V)
            # This should be $OPERATOR_VERSION from standard.mk:
            # `{major}.{minor}.{commit-number}-{hash}`
            # Notably, it does *not* start with `v`.
            operator_version=${OPTARG}
            ;;
        s)
            supplementary_image=${OPTARG}
            ;;
        e)
            skip_range_enabled=${OPTARG}
            ;;
        *)
            usage
    esac
done

# Checking parameters
check_mandatory_params operator_channel operator_image operator_version operator_name operator_commit_hash operator_commit_number

# Use set container engine or select one from available binaries
if [[ -z "$CONTAINER_ENGINE" ]]; then
    CONTAINER_ENGINE=$(command -v podman || command -v docker || true)
fi

if [[ -z "$CONTAINER_ENGINE" ]]; then
    YQ_CMD="yq"
else
    yq_image="quay.io/app-sre/yq:4"
    $CONTAINER_ENGINE pull $yq_image
    YQ_CMD="$CONTAINER_ENGINE run --rm -i $yq_image"
fi

REPO_DIGEST=$(generateImageDigest $operator_image $operator_version)

# Given a supplementary image is specified,
# generate the image digest.
if [[ -n $supplementary_image ]]; then
    SECONDARY_REPO_DIGEST=$(generateImageDigest $supplementary_image $operator_version)
    SECONDARY_REPO_DIGEST="-s ${SECONDARY_REPO_DIGEST}"
else
    SECONDARY_REPO_DIGEST=""
fi

# If no override, using the gitlab repo
if [ -z "$GIT_PATH" ] ; then
    GIT_PATH="https://app:"${APP_SRE_BOT_PUSH_TOKEN}"@gitlab.cee.redhat.com/service/saas-${operator_name}-bundle.git"
fi

# Calculate previous version
SAAS_OPERATOR_DIR="saas-${operator_name}-bundle"
BUNDLE_DIR="$SAAS_OPERATOR_DIR/${operator_name}/"

rm -rf "$SAAS_OPERATOR_DIR"
BRANCH="$operator_channel"
if [[ "${RELEASE_BRANCHED_BUILDS}" ]]; then
    # operator version will be set to `X.Y.BUILD_NUMBER-commit sha`, this will
    # be `release-X.Y`
    BRANCH="release-${operator_version%.*}"
fi

if [[ "${RELEASE_BRANCHED_BUILDS}" ]]; then
  git clone ${GIT_PATH} "$SAAS_OPERATOR_DIR"
  pushd "${SAAS_OPERATOR_DIR}"
  # if branch doesn't exist, checkout a new branch based on main/master
  if git ls-remote --exit-code --heads "${GIT_PATH}" "${BRANCH}"; then
      git checkout "${BRANCH}"
  else
      git checkout -b "${BRANCH}"
  fi
  popd
else
  git clone --branch "$operator_channel" ${GIT_PATH} "$SAAS_OPERATOR_DIR"
fi

# If this is a brand new SaaS setup, then set up accordingly
if [[ ! -d "${BUNDLE_DIR}" ]]; then
    echo "Setting up new SaaS operator dir: ${BUNDLE_DIR}"
    mkdir -p "${BUNDLE_DIR}"
fi

# For testing purposes, support disabling anything that relies on
# querying the saas file in app-interface. This includes pruning
# undeployed commits in production.
# FIXME -- This should go away when we're querying app-interface via
# graphql.
if [[ -z "$SKIP_SAAS_FILE_CHECKS" ]]; then
    # PATH to saas file in app-interface
    SAAS_FILE_URL="https://gitlab.cee.redhat.com/service/app-interface/raw/master/data/services/osd-operators/cicd/saas/saas-${operator_name}.yaml"

    # MANAGED_RESOURCE_TYPE
    # SAAS files contain the type of resources managed within the OC templates that
    # are being applied to hive.
    # For customer cluster resources this should always be of type "SelectorSyncSet" resources otherwise
    # can't be sync'd to the customer cluster. We're explicity selecting the first element in the array.
    # We can safely assume anything that is not of type "SelectorSyncSet" is being applied to hive only
    # since it matches ClusterDeployment resources.
    # From this we'll assume that the namespace reference in resourceTemplates to be:
    # For customer clusters: /services/osd-operators/namespace/<hive shard>/namespaces/cluster-scope.yaml
    # For hive clusters: /services/osd-operators/namespace/<hive shard>/namespaces/<namespace name>.yaml
    MANAGED_RESOURCE_TYPE=$(curl -s "${SAAS_FILE_URL}" | \
            $YQ_CMD '.managedResourceTypes[0]' -
    )
    if [[ "${MANAGED_RESOURCE_TYPE}" == "" ]]; then
        echo "Unable to determine if SAAS file managed resource type"
        exit 1
    fi

    # Determine namespace reference path, output resource type
    if [[ "${MANAGED_RESOURCE_TYPE}" == "SelectorSyncSet" ]]; then
        echo "SAAS file is NOT applied to Hive, MANAGED_RESOURCE_TYPE=$MANAGED_RESOURCE_TYPE"
        resource_template_ns_path="/services/osd-operators/namespaces/hivep01ue1/cluster-scope.yml"
    else
        echo "SAAS file is applied to Hive, MANAGED_RESOURCE_TYPE=$MANAGED_RESOURCE_TYPE"
        resource_template_ns_path="/services/osd-operators/namespaces/hivep01ue1/${operator_name}.yml"
    fi

    # remove any versions more recent than deployed hash
    if [[ "$operator_channel" == "production" ]]; then
        if [ -z "$DEPLOYED_HASH" ] ; then
            deployed_hash_yq_filter=".resourceTemplates[].targets[] | select(.namespace.\$ref == \"${resource_template_ns_path}\") | .ref"
            DEPLOYED_HASH="$(curl -s "${SAAS_FILE_URL}" | $YQ_CMD "${deployed_hash_yq_filter}" -)"
        fi

        # Ensure that our query for the current deployed hash worked
        # Validate that our DEPLOYED_HASH var isn't empty.
        # Although we have `set -e` defined the docker container isn't returning
        # an error and allowing the script to continue
        echo "Current deployed production HASH: $DEPLOYED_HASH"

        if [[ ! "${DEPLOYED_HASH}" =~ [0-9a-f]{40} ]]; then
            echo "Error discovering current production deployed HASH"
            exit 1
        fi

        delete=false
        # Sort based on commit number
        for version in $(ls $BUNDLE_DIR | sort -t . -k 3 -g); do
            # skip if not directory
            [ -d "$BUNDLE_DIR/$version" ] || continue

            if [[ "$delete" == false ]]; then
                short_hash=$(echo "$version" | cut -d- -f2)
                
                # before comparing the short_hash to the deployment hash, remove the leading g added in https://issues.redhat.com/browse/OSD-13681
                # short_hash should be 7 char long without the leading g. 
                [ ${#short_hash} -gt 7 ] && short_hash=${short_hash:1:7}
                
                if [[ "$DEPLOYED_HASH" == "${short_hash}"* ]]; then
                    delete=true
                fi
            else
                rm -rf "${BUNDLE_DIR:?BUNDLE_DIR var not set}/$version"
            fi
        done
    fi
fi # End of SKIP_SAAS_FILE_CHECKS granny switch

OPERATOR_PREV_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)
OPERATOR_NEW_VERSION="${operator_version}"
OUTPUT_DIR=${BUNDLE_DIR}

VERSION_DIR="${OUTPUT_DIR}/${OPERATOR_NEW_VERSION}"

# Check if the VERSION_DIR already exists and is not empty - if so skip building
# anything, as only timestamps would be changed.
if [[ -d "${VERSION_DIR}" && -n $(ls -A "${VERSION_DIR}") ]]; then
    echo "Output directory for bundle already exists and is not empty: ${VERSION_DIR}. Skipping bundle creation."
    exit 0
fi

# If setting up a new SaaS repo, there is no previous version when building a bundle
# Optionally pass it to the bundle generator in that case.
if [[ -z "${OPERATOR_PREV_VERSION}" ]]; then
    PREV_VERSION_OPTS=""
else
    OPERATOR_PREV_COMMIT_NUMBER=$(echo "${OPERATOR_PREV_VERSION}" | awk -F. '{print $3}' | awk -F- '{print $1}')
    if [[ "${OPERATOR_PREV_COMMIT_NUMBER}" -gt "${operator_commit_number}" ]];
    then
        echo "Revert detected. Reverting OLM operators is not allowed"
	exit 99
    fi

    PREV_VERSION_OPTS="-p ${OPERATOR_PREV_VERSION}"
fi
# Jenkins can't be relied upon to have py3, so run the generator in
# a container.
# ...Unless we're already in a container, which is how boilerplate
# CI runs. We have py3 there, so run natively in that case.
if [[ -z "$CONTAINER_ENGINE" ]]; then
    ./boilerplate/openshift/golang-osd-operator/csv-generate/common-generate-operator-bundle.py -o ${operator_name} -d ${OUTPUT_DIR} ${PREV_VERSION_OPTS} -i ${REPO_DIGEST} -V ${operator_version} ${SECONDARY_REPO_DIGEST} -e ${skip_range_enabled}
else
    if [[ ${CONTAINER_ENGINE##*/} == "podman" ]]; then
        CE_OPTS="--userns keep-id -v `pwd`:`pwd`:Z"
    else
        CE_OPTS="-v `pwd`:`pwd`"
    fi
    $CONTAINER_ENGINE run --pull=always --rm ${CE_OPTS} -u `id -u`:0 -w `pwd` registry.access.redhat.com/ubi8/python-36 /bin/bash -c "python -m pip install --disable-pip-version-check oyaml; python ./boilerplate/openshift/golang-osd-operator/csv-generate/common-generate-operator-bundle.py -o ${operator_name} -d ${OUTPUT_DIR} ${PREV_VERSION_OPTS} -i ${REPO_DIGEST} -V ${operator_version} ${SECONDARY_REPO_DIGEST} -e ${skip_range_enabled}"
fi
