#!/usr/bin/env bash

# scripts must be run from project root
. hack/2-lib.sh || exit 1

# consts
KIND_CLUSTER_NAME=$PROJECT_NAME-test

VERSION=v0.0.1-dev # always use the same version to reuse config files
IMAGE_REGISTRY=localhost
IMAGE_NAME=wao-scheduler-v2

IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME:$VERSION

CONTAINER_NAME=$KIND_CLUSTER_NAME-control-plane
SCHED_POD=kube-scheduler-$CONTAINER_NAME

# lib

# Usage: reload-scheduler-conf <kube-scheduler.yaml> <kube-scheduler-configuration.yaml> <scheduler-image-name>
function load-scheduler {
    local sched_yaml=$1
    local sched_conf_yaml=$2
    local sched_image=$3

    command -v docker

    # load image to the KIND cluster
    "$KIND" load docker-image "$IMAGE" -n "$KIND_CLUSTER_NAME"

    # load config
    docker cp "$sched_yaml" "$CONTAINER_NAME":/etc/kubernetes/manifests/kube-scheduler.yaml
    docker cp "$sched_conf_yaml" "$CONTAINER_NAME":/etc/kubernetes/kube-scheduler-configuration.yaml

    # change container image
    "$KUBECTL" set image pod/"$SCHED_POD" kube-scheduler="$sched_image" -nkube-system

    # reset the scheduler
    "$KUBECTL" delete pod "$SCHED_POD" -nkube-system
    "$KUBECTL" wait pod "$SCHED_POD" -nkube-system --for condition=Ready --timeout=120s
}

# Usage: run-test <cmd_file> <expected_stdout>
function run-test {
    local cmd_file=$1
    local want_file=$2

    local want
    want=$(cat "$want_file")
    local got
    got=$(. "$cmd_file")

    if [[ "$got" == "$want" ]]; then
        printf "[OK] got=%s want=%s\n" "$got" "$want"
    else
        printf "[NG] got=%s want=%s\n" "$got" "$want"
        return 1
    fi
}

# main

make build-image IMAGE_REGISTRY=$IMAGE_REGISTRY IMAGE_NAME=$IMAGE_NAME VERSION="$VERSION"

cd test || exit 1

for d in case*/ ; do

    echo
    echo "################################"
    echo "# case" "$d"
    echo "# step 1/4: load kube-scheduler"
    echo "# "
    load-scheduler "$d/config/kube-scheduler.yaml" "$d/config/kube-scheduler-configuration.yaml" "$IMAGE"

    echo
    echo "################################"
    echo "# case" "$d"
    echo "# step 2/4: apply manifests"
    echo "# "
    "$KUBECTL" apply -f "$d/apply"

    echo
    echo "################################"
    echo "# case" "$d"
    echo "# step 3/4: do tests"
    echo "# "
    for f in "$d"/test/*.in ; do
        f2="${f%.in}.out"
        printf "[TEST] in=%s out=%s\n" "$f" "$f2"
        lib::retry 8 run-test "$f" "$f2" # total wait time = sum(1..8) = 36s 
    done

    echo
    echo "################################"
    echo "# case" "$d"
    echo "# step 4/4: cleanup"
    echo "# "
    "$KUBECTL" delete -f "$d/apply"

done
