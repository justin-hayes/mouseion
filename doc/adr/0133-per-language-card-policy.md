# ADR 0133: Each language has one card policy

Status: **Accepted** · Date: 2026-11-25 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided by Justin in the grillings
[Grilling: card policy for Latin (source evidence and the IsAncientGreek gate)](https://github.com/justin-hayes/mouseion/issues/2672#issuecomment-7164047240),
[Grilling: Latin lemma identity, normalisation and the dictionary](https://github.com/justin-hayes/mouseion/issues/2666#issuecomment-7163693391)
and [Grilling: reference translations for Latin passage context](https://github.com/justin-hayes/mouseion/issues/2668#issuecomment-7163219318),
and confirmed in the closing grilling of the map
[Wayfinder: classical Latin in Mouseion](https://github.com/justin-hayes/mouseion/issues/2658).

It amends [ADR 0101](0101-ancient-greek-cards-from-source-evidence.md),
[ADR 0104](0104-compact-lsj-definition-outline.md) and
[ADR 0105](0105-greek-evidence-assisted-pass-always-on.md). Their decisions are
written for Ancient Greek (`grc`) only. This ADR makes the source-evidence card
a policy that any language can take, and applies it to Latin (`la`) from
[ADR 0132](0132-classical-latin-from-opera-latina-adnotata.md).

## Context

ADR 0101 gave Greek a card built only from imported sources (passage context
and the LSJ entry) that makes no provider call, with an optional
evidence-assisted provider pass on top. In code, one predicate decides whether
a language gets that card: whether the language is Ancient Greek. The predicate
controls five things:

- the sentence-boundary rule;
- the source notices;
- the LSJ entry;
- Brownson passage context;
- whether a deck can be prepared with no provider.

Any other language takes the modern-language route. That route is refused when
no provider is configured. Otherwise it needs a provider-written contextual
Gloss, uses Kaikki/Wiktionary meaning evidence, and can produce Idiom cards.

Latin needs the Greek card, not the modern-language route, but with different
sources:

- **Dictionary.** Lewis & Short (L&S) at `PerseusDL/lexica@56061ca1`, the LSJ
  pin, is CC BY-SA Source content.
- **Translations.** Perseus `perseus-engN` at `canonical-latinLit@8cef5162`,
  one pinned file per Work.
- **Lookup.** OLA does not number homographs. Greek's "exactly one candidate or
  nothing" lookup would leave about 28.7 % of matched Latin tokens without an
  entry.

Moving the Greek branch behind a new name such as "is classical" would not
help: the lookup rule, translation source, notices and citation filter all
differ per language.

## Decision

### A card policy per language

**Each study language has exactly one card policy.** The policy is one value
that names:

- the dictionary and its lookup rule: Greek takes exactly one candidate,
  Latin takes all of them;
- the source of reference translations;
- the source notices;
- the citation filter for the full entry;
- the sentence-boundary rule;
- whether a deck can be prepared with no provider.

A language either uses **source-evidence cards** or takes the modern-language
provider route. There is no mixing within a language. The policy replaces the
Ancient Greek predicate everywhere, and Greek's policy reproduces today's Greek
behaviour exactly. How the value is stored is left to the implementation.

### Latin's policy

- **Card.** Latin uses the source-evidence card. A deck can be prepared with no
  provider configured.
- **Passage context.** ADR 0101's rule applies unchanged:
  - every translation segment whose Citation equals or contains one the
    sentence touches;
  - labelled "Not a translation of this sentence", with nothing highlighted;
  - no length cap;
  - no English, and no placeholder, when any touched section has no segment.
- **Dictionary entry display.**
  - **Lookup.** The Latin lookup returns **every** L&S entry under the folded
    key: NFC, strip marks, lowercase, `v`→`u`, `j`→`i`, with no `ae`/`oe` fold.
  - **Stacking.** The card shows every returned entry in source (`id`) order,
    with no cap and no reordering. Each entry is labelled with its L&S number
    and has its own outline and its own collapsed full entry.
  - **Head line only.** An entry with no senses (an unresolved
    cross-reference, "Part. of X") shows only its head line.
  - **No match.** When no entry matches, the entry block is omitted silently.
  - **Rejected alternatives.** Showing only the first entry was rejected
    because it is a silent guess. Letting the provider choose was rejected
    because the card must work with no provider.
- **Outline (amends ADR 0104).** ADR 0104's positional rule applies to each
  entry, plus three L&S-specific steps:
  - an abbreviation stop-list;
  - a cross-reference resolver;
  - ordering by `id`/`level` rather than L&S's repeated `n` labels.
- **Citations in the full entry.** Only citations of the card's own Work's
  author are kept. This generalises Greek's Xenophon-only filter. A `<bibl>`
  with no usable author reference is dropped, never guessed.
- **Sentence boundary.** Latin uses the rule for imported sources: a terminal
  mark only, with no capitalised start, as Greek does.
- **Retired for `la`.** The following never apply to Latin:
  - the required contextual Gloss, and the refusal to prepare with no provider;
  - Kaikki/Wiktionary meaning evidence, because L&S is the only meaning
    evidence;
  - Idiom cards, even if `la` is listed for them;
  - the notices for NLP-analysed languages.
- **Notices.** Latin notices name OLA (LatinPipe output, pinned release),
  Perseus `canonical-latinLit` with the pinned translation, and Lewis & Short,
  each with its pin and CC BY-SA attribution.

### Release checks for a Latin Work

- **Dictionary match.** At least 90 % of a Work's eligible occurrences
  (NOUN/VERB/ADJ/ADV, after Latin admission) must match an L&S key. This is a
  recorded release check, not a runtime gate.
- **Translation.** A Work enters only if its translation is at tier A or B:
  - **Tier A:** the translation uses the same Citation as the Latin.
  - **Tier B:** the translation is keyed one level up, with segments of at most
    400 words.

  It must also pass a mechanical join of its OLA Citations to the translation
  segments (on the order of 98 %) and a spot check that Justin records.
- **Failure.** A Work that fails either check is held out.

### Provider pass (amends ADR 0105)

- **Terms.** The evidence-assisted pass and the sense summary apply to any
  source-evidence language on Greek's terms.
- **Several entries.** The pass receives the senses of **every** returned
  entry, with handles that carry the entry. A Gloss it proposes may implicitly
  pick an entry, but the card still shows all entries.
- **Summaries.** The summary is cached per dictionary pin, entry ID and prompt
  version, with one summary per entry. It is labelled after its dictionary
  ("Summary of Lewis & Short, provider-written").
- **Assessments.** Each language needs its own provider payload assessment for
  each payload kind, with a prompt contract that names its dictionary. Justin
  reviews each one before anything is sent.
- **Latin's first slice.** It records no `la` assessment, so Latin decks are
  default cards only.

### Amendments to ADR 0101

- **Lookup rule.** "An identity is matched to an entry only when the match is
  unambiguous" becomes the Greek lookup rule within Greek's policy. It is not
  a rule for every source-evidence language.
- **Dictionary entry display.** It shows **every entry the lookup returns, in
  source order**, rather than one entry.

## Considered options

- **A predicate for "is classical".** Rejected: it would only move the
  `grc`/`la` branches behind a new name, and every branch would still test the
  language.
- **Latin on the modern-language provider route.** Rejected: it needs a
  provider, uses Wiktionary meaning evidence instead of L&S, and contradicts ADR 0101's
  reason for the source-evidence card.
- **The first L&S entry only, or a provider-chosen entry.** Rejected, as above.
- **A cap on stacked entries.** Rejected: after POS eligibility, the cases left
  (`dico`, `sum`) have few entries. A cap would hide an entry silently.

## Consequences

- **Greek.** Greek behaviour is unchanged. The existing Greek card tests are the
  guard on the refactor.
- **Latin decks.** A Latin deck works with no provider configured and contains
  default cards only until the `la` assessments are recorded.
- **Prepared decks.** A frozen prepared-deck item must be able to record
  several entry IDs. How it does so is left to the implementation.
- **Renaming.** CSS and labels named after LSJ need language-neutral names
  where they are shared.
- **Domain language.** `CONTEXT.md` gains **Source-evidence card**. Dictionary
  entry display, Evidence-assisted provider pass, Meaning evidence and
  Dictionary index are generalised to name Lewis & Short beside LSJ. These
  edits land with the implementation.

## Related

- [ADR 0101: Ancient Greek cards from source evidence](0101-ancient-greek-cards-from-source-evidence.md) — amended
- [ADR 0104: Compact LSJ definition outline](0104-compact-lsj-definition-outline.md) — amended
- [ADR 0105: The Ancient Greek evidence-assisted pass](0105-greek-evidence-assisted-pass-always-on.md) — amended
- [ADR 0111: One licence posture for third-party data in every language](0111-third-party-licence-posture.md)
- [ADR 0132: Classical Latin from Opera Latina Adnotata](0132-classical-latin-from-opera-latina-adnotata.md)
