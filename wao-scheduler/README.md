# WAO Scheduler Version 2

A kube-scheduler with MinimizePower plugin and PodSpread plugin to schedule pods with WAO.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Getting Started](#getting-started)
  - [Installation](#installation)
  - [Deploy Pods with MinimizePower](#deploy-pods-with-minimizepower)
  - [Deploy Pods with PodSpread](#deploy-pods-with-podspread)
- [Development](#development)
  - [Components](#components)
- [Changelog](#changelog)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

WAO Scheduler schedules pods with features focused on minimizing power consumption. This is done by the following scheduler plugins:

- `MinimizePower`: Score nodes based on expected power consumption.
- `PodSpread`: Keep availability high by spreading pods across nodes, works together with MinimizePower.

## Getting Started

### Installation

> [!NOTE]
> Make sure you have [wao-core](https://github.com/waok8s/wao-core) and [wao-metrics-adapter](https://github.com/waok8s/wao-metrics-adapter) set up.

Install WAO Scheduler with default configuration.

```sh
kubectl apply -f https://github.com/waok8s/wao-scheduler/releases/download/v1.27.0/wao-scheduler.yaml
```

Wait for the scheduler to be ready.

```sh
kubectl wait pod $(kubectl get pods -n kube-system -l app=wao-scheduler -o jsonpath="{.items[0].metadata.name}") -n kube-system --for condition=Ready
```

### Deploy Pods with MinimizePower

This plugin is enabled by default, so you only need to set `spec.schedulerName`.

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

Then 60 percent of the pods will be spread across nodes, and the rest 40 percent will be scheduled based on MinimizePower.

## Development

This project is following [Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/).

### Components

- `pkg/plugins/minimizepower`: MinimizePower plugin.
- `pkg/plugins/podspread`: PodSpread plugin.

## Changelog

Versioning: we use the same major.minor as Kubernetes, and the patch is our own.

- What comes next:
  - TBD
- 2023-xx-xx `v1.27.0`
  - First release.
  - `minimizepower` Add the scheduler plugin.
  - `podspread` Add the scheduler plugin.

## License

Copyright 2023 Bitmedia, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
