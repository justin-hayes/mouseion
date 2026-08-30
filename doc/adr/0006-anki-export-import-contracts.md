# ADR 0006: Anki export and known-vocabulary import contracts

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

> **Card-output amendment (2026-08-27):** New APKG and TSV notes use a
> recognition presentation with the complete source sentence and a visibly
> bolded target. The deterministic owner-scoped `Identity` hash over the
> representative identity columns is Anki's sort/deduplication field. The
> owner/language/canonical-lemma/UPOS key remains the stable Anki GUID and
> persistence key; the duplicate `SourceSentence` field is gone. `Article` is
> the noun's definite article rendered before the lemma. Morphology is consumed
> to derive that article and is not emitted as verbose JSON card content.

## Context

`mouseion` must exchange vocabulary with the learner's SRS and accept the learner's existing vocabulary. Two interchange contracts are needed, and both must round-trip against the vocabulary identity model from [ADR 0005](0005-vocabulary-identity-normalization-ranking.md): `(language, canonical lemma, UPOS)`, sense-agnostic in v1.

1. **Anki export** — the format in which generated, curated study material ships out to Anki (a "deck").
2. **Known-vocabulary import** — the format in which a learner's already-known words come in, landing directly in the per-user `known` lifecycle state (ADR 0002, ADR 0005).

These were Open Question 5 in `product.md` and consolidated as issue #27. The product spec already commits to "CSV compatible with Anki" with front fields (source sentence / cloze / hint) and back fields (full sentence, translation, target word, lemma, POS, morphology, source document, notes), but leaves the exact format, note type, deduplication key, and input set unspecified.

## Decision

### 1. Anki export — UTF-8 tab-separated (TSV) via Anki's CSV importer

- **Delimiter: tab, not comma.** German sentence text is full of commas. A tab-delimited file matches Anki's CSV importer with no field-escaping pitfalls.
- **Encoding: UTF-8** — required for umlauts and `ß`.

### 2. A custom recognition note type with a stable deduplication key

- **Note type:** a recognition note whose front remains the complete source sentence with the tested target visibly bolded. Its first note field is the deterministic `Identity` value, which Anki uses as the sort/deduplication field. The stable identity-based deduplication key remains the Anki GUID:
  `hash(language | canonical_lemma | upos | user_id)`.
  - This is what Anki uses to deduplicate on re-import. Because the key derives from identity (ADR 0005) plus the owning user (ADR 0002), re-importing the same item is **idempotent** — a second example sentence for the same word updates/replaces rather than spawning a duplicate card.
  - The source sentence is complete and the target form is visibly bolded; it is never clozed or shortened.
- **Fields (order):** `Identity`, `Text`, `Article`, `Lemma`, `POS`, `English`, `EnglishSentence`, `BookTitle`, then tags. `Article` is the noun's definite article rendered before `Lemma`. Morphology is used internally to derive the article and is not emitted as a verbose JSON card field. `SourceSentence` is deliberately absent because `Text` is the one learner-facing German source sentence.
- **Tags:** `Mouseion`, plus language, POS, and source book — so decks can be filtered per language/book.
- **Artifact: TSV only in v1.** `.apkg` (Anki's zip-of-SQLite-and-media container) is heavier to generate and would need the note type embedded; deferred as a later enhancement.

### 3. Known-vocabulary import — a per-language lemma list in v1

- **v1 input:** a UTF-8 text file containing a simple **per-language lemma list** — exactly one lemma per nonblank line. Paste input and POS columns are not accepted; tab-separated rows are reported as rejected. Each accepted lemma is stored with an empty UPOS so it acts as a wildcard across parts of speech.
- Imported words land directly in the per-user `known` state, scoped by user + language.
- Imported lemmas are run through the same **ADR 0005 normalization profile**, so imported words match candidates produced from corpus analysis.
- **Deferred:** Anki `.apkg`/deck import, and CEFR/JLPT/priority-list ingestion. CEFR/JLPT/priority lists are better handled as *priority lists* (feeding #12 / the ranking priority signal) than as known-vocabulary import.

## Alternatives considered

- **Comma-separated CSV.** Rejected: German sentences are comma-heavy; cloze markup interacts poorly with comma splitting. Tab avoids the escaping problem entirely.
- **Stock Anki Cloze note type (first field = cloze sentence).** Rejected: Anki dedupes on the first field, so different example sentences for the same word create duplicate cards, breaking idempotent accumulation.
- **`.apkg` export in v1.** Deferred: heavier generation and note-type embedding for no near-term benefit; TSV into Anki's import dialog is inspectable and sufficient for v1.
- **Import `.apkg`/deck or CEFR/JLPT lists as known vocabulary in v1.** Deferred: lower value than the manual/previous-run lemma list; priority/CEFR lists belong to the priority-list mechanism rather than known-vocab import.

## Consequences

- The export path (issue #22) produces a UTF-8 TSV with the custom note type and identity-based dedup key.
- The import path (issue #16) accepts a per-language lemma list and canonicalizes through the ADR 0005 profile.
- Anki dedup and the product's idempotent-accumulation goal are consistent: re-imports of the same identity update rather than duplicate.
- The custom note type means a one-time note-type definition must be imported into Anki before the deck; the export can ship the note-type definition alongside the TSV for convenience.
- Open Question 5 in `product.md` is resolved; #27 can be closed. Issues #22 and #16 are unblocked.

## Open questions

- Whether to ship a ready-made `.apkg` containing just the note type (for convenience) in a later iteration.
- Whether a future version should export a plain-CSV variant for users who prefer stock note types.

---

## Related

- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md) — the identity model the dedup key derives from.
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — per-user known state.
- [Product specification](product.md) — resolves Open Question 5; updates the Decision Register.
