# WAO Core Components

CRDs, controllers and libraries for WAO.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Getting Started](#getting-started)
  - [Installation](#installation)
  - [Prerequisites for NodeConfig\[Template\]](#prerequisites-for-nodeconfigtemplate)
- [Configuration](#configuration)
  - [NodeConfig CRD](#nodeconfig-crd)
    - [Metrics Collector: Inlet Temperature](#metrics-collector-inlet-temperature)
    - [Metrics Collector: Differential Pressure](#metrics-collector-differential-pressure)
    - [Predictor: Power Consumption](#predictor-power-consumption)
    - [Predictor: Power Consumption Endpoint Provider](#predictor-power-consumption-endpoint-provider)
  - [NodeConfigTemplate CRD](#nodeconfigtemplate-crd)
  - [Template Syntax](#template-syntax)
- [Development](#development)
  - [Components](#components)
- [Changelog](#changelog)
- [Acknowledgements](#acknowledgements)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

This repository contains CRDs, controllers and libraries for WAO. They are intended to be used with [wao-metrics-adapter](https://github.com/waok8s/wao-metrics-adapter), [wao-scheduler](https://github.com/waok8s/wao-scheduler), etc.

## Getting Started

### Installation

Install CRDs and controllers.

```sh
kubectl apply -f https://github.com/waok8s/wao-core/releases/download/v1.27.0/wao-core.yaml
```

Wait for the pod to be ready.

```sh
kubectl wait pod $(kubectl get pods -n wao-system -l control-plane=controller-manager -o jsonpath="{.items[0].metadata.name}") -n wao-system --for condition=Ready
```

### Prerequisites for NodeConfig[Template]

Before using NodeConfig[Template], you need to do the following.

- Make sure that your nodes have Redfish API enabled and inlet temperature sensors are available.
- Make sure that you have a differential pressure API server running.
- Make sure that you have a power consumption predictor running.
- (Optional) Make sure that your nodes have `/redfish/v1/Systems/{systemId}/MachineLearningModel` Redfish property. This property is currently not supported in most Redfish implementations, but you can use [MLMM]() to provide it.

We provide tools to test these requirements are met. Try the following.

```sh
# CLI to get inlet_temp from Redfish server.
go run github.com/waok8s/wao-core/pkg/metrics/redfish/cmd/redfish_inlettemp_cli@HEAD -h
# CLI to get delta_p from DifferentialPressureAPI server.
go run github.com/waok8s/wao-core/pkg/metrics/dpapi/cmd/dpapi_deltap_cli@HEAD -h
# CLI to get power consumption from V2InferenceProtocol server.
go run github.com/waok8s/wao-core/pkg/predictor/v2inferenceprotocol/cmd/v2ip_pcp_cli@HEAD -h
# CLI to get power consumption predictor endpoint from Redfish server.
go run github.com/waok8s/wao-core/pkg/predictor/redfish/cmd/redfish_ep_cli@HEAD -h
```

> [!NOTE]
> We also provide fake implementations for testing purposes. You can use them by setting `type` to `Fake` in NodeConfig[Template]. 

Then, you can configure your nodes with NodeConfig[Template]. See [Configuration](#configuration) for details.

## Configuration

### NodeConfig CRD

NodeConfig CRD is used to configure a node for other WAO components. It provides the following information:

- Metrics collector: how to collect metrics from the node.
- Predictor: how to predict power consumption of the node.

Here is an example.

> [!IMPORTANT]
> Currently, only "wao-system" namespace is supported for NodeConfig, NodeConfigTemplate and related Secrets. (This is due to RBAC.)

```yaml
apiVersion: wao.bitmedia.co.jp/v1beta1
kind: NodeConfig
metadata:
  name: worker-0
  namespace: wao-system
spec:
  nodeName: worker-0
  metricsCollector:
    inletTemp:
      type: Redfish
      endpoint: "https://10.0.0.100"
      basicAuthSecret:
        name: "worker-0-redfish-basicauth"
      fetchInterval: 10s
    deltaP:
      type: DifferentialPressureAPI
      endpoint: "http://10.0.0.1:5000"
      fetchInterval: 10s
  predictor:
    powerConsumption:
      type: V2InferenceProtocol
      endpoint: "http://10.0.0.1:8080/v2/models/myModel/versions/v0.1.0/infer"
```

The above example uses Redfish and DifferentialPressureAPI to collect inlet temperature and differential pressure, and uses V2InferenceProtocol to predict power consumption.

#### Metrics Collector: Inlet Temperature

This part of the spec is used to configure how to collect inlet temperature.

- `type`: `Redfish` or `Fake`.
- `endpoint`: Endpoint URL. Ignored when `type` is `Fake`.
- `basicAuthSecret` (Optional): Secret containing username and password for basic authentication. Ignored when the `type` does not require authentication.
- `fetchInterval` (Optional): Interval to fetch metrics. Default is `15s`.

```yaml
    inletTemp:
      type: Redfish
      endpoint: "https://10.0.0.100"
      basicAuthSecret:
        name: "worker-0-redfish-basicauth"
      fetchInterval: 10s
```

#### Metrics Collector: Differential Pressure

This part of the spec is used to configure how to collect differential pressure.

- `type`: `DifferentialPressureAPI` or `Fake`.
- `endpoint`: Endpoint URL. Ignored when `type` is `Fake`.
- `basicAuthSecret` (Optional): Secret containing username and password for basic authentication. Ignored when the `type` does not require authentication.
- `fetchInterval` (Optional): Interval to fetch metrics. Default is `15s`.

```yaml
    deltaP:
      type: DifferentialPressureAPI
      endpoint: "http://10.0.0.1:5000"
      fetchInterval: 10s
```

#### Predictor: Power Consumption

This part of the spec is used to configure how to predict power consumption.

- `type`: `V2InferenceProtocol` or `Fake`.
- `endpoint`: Endpoint URL. Ignored when `type` is `Fake`.
- `basicAuthSecret` (Optional): Secret containing username and password for basic authentication. Ignored when the `type` does not require authentication.
- `fetchInterval` (Unused): Ignored.

```yaml
    powerConsumption:
      type: V2InferenceProtocol
      endpoint: "http://10.0.0.1:8080/v2/models/myModel/versions/v0.1.0/infer"
```

#### Predictor: Power Consumption Endpoint Provider

This part of the spec is used to configure how to get endpoint for power consumption predictor. This is useful when the endpoint is described in Redfish or other APIs.

- `type`: `Redfish` or `Fake`.
- `endpoint`: Endpoint URL. Ignored when `type` is `Fake`.
- `basicAuthSecret` (Optional): Secret containing username and password for basic authentication. Ignored when the `type` does not require authentication.
- `fetchInterval` (Unused): Ignored.

```yaml
    powerConsumptionEndpointProvider:
      type: Redfish
      endpoint: "https://10.0.0.1"
      basicAuthSecret:
        name: "worker-0-redfish-basicauth"
```

If your predictor requires authentication, you can set your Secret in `powerConsumption.basicAuthSecret` while leaving other fields empty.

```yaml
    powerConsumption:
      type: ""      # Endpoint provider will set this.
      endpoint: ""  # Endpoint provider will set this.
      basicAuthSecret:
        name: "predictor-basicauth"
    powerConsumptionEndpointProvider:
      type: Redfish
      endpoint: "https://10.0.0.1"
      basicAuthSecret:
        name: "worker-0-redfish-basicauth"
```

### NodeConfigTemplate CRD

NodeConfigTemplate CRD is used to configure a group of nodes by selecting nodes with labels. The controller will create NodeConfig for each node.
Several fields can be templated and this is useful for configuring IP addresses, etc.

Here is an example.

```yaml
apiVersion: wao.bitmedia.co.jp/v1beta1
kind: NodeConfigTemplate
metadata:
  name: redfish-enabled-nodes
  namespace: wao-system
spec:
  nodeSelector:
    matchLabels:
      node.kubernetes.io/instance-type: "redfish-enabled"
  template:
    nodeName: "" # This will be set by the controller.
    metricsCollector:
      inletTemp:
        type: Redfish
        endpoint: "https://10.0.100.{{.IPv4.Octet4}}"
        basicAuthSecret:
          name: "redfish-basicauth-{{.Hostname}}"
        fetchInterval: 10s
      deltaP:
        type: DifferentialPressureAPI
        endpoint: "http://10.0.0.1:5000"
        fetchInterval: 10s
    predictor:
      powerConsumptionEndpointProvider:
        type: Redfish
        endpoint: "https://10.0.100.{{.IPv4.Octet4}}"
        basicAuthSecret:
          name: "redfish-basicauth-{{.Hostname}}"
```

After applying the above NodeConfigTemplate, you can see NodeConfig for each node.

```
$ kubectl get nodeconfig -n wao-system

NAME                              AGE
redfish-enabled-nodes-worker-0    10s
redfish-enabled-nodes-worker-1    10s
redfish-enabled-nodes-worker-2    10s
```

### Template Syntax

You can use [`text/template`](https://pkg.go.dev/text/template) style syntax in `type` `endpoint` and `basicAuthSecret.name` fields, and the following variables are available.

- `{{.Hostname}}`: `kubernetes.io/hostname` label value.
- `{{.IPv4.Address}}`: Address value of the first `InternalIP` in Node's `status.addresses`.
- `{{.IPv4.Octet1}}` `{{.IPv4.Octet2}}` `{{.IPv4.Octet3}}` `{{.IPv4.Octet4}}`: Octet value of the above address.

You can also use [`sprig`](http://masterminds.github.io/sprig/) functions to do some magic. Examples here.

- `{{ trimPrefix .Hostname "worker-" }}`: Remove `worker-` prefix from the hostname.
- `{{ add .IPv4.Octet3 10 }}`: Add `10` to the 3rd octet.

## Development

This project uses [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) (v3.11) to generate the CRDs and controllers. However, codes under `pkg` (contain libraries) do not follow Kubebuilder conventions.

### Components

- `api/wao`: CRDs.
- `internal/controller`: Controllers.
- `pkg/controller`: Controllers not run in the controller manager.
- `pkg/metrics`: Custom metrics library.
- `pkg/predictor`: Predictor library.
- `pkg/client`: Cached clients for metrics and predictors.

## Changelog

Versioning: we use the same major.minor as Kubernetes, and the patch is our own.

- 2024-03-29 `v1.28.0`
  - Support Kubernetes v1.28.
    - Use `controller-runtime` v0.16 which supports v1.28.
- 2024-03-04 `v1.27.0`
  - First release.
  - CRDs, controllers and libraries.

## Acknowledgements

This work is supported by the New Energy and Industrial Technology Development Organization (NEDO) under its "Program to Develop and Promote the Commercialization of Energy Conservation Technologies to Realize a Decarbonized Society" ([JPNP21005](https://www.nedo.go.jp/english/activities/activities_ZZJP_100197.html)).

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
