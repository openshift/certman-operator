FROM registry.svc.ci.openshift.org/openshift/release:golang-1.11 AS builder
COPY . /go/src/github.com/openshift/certman-operator
WORKDIR /go/src/github.com/openshift/certman-operator
RUN make gobuild

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
ENV OPERATOR_PATH=/go/src/github.com/openshift/certman-operator \
    OPERATOR_BIN=certman-operator

WORKDIR /root/
COPY --from=builder /go/src/github.com/openshift/certman-operator/build/_output/bin/${OPERATOR_BIN} /usr/local/bin/${OPERATOR_BIN}
LABEL io.openshift.managed.name="certman-operator" \
      io.openshift.managed.description="Operator to request and reissue Let's Encrypt certificates"
      
