# ADR 0046: Book language has one canonical base form enforced at the domain

Status: **Accepted** · Date: 2026-09-05 · Author: Justin + opencode

## Context

ADR 0035 requires a chosen Book language to carry a normalized tag, and ADR 0043
derives study languages from the distinct normalized tags of the learner's
chosen-language Books. What "normalized" means is left open, and the shipped
code implements it differently at different depths:

- `canonicalization.NormalizeLanguage` lowers and converts `_` to `-` but keeps
  region subtags (`de_DE` → `de-de`). Catalogue sync uses it when creating
  synced Books, but manual Book entry passes raw form values straight into the
  domain, and `Book.Validate` checks only non-emptiness.
- SQL re-implements the rule inconsistently: study-language derivation does
  `lower(replace(trim(language_tag),'_','-'))`, but My Books pill counts and the
  language-corpus filter only lowercase. The same learner's German can appear as
  `de`, `de-de`, and `de_DE` across surfaces, and known-vocabulary reads carry
  yet another spelling.
- Display-name logic is split: study languages and vocabulary resolve names
  through `supported_languages`, while the language-corpus panel hard-codes
  `de`, `it`, and `en`.

Because sync writes `de` (the capability tag) while `sameLanguage` already
treats `de` and `de-de` as the same language, a manually-entered `de-DE` yields a
second "German" study language, a second pill, and a second known-vocabulary
scope.

## Decision

**A chosen Book language has one canonical form: the base language tag,
lowercased with `_` normalized to `-` and region subtags collapsed.** `de_DE`,
`de-de`, and `de` all canonicalize to `de`. `NormalizeLanguage` itself performs
this collapse, so one function is the single canonical form across sync, OPDS,
capabilities, known-vocabulary, the domain, and the web app. "Base" means the
first hyphen-segment of the normalized tag; there is no ISO three-letter mapping
(`eng` stays distinct from `en`, matching today's behavior).

The canonical form is enforced at the domain:

- `NewBook` normalizes a chosen language tag; `Book.Validate` requires a
  canonical tag and rejects anything else. `canonicalization` remains
  dependency-free, and `domain` imports it.
- The manual-update path (`UpdateBookMetadata`) normalizes the form's tag in the
  handler before persisting; `Book.Validate` enforces the invariant. Catalogue
  sync already normalizes via `NormalizeLanguage` and inherits the collapse.
- A data migration converges existing rows in the three language-keyed tables —
  `books.language_tag`, `known_vocabulary.language`, and
  `supported_languages.language` — to the canonical base form, under ADR 0038's
  governance (deterministic, idempotent, documented rollback).

Once ingress guarantees canonical storage and the migration converges, the SQL
sites that re-implement normalization (study-language derivation, My Books
counts and browse filter, language-corpus filter, and the known-vocabulary
reads) are simplified to exact canonical comparison. This contract step lands
only after the migration so no legacy row breaks.

Display names route through `supported_languages` everywhere, with the canonical
tag as fallback during NLP outages; the language-corpus panel's hard-coded
`de`/`it`/`en` switch is removed.

The fixtureserver stores and matches the canonical base form so the two adapters
(PostgreSQL, fixtureserver) agree on language identity and the browser smoke
tests exercise the same spelling invariant as production.

## Alternatives considered

- **Keep region subtags in the canonical form** (`de_DE` → `de-de`). Rejected:
  sync writes `de`, and the product already treats region as insignificant for
  language identity (`sameLanguage` compares base). Two spellings for German
  would persist as a learner-facing inconsistency.
- **Enforce normalization only at call sites.** Rejected: this is today's leak —
  the next caller that forgets reintroduces the split. The Book owns the
  invariant instead.
- **Leave the tolerant SQL as defense.** Rejected: the re-implementation the
  deepening exists to remove would survive; exact comparison is safe once
  ingress and the migration guarantee canonical storage.
- **Leave the fixtureserver raw.** Rejected: a seam with two adapters must have
  them agree, or it is leaky rather than substitutable.

## Consequences

- One spelling of a language everywhere: one study language, one pill, one
  known-vocabulary scope per base language.
- Manual Book entry and updates produce canonical tags by construction;
  non-canonical input fails validation rather than persisting.
- The tolerant SQL re-implementations are removed once the migration converges.
- The fixtureserver and PostgreSQL adapters agree on language identity.

## Caveat

If a future NLP capability is genuinely region-distinct (for example simplified
versus traditional Chinese), base collapse would merge languages that the NLP
service treats separately. That decision revisits the canonical form; the
capability tags are the source of truth and must stay distinguishable.

## Related

- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0038: Schema-change governance](0038-schema-change-governance.md)
- [ADR 0043: Study languages are derived from the library and Settings is removed](0043-study-languages-derived-settings-removed.md)