FROM quay.io/app-sre/boilerplate:image-v5.0.1 AS builder

RUN mkdir -p /workdir
WORKDIR /workdir
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make go-build

####
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10-1086

ENV USER_UID=1001 \
    USER_NAME=certman-operator

COPY --from=builder /workdir/build/_output/bin/* /usr/local/bin/

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

LABEL io.openshift.managed.name="certman-operator" \
      io.openshift.managed.description="Operator to request and reissue Let's Encrypt certificates"
