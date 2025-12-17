#!/usr/bin/env bash
set -eo pipefail

if [ "$BOILERPLATE_SET_X" ]; then
  set -x
fi

REPO_ROOT=$(git rev-parse --show-toplevel)
source $REPO_ROOT/boilerplate/_lib/common.sh

GOLANGCI_LINT_VERSION="2.7.2"
OPM_VERSION="v1.23.2"
GRPCURL_VERSION="1.7.0"
DEPENDENCY=${1:-}
GOOS=$(go env GOOS)

case "${DEPENDENCY}" in

golangci-lint)
    GOPATH=$(go env GOPATH)
    if which golangci-lint ; then
        exit
    else
        mkdir -p "${GOPATH}/bin"
        if ! echo "${PATH}" | grep -q "${GOPATH}/bin"; then
            echo "${GOPATH}/bin not in $PATH"
            exit 1
        fi
        DOWNLOAD_URL="https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-${GOOS}-amd64.tar.gz"
        curl -sfL "${DOWNLOAD_URL}" | tar -C "${GOPATH}/bin" -zx --strip-components=1 "golangci-lint-${GOLANGCI_LINT_VERSION}-${GOOS}-amd64/golangci-lint"
    fi
    ;;

opm)
    mkdir -p .opm/bin
    cd .opm/bin

    if [[ -x ./opm  && "$(opm_version ./opm)" == "$OPM_VERSION" ]]; then
        exit 0
    fi

    if which opm && [[ "$(opm_version $(which opm))" == "$OPM_VERSION" ]]; then
        opm=$(realpath $(which opm))
    else
        opm="opm-$OPM_VERSION-$GOOS-amd64"
        opm_download_url="https://github.com/operator-framework/operator-registry/releases/download/$OPM_VERSION/$GOOS-amd64-opm"
        curl -sfL "${opm_download_url}" -o "$opm"
        chmod +x "$opm"
    fi

    ln -fs "$opm" opm
    ;;

grpcurl)
    mkdir -p .grpcurl/bin
    cd .grpcurl/bin

    if [[ -x ./grpcurl  && "$(grpcurl_version ./grpcurl)" == "$GRPCURL_VERSION" ]]; then
        exit 0
    fi

    if which grpcurl && [[ "$(grpcurl_version $(which grpcurl))" == "$GRPCURL_VERSION" ]]; then
        grpcurl=$(realpath $(which grpcurl))
    else
        # mapping from https://github.com/fullstorydev/grpcurl/blob/master/.goreleaser.yml
        [[ "$GOOS" == "darwin" ]] && os=osx || os="$GOOS"
        grpcurl="grpcurl-$GRPCURL_VERSION-$os-x86_64"
        grpcurl_download_url="https://github.com/fullstorydev/grpcurl/releases/download/v$GRPCURL_VERSION/grpcurl_${GRPCURL_VERSION}_${os}_x86_64.tar.gz"
        curl -sfL "$grpcurl_download_url" | tar -xzf - -O grpcurl > "$grpcurl"
        chmod +x "$grpcurl"
    fi

    ln -fs "$grpcurl" grpcurl
    ;;

venv)
    # Set up a python virtual environment
    python3 -m venv .venv
    # Install required libs, if a requirements file was given
    if [[ -n "$2" ]]; then
        .venv/bin/python3 -m pip install -r "$2"
    fi
    ;;

*)
    echo "Unknown dependency: ${DEPENDENCY}"
    exit 1
    ;;
esac
