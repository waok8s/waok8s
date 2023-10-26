VERSION?=$(shell git describe --tags --match "v*")
GO_BUILD_ARGS=-trimpath -ldflags "-s -w -X main.version=$(VERSION)"
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler

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
build: gen kube-scheduler

.PHONY: kube-scheduler
kube-scheduler:
	go build $(GO_BUILD_ARGS) -o bin/kube-scheduler cmd/kube-scheduler/main.go
