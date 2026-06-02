#!/usr/bin/env bash
set -euo pipefail

if ! command -v prek &>/dev/null; then
  echo "Error: prek is not installed. Install with: uv tool install prek" >&2
  exit 1
fi

prek run --config hack/prek.ci.toml --all-files
