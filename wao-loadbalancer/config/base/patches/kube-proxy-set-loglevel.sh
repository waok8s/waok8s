#!/usr/bin/env bash

kubectl -n kube-system patch daemonset kube-proxy --type='json' \
-p='[{"op": "add", "path": "/spec/template/spec/containers/0/command/-", "value": "--v=5"}]'
