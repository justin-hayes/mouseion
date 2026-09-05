# Study languages and known vocabulary workflow

Status: **Canonical supporting workflow.** Campaign remains an internal accepted
contract where needed for vocabulary provenance; learner-facing copy uses
reading, preparation, and vocabulary-transition facts as defined in
[`terminology.md`](../terminology.md).

## Goal

Give a learner one coherent Vocabulary destination for importing known vocabulary
without conflating runtime NLP capabilities, derived study languages, and
vocabulary state. Study-language preferences remain on the supporting Settings
route until that surface is removed.

The product behavior is defined primarily by:

- [Language Support](../../features/language-support.md)
- [ADR 0023: NLP capabilities](../../adr/0023-nlp-capabilities.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [ADR 0027: Learning campaigns and vocabulary graduation](../../adr/0027-learning-campaigns.md)

## Canonical destination

**Vocabulary** is the primary-navigation destination for known vocabulary. The
supporting Settings screen retains study-language preferences until that surface
is removed. Vocabulary has one explicit workflow:

1. **Known vocabulary** — owner-scoped vocabulary by derived study language,
   including file import.

The retained `/known-vocab` route is compatibility surface, not an independent
product area. The implementation redirects `GET /known-vocab` to `/vocabulary`.
New known-vocabulary functionality belongs in Vocabulary.

## Study-language model

Three concepts remain distinct:

- **Available analysis language** — currently advertised as ready by the NLP
  service;
- **Study-language preference** — a learner's saved catalogue-sync preference;
- **Library study language** — the distinct normalized language tags of the
  learner's active chosen-language Books;

Selecting a study-language preference does not silently mutate known-vocabulary
state. Library study languages define which languages may be viewed or imported
in Known vocabulary; ready preferences define the catalogue-sync scope.

### Add a study language

The interface lists only currently ready capabilities that are not already
saved. Adding a preference does not start analysis, acquire a book, or download
an NLP model.

### Degraded capability discovery

If capability discovery fails:

- saved study languages remain visible with their stored display names;
- the interface explains that availability cannot currently be verified;
- adding a newly available language is disabled;
- existing preferences are not removed or rewritten;
- unrelated known-vocabulary viewing remains available.

### Remove a study language

Removing a study-language preference does not delete known vocabulary, books,
analyses, prepared decks, or campaigns for that language. The confirmation and
success copy must state that only the preference is removed. If the current
known-vocabulary filter uses that language, the interface keeps the language
context long enough to explain the result or moves to another valid filter
without claiming the vocabulary was deleted.

## Known-vocabulary import

**Learner decision:** Which lemmas do I already know and want Mouseion to count
as known?

The import belongs to a selected language and accepts one lemma per line under
the existing import contract. Before submission, the screen identifies the
language and accepted file format. Import is additive and idempotent; duplicate
entries are reported rather than counted as new.

The asynchronous workflow is:

```text
Choose language
    -> Choose lemma file
    -> Start import
    -> Processing status
    -> Imported, duplicate, and rejected summary
    -> Updated known-vocabulary list
```

The result distinguishes:

- newly imported entries;
- already-known duplicates;
- rejected lines with actionable reasons;
- whole-import failure or cancellation.

A partial rejection does not hide successful imports. The learner can safely
leave and return while durable processing continues.

## Vocabulary provenance and correction boundary

The Settings list may contain explicitly imported vocabulary and vocabulary
added through a justified completed transition. It must not label generated,
reserved, or unfinished-work vocabulary as known. Where provenance is
available, the interface may distinguish imported entries from entries added
through the accepted transition without implying different coverage weight.

The current product supports additive import but not learner-facing removal of
individual known-vocabulary entries. Settings must not imply that removing a
a study language deletes vocabulary or that a completed vocabulary transition
can be undone. A future removal/correction workflow requires an explicit product
contract for provenance, wildcard entries, internal Campaign graduation, and
coverage recalculation.

## Screen hierarchy

Vocabulary answers questions in this order:

1. Which study languages are derived from my chosen-language books?
2. Which study language's known vocabulary am I viewing?
3. How can I import additional known lemmas?
4. What happened during the latest import?
5. Which entries are currently counted as known?

Account identity may appear as supporting context but should not displace these
learner tasks.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No study languages | Explain that chosen-language books define available vocabulary languages. | Connect a catalogue |
| Study languages available | Show derived study languages. | Select language for vocabulary |
| No selected vocabulary language | Prompt for a study-language context. | Select language |
| No known vocabulary | Explain current coverage implications without implying no language ability. | Import lemma file |
| Import ready | Show selected language and file contract. | Import known vocabulary |
| Import processing | Show durable status and safe-leave guidance. | Cancel only when supported |
| Import complete | Separate new, duplicate, and rejected counts. | Review known vocabulary |
| Import failed/cancelled | Preserve language and actionable recovery detail. | Retry when safe |
| Known list | Identify language and provenance where available. | Import additional entries |

## Accessibility and responsive contract

- The Vocabulary heading and language picker make the current study-language
  context direct and visible.
- Language controls use names as primary labels and codes as supporting detail.
- Import status and results use scoped live regions without repeatedly stealing
  focus.
- Rejected-row detail remains associated with its summary and is usable without
  color.
- On narrow screens, controls precede long vocabulary lists; lists or tables do
  not force page-level horizontal scrolling.
- The server-rendered form, processing result, and final list remain coherent
  before HTMX enhancement.
