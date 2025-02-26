#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts
KIND_CLUSTER_NAME=$PROJECT_NAME
CLUSTER_NAME=kind-$KIND_CLUSTER_NAME

VERSION=$(git describe --tags --match "v*")
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-metrics-adapter

IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME:$VERSION

# main

# label nodes
"$KUBECTL" label node "$KIND_CLUSTER_NAME"-worker   --overwrite "hoge"="fuga"
"$KUBECTL" label node "$KIND_CLUSTER_NAME"-worker2  --overwrite "hoge"="fuga"

# load image
make image IMAGE_REGISTRY=$IMAGE_REGISTRY IMAGE_NAME=$IMAGE_NAME VERSION="$VERSION"
"$KIND" load docker-image "$IMAGE" -n "$KIND_CLUSTER_NAME"

# deploy
cd config/base && "$KUSTOMIZE" edit set image wao-metrics-adapter="$IMAGE" && cd -
"$KUBECTL" delete -k config/base || true
"$KUBECTL" apply -k config/base
sleep 2

echo ''
echo 'Completed!'
echo ''
echo 'Check logs:'
echo "    kubectl logs $($KUBECTL get pods -l app=wao-metrics-adapter -o jsonpath="{.items[0].metadata.name}" --field-selector=status.phase=Running -n custom-metrics) -f -ncustom-metrics"
echo ''
echo 'Fetch metrics:'
echo '    kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta2/nodes/wao-metrics-adapter-worker/inlet_temp"'
