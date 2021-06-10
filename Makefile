REPO_ROOT         := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
VERSION				:= $(shell cat $(REPO_ROOT)/VERSION)
EFFECTIVE_VERSION	:= $(VERSION)-$(shell git rev-parse HEAD)
FROM_IMAGE_BUILDER		:= docker.io/library/golang:1.16
FROM_IMAGE		:= registry.access.redhat.com/ubi8/ubi-minimal:8.4
IMAGE 				:= quay.io/lcavajan/openshift-cluster-backup:$(EFFECTIVE_VERSION)
IMAGE_LATEST	:= quay.io/lcavajan/openshift-cluster-backup:latest

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"'

.PHONY: build-image
build-image:
	buildah bud \
		--build-arg FROM_IMAGE_BUILDER=$(FROM_IMAGE_BUILDER) \
        --build-arg FROM_IMAGE=$(FROM_IMAGE) \
		-t $(IMAGE) -t $(IMAGE_LATEST) .

.PHONY: push-image
push-image:
	buildah push $(IMAGE) $(IMAGE_LATEST)

.PHONY: build-push-image
build-push-image: build-image push-image
