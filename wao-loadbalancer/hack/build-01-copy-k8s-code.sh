#!/usr/bin/env bash

set -e -x

K8S_VERSION="v1.30.0"
K8S_DIR="bin/src/kubernetes"

# main

# clone k8s code
git clone --branch "${K8S_VERSION}" --depth 1 https://github.com/kubernetes/kubernetes.git "${K8S_DIR}" || true

# copy k8s code

# `k8s.io/kubernetes/pkg/proxy` -> `pkg/proxy`
mkdir -p pkg 
cp -r "${K8S_DIR}"/pkg/proxy pkg/

# `k8s.io/kubernetes/cmd/kube-proxy` -> `cmd/kube-proxy`
mkdir -p cmd
cp -r "${K8S_DIR}"/cmd/kube-proxy cmd/
