# ADR 0139: Greek Study looks up every LSJ homograph

Status: **Accepted; implementation pending (Study cutover prerequisite)** ·
Date: 2026-11-26 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Specified in
[#2759](https://github.com/justin-hayes/mouseion/issues/2759) and built in
[#2773](https://github.com/justin-hayes/mouseion/issues/2773). It amends:

- [ADR 0138](0138-study-for-source-evidence-languages.md), which leaves each
  language its own lookup rule and keeps Greek's existing pass contract for
  Study;
- [ADR 0133](0133-per-language-card-policy.md), whose Greek policy takes exactly
  one candidate entry for every card.

## Context

Greek's lookup matches a lemma only when exactly one LSJ entry competes for it.
A lemma with two or three entries matches none, and Study records the word as
`no-dictionary-entry`. The Greek evidence run for
[#2767](https://github.com/justin-hayes/mouseion/issues/2767) recorded ἔχω, ἄν,
λέγω and πᾶς that way. They are among the most frequent words in the Corpus, so
Greek Study cannot be switched on while they are skipped.

Latin already looks up every Lewis & Short homograph in source order, and its
pass contract carries each sense's entry (ADR 0138 §1).

## Decision

### 1. Study looks up every competing LSJ entry

- **Study lookup.** The card policy gains a Study lookup rule. For Greek it is
  every competing LSJ entry, in source order. The entries are those the exactly-one
  lookup counts: a phrase or redirect sub-entry never joins, and a capitalised
  proper name does not join a lowercase lemma.
- **Prepared deck unchanged.** The deck keeps its exactly-one lookup, the existing
  Greek pass contract and its recorded assessment
  ([#2171](https://github.com/justin-hayes/mouseion/issues/2171)). The deck output
  is byte-identical. [ADR 0136](0136-study-replaces-known-vocabulary-and-prepared-deck.md)
  retires the deck at the cutover.
- **Keyed to the policy.** Study branches on the card policy values, never on
  `grc`.

### 2. A Greek pass contract that names the entry

- **Contract.** The card policy names a Study pass contract, separate from the
  deck's. Greek's Study contract names LSJ and has its own payload kind and prompt
  version. Each candidate sense carries a short entry handle beside its short
  evidence handle, as in Latin's contract.
- **Handles.** Only short per-request handles are sent. No native LSJ entry or
  sense ID is sent. A cited handle decodes to one entry and one sense; an unknown
  or repeated handle is rejected.
- **Fails closed.** No assessment of the new payload is recorded here. The gate
  refuses it, and Greek Study and Fluency send nothing for it, until it is
  recorded ([#2774](https://github.com/justin-hayes/mouseion/issues/2774)).

### 3. Citations, card and Fluency

- **Citations.** A Gloss cites senses of one or more entries, and the stored
  citations name each sense with its own entry, as for Latin.
- **Card.** The Dictionary section shows each cited entry separately. The index
  carries no LSJ homograph label, so an entry is labelled by its position when
  several entries stack, and unlabelled when there is one.
- **Fluency.** The entry-mismatch shortcut of ADR 0138 §4 separates Greek
  homographs: a context citing only λέγω "lay" disagrees with a Learned meaning
  citing λέγω "say", with no agreement call.

### 4. The switch stays off

Greek's Study switch stays off. Turning it on remains a reviewed change after the
evidence run and the assessments (ADR 0138 §6).

## Consequences

- Greek homographs become Study material. An uncited or abstaining answer is
  still an omission for its meaning.
- A new payload kind exists for Greek Study. Until it is recorded, Greek Study
  waits as it does for any unrecorded assessment.
- The Greek Fluency agreement variant (ADR 0138 §4) is unchanged; it reads the
  entries the Gloss cites.

## Related

- [ADR 0138: Study serves source-evidence languages with a cited contextual Gloss](0138-study-for-source-evidence-languages.md)
- [ADR 0133: Per-language card policy](0133-per-language-card-policy.md)
- [ADR 0104: Compact LSJ definition outline](0104-compact-lsj-definition-outline.md)
