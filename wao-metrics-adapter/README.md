# WAO Metrics Adapter

A metrics adapter for Kubernetes Metrics APIs that exposes custom metrics for WAO.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Getting Started](#getting-started)
  - [Installation](#installation)
  - [Fetching Metrics](#fetching-metrics)
- [Development](#development)
  - [Components](#components)
- [Changelog](#changelog)
- [Acknowledgements](#acknowledgements)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

WAO Metrics Adapter exposes the following custom metrics for WAO:

- `inlet_temp`: Server inlet temperature in Celsius.
- `delta_p`: Server differential pressure in Pascal.

## Getting Started

### Installation

> [!NOTE]
> Make sure you have [wao-core](https://github.com/waok8s/waok8s/wao-core) set up.

Install WAO Metrics Adapter.

```sh
kubectl apply -f https://github.com/waok8s/waok8s/releases/download/wao-metrics-adapter/v1.30.3/wao-metrics-adapter.yaml
```

Wait for the pod to be ready.

```sh
kubectl wait pod $(kubectl get pods -n custom-metrics -l app=wao-metrics-adapter -o jsonpath="{.items[0].metadata.name}") -n custom-metrics --for condition=Ready
```

### Fetching Metrics

You can fetch the metrics using `kubectl get --raw` command.

```sh
# Your node name
NODE=worker-1

# Inlet temperature
kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta2/nodes/$NODE/inlet_temp"
# Differential pressure
kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta2/nodes/$NODE/delta_p"
```

Or you can use client libraries to fetch the metrics.

- `k8s.io/metrics/pkg/client/custom_metrics` has the official client
- `github.com/waok8s/waok8s/wao-metrics-adapter/pkg/client` has our cached client


## Development

This project is using [custom-metrics-apiserver](https://github.com/kubernetes-sigs/custom-metrics-apiserver), which is a library based on [Kubernetes API Aggregation Layer](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/).

### Components

- `pkg/provider`: Custom metrics provider.
- `pkg/controller`: Controllers.
- `pkg/metrics`: Custom metrics library.
- `pkg/predictor`: Predictor library.
- `pkg/client`: Cached clients for metrics and predictors.

## Changelog

Versioning: we use the same major.minor as Kubernetes, and the patch is our own.

- What comes next?
  - TBD
- 2025-xx-xx `v1.31.0`
  - Support Kubernetes v1.31.
- 2025-xx-xx `v1.30.3`
  - Change domain to `waok8s.github.io`.
- Older versions (<=v1.30.2) can be found at [`waok8s/wao-metrics-adapter`](https://github.com/waok8s/wao-metrics-adapter).

## Acknowledgements

See [here](https://github.com/waok8s/waok8s?tab=readme-ov-file#acknowledgements) for details.

## License

Apache-2.0. See [here](https://github.com/waok8s/waok8s?tab=readme-ov-file#license) for details.
