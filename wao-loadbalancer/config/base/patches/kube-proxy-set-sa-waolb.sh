#!/usr/bin/env bash

kubectl -n kube-system set serviceaccount daemonset/kube-proxy wao-loadbalancer
