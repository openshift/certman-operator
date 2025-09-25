# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Certman Operator is a Kubernetes operator that automates provisioning and management of TLS certificates from Let's Encrypt for OpenShift Dedicated clusters. It works with Hive to watch ClusterDeployment resources and manages CertificateRequest custom resources.

## Common Development Commands

### Build and Test
```bash
# Default build and test
make                        # Runs go-check, go-test, go-build

# Individual commands
make go-build              # Build the operator binary
make go-test               # Run unit tests with envtest
make go-check              # Run golangci-lint static analysis
make test                  # Run unit tests (alias for go-test)
make lint                  # Run linting and YAML validation
```

### Development Workflow
```bash
# Generate code and manifests
make generate              # Generate CRDs, code, and manifests
make op-generate           # Generate CRDs only
make go-generate           # Generate Go code only

# Validation
make validate              # Ensure code generation is up-to-date
make generate-check        # Check if generated files are current

# Local testing
hack/test/local_test.sh    # Automated local testing with minikube

# Run operator locally (requires proper secrets and config)
WATCH_NAMESPACE="certman-operator" OPERATOR_NAME="certman-operator" go run main.go
```

### Container Operations
```bash
# Build container images
make docker-build          # Build operator image
make docker-push           # Build and push operator image

# For macOS developers (cross-compile)
make go-mac-build          # Build for Linux on macOS
```

## Architecture

### Core Components
- **ClusterDeployment Controller** (`controllers/clusterdeployment/`): Watches Hive's ClusterDeployment CRDs and creates CertificateRequest resources when clusters are installed
- **CertificateRequest Controller** (`controllers/certificaterequest/`): Manages the Let's Encrypt certificate lifecycle using DNS-01 challenges
- **API Types** (`api/v1alpha1/`): CertificateRequest CRD definition

### Key Packages
- `pkg/leclient/`: Let's Encrypt ACME client implementation
- `pkg/clients/`: Cloud provider clients (AWS Route53, GCP, Azure DNS)
- `pkg/localmetrics/`: Prometheus metrics for certificate operations
- `pkg/acmeclient/`: ACME protocol client wrapper

### Dependencies
- **Hive**: Required for ClusterDeployment CRDs and cluster management
- **Let's Encrypt**: Certificate authority for TLS certificates
- **Cloud DNS**: AWS Route53, GCP DNS, or Azure DNS for DNS-01 challenges

### Operation Flow
1. Watches ClusterDeployment resources with `Installed: true`
2. Creates CertificateRequest for each certificate bundle in the cluster spec
3. Performs Let's Encrypt DNS-01 challenge via cloud DNS providers
4. Stores issued certificates in secrets for Hive to sync to target clusters
5. Monitors certificate expiry and renews 45 days before expiration

## Configuration

### Required Secrets
- `lets-encrypt-account`: Let's Encrypt account private key and URL
- `aws`/`gcp`: Cloud provider credentials for DNS challenge (platform-dependent)

### ConfigMap
- `certman-operator`: Contains `default_notification_email_address` for Let's Encrypt notifications

### Environment Variables
- `WATCH_NAMESPACE`: Namespace to watch for resources
- `OPERATOR_NAME`: Name of the operator instance
- `EXTRA_RECORD`: Additional SAN record for control plane certificates

## Testing

### Unit Tests
- Uses envtest for Kubernetes API testing
- Test targets defined in `TESTTARGETS` variable
- Coverage reporting available via `make coverage`

### E2E Testing
- `hack/test/local_test.sh` provides automated minikube-based testing
- Creates test cluster, installs dependencies, builds/deploys operator
- Uses spoofed ClusterDeployment to trigger certificate generation

### Manual Testing on OpenShift
- Requires proper Let's Encrypt account secrets
- Can be deployed to staging clusters for integration testing
- See README manual deployment section for fleet testing procedures