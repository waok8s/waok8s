#!/usr/bin/env bash

# scripts must be run from project root
. hack/1-bin.sh || exit 1

# consts
IMAGE=wao-scheduler
DIST=wao-scheduler.yaml

VERSION=$(git describe --tags --match "wao-scheduler/v*" | sed 's|^wao-scheduler/||')
REGISTRY=${REGISTRY:-""}
IMAGE_NAME=$IMAGE:$VERSION
if [ -n "$REGISTRY" ] ; then IMAGE_NAME=$REGISTRY/$IMAGE:$VERSION ; fi

# main
cd config/base && "$KUSTOMIZE" edit set image localhost/wao-scheduler="$IMAGE_NAME" && cd -
cd config/base && "$KUSTOMIZE" edit remove resource "deps/*" && cd -
"$KUSTOMIZE" build config/base > $DIST
