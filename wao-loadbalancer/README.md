# Kube-proxy with built-in power minimization policy

---

Kubernetes is a portable, extensible, open source platform for facilitating declarative configuration management and automation, and managing containerized workloads and services. Kubernetes has a huge and fast-growing ecosystem with a wide range of services, support and tools available.

This document shows the steps to build and deploy kube-proxy with power minimization policy.

---

## Prerequisites

* [Go-lang v1.15.1](https://golang.org/)
* [Kubernetes v1.19.7](https://github.com/kubernetes/kubernetes/releases/tag/v1.19.7)
* [ipmi_exporter](https://github.com/soundcloud/ipmi_exporter)  
  Running on each node
* local docker repository  
* docker image of tensorflow serving containing power consumption model of each node.  

## Build kube-proxy with power minimization policy

1. Download the Kubernetes source code

    *Since the directory structure is different in the latest version, use [Kubernetes v1.19.7](https://github.com/kubernetes/kubernetes/releases/tag/v1.19.7)
    ```
    curl -L -o kubernetes.tar.gz https://github.com/kubernetes/kubernetes/archive/v1.19.7.tar.gz
    tar xvzf kubernetes.tar.gz
    ```

2. Add Kube-proxy source code with power minimization policy
    ```
    git clone https://github.com/kaz260/wao-ploxy
    ```
    After cloning, copy to `kubernetes/pkg/proxy/ipvs`  
    *Overwrite the file with the same name

3. Add the required packages

    Add the prom2json package
    ```
    go get github.com/prometheus/prom2json
    ```

4. Build a proxy

    Build results are output to `kubernetes/cmd/proxy/`
    ```
    cd ./kubernetes/cmd/proxy/
    CGO_ENABLED=0 go build -mod=mod proxy.go
    ```

## Deploy to Kubernetes

Since there are multiple files required for deployment, it is recommended to create a suitable directory and work in it.

1. Create a Docker image for kube-proxy

    Create `Dockerfile` with the following contents.
    The original image has been confirmed and may be up to date.
    ``` Dockerfile
    FROM k8s.gcr.io/kube-proxy:v1.18.8
    COPY ./proxy /usr/local/bin/kube-proxy
    ```
    Copy the `proxy` built in the above steps to the same directory as the `Dockerfile`.  
    Create an image and push it to your local repository.
    ```
    docker build -t [repository-address]/[image-name] .
    docker image push [repository-address]/[image-name]
    ```

2. Preparing to start kube-proxy

    The following preparations are required for the first statup.
    * Change to a setting that users ipvs
        ```
        kubectl edit configmap kube-proxy -n kube-system
        Change two lines like below.
        mode: "ipvs"
        scheduler: "wrr"
        ```
    * Add role so that kube-proxy can get pods informations.
        ```
        kubectl create clusterrolebinding default-view --clusterrole=view --serviceaccount=kube-system:kube-proxy
        ```
    * Launch metrics-server ([Kubernetes Metrics Server](https://github.com/kubernetes-sigs/metrics-server))
        ```
        kubectl apply -f kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.6.1/components.yaml
        ```
    * Launch tensorflow

        ```
        kubectl create -f tensorflow-server-dep.yaml
        ```
    * Labeling
        Give each node the following label;
        * ambient/max : Maximum ambinet temperature in celsius
        * ambient/min : Mimimum ambinet temperature in celsius
        * cpu1/max : Maximum CPU1 temperature in celsius
        * cpu1/min : Minimum CPU1 temperature in celsius
        * cpu2/max : Maximum CPU2 temperature in celsius
        * cpu2/min : Minimum CPU2 temperature in celsius
        * tensorflow/host: IP address of tensorflow serving
        * tensorflow/port: Port number of tensorflow serving
        * tensorflow/name: model name of tensorflow serving

3. Launch kube-proxy

    ```
    Stop the currently running kube-proxy
    ```
    kubectl delete daemonset -n kube-system kube-proxy
    ```
    Lauch a new kube-proxy
    ```
    kubectl create -f kube-proxy.yaml
    ```
    Success if you can confirm the startup on each node with the following command
    (Successful if the pod status is [Running])
    ```
    kubectl get pod -n kube-system -o wide | grep kube-proxy
    ```
    If you want to see the result of ipvs, you need to install ipvsadm
    ```
    Installation
    ```
    sudo apt install ipset ipvsadm -y
    ```
    Verification
    ```
    sudo ipvsadm -Ln
    ```
