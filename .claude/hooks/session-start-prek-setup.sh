#!/usr/bin/env bash
#
# Session Start Hook: Prek Setup
#
# Ensures prek (pre-commit) is installed and configured when Claude Code starts
#
# What it does:
#   1. Checks if prek is installed
#   2. If installed, ensures git hooks are wired up (prek install)
#   3. Provides helpful guidance if prek is missing
#
set -uo pipefail

# Ensure we're running from the git repository root
REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [[ -z "$REPO_ROOT" ]]; then
  # Not in a git repo - nothing to do
  exit 0
fi
cd "$REPO_ROOT" || exit 0

# Check if prek is installed
if ! command -v prek &> /dev/null; then
  # Prek not installed - provide guidance but don't block
  cat >&2 <<'EOF'
┌────────────────────────────────────────────────────────────────┐
│ ⚠️  prek (pre-commit hooks) is not installed                  │
│                                                                │
│ This project uses prek for code quality validation.           │
│                                                                │
│ Install it:                                                    │
│   uv tool install prek      # recommended                     │
│   pipx install prek         # alternative                     │
│   pip install --user prek   # fallback                        │
│                                                                │
│ Then wire up git hooks: prek install                          │
└────────────────────────────────────────────────────────────────┘

EOF
  exit 0
fi

# Prek is installed - check if git hooks are configured
if [[ ! -f .git/hooks/pre-commit ]] || ! grep -q "prek" .git/hooks/pre-commit 2>/dev/null; then
  echo "⚙️  Setting up prek pre-commit hooks..." >&2

  if prek install >/dev/null 2>&1; then
    echo "✅ Pre-commit enabled" >&2
  else
    echo "⚠️  Failed to install prek hooks - you may need to run 'prek install' manually" >&2
  fi
else
  # Hooks already installed - confirm status
  echo "✅ Pre-commit enabled" >&2
fi

exit 0
