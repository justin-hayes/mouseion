# ADR 0119: Retire the German vocabulary backfill

Status: **Accepted** · Date: 2026-11-21 · Author: Justin + OpenCode

Amends [ADR 0065](0065-german-pre-1996-sharp-s-canonicalization.md) by retiring
its operator-run vocabulary backfill, and
[ADR 0091](0091-go-package-layering.md)'s example of that command as a pgx
owner. Implemented by
[issue #2531](https://github.com/justin-hayes/mouseion/issues/2531).

## Context

ADR 0065 introduced a one-time operator command that folded mutable learner
vocabulary onto the post-1996 German canonicalization profile. It covered Known
vocabulary, vocabulary states, Generated vocabulary, selection candidates,
example sentences, curated sentences and deck-preparation vocabulary, and it
carried a one-time Generated-vocabulary step.

On 2026-11-21 a copy of the production schema and of every table the backfill
touches was loaded into a throwaway PostgreSQL 18 database. That included
258,247 selection candidates, 49,935 vocabulary states, 18,870 deck-preparation
vocabulary rows, 11,474 Generated vocabulary rows and 4,513 Known vocabulary
rows. The backfill, built from `main`, reported
`owners=1 examined=0 updated=0 merged=0 conflicts=0`, and checksums of every
touched table were identical before and after. Production itself was only read.

The command therefore changes nothing in the only deployment, while still
requiring maintenance whenever the learner-vocabulary tables change. A future
change to the German profile would need a backfill written against the tables
as they are then, so retaining this one provides no reuse.

## Decision

1. **Delete the operator command.** `cmd/vocabularybackfill` is removed, and
   the maintenance command is removed from the operations guide.
2. **Delete the persistence backfill and its helpers.** This includes the
   embedded Generated-vocabulary step, the curated-sentence preflight, the
   deterministic merge rules and the report types. No non-test code applies a
   German canonicalization profile to stored learner vocabulary after the fact.
3. **Remove the tests that only exercised the backfill.** Canonicalization,
   selection and vocabulary behavior are unchanged, and their tests remain.
4. **Keep the record.** The backfill stays in Git history. ADR 0065's mapping
   decision remains accepted; only its re-normalization rollout is historical.

## Consequences

- Learners and operators see no change. No migration or data repair runs.
- `cmd/cataloguebackfill` is the remaining operator backfill command and the
  only backfill example in ADR 0091's pgx-owner explanation.
- A future German profile change must design its own backfill against the
  then-current tables; this decision does not authorize reusing the retired
  one.

## Alternatives considered

- **Keep the dormant command.** Rejected: it changes nothing today, still has
  to be maintained against the vocabulary schema, and invites an operator to
  run a stale merge policy after a future profile change.
- **Turn the backfill into a startup side effect.** Rejected: startup already
  avoids rewriting learner vocabulary, and the evidence shows no data needs
  rewriting.
- **Rewrite history to remove the backfill commit.** Rejected: the command and
  its evidence are a useful record of the profile rollout and remain
  reproducible from Git history.

## References

- [ADR 0065: Canonicalize pre-1996 German ß spellings](0065-german-pre-1996-sharp-s-canonicalization.md)
- [ADR 0091: Go package layering](0091-go-package-layering.md)
- [Issue #849: Pre-1996 German ß spellings canonicalize to post-1996 lemmas](https://github.com/justin-hayes/mouseion/issues/849)
- [Issue #2531: Retire the German vocabulary backfill](https://github.com/justin-hayes/mouseion/issues/2531)
