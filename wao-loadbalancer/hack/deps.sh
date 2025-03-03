#!/usr/bin/env bash

WAO_CORE_VER="v1.30.1"
WAO_METRICS_ADAPTER_VER="v1.30.1"
WAO_SCHEDULER_VER="v1.30.1"

cd config/base/deps && curl -LO "https://github.com/waok8s/waok8s/releases/download/wao-core/$WAO_CORE_VER/wao-core.yaml" && cd -
cd config/base/deps && curl -LO "https://github.com/waok8s/waok8s/releases/download/wao-metrics-adapter/$WAO_METRICS_ADAPTER_VER/wao-metrics-adapter.yaml" && cd -
cd config/base/deps && curl -LO "https://github.com/waok8s/waok8s/releases/download/wao-scheduler/$WAO_SCHEDULER_VER/wao-scheduler.yaml" && cd -
