# Upgrade Guide

## Upgrade Go Version

1. Edit `go.mod` file to update the library versions.
2. Edit `Dockerfile` to update the Go version.

## Upgrade Kubernetes Version

1. Edit `go.mod` file to update the library versions.
2. Edit `hack/0-env.sh` `hack/2-lib.sh` to update the versions in the scripts.
  - `wao-metrics-adapter` `wao-scheduler` `wao-loadbalancer`

## Release New Version

1. Tag the commit with <module_name>/<version> format (e.g., `wao-core/v1.31.0-alpha.0`).
2. Push the tag to the remote repository.
3. Wait for the CI/CD pipeline to build and push the image and manifest.

## Upgrade Component Versions

1. Edit `go.mod` file to update the library versions.
  - `wao-metrics-adapter` `wao-scheduler` `wao-loadbalancer`
2. Edit `hack/deps.sh` and run it to update the component versions.
  - `wao-metrics-adapter` `wao-scheduler` `wao-loadbalancer`
3. Edit related yaml files in `test` directory.
  - `wao-scheduler`

## Upgrade Kubebuilder Version

Do it manually.
