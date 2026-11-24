# ADR 0126: Analysis updates carry decisions onto identical tokens

Status: **Accepted; implemented** · Date: 2026-11-24 · Author: Justin + OpenCode

Amends [ADR 0107](0107-analysis-pipeline-identity-and-analysis-updates.md)'s
frozen boundaries ("never remaps Lemma corrections or Occurrence exclusions")
and its confirmation copy. [ADR 0116](0116-classical-catalog.md)'s Classical
Analysis updates inherit the amendment. Does not change
[ADR 0103](0103-source-structure-and-poetry-evidence.md)'s Source structure
updates. Decided by Justin in
[Grilling: Analysis update copy and carry-over for the syntax interpretation version](https://github.com/justin-hayes/mouseion/issues/2614#issuecomment-7099181118),
under [Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597).

## Context

Existing Greek Books gain syntax only through a learner-confirmed Analysis
update to the new OGA interpretation version
([ADR 0122](0122-ancient-greek-native-agdt-syntax.md)). Under ADR 0107, Lemma
corrections and Occurrence exclusions never carry over to the new run. A learner
with many corrections would pay for constructions with their own work, although
the new version keeps every printed token's span, surface and source lemma.

Facts:

- Lemma corrections and Occurrence exclusions are keyed per run on
  `(source_document_id, start_offset, end_offset)`
  (`migrations/000023_occurrence_lemma_corrections.up.sql`).
- Their writes already compare the expected surface, lemma and UPOS
  (`sqlc/queries/selection.sql`).
- No code copies decisions between runs.
- Greek ordinals and offsets come from `reconstruct`, which skips unprinted
  nodes, so storing them changes no span, provided `reconstruct` and the
  sentence-inclusion rules stay unchanged (a constraint ADR 0122 records).

## Decision

1. **Copy by exact token identity.** When an Analysis update publishes, each
   Lemma correction and Occurrence exclusion on the run being replaced is copied
   onto the new run's token when it matches exactly. Unmatched decisions are not
   copied. This copies decisions onto tokens shown to be identical; it does not
   remap them.
2. **Scope: every Analysis update.** The rule covers ADR 0107 NLP updates (such
   as a French normalization change) and ADR 0116 Classical updates. It does not
   cover ADR 0103 Source structure updates. The match is source-neutral.
3. **The match.** Same source document, same offset span, same surface and same
   **source** lemma, for corrections and exclusions alike. UPOS and morphology
   are not compared, because an update may change morphology deliberately. The
   match is therefore looser than the write-time check.
4. **Absorbed corrections.** When the new source lemma equals the lemma the
   learner corrected to, the correction is not copied and counts as carried
   over.
5. **When.** Inside the transaction that publishes the new run. It reads the
   decisions on the run published at that moment, including decisions made
   after the learner confirmed. It is a copy, not a move: the earlier run keeps
   its own decisions as frozen evidence for the decks and Readings that pin it.
6. **Copy.**
   - The notice is unchanged: "A newer analysis is available."
   - Each Classical interpretation version may carry an optional
     Mouseion-authored **change note**, held as data next to the definition and
     looked up at render; it is not evidence. A version with no note shows no
     line. The syntax version's note is about "Adds sentence constructions in
     Sentence study."
   - The per-Book confirmation reads: the change note (if any); "The published
     analysis stays in effect until the update publishes."; "Your N confirmed
     lemma corrections and M occurrence exclusions carry over wherever the word
     is unchanged in the new analysis."; "Prepared decks and earlier Readings
     keep the evidence they froze." Singular and zero forms follow the existing
     ones.
   - The Work level reads: "Together, the Books to be updated have X confirmed
     lemma corrections and Y occurrence exclusions, which carry over wherever
     the word is unchanged in their new analyses."
7. **Report.** The update's `/jobs` result line reads "N of M decisions carried
   over". There is no notice on the Book and no new UI pattern.

ADR 0107's other frozen boundaries stand: an Analysis update never rewrites
Reading vocabulary, Reserved or Known facts, completed history, current-reading
snapshots, corpora, earlier runs' decisions or prepared decks.

## Alternatives considered

- **Keep "never remap" unchanged:** rejected; the learner would lose their work
  to gain constructions.
- **All-or-nothing carry-over behind a hand-set "token-preserving"
  declaration on the version:** rejected; it is the hand-bump failure mode ADR
  0107 warns about, and a wrong declaration would silently misapply decisions.
- **Matching on UPOS and morphology too:** rejected; the syntax version changes
  morphology on purpose, so nearly every decision on a verb would fail to match.
- **Extending the rule to Source structure updates:** rejected; re-extraction
  can move offsets, so span identity is not evidence of the same word.

## Consequences

- Learners keep their corrections and exclusions across updates wherever the
  word is unchanged, and see how many carried over.
- A changed lemma or span drops the decision on that token. The earlier run
  still holds it.
- The confirmation copy promises less than a full carry-over and is accurate
  whatever the update changes.
- The publish transaction gains a copy step and a count.

**What would reopen this:** an interpretation or normalization change that moves
offsets for most tokens. Decisions would then mostly fail to carry over, and the
confirmation copy would overstate what is kept.

## Related

- [ADR 0107](0107-analysis-pipeline-identity-and-analysis-updates.md): the
  Analysis update this amends.
- [ADR 0116](0116-classical-catalog.md): Classical Analysis updates.
- [ADR 0122](0122-ancient-greek-native-agdt-syntax.md): the interpretation
  version that motivated the change, and its span constraint.
- [Learner review of analyzer lemmas](../features/lemma-review-and-correction.md):
  how decisions are made.
