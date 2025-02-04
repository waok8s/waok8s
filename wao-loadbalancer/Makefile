VERSION?=$(shell git describe --tags --match "v*")
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-loadbalancer

GO_BUILD_ARGS=-trimpath -ldflags "-s -w -X k8s.io/component-base/version.gitVersion=$(VERSION)"

-include .env

.PHONY: all
all: test build image

.PHONY: gen
gen:
	go generate ./...

.PHONY: test
test: gen
	go test -v -race -coverprofile=cover.out ./...

.PHONY: image
image: gen
	docker build -f ./Dockerfile -t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION) .

.PHONY: push
push:
	docker push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION)

.PHONY: build
build: gen kube-proxy

.PHONY: kube-proxy
kube-proxy:
	go build $(GO_BUILD_ARGS) -o bin/kube-proxy cmd/kube-proxy/proxy.go
