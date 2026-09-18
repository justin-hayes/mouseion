# Cleanup conventions

Cleanup is part of the operation's error contract. Handle it according to what
can still be recovered, not according to whether the call is deferred.

## Production code

- Preserve output-finalization errors. A writer, encoder, archive, or database
  close that can invalidate the returned artifact must be checked and joined
  with an earlier operation error when both occur.
- Propagate actionable shutdown errors. Service, client, file, and store
  shutdown failures must be returned when the caller can respond, or logged at
  the process boundary when the process cannot return an error.
- Treat rollback after a successful commit as expected. A deferred rollback
  must explicitly ignore only the driver's closed-transaction result and must
  retain any other rollback error. `internal/persistence.withTx` is the shared
  example.
- Keep best-effort cleanup explicit when no recovery action exists. Use a
  narrow source suppression with a reason only after checking that the cleanup
  cannot affect an artifact, durable state, or resource isolation. Do not add
  package-wide or generic function exclusions.

## Tests

Register cleanup with `t.Cleanup` so it runs after a test's assertions and after
an early `require` failure. For error-returning cleanup, use
`testutil.Cleanup(t, name, cleanup)`. It reports a cleanup error with
`Errorf`, preserving any earlier test failure instead of replacing it with
`FailNow`.

Cleanup that has no meaningful recovery action may remain best effort, but the
reason must be clear at the call site. Cleanup that affects database isolation,
durable jobs, generated artifacts, or process lifecycle is consequential and
must use the checked pattern.

## Lint policy

`errcheck` is an enforced gate in `.golangci.yml`, run by `make lint-go` and
therefore by `make lint`. It checks production and test code, including
integration-tagged source. The configuration also checks blank assignments and
type assertions, so every discarded result or unchecked assertion must be
audited rather than used to avoid deciding whether an error matters.

The gate does not replace the cleanup policy above: preserve output-finalization
errors, propagate actionable shutdown errors, retain unexpected rollback errors
after commit, and keep only narrowly justified best-effort cleanup suppressed.
