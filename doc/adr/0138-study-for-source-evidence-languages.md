# ADR 0138: Study serves source-evidence languages with a cited contextual Gloss

Status: **Accepted; implementation pending** · Date: 2026-11-26 ·
Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided in
[#2728](https://github.com/justin-hayes/mouseion/issues/2728) in a grilling
session. It satisfies the source-evidence prerequisite of the Study cutover in
[ADR 0136](0136-study-replaces-known-vocabulary-and-prepared-deck.md) §8.

It amends:

- [ADR 0134](0134-study-in-mouseion-vocabulary-review.md) §1, which offers
  Study only on the modern-language provider route, and §6, whose Learned
  meaning is a Gloss with nothing to check it against;
- [ADR 0101](0101-ancient-greek-cards-from-source-evidence.md), whose default
  card needs no provider. At the Study cutover, vocabulary work in a
  source-evidence language needs a provider.

## Context

Study is offered only for languages whose card policy is the modern-language
provider route ([ADR 0133](0133-per-language-card-policy.md)). Its Study
material needs a contextual Gloss and a sentence translation, and the Gloss
becomes the word's Learned meaning. Ancient Greek and Latin use source-evidence
cards. Their default card has no Gloss: ADR 0101 found a contextual Gloss
reliable only with human-approved alignment, and Latin's lookup returns every
Lewis & Short homograph.

ADR 0136 retires the Prepared deck once Study serves these languages. Without
this decision, Greek and Latin would lose their only vocabulary workflow.

Three anchors for a classical Learned meaning were weighed:

- **The dictionary entry.** It needs no provider and never guesses, but it
  teaches a lexical identity rather than a meaning in a sentence. For Latin it
  can be several entries.
- **The provider-proposed Gloss, falling back to the entry.** The anchor would
  then depend on whether an assessment was recorded on the day of
  introduction.
- **A sense the learner picks** at introduction. This puts a philological task
  on every new word, and the learner can pick wrongly.

The Evidence-assisted provider pass already proposes a contextual Gloss, a
sentence translation and an English target alignment. It works from the
dictionary's senses, each with a native ID, and from the passage context, and
it cites the senses that support its Gloss. Greek's pass assessment is recorded
(#2171). Latin records none.

## Decision

### 1. Parity with the modern languages

- **Learned meaning.** A source-evidence language gets the same kind of Learned
  meaning as a modern language: a contextual Gloss, with a sentence
  translation. Its Study material comes from the Evidence-assisted provider
  pass.
- **No providerless fallback.** Study for a source-evidence language waits for
  a configured provider and matching assessments, as German Study does. A word
  never falls back to an entry-only Learned meaning.
- **Keyed to the policy.** Study branches on the card policy, never on `grc`
  or `la`. Per-language differences, such as Latin's stacked entries, come from
  the policy value, as ADR 0133 requires. A further source-evidence language
  gets Study under the same rules.

### 2. The Gloss must cite the dictionary

- **Citation required.** A pass answer becomes ready Study material only if its
  Gloss cites at least one sense of the returned dictionary entries, and every
  cited handle exists. A Gloss with no cited sense, or an abstention, is an
  omission on the merits: the word is unpreparable with reason `meaning`.
- **No entry.** A word for which the lookup returns no entry has no senses to
  cite. It is unpreparable with its own reason, `no-dictionary-entry`, so a
  lookup gap is told apart from a provider's refusal.
- **What is recorded.** A classical Learned meaning records its Gloss, the
  sense IDs it cites, their entry IDs and the dictionary pin. A modern Learned
  meaning stays a Gloss.
- **Pin changes.** A Learned meaning keeps the pin and citations it was
  introduced with. A new dictionary pin never re-introduces a word. New Study
  material and Fluency contexts cite the current pin.

### 3. The card

- **Rows.** The card back shows the same rows as a modern word: Meaning (the
  Gloss), Translation and Word, with the English target highlighted when the
  pass returns an alignment and nothing highlighted when it does not. The Gloss
  carries no "provider-proposed" label, because a modern Gloss carries none.
- **Collapsed extras.** Below the rows, two collapsed sections:
  - **Dictionary:** the compact outline of each cited entry
    ([ADR 0104](0104-compact-lsj-definition-outline.md)), with the cited senses
    marked;
  - **Passage context:** the reference translation, with its "Not a
    translation of this sentence" label.
- **Attribution.** The Study page carries the dictionary and translation
  attribution its policy's source notices name.

### 4. Fluency

- **Each context runs the pass.** A Fluency context gets its own cited Gloss
  and translation from the pass, under §2's rules.
- **Agreement.** The modern Fluency agreement check decides whether a context
  agrees with the Learned meaning, with the cited senses of both Glosses added
  to its payload.
- **Entry mismatch.** A context whose Gloss cites only entries other than those
  the Learned meaning cites, on the same pin, disagrees without a provider call.
  A sense-level mismatch alone is not enough, because the dictionaries'
  sub-senses are too fine to reject on.

### 5. Reports

[ADR 0135](0135-study-reports.md) applies unchanged. A Gloss that cites the
wrong Latin homograph, or the wrong sense, is an **Unsuitable context**: the
identity is right and the meaning is wrong. A Wrong-word report is repaired
through a Lemma correction or Occurrence exclusion, which already work on
imported analyses.

### 6. Evidence gate and rollout

- **Evidence run.** Before Study turns on for a source-evidence language, the
  pass runs on a sample of that language's Study material, about 50 words from
  the real Corpus. Justin judges each Gloss and translation. The language turns
  on only when at least 90% of Glosses are acceptable and no answer cites an
  invented sense. The threshold is agreed before the run.
- **Assessments.** Each language needs a recorded assessment for the pass and
  for Fluency agreement (ADR 0133 §Assessments). Greek's pass assessment covers
  Study only while Study sends the same payload; any change needs a new
  recording.
- **Order.** Greek first, then Latin once its assessments are recorded and its
  evidence run passes.
- **Side by side.** Until the cutover, each language runs Study beside its
  Prepared decks, as German does in the pilot.

## Considered options

- **The dictionary entry as the Learned meaning.** This was rejected in favour
  of parity. Two kinds of Learned meaning would split every downstream rule:
  Fluency agreement, re-anchoring, reports and the card layout.
- **The provider Gloss with an entry fallback.** This was rejected because a
  word's anchor would depend on configuration at introduction.
- **A learner-chosen sense.** This was rejected because it makes every
  introduction a philological task and can be wrong.
- **An uncited Gloss for words with no dictionary entry.** This was rejected:
  an uncited Gloss is the invented meaning ADR 0101 guards against.
- **Fluency agreement by sense overlap alone.** This was rejected as too strict
  for LSJ's fine sub-senses.

## Consequences

- **Providerless vocabulary ends at the cutover.** Greek and Latin decks work
  with no provider today (ADR 0101, ADR 0133). After the cutover, Study needs a
  provider for every language.
- **The pass is measured for the first time.** ADR 0101 left its accuracy
  unmeasured. The evidence run is the first measurement, and it gates the
  rollout.
- **Study material gains citations.** A classical Learned meaning stores cited
  sense IDs, entry IDs and a pin, and unpreparable reasons gain
  `no-dictionary-entry`. The migration needs human review.
- **New assessments.** Latin's pass, and Fluency agreement for Greek and Latin,
  need payload assessments that Justin reviews before anything is sent.
- **The citation is a future meaning key.** In these languages, cited senses
  give the stable meaning identity that open question 7 of the Study feature
  would need. Nothing here tracks progress per meaning.

## Related

- [Study](../features/study.md)
- [ADR 0105: The evidence-assisted pass runs whenever its assessment is recorded](0105-greek-evidence-assisted-pass-always-on.md)
- [ADR 0132: Classical Latin from Opera Latina Adnotata](0132-classical-latin-from-opera-latina-adnotata.md)
- [ADR 0137: Idioms are Study items](0137-idioms-are-study-items.md)
