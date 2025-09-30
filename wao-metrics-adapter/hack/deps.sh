#!/usr/bin/env bash

WAO_CORE_VER="v1.31.0-beta.0"

cd config/base/deps && curl -LO "https://github.com/waok8s/waok8s/releases/download/wao-core/$WAO_CORE_VER/wao-core.yaml" && cd -
