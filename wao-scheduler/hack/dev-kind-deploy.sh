#!/usr/bin/env bash

# scripts must be run from project root
. hack/1-bin.sh || exit 1

# consts
KIND_CLUSTER_NAME=$PROJECT_NAME
CLUSTER_NAME=kind-$KIND_CLUSTER_NAME

VERSION=v0.0.1-dev # always use the same version to reuse config files
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler-v2

IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME:$VERSION

CONTAINER_NAME=$KIND_CLUSTER_NAME-control-plane
SCHED_POD=kube-scheduler-$CONTAINER_NAME

# main

command -v docker

# build image
make build-image IMAGE_REGISTRY=$IMAGE_REGISTRY IMAGE_NAME=$IMAGE_NAME VERSION="$VERSION"
# load image
"$KIND" load docker-image "$IMAGE" -n "$KIND_CLUSTER_NAME"
# load config
docker cp config/samples/kube-scheduler.yaml "$CONTAINER_NAME":/etc/kubernetes/manifests/kube-scheduler.yaml
docker cp config/samples/kube-scheduler-configuration.yaml "$CONTAINER_NAME":/etc/kubernetes/kube-scheduler-configuration.yaml

# change docker image
"$KUBECTL" set image pod/"$SCHED_POD" kube-scheduler="$IMAGE" -nkube-system
# reset the scheduler
"$KUBECTL" delete pod "$SCHED_POD" -nkube-system

"$KUBECTL" wait pod "$SCHED_POD" -nkube-system --for condition=Ready --timeout=60s

echo ''
echo 'Completed!'
echo ''
echo 'check logs:'
echo "    kubectl logs $SCHED_POD -f -nkube-system"
echo 'run a Deployment:'
echo '    kubectl delete -f config/samples/dep.yaml ; kubectl apply -f config/samples/dep.yaml && sleep 2 && kubectl get pod'
