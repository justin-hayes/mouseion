# ADR 0065: Canonicalize pre-1996 German ß spellings

Status: **Accepted** · Date: 2026-09-14 · Author: Justin + opencode

Amends **ADR 0005** (vocabulary identity and normalization). Builds on
**ADR 0064** and the shared normalization policy introduced for the dictionary
index.

## Context

The German standard-orthography profile currently preserves `ß` by design, but
its historical-equivalence table is necessarily explicit. A blanket `ß` →
`ss` replacement would make modern words such as *Straße* and *Maße* collide
with different lexemes such as *Strasse* and *Masse*. It would also make the
runtime canonicalization and the dictionary-index key derivation disagree about
which spellings are equivalent.

The post-1996 spelling is the canonical learner-facing form. Representative
pre-reform spellings include *Haß*, *Eßzimmer*, and *Fluß*. The project needs a
policy for the finite mapping, the unmapped fallback, and persisted identities
created by an older profile.

## Decision

### Mapping source and data

The source authority is the **Amtliches Regelwerk der deutschen
Rechtschreibung** and its Wörterverzeichnis, published by the Rat für deutsche
Rechtschreibung. The current online edition and the historical 2004 edition
are the reference for the reform boundary and exceptional word spellings. The
2004 edition is the first edition published by the Rat and is substantively
aligned with the 1996 reform, but it is not treated as an identical edition.

The repository's `internal/canonicalization/german_post1996.json` is the
versioned, reviewable projection of that authority. It is the normative data
consumed by both the embedded Go profile and the Python dictionary-index
derivation; neither consumer invents additional mappings. The v6 table must
retain existing entries and add the documented forms such as:

| Pre-1996 form | Canonical form |
| --- | --- |
| `haß` | `hass` |
| `eßzimmer` | `esszimmer` |
| `daß` | `dass` |
| `fluß` | `fluss` |
| `schloß` | `schloss` |

The table may contain other individually documented historical equivalences,
but it is not a general transliteration rule. Adding a mapping requires a
fixture, a source citation in the change, and a profile-version review.

### Canonicalization behavior

The profile is named `german-standard-post-1996`. Version 6 adds the complete
pre-1996 mapping policy; versions 4 and 5 remain available for reading
historical artifacts. For a new German lemma, the profile:

1. selects the first usable analyzer lemma alternative;
2. removes only configured edge punctuation/symbol decoration;
3. lowercases without Unicode case folding, preserving `ß`; and
4. applies the exact-equivalence table to the resulting string.

The producer-level separable-verb reattachment from ADR 0061 remains in force
before these steps: a separated verb is first reconstructed as its full lexeme,
then the full lexeme is normalized here. This ADR does not alter that rule or
the exclusion of separable particles from vocabulary candidates.

The fallback for a word absent from the table is the result after steps 1–3.
There is no dictionary lookup, heuristic spelling conversion, or blanket
`ß` → `ss` fallback. Thus `Straße` becomes `straße`, `Fuß` becomes `fuß`, and
`Maße` becomes `maße`; `Masse` becomes `masse` and remains distinct from
`Maße`. An explicitly listed historical form and its canonical form are the
same vocabulary identity by design, so `Fluß` and `Fluss` both identify
`fluss`.

The dictionary index derives keys with this same profile and versioned policy.
A lemma missing from the dictionary remains a valid canonical lemma; it simply
uses the existing unindexed fallback behavior for enrichment.

### Re-normalizing existing vocabulary

Issue [#849](https://github.com/justin-hayes/mouseion/issues/849) owns a
separate, operator-run data backfill. Activating profile version 6 and running
that backfill are two distinct rollout steps:

- The backfill applies the v6 profile to German identity keys in mutable learner
  projections: known vocabulary, vocabulary state, curated sentences, selection
  candidates, generated vocabulary, and deck-preparation vocabulary. It also
  updates any dependent mutable identity columns required to keep those keys
  consistent.
- Collision handling is deterministic. Known-vocabulary duplicates retain the
  oldest fact; generated-vocabulary duplicates retain the earliest generation
  provenance; vocabulary state uses `known > generated > accepted > ignored >
  candidate`, with the latest update breaking ties. A collision that cannot be
  merged without losing a curated or immutable fact aborts the owner batch and
  is reported for operator resolution.
- Completed normalized-corpus runs and prepared-deck manifests are immutable
  historical artifacts and are not rewritten. Current Journey members whose
  analysis must expose the new identity are re-analyzed through the normal
  content-revision flow; the old run remains operational history.

The application owns the backfill command, while the operator owns when it is
run. It takes an advisory lock, processes one owner in one database transaction,
and records counts and conflicts. A rollback leaves that owner unchanged. A
process failure after commit is safe to retry because v6 canonicalization is
idempotent and the deterministic merge rules produce the same result; a
successful rerun is a no-op. Profile version 6 becomes the active profile only
after the backfill verification reports no unresolved conflicts.

The expected impact is proportional to the number of German identity rows, not
the size of the corpus or source files. Most installations will have no rows
to change. During an owner transaction, writes to the affected identity rows
may wait briefly; there is no application-wide downtime and immutable corpus
or deck artifacts are not rewritten. Any required current-book re-analysis is
the existing asynchronous analysis flow and can temporarily leave the old run
visible until it completes.

## Consequences

- Newly analyzed historical German spellings resolve to modern dictionary keys
  and display modern spellings on generated cards.
- Modern `ß` remains semantically meaningful and distinct lexemes do not merge
  merely because they contain `ß`.
- The Go runtime and dictionary index cannot drift on this policy because they
  load the same data source.
- Profile version metadata distinguishes v6 analyses from v5 analyses, while
  preserving immutable historical evidence and prepared artifacts.
- The backfill is a data-only operational step, not a startup side effect or a
  rewrite of analysis history.

## Alternatives considered

- **Replace every `ß` with `ss`.** Rejected: it changes modern orthography and
  collapses `Maße` with `Masse`.
- **Ask the dictionary index to decide historical spelling.** Rejected: the
  dictionary is an enrichment source, not the vocabulary-identity authority;
  missing entries must still canonicalize deterministically.
- **Rewrite every historical analysis in place.** Rejected: normalized corpus
  runs and prepared manifests are immutable evidence and must remain
  reproducible under their recorded profile.
- **Leave old learner identities permanently unchanged.** Rejected: known and
  generated vocabulary would fail to exclude the corresponding modern lemma
  after a new analysis.

## References

- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md)
- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md)
- [ADR 0061: German separable-verb lemmatization from dependency data](0061-german-separable-verb-lemmatization.md)
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md)
- [Issue #847: German ß lemmas resolve from the dictionary index](https://github.com/justin-hayes/mouseion/issues/847)
- [Issue #849: Pre-1996 German ß spellings canonicalize to post-1996 lemmas](https://github.com/justin-hayes/mouseion/issues/849)
- [Rat für deutsche Rechtschreibung: Regeln und Wörterverzeichnis](https://www.rechtschreibrat.com/regeln-und-woerterverzeichnis/)
