# ADR 0005: Vocabulary identity, normalization, and initial ranking defaults

Status: **Partially superseded by [ADR 0017](0017-coverage-based-selection.md)** · Date: 2026-08-21 · Author: Justin + Hermes

> **Supersession notice:** ADR 0017 replaces this ADR's selection, ranking, and learner-review decisions with fixed 97% coverage selection and one-button deck generation. The vocabulary identity and normalization decisions remain authoritative.

## Context

For `mouseion` to know whether two occurrences are "the same thing to learn," it must settle three coupled questions. They are coupled because *what counts as the same word* (identity) determines *how occurrences aggregate* (normalization), which determines *what gets selected and ranked* (selection/ranking).

These were carried as Open Questions 2, 3, and 7 in `product.md`, and consolidated as issue #26.

1. **Vocabulary identity:** what uniquely identifies a learning item, and how do homographs, senses, and inflected forms map onto it?
2. **Normalization profiles:** how are surface forms, raw lemmas, and a canonical lemma related, and which German spelling-reform rules apply in v1?
3. **Selection & ranking:** which words become candidates, and in what order are they surfaced for study — particularly how corpus frequency, global (DWDS) frequency, priority membership, cross-text recurrence, POS, and proper-noun exclusion interact.
4. **Lifecycle states:** which states (`candidate`, `accepted`, `generated`, `ignored`, `known`) are required, and what reset/reopen behavior is permitted.

Two drivers shape the answers:

- **German is the initial language** (v1 German-first), but the model must be **pluggable** to other languages later (Italian, Greek, etc.).
- **Multi-user with per-user state** (ADR 0002): each user's known vocabulary, accepted/generated/ignored state, and curated choices are their own. Global reference data (DWDS frequency) is language-scoped and admin-managed.

## Decision

### 1. Vocabulary identity — `(language, canonical lemma, POS)`, sense-agnostic in v1

A learning item is uniquely identified by the tuple **(language, canonical lemma, part-of-speech)**.

- **Canonical lemma** is the normalized form used for matching, ranking, and deduplication (see §2).
- **POS** is the coarse UPOS tag (e.g. noun vs. verb) from the language analyzer. It separates homographs that differ in grammatical class (e.g. the same spelling as a noun vs. a verb become distinct items).
- **Senses are deliberately NOT part of identity in v1.** Same lemma + same POS with different senses (e.g. *die Bank* = bench vs. bank) is treated as **one item** that can carry **multiple example sentences**. The learner chooses the representative sentence during review, which is how they disambiguate the sense they intend to study. This defers full word-sense disambiguation (WSD) from a hard NLP problem in v1 to a review-time preference, with a clear later enhancement path.

### 2. Normalization — conservative, deterministic, versioned; a derived field

Preserve the complete source sentence, source offsets, and raw analyzer lemma.
`Token.Surface` is the derived lexical form used for selection and matching: it
removes surrounding Unicode punctuation/symbol decoration while retaining
lexical apostrophes and internal punctuation. A derived **canonical lemma** is
computed from the raw lemma by a **normalization profile**, and the profile name
and version that produced it are stored alongside. This paragraph is amended by
the accepted [Anki card output milestone](../features/anki-card-output.md).

- **v1 ships one profile:** *German standard orthography (post-1996 reform)* — version 4 removes analyzer-attached Unicode punctuation/symbols from lemma edges, lowercases without Unicode case folding, preserving modern `ß` (for example `Straße` → `straße`), selects the first usable analyzer lemma alternative, and applies explicit historical spelling equivalences such as `daß` → `dass`. Conservative only: no blanket `ß` → `ss` replacement, regional/dialectal forms, or merging of genuinely distinct lexemes such as `Maße` and `Masse`.
- **Deterministic and pure:** the profile is a pure function, so it is unit-testable and reproducible.
- **Versioned:** a profile update re-derives canonical lemmas and lexical surfaces for **new** analysis runs but leaves already-persisted items stable unless an explicit re-normalize migration is run. Rules never mutate source sentence text, source offsets, or the raw lemma.

### 3. Selection & ranking — global (DWDS) frequency as the primary signal

Candidate **selection** (a word becomes a candidate if it meets *at least one* of):

- appears ≥ 2 times in the corpus, **or**
- belongs to an active priority list (including learner-defined lists), **or**
- is above a global-frequency percentile cutoff (v1 default: top 5%) from the admin-loaded DWDS dataset.

Candidates are filtered before ranking: exclude **known** and **ignored** vocabulary and previously **generated** items (per ADR 0002 per-user state). Default to **content words** (noun, verb, adjective, adverb); exclude **proper nouns** (UPOS `PROPN`/NER) and **function words**, both togglable.

**Ranking score** — a deterministic weighted blend weighted toward *global* frequency:

```
score = 0.6 · pct(global_freq) + 0.3 · pct(in_corpus_freq) + 0.1 · priority + 0.05 · (cross_text − 1)
```

- `pct(global_freq)` — the word's percentile rank in the admin-loaded DWDS dataset (0 if absent/below cutoff).
- `pct(in_corpus_freq)` — normalized frequency within this corpus.
- `priority` — 1 if on an active priority list, else 0.
- `cross_text` — number of books the word appears in (capped), rewarding recurrence.

Rationale for **global-first weighting:** a word that appears once in this book but is top-1000 in German is more valuable to study than a word that appears 10× in this book but is rare in the language (likely niche/archaic). Global frequency is the better predictor of durable, transferable value. This matters most for the author when returning to a language after a hiatus (e.g. Italian) or picking up a new language from scratch — the tool re-surfaces the core vocabulary even when the immediate corpus is thin. For German, where the author already has a strong sense of the core vocabulary, the effect is less noticeable, which is expected.

In-corpus frequency is still weighted (0.3) because it reflects the immediate reading experience — a word the learner will encounter again soon is worth reinforcing.

### 4. Lifecycle states — five states, fully reversible

- **`candidate`** → **`accepted`** → **`generated`** is the forward path (`generated` = accepted *and* a card was produced).
- **`ignored`** — "I don't want to study this right now"; suppresses candidacy but is not "known."
- **`known`** — pre-seeded from known-vocabulary imports/previous runs; represents "I know this"; suppresses candidacy.
- **Learner control is a core goal, so every state is reversible:** `accepted`→`candidate`, `generated`→`candidate`, `ignored`→`candidate`. Nothing is one-way. `known` is sticky but can be unmarked.

## Alternatives considered

- **Word-sense identity in v1 (e.g. `(language, lemma, POS, sense)`).** Rejected for v1: WSD requires sense inventories and disambiguation that is heavy, language-specific, and error-prone. Treating senses as review-time preferences on a single item delivers most of the benefit at a fraction of the cost, with a clean later enhancement.
- **In-corpus-frequency-first ranking.** Rejected: underweights durable value and fails the "high-frequency in the language but rare in this book" case that is central to the product's purpose.
- **No profile versioning (recompute canonical lemmas eagerly).** Rejected: would silently rewrite persisted items' identity across runs. Versioning keeps the corpus stable and makes migration explicit.
- **Ranking as a configurable formula from day one.** Deferred: v1 ships one deterministic default (above) so behavior is inspectable and testable; configurability can be added after real use.

## Consequences

- Persistence keys learning items on `(language, canonical_lemma, upos)`; per-user state (known/accepted/generated/ignored/curated sentences) attaches to that identity.
- The normalized-corpus schema (ADR 0001 Protobuf) carries surface form, raw lemma, canonical lemma, profile name/version, UPOS, and source-location metadata.
- The candidate-selection and ranking implementation (issues #17, #18) follow the defaults above; weights and cutoff are **explicitly provisional and tunable**, and each score component must be surfaced for inspection in the review UI.
- German v1 profile is a pure, versioned function; other languages add their own profiles behind the same interface (ADR 0001 pluggable NLP boundary).
- Open Questions 2, 3, and 7 in `product.md` are resolved; #26 can be closed.

## Open questions

- Whether real use favors **configurable ranking weights** sooner than later (deferred to after initial corpora are processed).
- Whether a **language frequency signal** beyond DWDS (or a newer DWDS release) should be a priority-list input as well as a ranking input (touched by issue #12 / #30).

---

## Related

- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — pluggable NLP boundary; Protobuf contract.
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — per-user state and global frequency data.
- [Product specification](../product.md) — resolves Open Questions 2, 3, 7; updates the Decision Register.
