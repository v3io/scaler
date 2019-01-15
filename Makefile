GO_VERSION := $(shell go version | cut -d " " -f 3)
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

AUTOSCALER_DOCKER_REPO = levrado1/autoscaler-onbuild
DLX_DOCKER_REPO = levrado1/dlx-onbuild

#
# Docker build
#

.PHONY: autoscaler-onbuild
autoscaler-onbuild:
	@echo Building autoscaler-onbuild
	docker build -f cmd/autoscaler/Dockerfile -t $(AUTOSCALER_DOCKER_REPO):$(SCALER_LABEL) .

.PHONY: dlx-onbuild
dlx-onbuild:
	@echo Building dlx-onbuild
	docker build -f cmd/dlx/Dockerfile -t $(DLX_DOCKER_REPO):$(SCALER_LABEL) .

.PHONY: docker-images
docker-images: autoscaler-onbuild dlx-onbuild

.PHONY: push-docker-images
push-docker-images:
	@echo Pushing images
	docker push $(DLX_DOCKER_REPO):$(SCALER_LABEL)
	docker push $(AUTOSCALER_DOCKER_REPO):$(SCALER_LABEL)

#
# Build helpers
#

# tools get built with the specified OS/arch and inject version
GO_BUILD_TOOL_WORKDIR = /go/src/github.com/v3io/scaler
GO_BUILD_TOOL = docker run \
	--volume $(shell pwd):$(GO_BUILD_TOOL_WORKDIR) \
	--volume $(shell pwd)/../logger:$(GO_BUILD_TOOL_WORKDIR)/../logger \
	--volume $(GOPATH)/bin:/go/bin \
	--workdir $(GO_BUILD_TOOL_WORKDIR) \
	--env GOOS=$(SCALER_OS) \
	--env GOARCH=$(SCALER_ARCH) \
	golang:1.10 \
	go build -a \
	-installsuffix cgo \
	-ldflags="$(GO_LINK_FLAGS_INJECT_VERSION)"


.PHONY: lint
lint:
	@echo Installing linters...
	go get -u github.com/pavius/impi/cmd/impi
	go get -u gopkg.in/alecthomas/gometalinter.v2
	@$(GOPATH)/bin/gometalinter.v2 --install

	@echo Verifying imports...
	$(GOPATH)/bin/impi \
		--local github.com/v3io/scaler/ \
		--scheme stdLocalThirdParty \
		--skip pkg/platform/kube/apis \
		--skip pkg/platform/kube/client \
		./cmd/... ./pkg/...

	@echo Linting...
	@$(GOPATH)/bin/gometalinter.v2 \
		--deadline=300s \
		--disable-all \
		--enable-gc \
		--enable=deadcode \
		--enable=goconst \
		--enable=gofmt \
		--enable=golint \
		--enable=gosimple \
		--enable=ineffassign \
		--enable=interfacer \
		--enable=misspell \
		--enable=staticcheck \
		--enable=unconvert \
		--enable=varcheck \
		--enable=vet \
		--enable=vetshadow \
		--enable=errcheck \
		--exclude="_test.go" \
		--exclude="comment on" \
		--exclude="error should be the last" \
		--exclude="should have comment" \
		--skip=pkg/platform/kube/apis \
		--skip=pkg/platform/kube/client \
		./cmd/... ./pkg/...

	@echo Done.

.PHONY: test-undockerized
test-undockerized: ensure-gopath
	go test -v ./pkg/... -p 1

.PHONY: test
test: ensure-gopath
	docker build --file $(SCALER_DOCKER_TEST_DOCKERFILE_PATH) \
	--tag $(SCALER_DOCKER_TEST_TAG) .

	docker run --rm --volume /var/run/docker.sock:/var/run/docker.sock \
	--volume $(shell pwd):$(GO_BUILD_TOOL_WORKDIR) \
	--volume /tmp:/tmp \
	--workdir /go/src/github.com/v3io/scaler \
	--env SCALER_TEST_HOST=$(SCALER_TEST_HOST) \
	$(SCALER_DOCKER_TEST_TAG) \
	/bin/bash -c "make test-undockerized"

.PHONY: ensure-gopath
ensure-gopath:
ifndef GOPATH
	$(error GOPATH must be set)
endif
