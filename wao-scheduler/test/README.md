# Integration tests

## Test flow

Test cases should be placed in separate directories `test/case*`, e.g. `test/case-1-hoge`, `test/case-2-fuga`. Each test case will be executed following the flow below:

- step 1/4: load kube-scheduler
  - use the kube-scheduler static Pod manifest in `case*/config/kube-scheduler.yaml`
  - use the KubeSchedulerConfiguration manifest in `case*/config/kube-scheduler.yaml`
- step 2/4: apply manifests
  - apply all manifests in `case*/apply/`
- step 3/4: do tests
  - run `case*/test/*.in` and check if stdout==`case*/test/*.out` (with retry)
- step 4/4: cleanup
  - delete all manifests in `case*/apply/`

## Test cases

- case1
  - normal case
- case2
  - the Deployment has invalid "podspread/rate" value "2"
