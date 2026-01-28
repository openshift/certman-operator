# PKO Migration Script Tests

This directory contains comprehensive tests for the `olm_pko_migration.py` script.

## Test Files

### 1. `test_olm_pko_migration.py` - Python Unit Tests

Comprehensive unit tests for the migration script using Python's `unittest` framework.

**Test Coverage:**

- **Git Operations** (`TestGitOperations`)
  - Parsing git remotes from various formats (SSH, HTTPS)
  - Extracting GitHub URLs
  - Deriving operator names from repository URLs
  - Error handling for non-git repositories

- **Manifest Annotation** (`TestManifestAnnotation`)
  - Adding PKO phase annotations to manifests
  - Preserving existing annotations
  - Replacing container images with template variables
  - Handling edge cases (missing fields, etc.)

- **Manifest Processing** (`TestManifestProcessing`)
  - Phase assignment based on resource kind:
    - CRDs → `crds` phase
    - RBAC resources → `rbac` phase
    - Deployments → `deploy` phase with image templating
    - Services → `deploy` phase
  - Handling invalid YAML gracefully

- **File Discovery** (`TestFileDiscovery`)
  - Recursive and non-recursive file scanning
  - YAML file filtering (.yaml and .yml)
  - Handling nonexistent paths

- **PKO Manifest Generation** (`TestPKOManifestGeneration`)
  - PackageManifest structure validation
  - Phase definitions
  - Availability probes
  - Config schema

- **File Writing** (`TestFileWriting`)
  - Creating manifest files with correct names
  - Using `.gotmpl` extension for Deployments
  - Custom filename support
  - Skipping/forcing package kinds

- **Template Generation** (`TestTemplateGeneration`)
  - Dockerfile.pko generation
  - Tekton pipeline manifests (push and PR)
  - ClusterPackage template
  - Error handling (missing directories)

- **Integration Tests** (`TestIntegration`)
  - End-to-end conversion process
  - Verifying complete output structure
  - Checking all generated files

**Running the Python tests:**

```bash
# Install dependencies (if not already installed)
pip install pyyaml pytest

# Run all tests with verbose output
python3 -m pytest boilerplate/openshift/golang-osd-operator/test_olm_pko_migration.py -v

# Run specific test class
python3 -m pytest boilerplate/openshift/golang-osd-operator/test_olm_pko_migration.py::TestGitOperations -v

# Run specific test method
python3 -m pytest boilerplate/openshift/golang-osd-operator/test_olm_pko_migration.py::TestManifestAnnotation::test_annotate_adds_phase_annotation -v

# Run with coverage report
python3 -m pytest boilerplate/openshift/golang-osd-operator/test_olm_pko_migration.py --cov=olm_pko_migration --cov-report=html
```

### 2. `test/case/convention/openshift/golang-osd-operator/08-pko-migration` - Bash Integration Test

Integration test following the boilerplate repository's test framework conventions.

**Test Coverage:**

- **Complete Migration Workflow**
  - Creating test operator structure
  - Running migration script
  - Verifying all generated files

- **PKO Manifest Validation**
  - PackageManifest structure and phases
  - Correct phase annotations on all resources
  - Collision protection annotations

- **Resource Type Handling**
  - CRDs annotated with `crds` phase
  - RBAC resources (ServiceAccount, ClusterRole) with `rbac` phase
  - Deployments with `deploy` phase and `.gotmpl` extension
  - Image templating in Deployments
  - Services with `deploy` phase

- **Cleanup Job Template**
  - Generated cleanup Job with proper structure
  - ServiceAccount, Role, RoleBinding, and Job resources
  - Correct phase annotations (`cleanup-rbac`, `cleanup-deploy`)

- **Recursive vs Non-Recursive Mode**
  - `--no-recursive` flag skips subdirectories
  - Default recursive mode includes all nested YAML files

- **Optional Component Generation**
  - Tekton pipeline generation (push and PR variants)
  - Dockerfile.pko generation
  - ClusterPackage template generation
  - Flag handling (`--no-tekton`, `--no-dockerfile`)

**Running the bash test:**

```bash
# Run the specific PKO migration test
./test/case/convention/openshift/golang-osd-operator/08-pko-migration

# Run all tests in the repository
make test

# Run with preserved temp directories for debugging
PRESERVE_TEMP_DIRS=1 ./test/case/convention/openshift/golang-osd-operator/08-pko-migration

# Run tests matching a pattern
make test CASE_GLOB="*pko*"
```

## Test Strategy

The test suite uses a two-tiered approach:

1. **Unit Tests (Python)**: Fast, isolated tests for individual functions and edge cases
2. **Integration Tests (Bash)**: End-to-end validation of the complete migration workflow

This combination ensures:
- Individual components work correctly in isolation
- The complete system works as expected in realistic scenarios
- Edge cases and error conditions are handled properly
- The script integrates well with the boilerplate repository conventions

## Adding New Tests

### Adding Python Unit Tests

Add new test methods to the appropriate test class in `test_olm_pko_migration.py`:

```python
class TestManifestProcessing(unittest.TestCase):
    def test_new_feature(self):
        """Test description."""
        # Arrange
        manifest_str = "..."
        
        # Act
        result = migration.some_function(manifest_str)
        
        # Assert
        self.assertEqual(result, expected_value)
```

### Adding Bash Integration Tests

Add new test sections to the `08-pko-migration` script:

```bash
echo "Testing new feature"

# Setup
# ... preparation ...

# Test
python3 "${REPO_ROOT}/boilerplate/openshift/golang-osd-operator/olm_pko_migration.py" \
    ... args ...

# Verify
if [[ ! -f expected_file ]]; then
    err "Expected file was not created"
fi
```

## CI/CD Integration

The bash test is automatically run as part of the boilerplate repository's CI pipeline:
- Triggered on pull requests
- Runs in containerized environment
- Must pass for PR approval

To run the same checks locally:
```bash
make container-pr-check
```

## Troubleshooting

### Tests Failing Locally

1. **Check Python dependencies**: Ensure `pyyaml` is installed
2. **Git configuration**: Tests require a git repository with configured user
3. **File permissions**: Ensure test files are executable (`chmod +x`)

### Debugging Test Failures

```bash
# For bash tests, preserve temp directories
PRESERVE_TEMP_DIRS=1 ./test/case/convention/openshift/golang-osd-operator/08-pko-migration

# For Python tests, use verbose mode
python3 -m pytest test_olm_pko_migration.py -vv -s
```

### Common Issues

**ImportError: No module named 'yaml'**
- Solution: `pip install pyyaml`

**Git errors in tests**
- Solution: Ensure you're in a git repository or the test creates one properly

**Permission denied**
- Solution: `chmod +x test/case/convention/openshift/golang-osd-operator/08-pko-migration`
