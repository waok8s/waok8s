#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts

KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.30.0@sha256:047357ac0cfea04663786a612ba1eaba9702bef25227a794b52890dd8bcd692e"}

# main

cluster=$PROJECT_NAME

lib::start-docker

lib::create-cluster "$cluster" "$KIND_IMAGE"
