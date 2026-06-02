---
name: prow-ci
description: Fetch and analyze OpenShift Prow CI job failures with automated artifact download and failure pattern detection
trigger: prow, prow-ci, /prow-ci, ci results, check ci, analyze ci failure
---

# Prow CI Analysis

This skill fetches Prow CI job artifacts from Google Cloud Storage and provides automated failure analysis.

## Prerequisites

Before using this skill, verify gcloud CLI is installed:
```bash
which gcloud
```

If not installed, provide instructions from: https://cloud.google.com/sdk/docs/install

**Note**: The `test-platform-results` GCS bucket is publicly accessible - no authentication required.

## Quick Start

```bash
# Option 1: Direct URL (no gh required)
/prow-ci <prow-job-url>

# Option 2: Use gh CLI to find URLs
gh pr checks <PR_NUMBER>

# Analyze a failed job
/prow-ci <prow-job-url>

# Or ask naturally:
"Analyze the lint failure in PR <NUMBER>"
"Check why the validate job failed"
"Show me what broke in the coverage job"
```

**Note**: gh CLI commands are optional helpers. You can always provide Prow job URLs directly.

## Implementation

When invoked, this skill:

1. **Fetches artifacts** using `fetch_prow_artifacts.py`:
   - Downloads **prowjob.json** (job metadata)
   - Downloads **build-log.txt** (complete build output with all errors)
   - Saves to `.work/prow-artifacts/<build-id>/`
   - **Note**: Script is optimized to only download essential files. Optional artifacts (JUnit XML, per-target logs) are skipped as build-log.txt contains all needed information.

2. **Analyzes failures** using `analyze_failure.py`:
   - Parses build-log.txt for error patterns
   - Detects common failure patterns (lint, build, timeout, OOM)
   - Extracts error messages and stack traces
   - Identifies compilation errors and test failures

3. **Generates report**:
   - Markdown format with failure summary
   - Test failures with details
   - Pattern detection (compilation errors, lint failures, timeouts)
   - Actionable error messages

## Usage Instructions

### Step 1: Get Prow Job URL

```bash
# View PR checks to find failed jobs
/prow-ci <prow-job-url>

# Option 2: Use gh CLI to find URLs
gh pr checks <PR_NUMBER>

# Or get detailed status
gh pr view <PR_NUMBER> --json statusCheckRollup --jq '.statusCheckRollup[] | select(.state == "FAILURE")'
```

Example Prow job URL:
```
https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/openshift_certman_operator/<PR_NUMBER>/pull-ci-openshift-certman-operator-master-lint/<BUILD_ID>
```

### Step 2: Fetch and Analyze

Run the fetch script first:
```bash
python3 .claude/skills/prow-ci/fetch_prow_artifacts.py "<prow-job-url>" -o .work/prow-artifacts

```

This downloads only the essential files:
- `prowjob.json` - Job metadata (job name, state, type, URL)
- `build-log.txt` - Complete build output (contains all errors, test failures, and output)

### Step 3: Analyze Failures

```bash
python3 analyze_failure.py .work/prow-artifacts/<build-id> -f markdown
```

Output includes:
- Job information (name, state, URL)
- JUnit test failures with messages and stack traces
- Detected failure patterns (lint errors, build failures, timeouts)
- Top error messages from build log

### Step 4: Present Findings

Create a clear summary for the user with:
- Root cause identification
- Failed tests with error messages
- Detected patterns (lint, build, timeout, etc.)
- Actionable next steps to fix the issue

### Example Workflow

```bash
# User provides: "Analyze the lint failure in PR <NUMBER>"

# 1. Get Prow job URL
/prow-ci <prow-job-url>

# Option 2: Use gh CLI to find URLs
gh pr checks <PR_NUMBER> | grep lint

# 2. Fetch artifacts
python3 .claude/skills/prow-ci/fetch_prow_artifacts.py \
  "https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/openshift_certman_operator/<PR_NUMBER>/pull-ci-openshift-certman-operator-master-lint/<BUILD_ID>"

# 3. Analyze
python3 .claude/skills/prow-ci/analyze_failure.py \
  .work/prow-artifacts/<BUILD_ID> \
  -f markdown

# 4. Review the output and provide actionable summary
```

## Prow Resources

**Main Dashboard**: https://prow.ci.openshift.org/  
**CI Search**: https://github.com/openshift/ci-search  
**Job History**: https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator

## Common Use Cases

### 1. Check Recent CI Results

```bash
# View recent PR jobs
curl -s "https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator&type=presubmit" | grep -E "pull-ci-openshift-certman-operator"

# Check latest job status for specific PR
# Replace PR_NUMBER with actual PR number
gh pr view PR_NUMBER --json statusCheckRollup --jq '.statusCheckRollup[] | select(.context | contains("prow"))'
```

### 2. Access Build Logs

Prow logs are stored at:
- **Pull request jobs**: `gs://test-platform-results/pr-logs/pull/openshift_certman_operator/[PR_NUMBER]/[JOB_NAME]/[JOB_ID]`
- **Periodic jobs**: `gs://test-platform-results/logs/[JOB_NAME]/[JOB_ID]`

**Viewing logs via web**:
```text
https://prow.ci.openshift.org/view/gs/test-platform-results/pr-logs/pull/openshift_certman_operator/[PR_NUMBER]/[JOB_NAME]/[JOB_ID]
```

### 3. Analyze Test Failures

```bash
# Get PR checks
gh pr view PR_NUMBER --json statusCheckRollup

# Find failed jobs
gh pr checks PR_NUMBER | grep -i "fail"

# Access specific job artifacts
# Navigate to Prow UI and click on:
# - Build Log (for compilation/test output)
# - JUnit (for structured test results)
# - Artifacts (for generated files, coverage, etc.)
```

### 4. Common Job Names

**Prow CI Jobs** (configured in openshift/release):
- `pull-ci-openshift-certman-operator-master-e2e-binary-build-success` - E2E binary build verification
- `pull-ci-openshift-certman-operator-master-coverage` - Code coverage analysis (with Codecov)
- `pull-ci-openshift-certman-operator-master-lint` - Linting checks
- `pull-ci-openshift-certman-operator-master-test` - Unit tests
- `pull-ci-openshift-certman-operator-master-validate` - Validation checks

**Tekton Pipelines** (configured in `.tekton/`):
- `certman-operator-pull-request` - Main PR pipeline (docker build with OCI-TA)
- `certman-operator-e2e-pull-request` - E2E testing pipeline
- `certman-operator-pko-pull-request` - PKO (Package Operator) pipeline
- Corresponding `-push` pipelines for merged commits

## Debugging CI Failures

### Step 1: Identify Failed Job
```bash
gh pr checks PR_NUMBER
```

### Step 2: Access Prow UI
Open the Prow link from PR checks or construct manually:
```text
https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator&type=presubmit
```

### Step 3: Review Logs
Click on failed job → "Build Log" tab

### Step 4: Check Artifacts
Look for:
- Test failure logs
- Coverage reports
- Generated artifacts

### Step 5: Reproduce Locally
Many Prow jobs can be reproduced with:
```bash
# For unit tests (matches: pull-ci-...-test)
make go-test

# For linting (matches: pull-ci-...-lint)
make go-check
# OR use pre-commit for comprehensive linting
pre-commit run --all-files

# For validation (matches: pull-ci-...-validate)
make validate

# For coverage (matches: pull-ci-...-coverage)
make coverage

# For E2E binary build (matches: pull-ci-...-e2e-binary-build-success)
make e2e-binary-build

# For container builds (Tekton pipelines)
make docker-build
```

## CI/Prow Integration in This Repo

This repo uses **both Prow and Tekton** for comprehensive CI:

**Prow CI** (openshift/release):
- Configuration: `ci-operator/config/openshift/certman-operator/openshift-certman-operator-master.yaml`
- Runs: lint, test, validate, coverage, e2e-binary-build
- Uses Codecov for coverage reporting (secret: `certman-operator-codecov-token`)
- Skip rules: Changes to `.tekton/`, `.github/`, `.md` files, `OWNERS`, `LICENSE` don't trigger most jobs

**Tekton Pipelines** (`.tekton/`):
- Primary build pipeline using Pipelines as Code
- Three pipeline types: main, e2e, pko
- Builds container images to Quay (certman-operator-tenant)
- Pull request images expire after 5 days
- Uses boilerplate framework from `openshift/boilerplate` (docker-build-oci-ta pipeline)

## Quick Reference Commands

```bash
# Check all PR checks status
/prow-ci <prow-job-url>

# Option 2: Use gh CLI to find URLs
gh pr checks <PR_NUMBER>

# View detailed status for a specific PR
gh pr view <PR_NUMBER> --json statusCheckRollup

# Filter only Prow jobs
/prow-ci <prow-job-url>

# Option 2: Use gh CLI to find URLs
gh pr checks <PR_NUMBER> | grep "pull-ci-openshift-certman-operator"

# Check Tekton pipeline status
gh pr view <PR_NUMBER> --json statusCheckRollup --jq '.statusCheckRollup[] | select(.context | contains("Tekton"))'

# Open Prow dashboard in browser (cross-platform)
# Copy and paste this URL into your browser:
# https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator

# Or use platform-specific command:
# macOS: open "https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator"
# Linux: xdg-open "https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator"
# Windows: start "https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator"

# View specific PR on Prow (replace <PR_NUMBER>)
# https://prow.ci.openshift.org/?repo=openshift%2Fcertman-operator&type=presubmit&pull=<PR_NUMBER>
```

## Troubleshooting

### Can't find job results?
- Check both Prow AND Tekton - this repo uses both systems
- Prow jobs: `pull-ci-openshift-certman-operator-master-*`
- Tekton jobs: Usually show as "Tekton" or pipeline names in PR checks
- Verify repo name format in Prow: `openshift_certman_operator` (underscore, not dash)
- Ensure PR has been opened and CI has run

### Logs show permission denied?
- Prow logs are public for openshift org
- Use web UI (prow.ci.openshift.org) instead of gsutil
- Check if job ID is correct

### Job still running?
- Check Prow dashboard for in-progress jobs
- Look for "Pending" or "Running" status
- Wait for completion before accessing artifacts

### Tekton pipeline failures?
- Check the pipeline link in PR checks (usually links to Konflux/AppStudio UI)
- Tekton logs are in the AppStudio dashboard, not Prow
- Common issues:
  - Image build failures → Check Dockerfile syntax and build context
  - Pipeline timeout → Check for slow steps or network issues
  - Auth failures → Secret configuration in `certman-operator-tenant` namespace
- Local validation:
  ```bash
  # Validate Tekton YAML syntax
  kubectl apply --dry-run=client -f .tekton/

  # Test container build locally
  podman build -f build/Dockerfile -t test:local .
  ```

## Advanced: CI Search

For historical job searches:
```bash
# Clone ci-search tool
git clone https://github.com/openshift/ci-search.git

# Use web interface at search.ci.openshift.org (if available)
# Search for patterns in build logs across all jobs
```

## References

- [Prow Dashboard](https://prow.ci.openshift.org/)
- [CI Search Tool](https://github.com/openshift/ci-search)
- [OpenShift CI Documentation](https://docs.ci.openshift.org/)

## CI Configuration Files

**Prow Configuration** (in openshift/release repo):
- Location: `ci-operator/config/openshift/certman-operator/openshift-certman-operator-master.yaml`
- Update process: Submit PR to openshift/release repository
- Auto-generated jobs in: `ci-operator/jobs/openshift/certman-operator/`

**Tekton Pipelines** (in this repo):
- Location: `.tekton/` directory
- Files:
  - `certman-operator-pull-request.yaml` - Main PR pipeline
  - `certman-operator-push.yaml` - Post-merge pipeline
  - `certman-operator-e2e-pull-request.yaml` - E2E testing
  - `certman-operator-pko-pull-request.yaml` - PKO validation
- Triggered by: Pipelines as Code (via Tekton)
- Uses: Boilerplate docker-build-oci-ta pipeline from openshift/boilerplate

## Coverage Reporting

This repository uses Codecov for coverage tracking:
- Secret: `certman-operator-codecov-token` (stored in Prow)
- Generate coverage locally: `make coverage`
- Coverage runs on PRs and post-merge (`publish-coverage`)
- Dashboard: Check Codecov for certman-operator

## Integration with Other Skills

- Use with **test-agent** to compare local test results with CI
- Use with **ci-agent** to validate CI configuration
- Use with **lint-agent** when investigating lint failures in CI
- Use with **security-agent** when investigating pre-commit hook failures
