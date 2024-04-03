#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts

KIND_IMAGE=${KIND_IMAGE:-"kindest/node:v1.28.7@sha256:9bc6c451a289cf96ad0bbaf33d416901de6fd632415b076ab05f5fa7e4f65c58"}

# main

cluster=$PROJECT_NAME

lib::start-docker

lib::create-cluster "$cluster" "$KIND_IMAGE"
