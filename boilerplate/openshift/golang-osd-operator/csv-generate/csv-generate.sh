#!/usr/bin/env bash

set -e

source `dirname $0`/common.sh

usage() { echo "Usage: $0 -o operator-name -c saas-repository-channel -H operator-commit-hash -n operator-commit-number -i operator-image -V operator-version -g [hack|common][-d]" 1>&2; exit 1; }

# TODO : Add support of long-options
while getopts "c:dg:H:i:n:o:V:" option; do
    case "${option}" in
        c)
            operator_channel=${OPTARG}
            ;;
        d)
            diff_generate=true
            ;;
        g)
            if [ "${OPTARG}" = "hack" ] || [ "${OPTARG}" = "common" ] ; then
                generate_script=${OPTARG}
            else
                # TODO : Case to be tested
                echo "Incorrect value for '-g'. Expecting 'hack' or 'common'. Got ${OPTARG}"
            fi
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
        *)
            usage
    esac
done

# Checking parameters
check_mandatory_params operator_channel operator_image operator_version operator_name operator_commit_hash operator_commit_number generate_script

# Use set container engine or select one from available binaries
if [[ -z "$CONTAINER_ENGINE" ]]; then
    CONTAINER_ENGINE=$(command -v podman || command -v docker || true)
fi

if [[ -z "$CONTAINER_ENGINE" ]]; then
    YQ_CMD="yq"
else
    YQ_CMD="$CONTAINER_ENGINE run --rm -i quay.io/app-sre/yq:3.4.1 yq"
fi

# Get the image URI as repo URL + image digest
IMAGE_DIGEST=$(skopeo inspect docker://${operator_image}:v${operator_version} | jq -r .Digest)
if [[ -z "$IMAGE_DIGEST" ]]; then
    echo "Couldn't discover IMAGE_DIGEST for docker://${operator_image}:v${operator_version}!"
    exit 1
fi
REPO_DIGEST=${operator_image}@${IMAGE_DIGEST}

# If no override, using the gitlab repo
if [ -z "$GIT_PATH" ] ; then
    GIT_PATH="https://app:"${APP_SRE_BOT_PUSH_TOKEN}"@gitlab.cee.redhat.com/service/saas-${operator_name}-bundle.git"
fi

# Calculate previous version
SAAS_OPERATOR_DIR="saas-${operator_name}-bundle"
BUNDLE_DIR="$SAAS_OPERATOR_DIR/${operator_name}/"

if [ "$diff_generate" = true ] ; then
    OPERATOR_NEW_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)
    OPERATOR_PREV_VERSION=$(ls "${BUNDLE_DIR}" | sort -t . -k 3 -g | tail -n 2 | head -n 1)
    OUTPUT_DIR="output-comparison"

    # For diff usecase, checking there is already a generated CSV
    if [ ! -f ${BUNDLE_DIR}/${OPERATOR_NEW_VERSION}/*.clusterserviceversion.yaml ] ; then
        echo "You need to generate CSV with your legacy script before trying to run the diff option"
        exit 1
    fi
else
    rm -rf "$SAAS_OPERATOR_DIR"
    git clone --branch "$operator_channel" ${GIT_PATH} "$SAAS_OPERATOR_DIR"

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
                $YQ_CMD r - "managedResourceTypes[0]"
        )

        if [[ "${MANAGED_RESOURCE_TYPE}" == "" ]]; then
            echo "Unabled to determine if SAAS file managed resource type"
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
                DEPLOYED_HASH=$(
                    curl -s "${SAAS_FILE_URL}" | \
                        $YQ_CMD r - "resourceTemplates[*].targets(namespace.\$ref==${resource_template_ns_path}).ref"
                )
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
fi

if [[ "$generate_script" = "common" ]] ; then
    # Jenkins can't be relied upon to have py3, so run the generator in
    # a container.
    # ...Unless we're already in a container, which is how boilerplate
    # CI runs. We have py3 there, so run natively in that case.
    if [[ -z "$CONTAINER_ENGINE" ]]; then
        ./boilerplate/openshift/golang-osd-operator/csv-generate/common-generate-operator-bundle.py -o ${operator_name} -d ${OUTPUT_DIR} -p ${OPERATOR_PREV_VERSION} -i ${REPO_DIGEST} -V ${operator_version}
    else
        $CONTAINER_ENGINE run --rm -v `pwd`:`pwd` -u `id -u`:0 -w `pwd` registry.access.redhat.com/ubi8/python-36:1-134 /bin/bash -c "python -m pip install oyaml; python ./boilerplate/openshift/golang-osd-operator/csv-generate/common-generate-operator-bundle.py -o ${operator_name} -d ${OUTPUT_DIR} -p ${OPERATOR_PREV_VERSION} -i ${REPO_DIGEST} -V ${operator_version}"
    fi
elif [[ "$generate_script" = "hack" ]] ; then
    if [ -z "$OPERATOR_PREV_VERSION" ] ; then
        OPERATOR_PREV_VERSION="no-version"
        DELETE_REPLACE=true
    fi

    ./hack/generate-operator-bundle.py ${OUTPUT_DIR} ${OPERATOR_PREV_VERSION} ${operator_commit_number} ${operator_commit_hash} ${REPO_DIGEST}

    if [ ! -z "${DELETE_REPLACE}" ] ; then
        yq d -i output-comparison/${OPERATOR_NEW_VERSION}/*.clusterserviceversion.yaml 'spec.replaces'
    fi
fi

if [ "$diff_generate" = true ] ; then
    # TODO : Current hack script does not allow to generate the CSV for the comparison (it will generate a different version that the common one because there is 1 extra version in the history)
    if [[ "$generate_script" = "hack" ]] ; then
        echo "Generating with the common script and after, generating with the hack script is not supported yet. For comparison, please first generate with hack script, and then build/compare with the common script"
        exit 1
    # Preparing yamls for the diff by removing the creation timestamp
    elif [ -f ${BUNDLE_DIR}/${OPERATOR_NEW_VERSION}/*.clusterserviceversion.yaml ] ; then
        yq d ${BUNDLE_DIR}/${OPERATOR_NEW_VERSION}/*.clusterserviceversion.yaml 'metadata.annotations.createdAt' > output-comparison/hack_generate.yaml
        yq d output-comparison/${OPERATOR_NEW_VERSION}/*.clusterserviceversion.yaml 'metadata.annotations.createdAt' > output-comparison/common_generate.yaml
        # Diff on the filtered files
        diff output-comparison/hack_generate.yaml output-comparison/common_generate.yaml
    fi
fi

