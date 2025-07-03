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

#### E2E Local Testing

Please follow [this README](https://github.com/openshift/ops-sop/blob/master/v4/howto/osde2e/operator-test-harnesses.md#using-ginkgo) to run your e2e tests locally

