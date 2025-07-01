# Conventions for OSD operators written in Go

- [Conventions for OSD operators written in Go](#conventions-for-osd-operators-written-in-go)
  - [`make` targets and functions.](#make-targets-and-functions)
    - [Prow](#prow)
      - [Local Testing](#local-testing)
    - [app-sre](#app-sre)
  - [Code coverage](#code-coverage)
  - [Linting and other static analysis with `golangci-lint`](#linting-and-other-static-analysis-with-golangci-lint)
  - [Checks on generated code](#checks-on-generated-code)
  - [FIPS](#fips-federal-information-processing-standards)
  - [Additional deployment support](#additional-deployment-support)
  - [OLM SkipRange](#olm-skiprange)

This convention is suitable for both cluster- and hive-deployed operators.

The following components are included:

## `make` targets and functions.

**Note:** Your repository's main `Makefile` needs to be edited to include the
"nexus makefile include":

```
include boilerplate/generated-includes.mk
```

One of the primary purposes of these `make` targets is to allow you to
standardize your prow and app-sre pipeline configurations using the
following:

### Prow

| Test name / `make` target | Purpose                                                                                                         |
| ------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `validate`                | Ensure code generation has not been forgotten; and ensure generated and boilerplate code has not been modified. |
| `lint`                    | Perform static analysis.                                                                                        |
| `test`                    | "Local" unit and functional testing.                                                                            |
| `coverage`                | [Code coverage](#code-coverage) analysis and reporting.                                                         |

To standardize your prow configuration, you may run:

```shell
$ make prow-config
```

If you already have the openshift/release repository cloned locally, you
may specify its path via `$RELEASE_CLONE`:

```shell
$ make RELEASE_CLONE=/home/me/github/openshift/release prow-config
```

This will generate a delta configuring prow to:

- Build your `build/Dockerfile`.
- Run the above targets in presubmit tests.
- Run the `coverage` target in a postsubmit. This is the step that
  updates your coverage report in codecov.io.

#### Local Testing

You can run these `make` targets locally during development to test your
code changes. However, differences in platforms and environments may
lead to unpredictable results. Therefore boilerplate provides a utility
to run targets in a container environment that is designed to be as
similar as possible to CI:

```shell
$ make container-{target}
```

or

```shell
$ ./boilerplate/_lib/container-make {target}
```

### app-sre

The `build-push` target builds and pushes the operator and OLM registry images,
ready to be SaaS-deployed.
By default it is configured to be run from the app-sre jenkins pipelines.
Consult [this doc](app-sre.md) for information on local execution/testing.

## Code coverage

- A `codecov.sh` script, referenced by the `coverage` `make` target, to
  run code coverage analysis per [this SOP](https://github.com/openshift/ops-sop/blob/93d100347746ce04ad552591136818f82043c648/services/codecov.md).

- A `.codecov.yml` configuration file for
  [codecov.io](https://docs.codecov.io/docs/codecov-yaml). Note that
  this is copied into the repository root, because that's
  [where codecov.io expects it](https://docs.codecov.io/docs/codecov-yaml#can-i-name-the-file-codecovyml).

## Linting and other static analysis with `golangci-lint`

- A `go-check` `make` target, which
- ensures the proper version of `golangci-lint` is installed, and
- runs it against
- a `golangci.yml` config.

## Checks on generated code

The convention embeds default checks to ensure generated code generation is current, committed, and unaltered.
To trigger the check, you can use `make generate-check` provided your Makefile properly includes the boilerplate-generated include `boilerplate/generated-includes.mk`.

Checks consist of:

- Checking all files are committed to ensure a safe point to revert to in case of error
- Running the `make generate` command (see below) to regenerate the needed code
- Checking if this results in any new uncommitted files in the git project or if all is clean.

`make generate` does the following:

- generate crds and deepcopy via controller-gen. This is a no-op if your
  operator has no APIs.
- `openapi-gen`. This is a no-op if your operator has no APIs.
- `go generate`. This is a no-op if you have no `//go:generate`
  directives in your code.

## FIPS (Federal Information Processing Standards)

To enable FIPS in your build there is a `make ensure-fips` target.

Add `FIPS_ENABLED=true` to your repos Makefile. Please ensure that this variable is added **before** including boilerplate Makefiles.

e.g.

```.mk
FIPS_ENABLED=true

include boilerplate/generated-includes.mk
```

`ensure-fips` will add a [fips.go](./fips.go) file in the same directory as the `main.go` file. (Please commit this file as normal)

`fips.go` will import the necessary packages to restrict all TLS configuration to FIPS-approved settings.

With `FIPS_ENABLED=true`, `ensure-fips` is always run before `make go-build`

## Additional deployment support

- The convention currently supports a maximum of two deployments. i.e. The operator deployment itself plus an optional additional deployment.
- If an additional deployment image has to be built and appended to the CSV as part of the build process, then the consumer needs to:
  - Specify `SupplementaryImage` which is the deployment name in the consuming repository's `config/config.go`.
  - Define the image to be built as `ADDITIONAL_IMAGE_SPECS` in the consuming repository's Makefile, Boilerplate later parses this image as part of the build process; [ref](https://github.com/openshift/boilerplate/blob/master/boilerplate/openshift/golang-osd-operator/standard.mk#L56).
  
  e.g.

    ```.mk
    # Additional Deployment Image
    define ADDITIONAL_IMAGE_SPECS
    build/Dockerfile.webhook $(SUPPLEMENTARY_IMAGE_URI)
    end
    ```
  - Ensure the CSV template of the consuming repository has the additional deployment name.

## OLM SkipRange

- OLM currently doesn't support cross-catalog upgrades.
- The convention standardizes the catalog repositories to adhere to the naming convention `${OPERATOR_NAME}-registry`.
- For an existing operator that has been deployed looking to onboard Boilerplate is a problem. Once deployed, for an existing operator to upgrade to the new Boilerplate-deployed operator which refers to the new catalog registry with `staging/production` channels, OLM needs to support cross-catalog upgrades.
- Cross catalog upgrades are only possible via [OLM Skiprange](https://v0-18-z.olm.operatorframework.io/docs/concepts/olm-architecture/operator-catalog/creating-an-update-graph/#skiprange).
- The consumer can explictly enable OLM SkipRange for their operator by specifying `EnableOLMSkipRange="true"` in the repository's `config/config.go`.
- If specified, the `olm.skipRange` annotation will be appended to the CSV during the build process creating an upgrade path for the operator.
