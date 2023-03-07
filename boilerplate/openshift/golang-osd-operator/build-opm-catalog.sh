#!/usr/bin/env bash

set -euo pipefail

## Global vars to modify behaviour of the script
SET_X=${SET_X:-false}
[[ "$SET_X" != "false" ]] && set -x

DRY_RUN=${DRY_RUN:-false}
DELETE_TEMP_DIR=${DELETE_TEMP_DIR:-true}
NO_LOG=${NO_LOG:-false}
OLM_BUNDLE_VERSIONS_REPO=${OLM_BUNDLE_VERSIONS_REPO:-gitlab.cee.redhat.com/service/saas-operator-versions.git}
OLM_BUNDLE_VERSIONS_REPO_BRANCH=${OLM_BUNDLE_VERSIONS_REPO_BRANCH:-master}

# Global vars
REPO_ROOT=$(git rev-parse --show-toplevel)

function log() {
    local to_log=${1}
    [[ "$NO_LOG" != "false" ]] && return 0
    local msg
    msg="$(date "+%Y-%m-%d %H:%M:%S")"
    [[ "$DRY_RUN" != "false" ]] && msg="$msg -- DRY-RUN"
    echo "$msg -- $to_log" 1>&2
}

function check_required_environment() {
    local count=0
    for var in OLM_BUNDLE_IMAGE \
               OLM_CATALOG_IMAGE \
               OPERATOR_IMAGE \
               OPERATOR_IMAGE_TAG \
               OPERATOR_VERSION \
               OPERATOR_NAME \
               CONTAINER_ENGINE \
               CONTAINER_ENGINE_CONFIG_DIR \
               CURRENT_COMMIT \
               OLM_CHANNEL
    do
        if [ ! "${!var:-}" ]; then
          log "$var is not set"
          count=$((count + 1))
        fi
    done

    [[ $count -eq 0 ]] && return 0 || return 1
}

function setup_temp_dir() {
    local temp_dir
    temp_dir=$(mktemp -d --suffix "-$(basename "$0")")
    [[ "$DELETE_TEMP_DIR" == "true" ]] && trap "rm -rf $temp_dir" EXIT

    echo "$temp_dir"
}

function setup_local_executable() {
    local executable=${1}
    "$REPO_ROOT"/boilerplate/openshift/golang-osd-operator/ensure.sh "$executable" >&2
    echo "$REPO_ROOT/.$executable/bin/$executable"
}

function check_bundle_contents_cmd() {
    bundle_contents_cmd="$REPO_ROOT/hack/generate-operator-bundle-contents.py"
    if [[ ! -x "$bundle_contents_cmd" ]]; then
        log "$bundle_contents_cmd is either missing or non-executable"
        return 1
    fi
}

# Check we are running an opm supported container engine
function check_opm_supported_container_engine() {
    local image_builder=${1}
    if [[ "$image_builder" != "docker" && "$image_builder" != "podman" ]]; then
        # opm error messages are obscure. Let's make this clear
        log "image_builder $image_builder is not one of docker or podman"
        return 1
    fi

    return 0
}

# Check we will be able to properly authenticate ourselves against the registry
function check_authenticated_registry_command() {
    if [[ ! -f "$CONTAINER_ENGINE_CONFIG_DIR/config.json" ]]; then
        log "$CONTAINER_ENGINE_CONFIG_DIR/config.json missing"
        return 1
    fi

    return 0
}

function setup_authenticated_registry_command() {
    echo "$CONTAINER_ENGINE --config=$CONTAINER_ENGINE_CONFIG_DIR"
}

function clone_olm_bundle_versions_repo() {
    local saas_root_dir=${1}

    local bundle_versions_repo_url
    if [[ -n "${APP_SRE_BOT_PUSH_TOKEN:-}" ]]; then
        log "Using APP_SRE_BOT_PUSH_TOKEN credentials to authenticate"
        bundle_versions_repo_url="https://app:${APP_SRE_BOT_PUSH_TOKEN}@$OLM_BUNDLE_VERSIONS_REPO"
    else
        bundle_versions_repo_url="https://$OLM_BUNDLE_VERSIONS_REPO"
    fi

    log "Cloning $OLM_BUNDLE_VERSIONS_REPO into $saas_root_dir"
    git clone --branch "$OLM_BUNDLE_VERSIONS_REPO_BRANCH" "$bundle_versions_repo_url" "$saas_root_dir"
}

function get_prev_operator_version() {
    local bundle_versions_file=${1}

    local return_message=""

    # if the line contains SKIP it will not be included
    local prev_operator_version=""
    local prev_good_operator_version=""
    local skip_versions=()
    if [[ -s "$bundle_versions_file" ]]; then
        log "$bundle_versions_file exists. We'll use to determine current version"

        prev_operator_version=$(tail -n 1 "$bundle_versions_file" | awk '{print $1}')

        # we traverse the bundle versions file backwards
        # we cannot use pipes here or we would lose the inner variables changes
        local version
        while read -r line; do
            if [[ "$line" == *SKIP* ]]; then
                version=$(echo "$line" | awk '{print $1}')
                skip_versions+=("$version")
            else
                prev_good_operator_version="$line"
                break
            fi
        done < <(sort -r -t . -k 3 -g "$bundle_versions_file")

        if [[ -z "$prev_good_operator_version" ]]; then
            # This means that we have skipped all the available versions. In this case we're going to use the last
            # SKIP version as the prev_good_operator_version to have something to feed replaces in the CSV
            log "No unskipped version in $bundle_versions_file. We'll use the last skipped one: ${skip_versions[0]}"
            prev_good_operator_version="${skip_versions[0]}"
        fi

        log "Previous operator version is $prev_operator_version"
        log "Previous good operator version is $prev_good_operator_version"
        return_message="$prev_operator_version $prev_good_operator_version"

        if [[ ${#skip_versions[@]} -gt 0 ]]; then
            log "We will be skipping: ${skip_versions[*]}"
            return_message="$return_message ${skip_versions[*]}"
        fi
    else
        log "No $bundle_versions_file exist or it is empty. This is the first time the operator is built"
        if [[ ! -d "$(dirname "$bundle_versions_file")" ]]; then
            log "Operator directory doesn't exist in versions repository. Exiting"
            exit 1
        fi
    fi

    echo "$return_message"

}

function build_opm_bundle() {
    local temp_dir=${1}
    local bundle_contents_cmd=${2}
    local opm_local_executable=${3}
    local image_builder=${4}
    local prev_good_operator_version=${5}
    # shellcheck disable=SC2206
    local skip_versions=(${@:6})

    local bundle_temp_dir
    bundle_temp_dir=$(mktemp -d -p "$temp_dir" bundle.XXXX)
    local generate_csv_template_args=""
    [[ -n "$prev_good_operator_version" ]] && generate_csv_template_args="--replaces $prev_good_operator_version"
    if [[ ${#skip_versions[@]} -gt 0 ]]; then
        for version in "${skip_versions[@]}"; do
            generate_csv_template_args="$generate_csv_template_args --skip $version"
        done
    fi

    local manifests_temp_dir
    manifests_temp_dir=$(mktemp -d -p "$bundle_temp_dir" manifests.XXXX)
    # shellcheck disable=SC2086
    $bundle_contents_cmd --name "$OPERATOR_NAME" \
                         --current-version "$OPERATOR_VERSION" \
                         --image "$OPERATOR_IMAGE" \
                         --image-tag "$OPERATOR_IMAGE_TAG" \
                         --output-dir "$manifests_temp_dir" \
                         $generate_csv_template_args

    # opm won't get anything locally, so we need to push the bundle even in dry run mode
    # we will use a different tag to make sure those leftovers are clearly recognized
    # TODO: remove this tag if we're in dry-run mode in the cleanup trap script
    [[ "$DRY_RUN" == "false" ]] || bundle_image_current_commit="${bundle_image_current_commit}-dryrun"

    log "Creating bundle image $bundle_image_current_commit"
    local current_dir="$PWD"
    cd "$bundle_temp_dir"
    $opm_local_executable alpha bundle build --directory "$manifests_temp_dir" \
                                             --channels "$OLM_CHANNEL" \
                                             --default "$OLM_CHANNEL" \
                                             --package "$OPERATOR_NAME" \
                                             --tag "$bundle_image_current_commit" \
                                             --image-builder "$image_builder" \
                                             --overwrite \
                                             1>&2
    cd "$current_dir"

    echo "$bundle_image_current_commit"
}

function build_opm_catalog() {
    local opm_local_executable=${1}
    local image_builder=${2}
    local bundle_image_current_commit=${3}
    local catalog_image_current_commit=${4}
    local prev_operator_version=${5}

    local from_arg=""
    if [[ "$prev_operator_version" ]]; then
        local prev_commit=${prev_operator_version#*-}
        from_arg="--from-index $OLM_CATALOG_IMAGE:$prev_commit"
    fi

    log "Creating catalog image $catalog_image_current_commit using opm"
    # shellcheck disable=SC2086
    $opm_local_executable index add --bundles "$bundle_image_current_commit" \
                                    --tag "$catalog_image_current_commit" \
                                    --build-tool "$image_builder" \
                                    $from_arg
}

function check_opm_catalog() {
    local catalog_image_current_commit=${1}
    local engine_cmd=${2}
    local grpcurl_local_executable=${3}

    # Check that catalog works fine
    log "Checking that catalog we have built returns the correct version $OPERATOR_VERSION"

    local free_port
    free_port=$(python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1]); s.close()')

    log "Running $catalog_image_current_commit and exposing $free_port"
    local catalog_container_id
    catalog_container_id=$($engine_cmd run -d -p "$free_port:50051" "$catalog_image_current_commit")

    log "Getting current version from running catalog"
    current_version_from_catalog=$(
        $grpcurl_local_executable -plaintext -d '{"name": "'"$OPERATOR_NAME"'"}' \
            "localhost:$free_port" api.Registry/GetPackage | \
                jq -r '.channels[] | select(.name=="'"$OLM_CHANNEL"'") | .csvName' | \
                sed "s/$OPERATOR_NAME\.//"
    )

    log "Removing docker container $catalog_container_id"
    $engine_cmd rm -f "$catalog_container_id"

    if [[ "$current_version_from_catalog" != "v$OPERATOR_VERSION" ]]; then
        log "Version from catalog $current_version_from_catalog != v$OPERATOR_VERSION"
        return 1
    fi

    return 0
}

function add_current_version_to_bundle_versions_file() {
    local bundle_versions_file=${1}
    local saas_root_dir=${2}
    local prev_operator_version=${3}

    log "Adding the current version $OPERATOR_VERSION to the bundle versions file in $OLM_BUNDLE_VERSIONS_REPO"
    echo "$OPERATOR_VERSION" >> "$bundle_versions_file"

    local current_directory="$PWD"

    cd "$saas_root_dir/$OPERATOR_NAME"
    git add .
    message="add version $OPERATOR_VERSION"
    [[ "$prev_operator_version" ]] && message="$message

    replaces $prev_operator_version"

    git commit -m "$message"

    log "Pushing the repository changes to $OLM_BUNDLE_VERSIONS_REPO into $OLM_BUNDLE_VERSIONS_REPO_BRANCH branch"
    [[ "$DRY_RUN" == "false" ]] && git push origin "$OLM_BUNDLE_VERSIONS_REPO_BRANCH"

    cd "$current_directory"
}

function main() {
    log "Building $OPERATOR_NAME version $OPERATOR_VERSION"

    check_required_environment || return 1
    check_bundle_contents_cmd || return 1
    check_authenticated_registry_command || return 1

    local temp_dir
    local opm_local_executable
    local grpcurl_local_executable
    local engine_cmd
    local image_builder
    temp_dir=$(setup_temp_dir)
    opm_local_executable=$(setup_local_executable opm)
    grpcurl_local_executable=$(setup_local_executable grpcurl)
    engine_cmd=$(setup_authenticated_registry_command)
    image_builder=$(basename "$CONTAINER_ENGINE" | awk '{print $1}')

    check_opm_supported_container_engine "$image_builder" || return 1

    local saas_root_dir="$temp_dir/saas-operator-versions"
    clone_olm_bundle_versions_repo "$saas_root_dir"

    local bundle_versions_file="$saas_root_dir/$OPERATOR_NAME/${OPERATOR_NAME}-versions.txt"
    local versions
    # shellcheck disable=SC2207
    versions=($(get_prev_operator_version "$bundle_versions_file"))
    # This condition is triggered when an operator is built for the first time. In such case the
    # get_prev_operator_version returns an empty string and causes undefined variables failures
    # in a few lines below.
    if [ -z ${versions+x} ]
    then
        versions[0]=""
        versions[1]=""
    fi
    local prev_operator_version="${versions[0]}"
    local prev_good_operator_version="${versions[1]}"
    local skip_versions=("${versions[@]:2}")

    if [[ "$OPERATOR_VERSION" == "$prev_operator_version" ]]; then
        log "stopping script as $OPERATOR_VERSION version was already built, so no need to rebuild it"
        return 0
    fi

    local bundle_image_current_commit="$OLM_BUNDLE_IMAGE:$CURRENT_COMMIT"
    local bundle_image_latest="$OLM_BUNDLE_IMAGE:latest"
    local catalog_image_current_commit="$OLM_CATALOG_IMAGE:$CURRENT_COMMIT"
    local catalog_image_latest="$OLM_CATALOG_IMAGE:latest"

    bundle_image_current_commit=$(build_opm_bundle "${temp_dir}" \
                                                   "$bundle_contents_cmd" \
                                                   "$opm_local_executable" \
                                                   "$image_builder" \
                                                   "$prev_good_operator_version" \
                                                   "${skip_versions[*]:-}")

    log "Pushing bundle image $bundle_image_current_commit"
    $engine_cmd push "$bundle_image_current_commit"

    # Make sure this is run after pushing the image
    log "Validating bundle $bundle_image_current_commit"
    $opm_local_executable alpha bundle validate --tag "$bundle_image_current_commit" \
                                                --image-builder "$image_builder"

    log "Tagging bundle image $bundle_image_current_commit as $bundle_image_latest"
    $engine_cmd tag "$bundle_image_current_commit" "$bundle_image_latest"

    build_opm_catalog "$opm_local_executable" \
                      "$image_builder" \
                      "$bundle_image_current_commit" \
                      "$catalog_image_current_commit" \
                      "$prev_operator_version"

    check_opm_catalog "$catalog_image_current_commit" "$engine_cmd" "$grpcurl_local_executable" || return 1

    log "Tagging catalog image $catalog_image_current_commit as $catalog_image_latest"
    $engine_cmd tag "$catalog_image_current_commit" "$catalog_image_latest"

    add_current_version_to_bundle_versions_file "$bundle_versions_file" \
                                                "$saas_root_dir" \
                                                "$prev_operator_version"

    log "Pushing catalog image $catalog_image_current_commit"
    [[ "$DRY_RUN" == "false" ]] && $engine_cmd push "$catalog_image_current_commit"

    log "Pushing bundle image $catalog_image_latest"
    [[ "$DRY_RUN" == "false" ]] && $engine_cmd push "$catalog_image_latest"

    log "Pushing bundle image $bundle_image_latest"
    [[ "$DRY_RUN" == "false" ]] && $engine_cmd push "$bundle_image_latest"

    return 0
}

# Main
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main
fi
