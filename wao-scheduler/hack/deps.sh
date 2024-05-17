#!/usr/bin/env bash

WAO_CORE_VER="v1.30.0"
WAO_METRICS_ADAPTER_VER="v1.30.0"

cd config/base/deps && curl -LO "https://github.com/waok8s/wao-core/releases/download/$WAO_CORE_VER/wao-core.yaml" && cd -
cd config/base/deps && curl -LO "https://github.com/waok8s/wao-metrics-adapter/releases/download/$WAO_METRICS_ADAPTER_VER/wao-metrics-adapter.yaml" && cd -
