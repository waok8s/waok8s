#!/usr/bin/env bash

kubectl -n kube-system set image daemonset/kube-proxy kube-proxy=localhost/wao-loadbalancer:v0.0.1-dev
