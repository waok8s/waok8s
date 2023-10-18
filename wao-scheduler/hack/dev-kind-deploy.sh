#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts
KIND_CLUSTER_NAME=$PROJECT_NAME
CLUSTER_NAME=kind-$KIND_CLUSTER_NAME

VERSION=v0.0.1-dev # always use the same version to reuse config files
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler

IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME:$VERSION

# main

make image IMAGE_REGISTRY=$IMAGE_REGISTRY IMAGE_NAME=$IMAGE_NAME VERSION="$VERSION"

lib::deploy-scheduler config/base "$IMAGE" "$KIND_CLUSTER_NAME"

echo ''
echo 'Completed!'
echo ''
echo 'Check logs:'
echo "    kubectl logs $($KUBECTL get pods -l component=scheduler -o jsonpath="{.items[0].metadata.name}" --field-selector=status.phase=Running -n kube-system) -f -nkube-system"
echo 'Run a Deployment:'
echo '    kubectl delete -f config/samples/dep.yaml ; kubectl apply -f config/samples/dep.yaml && sleep 2 && kubectl get pod'
