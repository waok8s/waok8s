#!/usr/bin/env bash

VERSION=$(git describe --tags --match "v*")
IMAGE=wao-controller
REGISTRY=${REGISTRY:-""}

IMAGE_NAME=$BIN:$VERSION
if [ -n "$REGISTRY" ]; then
    IMAGE_NAME=$REGISTRY/$IMAGE:$VERSION
fi
DIST=wao-core-$VERSION.yaml

make kustomize

cd config/manager && "../../bin/kustomize" edit set image controller="$IMAGE_NAME" && cd -
"./bin/kustomize" build config/default > $DIST
