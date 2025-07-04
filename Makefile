# Copyright 2019 Iguazio
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
GOPATH ?= $(shell go env GOPATH)
OS_NAME = $(shell uname)

# get default os / arch from go env
SCALER_DEFAULT_OS := $(shell go env GOOS)
SCALER_DEFAULT_ARCH := $(shell go env GOARCH)

ifeq ($(OS_NAME), Linux)
	SCALER_DEFAULT_TEST_HOST := $(shell docker network inspect bridge | grep "Gateway" | grep -o '"[^"]*"$$')
# On EC2 we don't have gateway, use default
	ifeq ($(SCALER_DEFAULT_TEST_HOST),)
		SCALER_DEFAULT_TEST_HOST := "172.17.0.1"
	endif
else
	SCALER_DEFAULT_TEST_HOST := "docker.for.mac.host.internal"
endif

SCALER_DOCKER_TEST_DOCKERFILE_PATH := test/Dockerfile
SCALER_DOCKER_TEST_TAG := scaler/tester

SCALER_OS := $(if $(SCALER_OS),$(SCALER_OS),$(SCALER_DEFAULT_OS))
SCALER_ARCH := $(if $(SCALER_ARCH),$(SCALER_ARCH),$(SCALER_DEFAULT_ARCH))
SCALER_LABEL := $(if $(SCALER_LABEL),$(SCALER_LABEL),latest)
SCALER_TEST_HOST := $(if $(SCALER_TEST_HOST),$(SCALER_TEST_HOST),$(SCALER_DEFAULT_TEST_HOST))
SCALER_VERSION_GIT_COMMIT = $(shell git rev-parse HEAD)

AUTOSCALER_DOCKER_REPO = quay.io/v3io/autoscaler
DLX_DOCKER_REPO = quay.io/v3io/dlx

#
# Docker build
#

.PHONY: docker-images
docker-images: autoscaler-onbuild dlx-onbuild

.PHONY: autoscaler-onbuild
autoscaler-onbuild:
	@echo Building autoscaler-onbuild
	docker build -f cmd/autoscaler/Dockerfile -t $(AUTOSCALER_DOCKER_REPO):$(SCALER_LABEL) .

.PHONY: dlx-onbuild
dlx-onbuild:
	@echo Building dlx-onbuild
	docker build -f cmd/dlx/Dockerfile -t $(DLX_DOCKER_REPO):$(SCALER_LABEL) .

.PHONY: push-docker-images
push-docker-images:
	@echo Pushing images
	docker push $(DLX_DOCKER_REPO):$(SCALER_LABEL)
	docker push $(AUTOSCALER_DOCKER_REPO):$(SCALER_LABEL)

#
# Build helpers
#

# tools get built with the specified OS/arch and inject version
GO_BUILD_TOOL_WORKDIR = /scaler
GOLANGCI_LINT_VERSION := v1.64.6

.PHONY: lint
lint: modules
	@test -e .bin/golangci-lint || \
    	  	(curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b .bin $(GOLANGCI_LINT_VERSION))

	@echo Linting...
	.bin/golangci-lint run -v
	@echo Done.

.PHONY: fmt
fmt:
	gofmt -s -w .
	.bin/golangci-lint run --fix ./...

.PHONY: test-undockerized
test-undockerized: modules
	go test -race -v ./pkg/... -p 1

.PHONY: test
test:
	docker build --file $(SCALER_DOCKER_TEST_DOCKERFILE_PATH) \
	--tag $(SCALER_DOCKER_TEST_TAG) .

	docker run --rm --volume /var/run/docker.sock:/var/run/docker.sock \
	--volume $(shell pwd):$(GO_BUILD_TOOL_WORKDIR) \
	--volume /tmp:/tmp \
	--workdir $(GO_BUILD_TOOL_WORKDIR) \
	--env SCALER_TEST_HOST=$(SCALER_TEST_HOST) \
	$(SCALER_DOCKER_TEST_TAG) \
	/bin/bash -c "make test-undockerized"


.PHONY: modules
modules:
	@echo Getting go modules
	@go mod download
