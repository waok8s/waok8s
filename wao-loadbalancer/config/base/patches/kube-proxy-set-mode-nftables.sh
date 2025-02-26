#!/usr/bin/env bash

echo "=== diff ==="
kubectl get configmap kube-proxy -n kube-system -o yaml | \
sed -e "s# mode: iptables# mode: nftables#" | \
kubectl diff -f - -n kube-system || true
echo "============"

echo ""

echo "=== apply ==="
kubectl get configmap kube-proxy -n kube-system -o yaml | \
sed -e "s# mode: iptables# mode: nftables#" | \
kubectl apply -f - -n kube-system
echo "============"
