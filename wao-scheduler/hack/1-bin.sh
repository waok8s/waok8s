#!/usr/bin/env bash

# scripts must be run from project root
. hack/0-env.sh || exit 1

### main ###

test -s "$KIND" || GOBIN="$LOCALBIN" go install sigs.k8s.io/kind@"$KIND_VERSION"
test -s "$KUBECTL" || (mkdir -p "$KUBECTL_DIR" ; curl -L https://dl.k8s.io/release/"$KUBECTL_VERSION"/bin/linux/amd64/kubectl > "$KUBECTL" ; chmod +x "$KUBECTL")

echo -e "= version info ="

"$KIND" version
"$KUBECTL" version --client

echo -e "================\n"
