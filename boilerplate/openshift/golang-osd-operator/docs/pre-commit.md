# Pre-Commit Hooks Setup Guide

## Installation

### Recommended: Using uv

[uv](https://github.com/astral-sh/uv) is recommended for Python dependency management. It provides dependency locking with package hashes (supply-chain protection), virtual environment management, and is 10-100x faster than pip.

**Install uv:**

To avoid piping unverified remote scripts and avoid using `sudo`, install `uv` via `pip` into your user directory:

```bash
# Install to user directory (never use sudo)
pip install --user uv
```

**First-time setup:**
```bash
uv init --bare                    # creates pyproject.toml
uv add --dev pre-commit==4.6.0    # adds dependency, generates uv.lock
source .venv/bin/activate         # macOS/Linux (.venv\Scripts\activate on Windows)
pre-commit install
```

**Subsequent setup** (when `pyproject.toml` and `uv.lock` exist):
```bash
uv sync
source .venv/bin/activate
pre-commit install
```

### Alternative: Using pip

```bash
pip install 'pre-commit==4.6.0'   # pinned version (Golden Rule 15)
pre-commit install
```

Add to `requirements-dev.txt`: `pre-commit==4.6.0`

## First-Time Setup

Run on all files to catch existing issues:
```bash
pre-commit run --all-files
```

Auto-fix hooks will modify files on first run. Stage and commit these separately:
```bash
git diff
git add .
git commit -m "Fix: Apply pre-commit auto-fixes"
```

**Exclude fix commits from git blame:**
```bash
# Create .git-blame-ignore-revs with commit hashes
git config blame.ignoreRevsFile .git-blame-ignore-revs
```

See [git-blame docs](https://git-scm.com/docs/git-blame#Documentation/git-blame.txt---ignore-revs-filefile).

## Usage

**Automatic** (runs on `git commit`):
```bash
git add <files>
git commit -m "Message"
```

**Manual:**
```bash
pre-commit run                              # staged files only
pre-commit run --all-files                  # entire repo
pre-commit run --files path/to/file         # specific files
```

**Bypass (use sparingly):**
```bash
SKIP=hook-id git commit -m "Message"        # skip one hook
git commit --no-verify                      # NEVER use (Golden Rule 16)
```

Rules: Agents never bypass hooks. Security hooks (gitleaks) never bypassable.

## Troubleshooting

**macOS timeout issues:**
```bash
brew install coreutils  # provides gtimeout
```

**Virtual environment not found:**
```bash
source .venv/bin/activate
uv sync
```

**Hooks not running:**
```bash
ls -la .git/hooks/pre-commit  # verify installation
pre-commit install            # reinstall
```

**Hook failures:** Read error messages and fix issues:
- `go-build`: Fix compilation errors
- `go-mod-tidy`: Run `go mod tidy` and stage go.mod/go.sum
- `check-yaml`: Fix YAML syntax

## CI Integration

Pre-commit mirrors `ci/prow/lint`. CI is authoritative; pre-commit is developer convenience. All hooks run in CI with same config.

If pre-commit passes but CI fails: `pre-commit autoupdate`

## Resources

- [Pre-Commit Documentation](https://pre-commit.com/)
- [uv Documentation](https://github.com/astral-sh/uv)
