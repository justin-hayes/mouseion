# Study languages and known vocabulary workflow

Status: **Canonical supporting workflow.** Campaign remains an internal accepted
contract where needed for vocabulary provenance; learner-facing copy uses
reading, preparation, and vocabulary-transition facts as defined in
[`terminology.md`](../terminology.md).

## Goal

Give a learner one coherent Settings destination for maintaining study-language
preferences and importing known vocabulary without conflating runtime NLP
capabilities, learner preferences, and vocabulary state.

The product behavior is defined primarily by:

- [Language Support](../../features/language-support.md)
- [ADR 0023: NLP capabilities](../../adr/0023-nlp-capabilities.md)
- [ADR 0024: Learner-owned catalogs and no administrator role](../../adr/0024-learner-owned-catalogs-no-admin.md)
- [ADR 0027: Learning campaigns and vocabulary graduation](../../adr/0027-learning-campaigns.md)

## Canonical destination

**Settings** is the sole primary-navigation destination for study languages and
known vocabulary. The screen has two explicit sections:

1. **Study languages** — learner preferences chosen from currently ready NLP
   capabilities;
2. **Known vocabulary** — owner-scoped vocabulary by language, including file
   import.

The retained `/known-vocab` route is compatibility surface, not an independent
product area. The implementation redirects `GET /known-vocab` to
`/settings#known-vocabulary`, preserving a valid language selection as a
`?language=` query parameter when that language is saved; Settings is
canonical. New functionality belongs in Settings first.

## Study-language model

Two concepts remain distinct:

- **Available analysis language** — currently advertised as ready by the NLP
  service;
- **Study language** — a learner's saved preference;

Selecting a study language does not silently mutate NLP capability state or
known-vocabulary state. Ready study languages also define the catalogue-sync
scope; catalogue browsing itself happens in My Books.

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

Settings answers questions in this order:

1. Which study languages have I saved, and is capability discovery healthy?
2. Which ready languages can I add?
3. Which language's known vocabulary am I viewing?
4. How can I import additional known lemmas?
5. What happened during the latest import?
6. Which entries are currently counted as known?

Account identity may appear as supporting context but should not displace these
learner tasks.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No study languages | Explain preferences and currently ready options. | Add study language |
| Saved languages available | Show saved preferences separately from addable capabilities. | Select language for vocabulary |
| Discovery degraded | Preserve saved preferences and explain blocked additions. | Retry later |
| No newly available languages | Explain that all ready languages are already saved. | Manage known vocabulary |
| Remove preference confirmation | State that books and known vocabulary are preserved. | Remove study language |
| No selected vocabulary language | Prompt for a saved or otherwise valid language context. | Select language |
| No known vocabulary | Explain current coverage implications without implying no language ability. | Import lemma file |
| Import ready | Show selected language and file contract. | Import known vocabulary |
| Import processing | Show durable status and safe-leave guidance. | Cancel only when supported |
| Import complete | Separate new, duplicate, and rejected counts. | Review known vocabulary |
| Import failed/cancelled | Preserve language and actionable recovery detail. | Retry when safe |
| Known list | Identify language and provenance where available. | Import additional entries |

## Accessibility and responsive contract

- Section headings and anchors make Study languages and Known vocabulary direct
  destinations within Settings.
- Language controls use names as primary labels and codes as supporting detail.
- Import status and results use scoped live regions without repeatedly stealing
  focus.
- Rejected-row detail remains associated with its summary and is usable without
  color.
- On narrow screens, controls precede long vocabulary lists; lists or tables do
  not force page-level horizontal scrolling.
- The server-rendered form, processing result, and final list remain coherent
  before HTMX enhancement.
