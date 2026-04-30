Run pre-commit hooks on this repository following the agentic SDLC golden rules (SREP-4450).

## Usage
- `/pre-commit` — run on staged files (default, fastest)
- `/pre-commit --all-files` — run on all files (first-time setup, CI equivalent)
- `/pre-commit <hook-id>` — run a single hook by ID (targeted debugging)

## What you must do

### Step 1 — Preflight checks

1. Confirm `.pre-commit-config.yaml` exists in the repo root. If not, tell the user and stop.
2. Confirm `pre-commit` is installed: run `which pre-commit`. If not found, run `pip install pre-commit` or `pip3 install pre-commit`.
3. Confirm hooks are installed: check if `.git/hooks/pre-commit` exists. If not, run `pre-commit install`.

### Step 2 — Run hooks

Determine the run mode from `$ARGUMENTS`:
- `--all-files` → run `pre-commit run --all-files`
- `<hook-id>` (a word that is not a flag) → run `pre-commit run <hook-id>`
- empty or default → run `pre-commit run` (staged files only)

Capture the full stdout and stderr output.

### Step 3 — Parse and categorise results

For each hook in the output, classify it as one of:
- **Passed** — hook exited 0, no changes
- **Auto-fixed** — hook exited non-zero but modified files (trailing-whitespace, end-of-file-fixer)
- **Failed** — hook exited non-zero, no auto-fix

Extract for each failure:
- Hook ID and name
- Affected files and line numbers if present
- The error message
- Whether it is a security hook (gitleaks, rbac-wildcard-check)

### Step 4 — Handle auto-fixes (idempotency loop, golden rule 9)

If any hooks auto-fixed files:
1. Stage the modified files: `git add <auto-fixed files>`
2. Re-run the hooks on staged files
3. Report what was fixed

### Step 5 — Retry on failure (golden rule 19, max 2 iterations)

Track `attempt_count` starting at 1.

For each non-security failure with an identifiable fix:
1. Apply the fix (edit the file, run the suggested command)
2. Stage the changes
3. Re-run `pre-commit run`
4. Increment `attempt_count`

**Stop retrying when:**
- All hooks pass → report success
- `attempt_count` reaches 3 → stop, escalate to human (see Step 6)
- A security hook fails → stop immediately, escalate to human (see Step 6)

### Step 6 — Escalate to human when required

Escalate (do not retry further) when:
- A **security hook** fires (gitleaks, rbac-wildcard-check) — these require human judgment
- Hooks still fail after **2 fix-and-retry attempts**
- A hook **timed out** — this indicates a systemic issue, not a fixable code problem

When escalating, report:
- Which hook is failing
- The exact error output
- What was already attempted
- The recommended next action for the human

### Step 7 — Final report

Always end with a structured summary:

```
PRE-COMMIT SUMMARY
==================
Passed:     <list of hook IDs>
Auto-fixed: <list of hook IDs> → files staged
Fixed:      <list of hook IDs> → changes applied
Failed:     <list of hook IDs> → escalated to human
Attempts:   <N> of 2 maximum
```

## Rules you must never break

- **Never run `git commit --no-verify`** — bypassing all hooks is not permitted
- **Never modify `.pre-commit-config.yaml`** to suppress a failing hook
- **Never retry more than twice** — escalate on the third failure
- **Never auto-fix a security hook failure** — always escalate to human
- **Always stage auto-fixed files** before re-running — do not leave unstaged modifications
- **Always report what changed** — the human must be able to review every fix you applied
