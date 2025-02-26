#!/usr/bin/env bash

# scripts must be run from project root
. hack/0-env.sh || exit 1

### main ###

test -s "$KIND" || GOBIN="$LOCALBIN" go install sigs.k8s.io/kind@"$KIND_VERSION"
test -s "$KUBECTL" || (mkdir -p "$KUBECTL_DIR" ; curl -L https://dl.k8s.io/release/"$KUBECTL_VERSION"/bin/linux/amd64/kubectl > "$KUBECTL" ; chmod +x "$KUBECTL")
test -s "$KUSTOMIZE" || (curl -s -L https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F"$KUSTOMIZE_VERSION"/kustomize_"$KUSTOMIZE_VERSION"_linux_amd64.tar.gz | tar xvzf - -C "$LOCALBIN")
test -s "$DOCKER" || (echo "[ERROR] command not found: docker" && exit 1)

echo -e "= version info =\n"

"$KIND" version
"$KUBECTL" version -oyaml
"$KUSTOMIZE" version -oyaml
"$DOCKER" version

echo -e "================\n"
