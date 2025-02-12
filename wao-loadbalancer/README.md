# WAO Load Balancer Version 2

A kube-proxy with energy-aware load balancing feature.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
  - [Architecture](#architecture)
  - [Weight Calculation](#weight-calculation)
- [Getting Started](#getting-started)
  - [Installation](#installation)
    - [Use WAO-LB as Non-Default Service Proxy (Recommended)](#use-wao-lb-as-non-default-service-proxy-recommended)
    - [Use WAO-LB as the Default Service Proxy](#use-wao-lb-as-the-default-service-proxy)
  - [Deploy Services](#deploy-services)
  - [Check Current Weights](#check-current-weights)
- [Configuration](#configuration)
  - [Service Configuration](#service-configuration)
  - [WAO-LB Configuration](#wao-lb-configuration)
- [Development](#development)
  - [Components](#components)
- [Changelog](#changelog)
- [Acknowledgements](#acknowledgements)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

WAO Load Balancer (WAO-LB) is a custom kube-proxy that uses WAO to achieve energy-aware load balancing. Calculating the weight of each node based on the power consumption model, then updating the weight of the node in the kube-proxy implementation. Only kube-proxy in `nftables` mode is supported.

> [!NOTE]
> We previously supported `ipvs` mode, but it is now removed.

### Architecture

Basically same as kube-proxy.

### Weight Calculation

The weight of each node is calculated based on its estimated delta power consumption per request.

The weight of each node is calculated based on its estimated delta power consumption per request. The idea is to assign a score in the range `(0, 100]` such that a lower delta power (i.e., more energy-efficient) corresponds to a higher score. 

When using this method, the node with the lowest delta power consumption receives the maximum score of 100, and nodes with higher power consumption receive proportionally lower scores.

$score_i = 100 \times \frac{\min(\mathbf{deltaPowers})}{power_i}$

## Getting Started

### Installation

> [!NOTE]
> Make sure you have [wao-core](https://github.com/waok8s/wao-core) and [wao-metrics-adapter](https://github.com/waok8s/wao-metrics-adapter) set up.

There are two ways to use WAO-LB: as a non-default service proxy or as the default service proxy, and there are some notable points:

- In both modes:
  - WAO-LB is based on kube-proxy. We just changed some logic.
  - Proxy mode is always `nftables`, the value in config file is ignored.
  - nftables table name is `wao-loadbalancer` instead of `kube-proxy`.
- In non-default service proxy mode:
  - Healthz server is running on `0.0.0.0:10356`, the value in config file is ignored.
  - Metrics server is running on `0.0.0.0:10349`, the value in config file is ignored.
  - Environment variable `WAO_SERVICE_PROXY_NAME` must be set (this triggers the non-default service proxy mode, recommended value is `wao-loadbalancer`).
- In default service proxy mode:
  - Healthz server and metrics server are set by the config file.
  - Environment variable `WAO_SERVICE_PROXY_NAME` must be empty or not set.

#### Use WAO-LB as Non-Default Service Proxy (Recommended)

Kubernetes has `service.kubernetes.io/service-proxy-name` label for this purpose.
Set the label with specific value to Services, then the default kube-proxy will ignore them.

So you can run WAO-LB as a non-default service proxy by following these steps:
1. Edit kube-proxy ConfigMap to set the proxy mode to `nftables`.
  - WAO-LB ignores this value, but `iptables` kube-proxy fails if there are another kube-proxy in `nftables` mode.
2. Deploy the WAO-LB as a non-default service proxy.
  - Environment variable `WAO_SERVICE_PROXY_NAME` must be set.
3. Set the label `service.kubernetes.io/service-proxy-name: wao-loadbalancer` to Services that you want to use WAO-LB.
  - If you want to change service proxy name of WAO-LB, edit environment variable `WAO_SERVICE_PROXY_NAME`.

> [!NOTE]
> This kube-proxy feature is not described in the official documentation yet, but can be found in [KEP-2447](https://github.com/kubernetes/enhancements/tree/13a4bd1c2eb29d39275ba433ecf952882e0092c5/keps/sig-network/2447-Make-kube-proxy-service-abstraction-optional), and also supported by other service proxies (e.g., [Kube-router](https://github.com/cloudnativelabs/kube-router/issues/979), [Cilium](https://docs.cilium.io/en/stable/network/kubernetes/kubeproxy-free/)).

#### Use WAO-LB as the Default Service Proxy

WAO-LB can be used as a drop-in replacement for the default kube-proxy by following these steps:
1. Replace the container image of kube-proxy with our custom image.
  - Ensure that the environment variable `WAO_SERVICE_PROXY_NAME` is empty or not set.

### Deploy Services

> [!NOTE]
> If you are using WAO-LB as the default Service Proxy, just deploy your services as usual.

Do like this:

```diff
  apiVersion: v1
  kind: Service
  metadata:
    name: nginx-waolb
    labels:
      app: nginx
+     service.kubernetes.io/service-proxy-name: wao-loadbalancer
  spec:
    ports:
      - port: 80
        targetPort: 80
    selector:
      app: nginx
```

### Check Current Weights

Use `nft` command is the easiest way.

```sh
kubectl exec -n kube-system <kube-proxy-pod> -- nft list table
kubectl exec -n kube-system <kube-proxy-pod> -- nft list ruleset
```

// TODO: write more details

## Configuration

### Service Configuration

```diff
  apiVersion: v1
  kind: Service
  metadata:
    name: nginx-waolb
    labels:
      app: nginx
+     service.kubernetes.io/service-proxy-name: wao-loadbalancer
    annotations:
+     waok8s.github.io/cpu-per-request: "500m" # CPU per request
  ...
```

- `service.kubernetes.io/service-proxy-name` label
  - Set this value if WAO-LB is running as a non-default service proxy.
- (not yet implemented) `waok8s.github.io/cpu-per-request` annotation
  - Set this value to specify the CPU request per request.
  - The default value is `100m` (0.1 CPU).
  - AI inference or other heavy tasks should set a higher value.
  - The value is used to predict node power consumption.

### WAO-LB Configuration

Currently we don't have a configuration file for WAO-LB specific settings.
Here is a non-comprehensive list of the variables and their implementation status:

- Service Proxy Name
  - Environment variable `WAO_SERVICE_PROXY_NAME`
  - Set this value triggers the non-default service proxy mode
- nftables Table Name
  - Not implemented yet
  - Fixed to `wao-loadbalancer`
- WAO Metrics Cache TTL
  - Not implemented yet
  - Fixed to `30s`
- WAO Predictor Cache TTL
  - Not implemented yet
  - Fixed to `30m`
- Predictor Pallarelism
  - Not implemented yet
  - Fixed to `64`
- CPU request per access
  - (not yet implemented) Annotation `waok8s.github.io/cpu-per-request` in Service do this
  - The default value is fixed to `100m` (0.1 CPU) for now

## Development

> [!WARNING]
> Do not edit the files in `cmd` and `pkg`, as they are generated by the build scripts.
> Instead, modify the files in `_src` and follow the build steps.

- Each kube-proxy mode has its own implementation, and we modify them to use WAO.
- The modified files are located in `_src` directory.
  - If you want to change the implementation, modify the files in `_src` and follow the build steps.
- Build scripts are provided in `hack` directory.
  - `build-01-copy-k8s-code.sh` copies the related files to `cmd` and `pkg` directories. 
  - `build-02-apply-patches.sh` overwrites the original files with the modified files in `_src`.
- After the preparation, you can just run `make build` to build the kube-proxy.

### Components

- See `_src/README.md` for details.

## Changelog

Versioning: we use the same major.minor as Kubernetes, and the patch is our own.

- 2025-02-12 `v1.30.0-alpha.1`
  - Drop support for `ipvs` mode (now only `nftables` mode is supported).
  - Support non-default service proxy deployment.
  - Internal improvements.
  - Example K8s configs are available.
- 2025-02-05 `v1.30.0-alpha.0`
  - Support both `ipvs` and `nftables` mode.
  - Now the container image is available.
- 2024-07-01 `v1.29.0-alpha.0`
  - Support both `ipvs` and `nftables` mode.
  - You need to build the image by yourself.

## Acknowledgements

This work is supported by the New Energy and Industrial Technology Development Organization (NEDO) under its "Program to Develop and Promote the Commercialization of Energy Conservation Technologies to Realize a Decarbonized Society" ([JPNP21005](https://www.nedo.go.jp/english/activities/activities_ZZJP_100197.html)).

## License

> [!NOTE]
> The original Kubernetes code is [licensed under Apache-2.0](https://github.com/kubernetes/kubernetes/blob/master/LICENSE).

Copyright 2021 Osaka University.  
Copyright 2022 Bitmedia, Inc.  
Copyright 2024 Neutrix Cloud Japan Corporation.  
Copyright 2025 Bitmedia, Inc.  

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
