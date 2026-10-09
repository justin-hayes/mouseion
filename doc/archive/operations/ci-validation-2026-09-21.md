# Self-Hosted CI Validation: 2026-09-21

This record captures two consecutive manual CI dispatches on `ci01` after the
self-hosted runner cache changes and workflow action updates landed on `main`.

## Workflow Runs

- [Run 35583033015](https://github.com/justin-hayes/mouseion/actions/runs/35583033015)
  passed at commit `186dc485`.
- [Run 35583317175](https://github.com/justin-hayes/mouseion/actions/runs/35583317175)
  passed at commit `186dc485`.

Both runs completed the Build and test, Integration tests, Go lint, and Browser
smoke jobs successfully.

## Evidence

- Every job ran on `ci01`; the logs identify both the runner and machine as
  `ci01`.
- `actions/setup-go` selected Go `1.24.0` from the repository's `go.mod` and
  found the distribution in the runner tool cache on both runs.
- The integration command used `go test -count=1`. Package durations were
  `37.121s`, `21.806s`, and `14.837s` on the first run, followed by `36.032s`,
  `20.621s`, and `14.527s` on the second run. Neither run reported `(cached)`.
- sqlc setup requested `1.31.1`; `make sqlc` asserted `v1.31.1`, generated
  SQL successfully, and `git diff --exit-code` passed. The logs show no Go 1.26
  toolchain download.
- The two Build and test jobs each passed Go build/test, 50 Python tests,
  Python lint, protobuf generation, sqlc generation, and generated-code checks.
  Browser smoke passed 139 tests on each run.
- The logs contain no `Cannot open: File exists`, `Failed to restore`, or
  duplicate dependency-cache save warnings. Browser failure artifact upload
  remained configured and was correctly skipped because both browser jobs
  passed.
