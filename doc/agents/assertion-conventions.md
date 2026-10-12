# Assertion conventions

Go tests use `testify`. The `testifylint` gate in `.golangci.yml` enforces the
assertion style, so a suppression is a sign the test should be restructured
rather than silenced. Apply these three rules when a test has several
expectations.

## Rules

1. **Independent checks become subtests that use `require`.** Checks that do
   not depend on each other each get a `t.Run` with a name that says what is
   being asserted. Use `require`, so a failing check stops only its own
   subtest. Cases that differ only in input become a table of named cases
   looped over with `t.Run`. Name each subtest after the behaviour it asserts,
   not after the test's internals.
2. **Steps that depend on each other stay one sequence.** When a later step
   needs an earlier result, keep them in the same flow. Put `require` on the
   gating error so the test stops before dependent assertions run against a
   zero value.
3. **Results from concurrent workers are collected first and asserted once.**
   Gather the outcomes into a slice or map, then assert on the aggregate. Do not
   call `assert` or `require` for each element inside a goroutine or loop.
   `require` must not run on a goroutine other than the test's own, and
   per-element assertions produce noisy output.

## Worked example

`internal/opds/client_test.go` shows the three rules:

- `TestDiscoveredCatalogTargetsAreValidatedBeforeFetching` is a table of
  discovered-target cases, each run as a named subtest that requires an error.
  The shared `externalHit` check is its own subtest after the loop, since it is
  an independent check across all cases.
- `TestHTTPSRedirectCannotDowngradeCatalogRequest` splits the error check and
  the request-count check into two subtests. Both are independent, and the
  request count is still read only after `require.Len` confirms its length.
- `TestRedirectsStayOnCatalogOriginAndPreserveCustomRedirectPolicy` keeps the
  same-origin redirect as a sequential `require` step, because later redirect
  checks rely on the client it configures.

`internal/opds/service_test.go` applies the same pattern to the browse and
rejection tests.

## Suppressions

Do not add `nolint:testifylint` for a test that can be restructured with the
rules above. If one case genuinely cannot be expressed as a subtest, keep a
single suppression on that line, and make its comment say why a subtest does
not fit.
