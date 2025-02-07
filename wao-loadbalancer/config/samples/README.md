```sh
kubectl apply -f .
kubectl exec nginx -- curl nginx-normal # kube-proxy
kubectl exec nginx -- curl nginx-waolb # wao-loadbalancer
```
