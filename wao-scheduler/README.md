# WAO Scheduler Version 2

A kube-scheduler with MinimizePower plugin and PodSpread plugin to schedule pods with WAO.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Getting Started](#getting-started)
  - [Installation](#installation)
  - [Deploy Pods with MinimizePower](#deploy-pods-with-minimizepower)
  - [Deploy Pods with PodSpread](#deploy-pods-with-podspread)
- [Configuration](#configuration)
  - [Client QPS and Burst](#client-qps-and-burst)
  - [MinimizePowerArgs](#minimizepowerargs)
- [Development](#development)
  - [Components](#components)
- [Changelog](#changelog)
- [Acknowledgements](#acknowledgements)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

WAO Scheduler schedules pods with features focused on minimizing power consumption. This is done by the following scheduler plugins:

- `MinimizePower`: Score nodes based on expected power consumption.
- `PodSpread`: Keep availability high by spreading pods across nodes, works together with MinimizePower.

## Getting Started

### Installation

> [!NOTE]
> Make sure you have [wao-core](https://github.com/waok8s/waok8s/wao-core) and [wao-metrics-adapter](https://github.com/waok8s/waok8s/wao-metrics-adapter) set up.

Install WAO Scheduler with default configuration.

```sh
kubectl apply -f https://github.com/waok8s/waok8s/releases/download/wao-scheduler/v1.30.3/wao-scheduler.yaml
```

Wait for the scheduler to be ready.

```sh
kubectl wait pod $(kubectl get pods -n kube-system -l app=wao-scheduler -o jsonpath="{.items[0].metadata.name}") -n kube-system --for condition=Ready
```

### Deploy Pods with MinimizePower

This plugin is enabled by default, so you only need to set `spec.schedulerName`.

> [!WARNING]
> Note that this plugin requires that at least one container in the pod has `requests.cpu` or `limits.cpu` set, otherwise the pod will be rejected. To be exact, if `requests.cpu` is set, it will be used as the expected CPU usage of the pod, otherwise `limits.cpu` will be used, otherwise the pod will be rejected.

```diff
  apiVersion: v1
  kind: Pod
  metadata:
    name: nginx
  spec:
+   schedulerName: wao-scheduler
    containers:
    - name: nginx
      image: nginx:1.14.2
      resources:
        requests:
          cpu: 100m
        limits:
          cpu: 200m
```

### Deploy Pods with PodSpread

This plugin only effects pods controlled by Deployment (and ReplicaSet), and it needs to be enabled by setting `wao.bitmedia.co.jp/podspread-rate` annotation.

```diff
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: nginx-deployment
    labels:
      app: nginx
+   annotations:
+     wao.bitmedia.co.jp/podspread-rate: "0.6"
  spec:
    replicas: 10
    selector:
      matchLabels:
        app: nginx
    template:
      metadata:
        labels:
          app: nginx
      spec:
+       schedulerName: wao-scheduler
        containers:
        - name: nginx
          image: nginx:1.14.2
```

Then 60 percent of the pods will be spread across nodes, and the rest 40 percent will be scheduled normally (i.e. MinimizePower plugin will be used).

## Configuration

Just like `kube-scheduler`, WAO Scheduler reads configuration from a file specified by `--config` flag.

Here is an example `KubeSchedulerConfiguration` with MinimizePower and PodSpread plugins enabled.

> [!NOTE]
> It is recommended to use a higher weight for MinimizePower plugin,
> as the normalized score might have small difference between nodes, particularly when the cluster has many nodes.

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
leaderElection:
  leaderElect: false
profiles:
  - schedulerName: wao-scheduler
    plugins:
      multiPoint:
        enabled:
        - name: MinimizePower
          weight: 20
        - name: PodSpread
```

### Client QPS and Burst

Our scheduler uses metrics client to get server inlet temperature and delta pressure of each node.
It is recommended to set a higher QPS and burst to avoid client throttling.
For a normal cluster (up to several hundreds of nodes), the following configuration should work well.

```diff
  apiVersion: kubescheduler.config.k8s.io/v1
  kind: KubeSchedulerConfiguration
+ clientConnection:
+   qps: 150
+   burst: 300
```

### MinimizePowerArgs

The following args can be set in `pluginConfig` to configure MinimizePower plugin.

```yaml
    pluginConfig:
      - name: MinimizePower
        args:
          metricsCacheTTL: 15s
          predictorCacheTTL: 15m
          podUsageAssumption: 0.8
          cpuUsageFormat: Percent
```

Preset values can be found in `config/base/cm.yaml`, and default values can be found in `pkg/plugins/minimizepower/types.go`. 

- `metricsCacheTTL`: The TTL of metrics cache. Too short TTL will cause frequent requests to metrics server.
- `predictorCacheTTL`: The TTL of predictor cache. Predictor always returns the same result for the same input, so it is safe to set a long TTL.
- `podUsageAssumption`: The rate of expected CPU usage for a pod that is binded to a node but not yet started. This is used to count the expected CPU usage when scheduling a set of pods (e.g. a Deployment). The scheduler will assume that a pending pod (that is binded to a node) will use `requests.cpu * podUsageAssumption` CPUs. 
- `cpuUsageFormat`: The format of CPU usage send to predictor.
  - `Raw`: [0.0, NumLogicalCores]
  - `Percent`: [0.0, 100.0]

## Development

This project is following [Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/).

### Components

- `pkg/plugins/minimizepower`: MinimizePower plugin.
- `pkg/plugins/podspread`: PodSpread plugin.

## Changelog

Versioning: we use the same major.minor as Kubernetes, and the patch is our own.

- What comes next?
  - TBD
- 2025-xx-xx `v1.31.0`
  - Support Kubernetes v1.31.
- 2025-xx-xx `v1.30.3`
  - Change domain to `waok8s.github.io`.
- Older versions (<=v1.30.2) can be found at [`waok8s/wao-scheduler`](https://github.com/waok8s/wao-scheduler).

## Acknowledgements

See [here](https://github.com/waok8s/waok8s?tab=readme-ov-file#acknowledgements) for details.

## License

Apache-2.0. See [here](https://github.com/waok8s/waok8s?tab=readme-ov-file#license) for details.
