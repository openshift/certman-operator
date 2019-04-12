.PHONY: build
build:
	go build -o bin/manager github.com/openshift/certman-operator/cmd/manager

.PHONY: test
test:
	go test ./pkg/...
