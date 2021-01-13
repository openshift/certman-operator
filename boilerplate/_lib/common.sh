err() {
  echo "==ERROR== $@" >&2
  exit 1
}

banner() {
    echo
    echo "=============================="
    echo "$@"
    echo "=============================="
}

## osdk_version BINARY
#
# Print the version of the specified operator-sdk BINARY
osdk_version() {
    local osdk=$1
    # `operator-sdk version` output looks like
    #       operator-sdk version: v0.8.2, commit: 28bd2b0d4fd25aa68e15d928ae09d3c18c3b51da
    # or
    #       operator-sdk version: "v0.16.0", commit: "55f1446c5f472e7d8e308dcdf36d0d7fc44fc4fd", go version: "go1.13.8 linux/amd64"
    # Peel out the version number, accounting for the optional quotes.
    $osdk version | sed 's/operator-sdk version: "*\([^,"]*\)"*,.*/\1/'
}

## opm_version BINARY
#
# Print the version of the specified opm BINARY
opm_version() {
    local opm=$1
    # `opm version` output looks like:
    #    Version: version.Version{OpmVersion:"v1.15.2", GitCommit:"fded0bf", BuildDate:"2020-11-18T14:21:24Z", GoOs:"darwin", GoArch:"amd64"}
    $opm version | sed 's/.*OpmVersion:"//;s/".*//'
}

## grpcurl_version BINARY
#
# Print the version of the specified grpcurl BINARY
grpcurl_version() {
    local grpcurl=$1
    # `grpcurl -version` output looks like:  grpcurl 1.7.0
    $grpcurl -version 2>&1 | cut -d " " -f 2
}

repo_name() {
    # Account for remotes which are
    # - upstream or origin
    # - ssh ("git@host.com:org/name.git") or https ("https://host.com/org/name.git")
    (git -C $1 config --get remote.upstream.url || git -C $1 config --get remote.origin.url) | sed 's,git@[^:]*:,,; s,https://[^/]*/,,; s/\.git$//'
}

## current_branch REPO
#
# Outputs the name of the current branch in the REPO directory
current_branch() {
    (
        cd $1
        git rev-parse --abbrev-ref HEAD
    )
}

if [ "$BOILERPLATE_SET_X" ]; then
  set -x
fi

# Only used for error messages
_lib=${BASH_SOURCE##*/}

# When this lib is sourced (which is what it's designed for), $0 is the
# script that did the sourcing.
SOURCER=$(realpath $0)
[[ -n "$SOURCER" ]] || err "$_lib couldn't discover where it was sourced from"

HERE=${SOURCER%/*}
[[ -n "$HERE" ]] || err "$_lib failed to discover the dirname of sourcer at $SOURCER"

REPO_ROOT=$(git rev-parse --show-toplevel)
[[ -n "$REPO_ROOT" ]] || err "$_lib couldn't discover the repo root"

CONVENTION_ROOT=$REPO_ROOT/boilerplate
[[ -d "$CONVENTION_ROOT" ]] || err "$CONVENTION_ROOT: not a directory"

# Were we sourced from within a convention?
if [[ "$HERE" == "$CONVENTION_ROOT/"* ]]; then
  # Okay, figure out the name of the convention
  CONVENTION_NAME=${HERE#$CONVENTION_ROOT/}
  # If we got here, we really expected to be able to identify the
  # convention name.
  [[ -n "$CONVENTION_NAME" ]] || err "$_lib couldn't discover the name of the sourcing convention"
fi

if [ -z "$BOILERPLATE_GIT_REPO" ]; then
  export BOILERPLATE_GIT_REPO=https://github.com/openshift/boilerplate.git
fi
if [ -z "$BOILERPLATE_GIT_CLONE" ]; then
  export BOILERPLATE_GIT_CLONE="git clone $BOILERPLATE_GIT_REPO"
fi

# The namespace of the ImageStream by which prow will import the image.
IMAGE_NAMESPACE=openshift
IMAGE_NAME=boilerplate
# LATEST_IMAGE_TAG may be set by `update`, in which case that's the
# value we want to use.
# Accommodate older consumers who don't have backing-image-tag yet.
if [[ -z "$LATEST_IMAGE_TAG" ]] && [[ -f ${CONVENTION_ROOT}/_data/backing-image-tag ]]; then
    LATEST_IMAGE_TAG=$(cat ${CONVENTION_ROOT}/_data/backing-image-tag)
fi
# The public image location
IMAGE_PULL_PATH=quay.io/app-sre/$IMAGE_NAME:$LATEST_IMAGE_TAG
