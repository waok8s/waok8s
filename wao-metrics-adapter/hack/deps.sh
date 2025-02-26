#!/usr/bin/env bash

WAO_CORE_VER="v1.30.1"

cd config/base/deps && curl -LO "https://github.com/waok8s/wao-core/releases/download/$WAO_CORE_VER/wao-core.yaml" && cd -
