FROM quay.io/redhat-services-prod/openshift/boilerplate:image-v8.3.2 AS builder

RUN mkdir -p /workdir
WORKDIR /workdir
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make go-build

####
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.7-1771346502

ENV USER_UID=1001 \
    USER_NAME=certman-operator

COPY --from=builder /workdir/build/_output/bin/* /usr/local/bin/

COPY build/bin /usr/local/bin
RUN ls -l /usr/local/bin/
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="certman-operator" \
      io.openshift.managed.description="Operator to request and reissue Let's Encrypt certificates"
