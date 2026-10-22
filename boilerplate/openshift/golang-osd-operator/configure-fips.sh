#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
CONVENTION_DIR="${REPO_ROOT}/boilerplate/openshift/golang-osd-operator"
PRE_V1_SDK_MANAGER_DIR="${REPO_ROOT}/cmd/manager"

if [[ -d "${PRE_V1_SDK_MANAGER_DIR}" ]]; then
  MAIN_DIR="${PRE_V1_SDK_MANAGER_DIR}"
else
  MAIN_DIR="${REPO_ROOT}"
fi

FIPS_FILE="${MAIN_DIR}/fips.go"
is_previous_fips_template() {
  cmp -s "${FIPS_FILE}" <(cat <<'EOF'
//go:build boilerplate_fips_assertion
// +build boilerplate_fips_assertion

// BOILERPLATE GENERATED -- DO NOT EDIT
// Run 'make ensure-fips' to regenerate

package main

import "crypto/fips140"

func init() {
	// Assert that FIPS mode is enabled via GODEBUG=fips140=on (or =only)
	if !fips140.Enabled() {
		panic("CRITICAL: Application startup aborted. FIPS 140 mode is NOT enabled!")
	}
}
EOF
  ) || cmp -s "${FIPS_FILE}" <(cat <<'EOF'
//go:build fips_enabled
// +build fips_enabled

// BOILERPLATE GENERATED -- DO NOT EDIT
// Run 'make ensure-fips' to regenerate

package main

import "crypto/fips140"

func init() {
	// Assert that FIPS mode is enabled via GODEBUG=fips140=on (or =only)
	if !fips140.Enabled() {
		panic("CRITICAL: Application startup aborted. FIPS 140 mode is NOT enabled!")
	}
}
EOF
  )
}
if [[ -e "${FIPS_FILE}" ]] && ! cmp -s "${FIPS_FILE}" "${CONVENTION_DIR}/fips.go.tmplt" && ! is_previous_fips_template; then
  echo "[ERROR] ${FIPS_FILE} exists and is not a recognized boilerplate FIPS template; refusing to overwrite it. Review or remove it before building with FIPS_ENABLED=true." >&2
  exit 1
fi

echo "Writing FIPS startup assertion to ${FIPS_FILE}"
cp "${CONVENTION_DIR}/fips.go.tmplt" "${FIPS_FILE}"
