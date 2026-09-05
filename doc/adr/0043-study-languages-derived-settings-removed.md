# ADR 0043: Study languages are derived from the library and Settings is removed

Status: **Accepted** · Date: 2026-09-04 · Author: Justin + opencode

## Context

Settings is the only home of two things: saved study-language preferences and
known-vocabulary import. Today the catalogue sync's language scope is gated on
saved `language_profiles`: `eligibleLanguages` cross-references them against NLP
capabilities, and a book's `language_tag` comes from the study language that
produced the feed walk — it is never inferred from catalogue or content. A
learner with no study languages syncs nothing. The authenticated shell has
exactly three destinations (My Books, Reading Journey, Settings), and the
information architecture's "Study language" object is an "owner-scoped
preference selected from capabilities."

The study-language preference is redundant configuration: the languages a
learner reads are exactly the languages the catalogue sync already acquires.
This ADR flips the causality — languages flow out of the library, not into it —
and removes the Settings destination.

## Decision

- **Study languages become a derived set.** A learner's study languages are the
  distinct normalized `language_tag` values of their books where
  `language_state='chosen'`. No stored selection remains; `language_profiles`
  is dropped in a new migration. No backfill into `books` is needed because
  existing books already carry the tags a catalogue-driven sync would produce.
- **Catalogue sync becomes catalogue-driven.** The sync enumerates the OPDS
  `/language` navigation feed and walks every non-English language the NLP
  service reports ready, tagging each book from the walked language. The first
  sync therefore ingests every ready-language book a catalogue offers rather
  than the learner's chosen subset.
- **Known-vocabulary import moves to a new Vocabulary destination.** The shell
  becomes **My Books | Reading Journey | Vocabulary**. Import eligibility is
  the derived study-language set. Routes become `/vocabulary`,
  `/vocabulary/import`, and `/vocabulary/imports/{id}/status`; `GET /known-vocab`
  redirects to `/vocabulary`; the `return_to` mechanism and the legacy
  standalone `KnownVocabPageWithResult` page are removed.
- **`supported_languages` remains the server-wide display-name reference**,
  now populated whenever capabilities are fetched (not on language add). Derived
  languages LEFT JOIN it, falling back to the raw tag when NLP is unavailable.
- **The per-book language control on My Books stays** and feeds the derived set.
- **`/settings` redirects to `/library`**; the My Books empty-state study-language
  CTA and the connections-page language-scope line are removed; the
  "Open Settings" guidance link repoints to `/vocabulary`.
- **The term "study language" is retained and redefined** as derived-from-library
  (recorded in the repo glossary).
- **Migration rollout is backup-first.** Migration 000046 drops the obsolete
  preference table after a pre-migration database backup. Its down migration can
  recreate the table shape only; deleted preference rows require recovery from
  that backup. No book, vocabulary, or analysis rows are rewritten.

## Alternatives considered

- **Materialize study languages** (repopulate `language_profiles` from books
  after sync). Rejected: a cache of derived state with an invalidation burden,
  and the table's meaning — "a language I chose" — becomes false.
- **Keep study-language-driven sync.** Rejected: contradicts the premise that
  languages come from the library and preserves the configuration step.
- **Per-connection language configuration** (resurrect
  `opds_connections.language`). Rejected: reintroduces the step being removed.
- **Vocabulary embedded per-language** across My Books / Reading Journey.
  Rejected in favour of one dedicated destination for the whole import flow.
- **Import eligibility = any NLP-ready language.** Rejected: reintroduces a
  non-derived language source.
- **Drop `supported_languages` too.** Rejected: loses resilient display names
  during NLP outages.
- **Rename "study language" to "library language".** Rejected: copy churn
  without a clarity gain; the word "study" still fits a derived-by-construction
  reading set.

## Consequences

- The first sync after upgrade ingests the full ready-language library of each
  connected catalogue (a behavioural change in volume and time).
- Existing learners with a configured language but no books in it lose that
  language from the derived set until sync brings books; known-vocabulary rows
  are untouched and their languages remain displayable.
- `supported_languages` gains a new write path on capability fetch.
- The three-destination shell changes; `information-architecture.md`,
  `product.md`, `screen-inventory.md`, and the catalogue-sync and language-support
  feature docs must be reconciled, and ADR 0042's "three principal destinations"
  reference updated, via the documented architecture checkpoint
  (`information-architecture.md#contract-changes-requiring-planneradr-work`).
- `fixtureserver` derives languages from fixture books instead of profile stubs;
  Playwright smoke tests and broad webapp/cataloguesync test fallout follow.

## Open questions

- Does the **Vocabulary** destination also present the learner's current
  known-vocabulary inventory per language, or only the import flow? The
  Settings page showed a per-language known table and the retained
  `KnownVocabResult` component renders one, so the page is expected to keep
  that inventory — but it is not yet part of a settled contract.
- Should the first-sync volume increase (full ready-language library) surface a
  pagination or cap adjustment when walking each language feed, or does the
  existing `BrowseLanguage` pagination already cover it?

## Related

- [ADR 0038: Schema-change governance](0038-schema-change-governance.md)
- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0042: Derive a per-language corpus view without a persisted corpus object](0042-derived-language-corpus-view.md)
- [Information architecture](../design/information-architecture.md)
- [Screen inventory](../design/screen-inventory.md)
- [Feature: Catalogue Sync](../features/catalog-sync.md)
- [Feature: Language Support](../features/language-support.md)
- [Feature: Language Corpus View](../features/language-corpus-view.md)
