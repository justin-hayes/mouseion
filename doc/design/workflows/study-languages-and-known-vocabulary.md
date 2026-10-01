# Study languages and known vocabulary workflow

Status: **Canonical supporting workflow.** Study languages are derived from My
Books and Vocabulary is the sole learner-facing home for known-vocabulary
import. It is an import-and-status surface, not a display of the known-vocabulary
read model. Starting current reading freezes the active vocabulary snapshot;
completion adds eligible identities to modeled Known vocabulary. Earlier Goal
and forecast behavior under [ADR 0072](../../adr/0072-goal-owned-vocabulary-and-journey-forecast.md)
is historical. Learner-facing copy uses reading and preparation facts as defined in
[`terminology.md`](../terminology.md).

This document describes the **shipped import workflow**. The accepted,
not-yet-shipped [Vocabulary Browse, Concordance, and Custom deck
specification](../../features/vocabulary-browse-concordance-and-custom-decks.md)
adds peer views beneath the same destination; it does not replace the import
contract or turn import into a Known-vocabulary list.

## Goal

Give a learner one coherent Vocabulary destination for importing known vocabulary
without conflating runtime NLP capabilities, derived study languages, and
vocabulary state. No language preference needs to be configured: chosen-language
Books define the available study languages, and the active study language
([ADR 0050](../../adr/0050-active-study-language.md)) selects which one
Vocabulary presents instead of a per-page picker.

The product behavior is defined primarily by:

- [Language Support](../../features/language-support.md)
- [ADR 0023: NLP capabilities](../../adr/0023-nlp-capabilities.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [Reading workflow](../../features/reading-workflow.md)

## Canonical destination

**Vocabulary** is currently the primary-navigation destination for
known-vocabulary import. The shipped view has one explicit workflow:

1. **Known-vocabulary import** — submit an additive lemma file for the active
   study language and understand its durable processing result.

The retained `/known-vocab` route is compatibility surface, not an independent
product area. The implementation redirects `GET /known-vocab` to `/vocabulary`.
New known-vocabulary functionality belongs in Vocabulary. The accepted target
adds Browse as its landing view and Concordance as another peer view, while
keeping import distinct; until implemented, the shipped route remains
import-first.

## Study-language model

Four concepts remain distinct:

- **Available analysis language** — currently advertised as ready by the NLP
  service;
- **Study language** — the distinct normalized language tags of the learner's
  active chosen-language Books;
- **Active study language** — the one study language the learner is currently
  working in; a stored context pointing into the derived set that scopes the
  language-dependent surfaces ([ADR 0050](../../adr/0050-active-study-language.md));
- **Known vocabulary** — owner-scoped lemmas explicitly imported or accepted
  through completed-Goal snapshot transition; this is modeled knowledge, not
  verified mastery.

Derived study languages define which languages may be viewed or imported. The
active study language selects which one Vocabulary shows, instead of a per-page
picker; a known-vocabulary-only language (no current chosen-language Book)
remains selectable in the shell switcher as a read-only "no books" entry, where
import stays disabled. A catalog-synced Book's language comes from its
catalog entry; connection re-sync is the only way that language changes.
Metadata changes do not delete known vocabulary or the Book's derived history.
Capability discovery still controls which catalog feeds can be synchronized,
but it does not directly create or remove known-vocabulary data.

## Known-vocabulary import

**Learner decision:** Which lemmas do I already know and want Mouseion to count
as known?

The import belongs to the active study language and accepts one lemma per line
under the existing import contract. Before submission, the screen identifies the
language and accepted file format. Import is additive and idempotent; duplicate
entries are reported rather than counted as new.

The asynchronous workflow is:

```text
Confirm the active study language in the shell switcher
    -> Choose lemma file
    -> Start import
    -> Processing status
    -> Imported, duplicate, and rejected summary
```

The result distinguishes:

- newly imported entries;
- already-known duplicates;
- rejected lines with actionable reasons;
- whole-import failure or cancellation.

A partial rejection does not hide successful imports. The learner can safely
leave and return while durable processing continues.

## Vocabulary provenance and correction boundary

Known vocabulary may contain explicitly imported vocabulary and vocabulary
accepted through completed Goal snapshots. The import surface reports only the
import outcome; it does not display the known-vocabulary read model or label
generated or Reserved vocabulary as Known. Provenance remains available to the
selection, coverage, forecast, and history consumers that need it.

The current product supports additive import but not learner-facing removal of
individual known-vocabulary entries. Vocabulary must not imply that changing a
Book's language deletes vocabulary or that a completed vocabulary transition
can be undone. A future removal/correction workflow requires an explicit product
contract for provenance, wildcard entries, Goal snapshot acceptance, and
coverage recalculation.

## Screen hierarchy

Vocabulary answers questions in this order:

1. Which study language am I currently working in?
2. How can I import additional known lemmas?
3. What happened during the latest import?

Account identity may appear as supporting context but should not displace these
learner tasks.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No study languages | Explain that chosen-language books define available vocabulary languages. | Connect a catalog |
| Active study language available | Show the language context and import form; the shell switcher carries the context. | Import lemma file |
| Known-vocabulary-only language | Explain that importing is unavailable until a chosen-language Book exists. | Switch back to a study language |
| No prior import | Keep the import form available without making a claim about language ability. | Import lemma file |
| Import ready | Show the active language and file contract. | Import known vocabulary |
| Import processing | Show durable status and safe-leave guidance. | Cancel only when supported |
| Import complete | Separate new, duplicate, and rejected counts. | Import another file |
| Import failed/cancelled | Preserve the active language and actionable recovery detail. | Retry when safe |

## Accessibility and responsive contract

- The Vocabulary heading and the shell's active-language switcher make the
  current study-language context direct and visible.
- Language controls use names as primary labels and codes as supporting detail.
- Import status and results use scoped live regions without repeatedly stealing
  focus.
- Rejected-row detail remains associated with its summary and is usable without
  color.
- On narrow screens, the import controls and status summary remain in document
  order and rejected-row tables do not force page-level horizontal scrolling.
- The server-rendered form, processing result, and final import summary remain
  coherent before HTMX enhancement.
