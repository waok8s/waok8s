#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts
KIND_CLUSTER_NAME=$PROJECT_NAME-test

VERSION=v0.0.1-dev # always use the same version to reuse config files
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler

IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME:$VERSION

# main

make image IMAGE_REGISTRY=$IMAGE_REGISTRY IMAGE_NAME=$IMAGE_NAME VERSION="$VERSION"

lib::run-tests test "$IMAGE" "$KIND_CLUSTER_NAME"
