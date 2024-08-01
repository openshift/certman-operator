#!/usr/bin/env bash

set -e

REPOSITORY=${REPOSITORY:-"https://github.com/openshift/managed-release-bundle-osd.git"}
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD|egrep '^main$|^release-[0-9]+\.[0-9]+$'|cat)
RVMO_BRANCH=${CURRENT_BRANCH:-main}
# You can override any branch detection by setting RELEASE_BRANCH
BRANCH=${RELEASE_BRANCH:-$RVMO_BRANCH}
DELETE_TEMP_DIR=${DELETE_TEMP_DIR:-true}
TMPD=$(mktemp -d --suffix -rvmo-bundle)
[[ "${DELETE_TEMP_DIR}" == "true" ]] && trap 'rm -rf ${TMPD}' EXIT

cd "${TMPD}"
echo "Cloning RVMO from ${REPOSITORY}:${BRANCH}"
git clone --single-branch -b "${BRANCH}" "${REPOSITORY}" .
bash hack/update-operator-release.sh
