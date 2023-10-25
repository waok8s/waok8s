#!/usr/bin/env bash

WAO_CORE_VER="v0.27.0-alpha.3"

cd config/base/deps && curl -LO "https://github.com/waok8s/wao-core/releases/download/$WAO_CORE_VER/wao-core.yaml" && cd -
