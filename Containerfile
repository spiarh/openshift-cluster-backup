ARG FROM_IMAGE
ARG FROM_IMAGE_BUILDER

FROM ${FROM_IMAGE_BUILDER} AS builder

WORKDIR /go/bin
COPY . .

RUN make build

FROM ${FROM_IMAGE}

COPY --from=builder /go/bin/openshift-cluster-backup /usr/local/bin/openshift-cluster-backup

ENTRYPOINT ["/usr/local/bin/openshift-cluster-backup"]
