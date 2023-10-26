# Integration tests

## Test flow

Test cases should be placed in separate directories `test/case*`, e.g. `test/case-1-hoge`, `test/case-2-fuga`. Each test case will be executed following the flow below:

- Step 1/4: load kube-scheduler
  - Apply all manifests in `case*/config`.
- Step 2/4: apply manifests
  - Apply all manifests in `case*/apply`.
- Step 3/4: do tests
  - Run `case*/test/*.in` and check if stdout==`case*/test/*.out` with retry.
- Step 4/4: cleanup
  - Delete all manifests in `case*/apply`.
