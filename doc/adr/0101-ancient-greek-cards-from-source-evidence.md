# ADR 0101: Ancient Greek cards from source evidence, with an optional evidence-assisted provider pass

Status: **Accepted** · Date: 2026-10-17 · Author: Justin + opencode

Tracked by [#2131](https://github.com/justin-hayes/mouseion/issues/2131), the
decision ticket of the map
[#2127](https://github.com/justin-hayes/mouseion/issues/2127). Amends, for
Ancient Greek (`grc`) only, the LLM requirement of
[ADR 0079](0079-contextual-glosses-require-llm.md) and the LLM-proposed
alignment of [ADR 0080](0080-llm-proposed-english-target-alignment.md), and the Cards
section of the Ancient Greek MVP spec
[#2098](https://github.com/justin-hayes/mouseion/issues/2098) (user stories
29–31, and its Wiktionary meaning-evidence and configured-provider bullets).
[ADR 0099](0099-ancient-greek-imported-analyses.md) stays in force: it decides
the import, not cards. Modern languages are unchanged.

## Context

Greek card preparation sends each card's sentence and Wiktionary meaning
evidence to the configured provider, which proposes the contextual Gloss, the
sentence translation and the English target alignment. That path depends on
provider reliability and a recorded personal-use payload assessment. It can
also invent meanings. Its recorded assessment (#2110) went stale when the
payload changed (#2125), so real Greek preparation currently fails closed.

The map investigated whether public-domain English and an open lexicon could
replace the provider, using *Anabasis* Book 1:

- [Translation research (#2128)](https://github.com/justin-hayes/mouseion/issues/2128#issuecomment-6231714384).
  Brownson's 1921–22 translation, in the Perseus `perseus-eng2` digitisation
  (CC BY-SA 4.0), matches all 205 Book 1 section keys of the pinned OGA source.
  It is section-aligned, not sentence- or word-aligned, and it translates some
  bracketed material that OGA lacks. The pretrained Ugarit aligner's training
  inputs are not confirmed non-NC.
- [Lexicon research (#2133)](https://github.com/justin-hayes/mouseion/issues/2133#issuecomment-6232765398).
  The Perseus LSJ (`PerseusDL/lexica`) has native entry and sense IDs and
  passage citations, and its component README grants CC BY-SA 4.0.
- [Alignment prototype (#2129)](https://github.com/justin-hayes/mouseion/issues/2129#issuecomment-6232345453).
  On a human-confirmed sample, every tested cheap baseline got at best 5/8
  exact sentence spans. Dictionary anchors gave 8/24 complete target
  highlights. None is reliable enough to run unattended. Accepted rules: an
  uncertain target link stays unhighlighted; an uncertain sentence extent is
  shown only as labelled wider-passage context; English is withheld when even
  that is unreliable.
- [Word-sense prototype (#2130)](https://github.com/justin-hayes/mouseion/issues/2130#issuecomment-6236927327).
  LSJ beat Kaikki under every strategy. Choosing by morphology and source
  order was wrong about one time in three. A contextual Gloss was correct
  (7/7) only when a human-approved alignment link overlapped exactly one LSJ
  sense. Otherwise the card should abstain and show the entry.

### Limits of the evidence

- The sample is 8 sentences and 24 target occurrences, chosen on purpose and
  biased toward the agent's proposals. Justin confirmed the agent's proposed
  labels. This was not independent or expert annotation. There was no held-out
  test set.
- Unique-projection's 7/7 is an upper bound, because its links were
  human-approved. It is too small to calibrate a precision threshold.
- Only cheap baselines were tested. Full fast_align/eflomal, stronger non-LLM
  methods and larger parallel data were not; this ADR does not conclude that
  they cannot work.
- Book 1 is the only validation case. Nothing here is evidence for other
  books, authors or languages.
- The evidence-assisted provider pass was not measured. Any improvement it
  brings is a hypothesis.

## Decision

**Unattended replacement is not supported.** The evidence does not show that a
contextual Gloss, sentence translation or target alignment can be produced
without the LLM and without per-card human review. It does support a card
that is built only from source evidence and abstains where the evidence
cannot decide. That card becomes the Greek default. The LLM is an optional
layer on top of it.

**The default Greek card makes no provider call.** Its back shows:

- **Passage context**: Brownson's English for every cited section the
  representative sentence touches, so a sentence crossing 1.2.17–18 gets both
  sections. Perseus notes are removed and the text is otherwise shown as is,
  including bracketed material. It carries a label such as "Passage context:
  Brownson (1922), Xen. Anab. 1.2.17–18. Not a translation of this sentence"
  and highlights nothing. If any touched section has no matching translation
  key, the card has no English. Every Book 1 sentence has a match.
- **Dictionary entry display**: the matched LSJ entry with its full sense
  hierarchy in source order and LSJ's numbering. Each sense shows its English
  glosses, and only citations of Xenophon are kept. Senses are never reranked
  or truncated, because picking the first sense was wrong about a third of the
  time. An OGA identity is matched to an LSJ entry only when the match is
  unambiguous. A `morpheus:N` discriminator is never equated with LSJ's
  numbering. An unresolved homograph or a missing entry gives no entry. It is
  never a guess.
- **No contextual Gloss.** Unique-projection needs human-approved links, and
  production has no step for approving them. An in-app approval step would be
  a separate future decision.

**LSJ replaces Wiktionary for Greek.** It is the only source of Greek meaning
evidence, both on the default card and in what the provider pass sends.
Wiktionary meaning evidence is unchanged for modern languages.

**Sources are pinned and frozen.**

- LSJ: `PerseusDL/lexica@56061ca127f4a2844980baffc5f2b6d1332897b3`, derived
  into the dictionary index.
- Brownson: `canonical-greekLit@0189b9c8d8d1f148133f2f0af309b4ee90ea4332`,
  `data/tlg0032/tlg006/tlg0032.tlg006.perseus-eng2.xml`, SHA-256
  `f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7`. It is an
  operator-supplied file, imported by the existing Greek operator command and
  checked by size and digest like the OGA file. It is stored as source
  material keyed by citation.
- The passage text, the LSJ entry ID and both pins are frozen on the prepared
  deck item, so a prepared deck never changes. The real files never enter the
  repository or CI, which use synthetic fixtures.

**Export notices and scope.** A Greek deck's notices name OGA, `oga_src`,
Perseus/canonical-greekLit, Brownson's translation in the Perseus digitisation,
and LSJ in the Perseus digitisation, each with its pin and CC BY-SA 4.0
attribution. Greek decks are for personal use only, and public release stays
out of scope (#2054, #2056). The review of LSJ's territorial rights and extra
licence wording is needed before any redistribution, not before personal use.

**Evidence-assisted provider pass (opt-in).** The deck specification records
whether a Greek preparation uses this pass. It is off by default and offered
only while a matching provider payload assessment is recorded.

- The provider receives the representative sentence, its passage context, and
  the LSJ senses with their native IDs.
- It proposes a contextual Gloss, a sentence translation and an English target
  alignment. The card labels each one as provider-proposed.
- The pass can only add to the default card. A failed or abstaining provider
  answer leaves that item as a default card. Greek decks therefore have
  sentence-quality omissions only, never meaning or provider omissions.
- The pass can also produce an **LSJ sense summary**: a few short lines
  condensing one LSJ entry, written without the sentence and cached per LSJ
  pin, entry ID and prompt version. Its payload is only the LSJ entry. Every
  line must cite sense IDs that exist in that entry. Otherwise the summary is
  discarded. It is shown above the full entry and never replaces it, labelled
  "Summary of LSJ, provider-written". It never chooses a sense for the
  sentence.

**The current Greek provider path is retired.** Its stale assessment is not
re-recorded, and it is neither kept as a fallback nor run unchanged. The only
Greek payloads that may ever be assessed are those of the evidence-assisted
pass and the sense summary. Justin reviews and records each one before it
sends anything. Until then, Greek decks contain default cards only.

## Consequences

- Greek cards work with the provider stopped. Provider flakiness and payload
  review affect only the opt-in pass.
- The default card is less convenient than an LLM card: no short contextual
  Gloss, a passage instead of a sentence translation, and possibly long LSJ
  entries. This is the accepted price of not asserting unverified meanings.
- No transition is needed. Real Greek preparation has been blocked by the
  stale assessment. A Greek deck prepared before that would be re-prepared
  through the existing roll-forward ([ADR 0076](0076-reprepare-ready-deck.md)).
- The remaining local Anki rendering check of #2110 waits for the default card
  so that it covers the Brownson and LSJ notices.
- This ADR does not cover modern languages or other Greek books, and it
  approves no provider payload.

## Related

- [ADR 0079](0079-contextual-glosses-require-llm.md) and
  [ADR 0080](0080-llm-proposed-english-target-alignment.md): amended for `grc` as
  described above.
- [ADR 0099](0099-ancient-greek-imported-analyses.md): the import that supplies
  the Greek sentences and identities that cards use.
- [ADR 0100](0100-retryable-provider-omissions-preparation.md): Greek decks do
  not produce provider omissions under this decision.
