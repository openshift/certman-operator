# Testing APP-SRE Scripts Locally

- [Testing APP-SRE Scripts Locally](#testing-app-sre-scripts-locally)
  - [Create image repositories](#create-image-repositories)
  - [Fork the SaaS bundle repository](#fork-the-saas-bundle-repository)
  - [Set environment variables](#set-environment-variables)
  - [Execute](#execute)

Local testing of APP-SRE pipelines requires the following setup:

## Create image repositories
In your image registry*, create two repositories in your personal namespace.
Make sure they are **public**.
1. For the operator image, name the repository the same as the operator, e.g. `deadmanssnitch-operator`.
2. For the catalog image, append `-registry` to the operator name, e.g. `deadamnssnitch-operator-registry`.

*We assume you are using quay.io.
If not, you will need to set the `IMAGE_REGISTRY` environment variable (see [below](#set-environment-variables)).

## Fork the SaaS bundle repository
The SaaS bundle repository for `$OPERATOR_NAME` should be located at `https://gitlab.cee.redhat.com/service/saas-{operator}-bundle`, e.g. https://gitlab.cee.redhat.com/service/saas-deadmanssnitch-operator-bundle.
Fork it to your personal namespace.

## Set environment variables
```bash
# The process creates artifacts in your git clone. Some of the make targets
# will bounce when the repository is not clean. Override this behavior:
export ALLOW_DIRTY_CHECKOUT=true

# If you are using an image registry other than quay.io, configure it thus:
# export IMAGE_REGISTRY=docker.io

# Your personal repository space in the image registry.
export IMAGE_REPOSITORY=2uasimojo

# These are used to authenticate to your personal image registry.
# E.g. for quay.io, generate these values via
#    Account Settings => Generate Encrypted Password.
# Even if you're not using quay, the pipeline expects these variables to
# be named QUAY_*
export QUAY_USER=<your registry username>
export QUAY_TOKEN=<token obtained from the registry>

# Tell the scripts where to find your fork of the SaaS bundle repository.
# Except for the authentication part, this should correspond to what you see in the
# https "clone" button in your fork.
# Generate an access token via Settings => Access Tokens. Enable `write_repository`.
# - {gitlab-user} is your username in gitlab
# - {gitlab-token} is the authentication token you generated above
# - {operator} is the name of the consumer repository, e.g. `deadmanssnitch-operator`
export GIT_PATH=https://{gitlab-user}:{gitlab-token}@gitlab.cee.redhat.com/{gitlab-user}/saas-{operator}-bundle.git
```

## Execute
At this point you should be able to run
```
make build-push
```

This will create the following artifacts if it succeeds
(`{hash}` is the 7-digit SHA of the current git commit in the repository under test):
- Operator image in your personal operator repository, tagged `v{major}.{minor}.{commit-count}-{hash}` (e.g. `v0.1.228-e0b6129`) and `latest`
- Two catalog images in your personal registry repository:
  - One image tagged `staging-{hash}` and `staging-latest`
  - The other tagged `production-{hash}` and `production-latest`
- Two commits in your fork of the SaaS bundle repository:
  - One in the `staging` branch
  - The other in the `production` branch
  These are also present locally in a `saas-{operator-name}-bundle` subdirectory of your operator repository clone.
  You can inspect the artifacts therein to make sure e.g. the CSV was generated correctly.
