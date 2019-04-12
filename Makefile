BINDIR = bin
SRC_DIRS = pkg
GOFILES = $(shell find $(SRC_DIRS) -name '*.go' | grep -v bindata)

BUILD_CMD ?= docker build

DOCKER_CMD ?= docker

# Image URL to use all building/pushing image targets
IMG ?= certman-operator:latest

# Look up distro name (e.g. Fedora)
DISTRO ?= $(shell if which lsb_release &> /dev/null; then lsb_release -si; else echo "Unknown"; fi)

OPERATOR_NAME = certman-operator

BINFILE=build/_output/bin/$(OPERATOR_NAME)
MAINPACKAGE=./cmd/manager
GOENV=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOFLAGS=-gcflags="all=-trimpath=${GOPATH}" -asmflags="all=-trimpath=${GOPATH}"

TESTTARGETS := $(shell go list -e ./... | egrep -v "/(vendor)/")
TESTOPTS :=

default: gobuild

.PHONY: clean
clean:
	rm -rf ./build/_output

.PHONY: gotest
gotest:
	go test $(TESTOPTS) $(TESTTARGETS)

.PHONY: gocheck
gocheck: ## Lint code
	gofmt -s -l $(shell go list -f '{{ .Dir }}' ./... ) | grep ".*\.go"; if [ "$$?" = "0" ]; then gofmt -s -d $(shell go list -f '{{ .Dir }}' ./... ); exit 1; fi
	go vet ./cmd/... ./pkg/...

.PHONY: gobuild
gobuild: gocheck gotest ## Build binary
	${GOENV} go build ${GOFLAGS} -o ${BINFILE} ${MAINPACKAGE}

# Build the docker image
.PHONY: docker-build
docker-build:
	$(BUILD_CMD) -t ${IMG} ./build/

# Push the docker image
.PHONY: docker-push
docker-push:
	$(BUILD_CMD) -t ${IMG} -f ./build/Dockerfile .

.PHONY: build
build:
	go build -o bin/manager github.com/openshift/certman-operator/cmd/manager

.PHONY: test
test:
	go test ./pkg/...
