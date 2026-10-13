# Study languages and known vocabulary workflow

Status: **Canonical supporting workflow.** Study languages are derived from My
Books. Known vocabulary comes only from Reading completion: starting current
reading freezes the active vocabulary snapshot, and finishing it adds eligible
identities to modeled Known vocabulary. Known-vocabulary import is retired by
[ADR 0092](../../adr/0092-retire-known-vocabulary-import.md). Earlier Goal and
forecast behavior under [ADR 0072](../../adr/0072-goal-owned-vocabulary-and-journey-forecast.md)
is historical. Learner-facing copy uses reading and preparation facts as defined in
[`terminology.md`](../terminology.md).

## Goal

Keep runtime NLP capabilities, derived study languages, and vocabulary state
distinct. No language preference needs to be configured: chosen-language Books
define the available study languages, and the active study language
([ADR 0050](../../adr/0050-active-study-language.md)) selects which one the
per-language surfaces present instead of a per-page picker.

The product behavior is defined primarily by:

- [Language Support](../../features/language-support.md)
- [ADR 0023: NLP capabilities](../../adr/0023-nlp-capabilities.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [Reading workflow](../../features/reading-workflow.md)

## Canonical destination

**Vocabulary** is the primary-navigation destination for vocabulary
investigation; it is Concordance at `/vocabulary`. Browse for the current Book
is hosted on Reading. The retired `/known-vocab`, `/vocabulary/import`, and
import status routes return 404 without a redirect.

## Study-language model

Four concepts remain distinct:

- **Available analysis language** — currently advertised as ready by the NLP
  service;
- **Study language** — the distinct normalized language tags of the learner's
  active chosen-language Books;
- **Active study language** — the one study language the learner is currently
  working in; a stored context pointing into the derived set that scopes the
  language-dependent surfaces ([ADR 0050](../../adr/0050-active-study-language.md));
- **Known vocabulary** — owner-scoped identities accepted when a current reading
  is finished; this is modeled knowledge, not verified mastery.

The active study language selects which language Vocabulary shows, instead of a
per-page picker; a known-vocabulary-only language (no current chosen-language
Book) remains selectable in the shell switcher as a read-only "no books" entry.
A catalog-synced Book's language comes from its catalog entry; connection
re-sync is the only way that language changes. Metadata changes do not delete
known vocabulary or the Book's derived history. Capability discovery still
controls which catalog feeds can be synchronized, but it does not directly
create or remove known-vocabulary data.

## Vocabulary provenance and correction boundary

Every Known identity records the Reading completion that accepted it. No
surface displays the known-vocabulary read model or labels generated or Reserved
vocabulary as Known. Provenance remains available to the selection, coverage,
and history consumers that need it.

The product has no learner-facing removal of individual known-vocabulary
entries. Vocabulary must not imply that changing a Book's language deletes
vocabulary or that a completed vocabulary transition can be undone. A future
removal/correction workflow requires an explicit product contract for
provenance, snapshot acceptance, and coverage recalculation.
