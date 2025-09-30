```sh
kubectl apply -f .
kubectl exec nginx -- curl nginx-normal # kube-proxy
kubectl exec nginx -- curl nginx-waolb # wao-loadbalancer
```

```sh
kubectl exec -n kube-system wao-loadbalancer-xxxxx -- nft list ruleset
```
