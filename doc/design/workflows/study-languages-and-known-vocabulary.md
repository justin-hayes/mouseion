# Study languages and known vocabulary workflow

Status: **Canonical supporting workflow.** Study languages are derived from My
Books and Vocabulary is the sole learner-facing home for known-vocabulary
import. Book vocabulary study owns reservation and graduation provenance;
learner-facing copy uses reading, preparation, and vocabulary-transition facts as
defined in [`terminology.md`](../terminology.md).

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
- [ADR 0027: Learning campaigns and vocabulary graduation](../../adr/0027-learning-campaigns.md)

## Canonical destination

**Vocabulary** is the primary-navigation destination for known vocabulary. It has
one explicit workflow:

1. **Known vocabulary** — owner-scoped vocabulary by derived study language,
   including file import.

The retained `/known-vocab` route is compatibility surface, not an independent
product area. The implementation redirects `GET /known-vocab` to `/vocabulary`.
New known-vocabulary functionality belongs in Vocabulary.

## Study-language model

Four concepts remain distinct:

- **Available analysis language** — currently advertised as ready by the NLP
  service;
- **Study language** — the distinct normalized language tags of the learner's
  active chosen-language Books;
- **Active study language** — the one study language the learner is currently
  working in; a stored context pointing into the derived set that scopes the
  language-dependent surfaces ([ADR 0050](../../adr/0050-active-study-language.md));
- **Known vocabulary** — owner-scoped lemmas explicitly imported or added
  through a justified vocabulary transition.

Derived study languages define which languages may be viewed or imported. The
active study language selects which one Vocabulary shows, instead of a per-page
picker; a known-vocabulary-only language (no current chosen-language Book)
remains selectable in the shell switcher as a read-only "no books" entry, where
import stays disabled. A catalogue-synced Book's language comes from its
catalogue entry; connection re-sync is the only way that language changes.
Metadata changes do not delete known vocabulary or the Book's derived history.
Capability discovery still controls which catalogue feeds can be synchronized,
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

The Vocabulary list may contain explicitly imported vocabulary and vocabulary
added through a justified completed transition. It must not label generated,
reserved, or unfinished-work vocabulary as known. Where provenance is
available, the interface may distinguish imported entries from entries added
through the accepted transition without implying different coverage weight.

The current product supports additive import but not learner-facing removal of
individual known-vocabulary entries. Vocabulary must not imply that changing a
Book's language deletes vocabulary or that a completed vocabulary transition
can be undone. A future removal/correction workflow requires an explicit product
contract for provenance, wildcard entries, Book vocabulary-study graduation, and
coverage recalculation.

## Screen hierarchy

Vocabulary answers questions in this order:

1. Which study language am I currently working in?
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
| Active study language available | Show that language's known vocabulary; the shell switcher carries the context. | Import lemma file |
| Known-vocabulary-only language | Show read-only entries marked "no books"; import disabled. | Switch back to a study language |
| No known vocabulary | Explain current coverage implications without implying no language ability. | Import lemma file |
| Import ready | Show the active language and file contract. | Import known vocabulary |
| Import processing | Show durable status and safe-leave guidance. | Cancel only when supported |
| Import complete | Separate new, duplicate, and rejected counts. | Review known vocabulary |
| Import failed/cancelled | Preserve the active language and actionable recovery detail. | Retry when safe |
| Known list | Identify the active language and provenance where available. | Import additional entries |

## Accessibility and responsive contract

- The Vocabulary heading and the shell's active-language switcher make the
  current study-language context direct and visible.
- Language controls use names as primary labels and codes as supporting detail.
- Import status and results use scoped live regions without repeatedly stealing
  focus.
- Rejected-row detail remains associated with its summary and is usable without
  color.
- On narrow screens, controls precede long vocabulary lists; lists or tables do
  not force page-level horizontal scrolling.
- The server-rendered form, processing result, and final list remain coherent
  before HTMX enhancement.
