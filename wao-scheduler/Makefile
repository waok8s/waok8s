BIN_NAME=kube-scheduler
VERSION?=$(shell git describe --tags --match "v*")
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler-v2

-include .env

all: test build-bin build-image

push-image:
	docker push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION)

build-image:
	docker build -f ./Dockerfile -t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION) .

build-bin:
	go build -trimpath -ldflags "-s -w -X k8s.io/component-base/version.gitVersion=$(VERSION)" -o bin/$(BIN_NAME) cmd/$(BIN_NAME)/main.go

test:
	go test -v -cover -race ./...
