#!/usr/bin/env bash

# scripts must be run from project root
. hack/1-bin.sh || exit 1

# consts

CERT_MANAGER_YAML=${CERT_MANAGER_YAML:-"https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml"} # v1.18 for k8s v1.29-v1.33 https://cert-manager.io/docs/releases/
METRICS_SERVER_YAML=${METRICS_SERVER_YAML:-"https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.0/components.yaml"} # v0.8 for k8s v1.31+ https://github.com/kubernetes-sigs/metrics-server?tab=readme-ov-file#compatibility-matrix
METRICS_SERVER_PATCH=${METRICS_SERVER_PATCH:-'''[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'''}

# libs

# Usage: lib::create-cluster <name> <kind_image>
function lib::create-cluster {
    local kind_name=$1
    local name=kind-$1
    local kind_image=$2

    test -s "$KIND"
    test -s "$KUBECTL"

    "$KIND" delete cluster --name "$kind_name"

    "$KIND" create cluster --name "$kind_name" --image="$kind_image" --config=- <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
featureGates:
  NFTablesProxyMode: true
EOF

    "$KUBECTL" apply -f "$METRICS_SERVER_YAML"
    "$KUBECTL" patch -n kube-system deployment metrics-server --type=json -p "$METRICS_SERVER_PATCH"

    "$KUBECTL" apply -f "$CERT_MANAGER_YAML"
    "$KUBECTL" wait deploy -ncert-manager cert-manager --for=condition=Available=True --timeout=60s
    "$KUBECTL" wait deploy -ncert-manager cert-manager-cainjector --for=condition=Available=True --timeout=60s
    "$KUBECTL" wait deploy -ncert-manager cert-manager-webhook --for=condition=Available=True --timeout=60s
}

# Usage: lib::start-docker
function lib::start-docker {

    set +o nounset
    if [ "$CI" == "true" ]; then return 0; fi
    set -o nounset

    sudo systemctl start docker || sudo service docker start || true
    sleep 1

    # https://kind.sigs.k8s.io/docs/user/known-issues/#pod-errors-due-to-too-many-open-files
    sudo sysctl fs.inotify.max_user_watches=524288 || true
    sudo sysctl fs.inotify.max_user_instances=512 || true
}
