#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(git rev-parse --show-toplevel)
CI_SERVER_URL=https://prow.svc.ci.openshift.org/view/gcs/origin-ci-test
COVER_PROFILE=${COVER_PROFILE:-coverage.out}
JOB_TYPE=${JOB_TYPE:-"local"}

# Default concurrency to four threads. By default it's the number of procs,
# which seems to be 16 in the CI env. Some consumers' coverage jobs were
# regularly getting OOM-killed; so do this rather than boost the pod resources
# unreasonably.
COV_THREAD_COUNT=${COV_THREAD_COUNT:-4}
make -C "${REPO_ROOT}" go-test TESTOPTS="-coverprofile=${COVER_PROFILE}.tmp -covermode=atomic -coverpkg=./... -p ${COV_THREAD_COUNT}"

# Remove generated files from coverage profile
grep -v "zz_generated" "${COVER_PROFILE}.tmp" > "${COVER_PROFILE}"
rm -f "${COVER_PROFILE}.tmp"

# Configure the git refs and job link based on how the job was triggered via prow
if [[ "${JOB_TYPE}" == "presubmit" ]]; then
       echo "detected PR code coverage job for #${PULL_NUMBER}"
       REF_FLAGS="--pr ${PULL_NUMBER} --commit-sha ${PULL_PULL_SHA}"
       JOB_LINK="${CI_SERVER_URL}/pr-logs/pull/${REPO_OWNER}_${REPO_NAME}/${PULL_NUMBER}/${JOB_NAME}/${BUILD_ID}"
elif [[ "${JOB_TYPE}" == "postsubmit" ]]; then
       echo "detected branch code coverage job for ${PULL_BASE_REF}"
       REF_FLAGS="--branch ${PULL_BASE_REF} --commit-sha ${PULL_BASE_SHA}"
       JOB_LINK="${CI_SERVER_URL}/logs/${JOB_NAME}/${BUILD_ID}"
elif [[ "${JOB_TYPE}" == "local" ]]; then
       echo "coverage report available at ${COVER_PROFILE}"
       exit 0
else
       echo "${JOB_TYPE} jobs not supported" >&2
       exit 1
fi

# Configure certain internal codecov variables with values from prow.
export CI_BUILD_URL="${JOB_LINK}"
export CI_BUILD_ID="${JOB_NAME}"
export CI_JOB_ID="${BUILD_ID}"

if [[ "${JOB_TYPE}" != "local" ]]; then
       CODECOV_VERSION="${CODECOV_VERSION:-v11.3.1}"
       CODECOV_SHA256="${CODECOV_SHA256:-ca1d64196d2d34771084afe76ea657d581bf628e31d993ff8e52ea09cc88a56d}"
       CODECOV_BIN="$(mktemp -d)/codecov"

       curl -sSfL "https://github.com/codecov/codecov-cli/releases/download/${CODECOV_VERSION}/codecovcli_linux" \
              -o "${CODECOV_BIN}"
       echo "${CODECOV_SHA256}  ${CODECOV_BIN}" | sha256sum -c -
       chmod +x "${CODECOV_BIN}"

       "${CODECOV_BIN}" upload-process --fail-on-error --git-service github \
              --file "${COVER_PROFILE}" --slug "${REPO_OWNER}/${REPO_NAME}" ${REF_FLAGS}
else
       echo "coverage report available at ${COVER_PROFILE} (no upload in local mode)"
fi
