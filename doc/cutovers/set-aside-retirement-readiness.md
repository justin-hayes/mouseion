# Set Aside retirement: release-readiness evidence (issue #1593)

Slice 13 of #1580. **Status: implementation ready for review; corpus-scale
evidence is NOT recorded and remains a release-readiness blocker.**

## No data conversion

No production Book was ever Set Aside (confirmed by Justin), so the elaborate
stopped-writer conversion in ADR 0086 section 5 is unnecessary. Migration
`000033_retire_set_aside_disposition` replaces the disposition check with
`inbox`/`to_read` only. It converts nothing and fails loudly, rather than
repairing silently, if a legacy row ever exists. A missing visibility row already
means a visible Book, so none is initialized. Its down migration restores the
three-value check.

Before applying to production, confirm with
`SELECT count(*) FROM book_dispositions WHERE disposition = 'set_aside'` (expected 0).

## Verified behavior

- `POST /library/books/{id}/set-aside` and the old `/reading/.../set-aside` paths
  are unregistered (404, never mapped to Hide or End); `?disposition=set_aside` is
  an unknown filter; no Set Aside control renders; `BookDisposition("set_aside")`
  fails validation; the constraint rejects it
  (`book_disposition_constraint_integration_test.go`).
- Finish still leaves Inbox; End still leaves To Read; retired Journey, selection,
  and Custom-deck boundaries and artifacts are untouched.
- Earlier slices own Hidden, switch/return, failure-safe, KWIC, and round-trip tests.

## Scale and performance evidence: NOT recorded (blocker)

Targets (`doc/features/vocabulary-browse-and-concordance.md`, ADR 0084): on a
documented warm corpus of about 500 analyzed Books / 50 million tokens, p95
completed Browse and ordinary Concordance requests at most 2 seconds; pending
feedback near 1 second; honest failed-interactive recovery by 10 seconds; no
sampled evidence; the threshold ten-versus-seven validation obligation is kept.
No harness or reference corpus exists in the repository and nothing was measured;
hardware, source mix, workload, measurement boundary, and results are all
_not measured_.
