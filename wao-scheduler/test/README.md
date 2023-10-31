# Integration tests

## Test flow

Test cases should be placed in separate directories `test/case*`, e.g. `test/case-1-hoge`, `test/case-2-fuga`. Each test case will be executed following the flow below:

- Step 1/5: load kube-scheduler
  - Apply all manifests in `case*/config`.
- Step 2/5: apply prerequisites
  - Apply all manifests in `case*/preapply` if exists.
- Step 3/5: apply manifests
  - Apply all manifests in `case*/apply`.
- Step 4/5: do tests
  - Run `case*/test/*.in` and check if stdout==`case*/test/*.out` with retry.
- Step 5/5: cleanup
  - Delete all manifests in `case*/apply` and `case*/preapply` (if exists).
