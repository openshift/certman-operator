# Conventions for Ginkgo based e2e tests

- [Conventions for Ginkgo based e2e tests](#conventions-for-ginkgo-based-e2e-tests)
    - [Consuming](#consuming)
    - [`make` targets and functions.](#make-targets-and-functions)
        - [E2E Test](#e2e-test)
            - [Local Testing](#e2e-local-testing)

## Consuming
Currently, this convention is only intended for OSD operators. To adopt this convention, your `boilerplate/update.cfg` should include:

```
openshift/golang-osd-e2e
```

## `make` targets and functions.

**Note:** Your repository's main `Makefile` needs to be edited to include:

```
include boilerplate/generated-includes.mk
```

One of the primary purposes of these `make` targets is to allow you to
standardize your prow and app-sre pipeline configurations using the
following:

### E2e Test

| `make` target          | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `e2e-binary-build`     | Compiles ginkgo tests under test/e2e and creates the ginkgo binary.                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `e2e-image-build-push` | Builds e2e image and pushes to operator's quay repo. Image name is defaulted to <operator-image-name>-test-harness. Quay repository must be created beforehand.                                                                                                                                                                                                                                                                                                                        |
| `e2e-local`            | Builds the e2e binary and runs it against a cluster via KUBECONFIG or backplane. Supports focused tests via GINKGO_FOCUS.                                                                                                                                                                                                                                                                                                                                                              |

#### E2E Local Testing

Run e2e tests locally against a managed cluster without waiting for the full Prow CI pipeline.

**Prerequisites:**
- `ocm` CLI logged into the appropriate environment (`ocm login --use-auth-code --url staging`)
- Access to a managed cluster (via KUBECONFIG or backplane)
- Go toolchain installed

**Option 1: Using backplane (recommended)**

```bash
# Run all tests against a cluster by ID
make e2e-local CLUSTER_ID=2rmlgv5dbdp2285n85o7h3aaa6pafkpq

# Run focused tests
make e2e-local CLUSTER_ID=2rmlgv5dbdp2285n85o7h3aaa6pafkpq GINKGO_FOCUS="is installed"

# Run tests with a label filter
make e2e-local CLUSTER_ID=2rmlgv5dbdp2285n85o7h3aaa6pafkpq GINKGO_LABEL_FILTER="!slow"
```

**Option 2: Using an existing KUBECONFIG**

```bash
# Set KUBECONFIG to your cluster's kubeconfig
export KUBECONFIG=/path/to/kubeconfig

# Run all tests
make e2e-local

# Run a single test
make e2e-local GINKGO_FOCUS="reconciles required resources"
```

**Output:**
- Test results print to stdout with verbose Ginkgo output
- JUnit XML report saved to `e2e-local-junit.xml`

**Tips:**
- Use lease clusters for testing (check `#rosa-prow-info` for available clusters)
- The operator must be deployed on the target cluster (via PKO or OLM)
- If tests fail with "not found" errors, verify the operator is running: `oc get deployment -n <operator-namespace>`

