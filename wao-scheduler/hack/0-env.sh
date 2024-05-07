#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT=$(realpath "$0")
PROJECT_ROOT=$(dirname "$(dirname "$SCRIPT")")
PROJECT_NAME=$(basename "$PROJECT_ROOT")

KUBECTL_VERSION=v1.29.3
KUSTOMIZE_VERSION=v5.3.0
KIND_VERSION=v0.22.0

LOCALBIN=$PROJECT_ROOT/bin # bin/

KIND=$LOCALBIN/kind # bin/kind
KUBECTL_DIR=$LOCALBIN/kubectl-$KUBECTL_VERSION # bin/kubectl-v1.29.3
KUBECTL=$KUBECTL_DIR/kubectl # bin/kubectl-v1.29.3/kubectl
KUSTOMIZE=$LOCALBIN/kustomize # bin/kustomize

DOCKER=$(which docker) # assumes docker command is available
