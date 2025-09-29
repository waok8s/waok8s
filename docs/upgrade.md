# Upgrade Guide

## Upgrade Go Version

1. Edit `go.mod` file to update the library versions.
2. Edit `Dockerfile` to update the Go version.

## Upgrade Kubernetes Version

1. Edit `go.mod` file to update the library versions.
2. Edit `hack/0-env.sh` `hack/2-lib.sh` to update the versions in the scripts.
  - `wao-metrics-adapter` `wao-scheduler` `wao-loadbalancer`
3. Edit envtest version in Makefile.
  - `wao-core` `wao-metrics-adapter`
4. Upgrade controller-runtime and envtest version if necessary.
  - Version matrix here: https://github.com/kubernetes-sigs/controller-runtime
  - controller-runtime: go.mod
  - envtest: Makefile
5. Upgrade controller-tools version if necessary.
  - Version matrix here: https://github.com/kubernetes-sigs/controller-tools
  - controller-tools: Makefile

## Release New Version

1. (Production release only) Create `release-1.xx` branch from `main` branch.
2. Tag the commit with <module_name>/<version> format (e.g., `wao-core/v1.31.0-alpha.0`).
3. Push the tag to the remote repository.
4. Wait for the CI/CD pipeline to build and push the image and manifest.

## Upgrade Component Versions

1. Edit `go.mod` file to update the library versions.
2. Edit `hack/deps.sh` and run it to update the component versions.
  - `wao-metrics-adapter` `wao-scheduler` `wao-loadbalancer`
3. Edit related yaml files in `test` directory.
  - `wao-scheduler`

## Upgrade Kubebuilder Version

Do it manually.
