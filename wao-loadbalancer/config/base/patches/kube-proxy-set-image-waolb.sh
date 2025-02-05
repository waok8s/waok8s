#!/usr/bin/env bash

kubectl -n kube-system set image daemonset/kube-proxy kube-proxy=localhost/wao-loadbalancer:v0.0.1-dev

kubectl -n kube-system patch daemonset kube-proxy --type='json' \
-p='[{"op": "add", "path": "/spec/template/spec/containers/0/command/-", "value": "--v=5"}]'
