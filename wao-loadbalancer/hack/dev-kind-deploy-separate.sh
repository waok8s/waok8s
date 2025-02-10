#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts
KIND_CLUSTER_NAME=$PROJECT_NAME
CLUSTER_NAME=kind-$KIND_CLUSTER_NAME

VERSION=v0.0.1-dev # always use the same version to reuse config files
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-loadbalancer

IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME:$VERSION

# main

make image IMAGE_REGISTRY=$IMAGE_REGISTRY IMAGE_NAME=$IMAGE_NAME VERSION="$VERSION"

# deploy WAO
"$KUBECTL" apply -f config/base/deps

# deploy WAO-LB
"$KIND" load docker-image "$IMAGE" -n "$KIND_CLUSTER_NAME"
. config/base/patches/kube-proxy-set-mode-nftables.sh # our implementation ignores this value but we cannot have both modes at the same time (it causes iptables one to fail)
"$KUBECTL" rollout restart daemonset kube-proxy -n kube-system
"$KUBECTL" rollout status daemonset kube-proxy -n kube-system --timeout=30s
"$KUBECTL" apply -k config/base
"$KUBECTL" rollout restart daemonset wao-loadbalancer -n kube-system
"$KUBECTL" rollout status daemonset wao-loadbalancer -n kube-system --timeout=30s

echo ''
echo 'Completed!'
echo ''
echo 'Run the sample Deployment and Services:'
echo '    kubectl delete -f config/samples ; kubectl apply -f config/samples && sleep 2 && kubectl get endpointslice'
