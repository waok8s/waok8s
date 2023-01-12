# WAO-Scheduler v2

## Developing

This scheduler follows [Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/).

### Prerequisites

Make sure you have the following tools installed:

- Git
- Make
- Go
- Docker


### Run a development cluster with [kind](https://kind.sigs.k8s.io/)

```sh
./hack/dev-kind-reset-cluster.sh # create a K8s cluster `kind-wao-scheduler-v2`
./hack/dev-kind-deploy.sh # build and deploy the scheduler
```
