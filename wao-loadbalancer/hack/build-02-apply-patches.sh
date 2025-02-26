#!/usr/bin/env bash

set -e -x

# main

# apply patches

cp -r _src/cmd/* cmd/
cp -r _src/pkg/* pkg/
