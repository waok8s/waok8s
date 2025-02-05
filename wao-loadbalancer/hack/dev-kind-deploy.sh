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
. config/base/patches/kube-proxy-set-mode-nftables.sh
. config/base/patches/kube-proxy-set-image-waolb.sh
"$KUBECTL" rollout restart daemonset kube-proxy -n kube-system
"$KUBECTL" rollout status daemonset/kube-proxy -n kube-system --timeout=30s

echo ''
echo 'Completed!'
echo ''
# TODO: add instructions
# echo 'Check Pods:'
# echo "    kubectl logs $($KUBECTL get pods -l app=wao-scheduler -o jsonpath="{.items[0].metadata.name}" --field-selector=status.phase=Running -n kube-system) -f -nkube-system"
# echo 'Run a Deployment:'
# echo '    kubectl delete -f config/samples/dep.yaml ; kubectl apply -f config/samples/dep.yaml && sleep 2 && kubectl get pod'
