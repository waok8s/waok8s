#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts

KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.29.2@sha256:51a1434a5397193442f0be2a297b488b6c919ce8a3931be0ce822606ea5ca245"}

# main

cluster=$PROJECT_NAME

lib::start-docker

lib::create-cluster "$cluster" "$KIND_IMAGE"
