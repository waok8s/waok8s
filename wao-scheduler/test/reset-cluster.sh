#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts

KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.25.3@sha256:f52781bc0d7a19fb6c405c2af83abfeb311f130707a0e219175677e366cc45d1"}

# main

cluster=$PROJECT_NAME-test

lib::start-docker

lib::create-cluster "$cluster" "$KIND_IMAGE"

"$KUBECTL" get nodes
