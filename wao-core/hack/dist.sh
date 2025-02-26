#!/usr/bin/env bash

# consts
IMAGE=wao-controller
DIST=wao-core.yaml

VERSION=$(git describe --tags --match "v*")
REGISTRY=${REGISTRY:-""}
IMAGE_NAME=$IMAGE:$VERSION
if [ -n "$REGISTRY" ] ; then IMAGE_NAME=$REGISTRY/$IMAGE:$VERSION ; fi

# main
make kustomize
cd config/manager && "../../bin/kustomize" edit set image controller="$IMAGE_NAME" && cd -
"./bin/kustomize" build config/default > $DIST
