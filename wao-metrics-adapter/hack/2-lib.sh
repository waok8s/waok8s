#!/usr/bin/env bash

# scripts must be run from project root
. hack/1-bin.sh || exit 1

# consts

CERT_MANAGER_YAML=${CERT_MANAGER_YAML:-"https://github.com/cert-manager/cert-manager/releases/download/v1.14.5/cert-manager.yaml"}
METRICS_SERVER_YAML=${METRICS_SERVER_YAML:-"https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.7.1/components.yaml"}
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

# Usage: lib::deploy-scheduler <manifests> <scheduler_image_name> <kind_cluster_name>
function lib::deploy-scheduler {
    local sched_yaml=$1
    local sched_image=$2
    local kind_cluster_name=$3

    local cp_container="$kind_cluster_name"-control-plane

    command -v "$DOCKER"

    # load image to the KIND cluster
    "$KIND" load docker-image "$sched_image" -n "$kind_cluster_name"

    # apply manifests
    "$KUBECTL" delete -k "$sched_yaml" || true
    "$KUBECTL" apply -k "$sched_yaml"
    sleep 2
    "$KUBECTL" wait pod $($KUBECTL get pods -l component=scheduler -o jsonpath="{.items[0].metadata.name}" -n kube-system) -nkube-system --for condition=Ready --timeout=120s
}

# Usage: lib::run-test <cmd_file> <expected_stdout>
function lib::run-test {
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

# Usage: lib::retry <max_attempts> <cmd>...
# Example: lib::retry 5 ls foo
#          lib::retry 5 timeout 3 curl 8.8.8.8
# Ref.: https://stackoverflow.com/questions/12321469/retry-a-bash-command-with-timeout
function lib::retry {
    local -r -i max_attempts="$1"; shift
    local -r cmd=$*
    local -i attempt_num=1

    until $cmd
    do
        if (( attempt_num == max_attempts ))
        then
            echo "attempt $attempt_num/$max_attempts failed, exit(1)"
            return 1
        else
            echo "attempt $attempt_num/$max_attempts failed, trying again in $attempt_num seconds..."
            sleep $(( attempt_num++ ))
        fi
    done
}

# Usage: lib::run-tests <cases_dir> <scheduler_image_name> <kind_cluster_name>
function lib::run-tests {
    local cases_dir=$1
    local sched_image=$2
    local kind_cluster_name=$3

    cd $1 || exit 1

    for d in case*/ ; do

        echo
        echo "################################"
        echo "# case" "$d"
        echo "# step 1/4: load kube-scheduler"
        echo "# "
        lib::deploy-scheduler "$d/config" "$sched_image" "$kind_cluster_name"

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
            lib::retry 8 lib::run-test "$f" "$f2" # total wait time = sum(1..8) = 36s 
        done

        echo
        echo "################################"
        echo "# case" "$d"
        echo "# step 4/4: cleanup"
        echo "# "
        "$KUBECTL" delete -f "$d/apply"

    done

}
