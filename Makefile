# SPDX-FileCopyrightText: 2022-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0

SHELL = bash -e -o pipefail

export CGO_ENABLED=1
export GO111MODULE=on
GOLANGCI_LINT_VERSION := v1.48

.PHONY: build

TOPO_DISCOVERY_APP_VERSION ?= latest

build-tools:=$(shell if [ ! -d "./build/build-tools" ]; then mkdir -p build && cd build && git clone https://github.com/onosproject/build-tools.git; fi)
include ./build/build-tools/make/onf-common.mk

mod-update: # @HELP Download the dependencies to the vendor folder
	go mod tidy
	go mod vendor

mod-lint: mod-update # @HELP ensure that the required dependencies are in place
	# dependencies are vendored, but not committed, go.sum is the only thing we need to check
	bash -c "diff -u <(echo -n) <(git diff go.sum)"


linters:
	@docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:${GOLANGCI_LINT_VERSION} golangci-lint run -v --config /app/.golangci.yml

build: # @HELP build the Go binaries and run all validations (default)
build: mod-update
	go build -mod=vendor -o build/_output/topo-discovery ./cmd/topo-discovery
test: # @HELP run the unit tests and source code validation producing a golang style report
test: mod-lint build linters license
	go test -race github.com/onosproject/topo-discovery/...


topo-discovery-app-docker: mod-update # @HELP build topo-discovery base Docker image
	docker build --platform linux/amd64 . -f build/topo-discovery/Dockerfile \
		-t ${DOCKER_REPOSITORY}topo-discovery:${TOPO_DISCOVERY_APP_VERSION}


images: # @HELP build all Docker images
images: topo-discovery-app-docker

all: build images

publish: images
	docker push onosproject/topo-discovery:latest

ifdef TAG
	docker tag onosproject/topo-discovery:latest onosproject/topo-discovery:$(TAG)
	docker push onosproject/topo-discovery:$(TAG)
endif


kind: # @HELP build Docker images and add them to the currently configured kind cluster
kind: images kind-only

kind-only: # @HELP deploy the image without rebuilding first
kind-only:
	@if [ "`kind get clusters`" = '' ]; then echo "no kind cluster found" && exit 1; fi
	kind load docker-image --name ${KIND_CLUSTER_NAME} ${DOCKER_REPOSITORY}topo-discovery:${TOPO_DISCOVERY_APP_VERSION}


clean:: # @HELP remove all the build artifacts
	rm -rf ./build/_output ./vendor ./cmd/topo-discovery/topo-discovery ./cmd/onos/onos
	go clean -testcache github.com/onosproject/topo-discovery/...