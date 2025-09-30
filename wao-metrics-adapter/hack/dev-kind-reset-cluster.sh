#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts

# KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a"}
# KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.33.4@sha256:25a6018e48dfcaee478f4a59af81157a437f15e6e140bf103f85a2e7cd0cbbf2"}
# KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.32.8@sha256:abd489f042d2b644e2d033f5c2d900bc707798d075e8186cb65e3f1367a9d5a1"}
KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.31.12@sha256:0f5cc49c5e73c0c2bb6e2df56e7df189240d83cf94edfa30946482eb08ec57d2"}

# main

cluster=$PROJECT_NAME

lib::start-docker

lib::create-cluster "$cluster" "$KIND_IMAGE"
