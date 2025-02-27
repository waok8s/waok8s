VERSION?=$(shell git describe --tags --match "wao-core/v*" | sed 's|^wao-core/||')
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler

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
	docker build --progress=plain --build-arg VERSION=$(VERSION) -f ./Dockerfile -t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION) .

.PHONY: push
push:
	docker push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION)

.PHONY: build
build: gen kube-scheduler

.PHONY: kube-scheduler
kube-scheduler:
	go build $(GO_BUILD_ARGS) -o bin/kube-scheduler cmd/kube-scheduler/main.go
