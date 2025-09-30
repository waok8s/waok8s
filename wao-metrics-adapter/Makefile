VERSION?=$(shell git describe --tags --match "wao-metrics-adapter/v*" | sed 's|^wao-metrics-adapter/||')
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-metrics-adapter

GO_BUILD_ARGS=-trimpath -ldflags "-s -w"

-include .env

.PHONY: all
all: gen test build image

.PHONY: gen
gen:
	go generate ./...

# .PHONY: test
# test: gen
# 	go test -v -cover -race ./...

.PHONY: image
image: gen
	docker build --progress=plain -f ./Dockerfile -t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION) .

.PHONY: push
push:
	docker push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION)

.PHONY: build
build: gen
	go build $(GO_BUILD_ARGS) -o bin/adapter cmd/adapter/main.go

#########################################
# envtest copied from wao-core/Makefile #
#########################################

.PHONY: test
test: envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

# Version of controller-runtime to use for envtest-setup
CONTROLLER_RUNTIME_VERSION = v0.19.7
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
ENVTEST ?= $(LOCALBIN)/setup-envtest

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(CONTROLLER_RUNTIME_VERSION)
