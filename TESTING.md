# Testing Guide

Testing guidelines for the Certman.

## Framework

- **Ginkgo v2**: BDD testing framework
- **Gomega**: Matchers and assertions
- **GoMock**: Interface mocking
- **envtest**: Kubernetes API server for controller testing

## Quick Commands

```bash
# Run all tests
make go-test

# Run tests with Ginkgo runner
ginkgo -r ./...

# Run specific package
go test ./controllers/certificaterequest/

# Verbose output
ginkgo -v ./...

# Run focused test
ginkgo -focus="NetworkPolicy" ./controllers/certificaterequest/

# Container-based (CI parity)
boilerplate/_lib/container-make go-test
```

## Writing Tests

### Test Structure

Each package with tests includes:
- `*_suite_test.go`: Ginkgo test suite setup
- `*_test.go`: Actual test cases

**Example:**
```go
package mypackage_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("MyFeature", func() {
    Context("when condition X", func() {
        It("should do Y", func() {
            result := MyFunction()
            Expect(result).To(Equal(expected))
        })
    })
})
```

### Bootstrapping Tests

```bash
cd pkg/newpackage
ginkgo bootstrap              # Creates suite
ginkgo generate myfile.go     # Creates test file
```

### Mocking Interfaces

Use GoMock for external dependencies:

```go
//go:generate mockgen -destination=mocks/mock_client.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client Client
```

**Regenerate all mocks:**
```bash
boilerplate/_lib/container-make generate
```

**Why container-make?**
- Ensures same mockgen version as CI
- Prevents version drift in generated code

## Test Organization

### Unit Tests
- Test individual functions and methods
- Mock external dependencies (Kubernetes client, HTTP calls)
- Fast execution (<1s per package)
- Located alongside source code

### Controller Tests
- Test reconciliation logic
- Use envtest for simulated Kubernetes API
- Test custom resource lifecycle
- Located in `controllers/*/`

### E2E Tests
- Full operator deployment
- Real cluster interaction
- Located in `test/e2e/`
- Run in CI via Tekton

## Agent-Driven Validation

When AI agents modify code:

**Minimal validation:**
```bash
# After changing controllers/certificaterequest/
go test ./controllers/certificaterequest/
```

**Full validation before commit:**
```bash
make go-test
```

**If tests fail:**
1. Read test output carefully
2. Fix the underlying issue (don't skip tests)
3. Rerun to confirm fix
4. Regenerate mocks if interface changed: `boilerplate/_lib/container-make generate`

## Common Patterns

### Testing Controllers

```go
It("should create certificate", func() {
    // Create CertificateRequest resource
    certRequest := &v1alpha1.CertificateRequest{...}
    Expect(k8sClient.Create(ctx, certRequest)).To(Succeed())

    // Trigger reconciliation
    _, err := reconciler.Reconcile(ctx, req)
    Expect(err).NotTo(HaveOccurred())

    // Verify certificate created
    secret := &corev1.Secret{}
    Expect(k8sClient.Get(ctx, secretKey, secret)).To(Succeed())
    Expect(secret.Type).To(Equal(corev1.SecretTypeTLS))
})
```

### Testing Error Conditions

```go
It("should return error when resource not found", func() {
    _, err := reconciler.Reconcile(ctx, reqForNonExistent)
    Expect(err).To(HaveOccurred())
})
```

### Using Matchers

```go
// Equality
Expect(result).To(Equal(expected))

// Nil checks
Expect(err).NotTo(HaveOccurred())
Expect(obj).To(BeNil())

// Collections
Expect(slice).To(ContainElement("item"))
Expect(slice).To(HaveLen(3))
Expect(slice).To(BeEmpty())

// Booleans
Expect(condition).To(BeTrue())
Expect(condition).To(BeFalse())

// Eventually (async)
Eventually(func() bool {
    return checkCondition()
}).Should(BeTrue())
```

## Coverage

Generate coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

**Note**: Aim for meaningful coverage, not arbitrary percentages.
- Test critical paths and error handling
- Don't test generated code or trivial getters/setters

## Debugging Tests

```bash
# Verbose Ginkgo output
ginkgo -v ./...

# Print statements in tests
fmt.Fprintf(GinkgoWriter, "Debug: %v\n", value)

# Skip flaky tests temporarily
ginkgo -skip="FlakyTest" ./...

# Run single test
ginkgo -focus="exact test name" ./...
```

## CI Expectations

Tests run in Tekton pipeline with:
- Fresh environment
- No cached dependencies
- Strict timeout limits

**Local CI parity:**
```bash
boilerplate/_lib/container-make go-test
```

## Test Performance

**Target timings:**
- Unit tests: <5s per package
- Controller tests: <15s per controller
- Full suite: <2min

**If tests are slow:**
- Check for unnecessary sleeps
- Use `Eventually` with shorter intervals
- Mock external calls
- Avoid creating unnecessary Kubernetes resources

## Common Issues

**Mock not found:**
```bash
# Regenerate mocks
boilerplate/_lib/container-make generate
```

**envtest not installed:**
```bash
make setup-envtest
```

**Test passes locally, fails in CI:**
```bash
# Run in container environment
boilerplate/_lib/container-make go-test

# Check for:
# - Time-dependent tests
# - Environment-specific assumptions
# - File path dependencies
```

**Flaky tests:**
- Use `Eventually` instead of `Expect` for async operations
- Avoid hardcoded delays
- Ensure test isolation (clean up resources)

## Pre-commit Integration

Tests run automatically in pre-commit when Go files change:
```yaml
- id: go-test
  entry: make go-test
  files: '\.go$'
```

This is NOT in current pre-commit config (too slow for pre-commit).
Run manually before pushing: `make go-test`

## Further Reading

- [Ginkgo Documentation](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [GoMock Guide](https://github.com/golang/mock)
- [controller-runtime Testing](https://book.kubebuilder.io/reference/testing.html)
