#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT=$(realpath "$0")
PROJECT_ROOT=$(dirname "$(dirname "$SCRIPT")")
PROJECT_NAME=$(basename "$PROJECT_ROOT")

KUBECTL_VERSION=v1.30.0
KUSTOMIZE_VERSION=v5.4.1
KIND_VERSION=v0.23.0

LOCALBIN=$PROJECT_ROOT/bin # bin/

KIND=$LOCALBIN/kind # bin/kind
KUBECTL_DIR=$LOCALBIN/kubectl-$KUBECTL_VERSION # bin/kubectl-v1.30.0
KUBECTL=$KUBECTL_DIR/kubectl # bin/kubectl-v1.30.0/kubectl
KUSTOMIZE=$LOCALBIN/kustomize # bin/kustomize

DOCKER=$(which docker) # assumes docker command is available
