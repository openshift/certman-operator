# Conventions for Ginkgo based e2e tests

- [Conventions for Ginkgo based e2e tests](#conventions-for-ginkgo-based-e2e-tests)
    - [Consuming](#consuming)
    - [`make` targets and functions.](#make-targets-and-functions)
        - [E2E Test Harness](#e2e-test-harness)
            - [Local Testing](#e2e-harness-local-testing)

## Consuming
Currently, this convention is only intended for OSD operators. To adopt this convention, your `boilerplate/update.cfg` should include:

```
openshift/golang-osd-operator-osde2e
```

## `make` targets and functions.

**Note:** Your repository's main `Makefile` needs to be edited to include the
"nexus makefile include":

```
include boilerplate/generated-includes.mk
```

One of the primary purposes of these `make` targets is to allow you to
standardize your prow and app-sre pipeline configurations using the
following:

### E2e Test Harness

| `make` target      | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
|--------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `e2e-harness-generate` | Generate scaffolding for an end to end test harness. The `test/e2e` directory is created where the tests and test runner reside. The harness has access to cloud client and harness passthrough secrets within the test job cluster. Add your operator  related ginkgo e2e tests under the `test/e2e/<operator-name>_tests.go` file. See [this README](https://github.com/openshift/osde2e-example-test-harness/blob/main/README.md#locally-running-this-example) for more details on test harness. |
| `e2e-harness-build`| Compiles ginkgo tests under test/e2e and creates the ginkgo binary.                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `e2e-image-build-push` | Builds e2e test harness image and pushes to operator's quay repo. Image name is defaulted to <operator-image-name>-test-harness. Quay repository must be created beforehand.                                                                                                                                                                                                                                                                                                                        |

#### E2E Harness Local Testing

Please follow [this README](https://github.com/openshift/osde2e-example-test-harness/blob/main/README.md#locally-running-this-example) to run your e2e tests locally

