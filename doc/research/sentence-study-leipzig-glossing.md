# Leipzig-style morphology glossing for sentence study

Research for [#1993](https://github.com/justin-hayes/mouseion/issues/1993) under the Sentence Study map [#1983](https://github.com/justin-hayes/mouseion/issues/1983). Throwaway research branch; no production code changed.

Related research, not repeated here:
- Sentence grammar and dependency views: `doc/research/sentence-study-grammar.md` on branch `research/sentence-study-grammar` ([#1987](https://github.com/justin-hayes/mouseion/issues/1987)).
- Visualization options: `doc/research/sentence-study-visualization.md` on branch `research/sentence-study-visualization` ([#1985](https://github.com/justin-hayes/mouseion/issues/1985)).

## Evidence labels

- **[V]** verified in source, schema, or code read during this research (file and line cited).
- **[R]** stated in an external spec (Leipzig Glossing Rules, UD CoNLL-U, Stanza source).
- **[I]** inference from [V]/[R] facts; not separately checked.
- **[U]** unknown; needs a check against live Stanza output or a decision.

All German, Italian, and Greek sentences below are **fabricated** for illustration. They are not drawn from learner text or any corpus.

## Summary

1. **Two different things are called "Leipzig-style".** A *word-aligned grammatical-feature line* labels each surface word with its features (for example `im` → `in.DEF.DAT.SG`). A *true interlinear morphemic gloss* splits each word into morphemes and glosses each morpheme. The Leipzig rules define the second. The persisted data supports only the first, and only partly (see §4).
2. **The Leipzig rules are a presentation convention, not a validator.** They say glossing "is rarely a complete morphological description" and "its purpose is not to state an analysis" [R]. They also say "glosses are part of the analysis, not part of the data" [R]. A Leipzig line therefore shows what the parser claims. It does not prove the claim.
3. **The local data cannot produce a faithful morphemic gloss today.** Stanza's contraction handling does not give per-morpheme spans for `zum`, `dell'`, or `στο`. The persisted surface is punctuation-cleaned, so the source form is lost unless it is rebuilt from `sentence.text` plus offsets [V §4].
4. **A word-aligned feature line is feasible with a small schema addition.** The morphology JSON is already stored per token [V]. Shared-span contractions need an explicit group key, because grouping by identical offsets is fragile [V §4.3].
5. **The Leipzig rules have no notation for ambiguous features.** Rule 4B's semicolon marks two distinguishable meanings in one word; it does not mark ambiguity. German `dem` (dative masculine or neuter) needs a local decision (§3.2).

## 1. Sources

| Source | What it settles | Label |
|---|---|---|
| Leipzig Glossing Rules, revised 2008 (eva.mpg.de, last change 2015) | Alignment, hyphen correspondence, uppercase labels, one-to-many periods, non-overt brackets, inherent-category parentheses, abbreviation list | [R] |
| UD CoNLL-U format spec | Feature values are comma-joined (`Case=Acc,Dat`); MWT range lines carry only FORM and MISC; MISC `MSeg`/`MGloss` are optional, treebank-specific morphemic glosses | [R] |
| Stanza v1.15.0 source (`doc.py` `set_mwt_expansions`, `mwt_processor.py`, `core.py`) | How sub-word `Word` objects get offsets | [V] (read from source) |
| Local: `nlp/src/mouseion_nlp/producer.py` | `_morphology`, `_clean_surface`, `_map_sentence` span fallback | [V] |
| Local: `migrations/000001_initialize.up.sql` | `corpus_tokens` columns | [V] |
| Local: `sqlc/queries/concordance.sql`, `internal/domain/domain.go`, `internal/webapp/concordance.templ` | What the sentence-study view reads and shows | [V] |
| Local: ADRs 0060, 0067, 0068, 0079, 0080; dictionary-gloss and dependency-parse feature docs | Persistence, card morphology, contextual gloss, LLM alignment | [V] |

### 1.1 Leipzig rules, as they bear on this design

- **Rule 1 (word alignment).** Gloss lines align left, word by word, with the example [R]. This is the rule the feature line already satisfies.
- **Rule 2 (morpheme correspondence).** Hyphens in the example and gloss must match exactly; `=` marks clitics [R]. A feature line has no morpheme segmentation, so Rule 2 cannot be met by it. It is a feature line, not a morphemic gloss.
- **Rule 3 (labels).** Grammatical morphemes take uppercase abbreviations from the standard list [R]. The list includes `SBJ` and `OBJ`, which collide with dependency role names. A rendering that puts `SBJ` on a morpheme and `nsubj` on the same token would confuse learners. [I]
- **Rule 4 (one-to-many).** Several glosses for one element are separated by periods: `aux` → `to.ART.PL` [R]. Optional variants: underscore for an unsegmentable element with no one-word equivalent (4A); semicolon for two distinguishable meanings in an unsegmentable element (4B); colon for a segmentable element whose segmentation is hidden (4C); backslash for morphophonological change (4D); `>` for agent-to-patient person-number (4E) [R].
- **Rule 5 (person and number).** Person and number are not separated by a period: `PRS.1PL`, not `PRS.1.PL` [R].
- **Rule 6 (non-overt).** A gloss element with no overt counterpart goes in square brackets, e.g. `[1SG]`, or is shown with an overt `Ø` [R].
- **Rule 7 (inherent categories).** Inherent, non-overt categories such as gender go in parentheses [R]. A noun's gender is inherent; an article's gender agreement is not, so Rule 7 applies to nouns, not to determiners [I].
- **Rule 8–10 (bipartite, infixes, reduplication).** Not applicable to de/it/el contractions. Not analyzed further.
- **Scope.** The rules "only allow for information about morphemes"; a corpus tagger "may want to add information about larger units such as words or phrases" [R]. A feature line is one of those larger-unit additions.

## 2. Representations and what each explains

| | Word-aligned feature line | True interlinear morphemic gloss | Dependency tree (existing) | Contextual gloss (ADR 0079) |
|---|---|---|---|---|
| Unit | Surface word | Morpheme | Token and head | One target |
| Explains | Grammatical features of each word | Segmentation plus meaning of each part | Syntactic relations | Meaning in this sentence |
| Source today | `morphology` jsonb per token [V] | Not produced (Stanza `morphemes` emission is [U]) | `dependency`, `head` per token [V] | Per-target LLM text [V] |
| Leipzig-compatible | Rules 1, 3, 4, 5, 6, 7 | Rules 1–10 | Not Leipzig | Not Leipzig |
| Sentence-study view today | Not shown; `GetVocabularySentenceStudy` omits `t.morphology` [V] | — | Shown: surface, UPOS, lemma, dependency, head [V] | Card-level |

The learner's "replace or companion" question is undecided and out of scope here. What the table shows is that the feature line and the tree explain different things: the feature line explains the word's form; the tree explains its role.

## 3. Examples (fabricated)

### 3.1 Italian contraction `al` (a + il)

Sentence (fabricated): *Vado al mercato.* "I go to the market."

- Non-overt subject: `vado` carries `Person=1, Number=Sing` but the subject pronoun is not written. Leipzig Rule 6 renders this `[1SG]` [R].
- `al` is one surface token. Stanza splits it into `a` (ADP) and `il` (DET: `Definite=Def, Gender=Masc, Number=Sing`) [I, pending the Stanza check in §5].
- Word-aligned feature line: `vado` → `go-PRS.[1SG]`; `al` → `to.DEF.M.SG`.
  - Rule 5 means no period between tense and person-number inside the verb: `PRS.1SG` is correct, `PRS.1.SG` is not [R]. The non-overt person is bracketed as a whole, so `[1SG]` stays together.
  - Rule 4 periods separate the two glosses for `al` [R].
  - `[1SG]` is bracketed because the element is non-overt [R].
  - `M`, `SG`, `DEF` are abbreviations from the standard list [R].

### 3.2 German contraction `im` (in + dem)

Sentence (fabricated): *Sie wohnt im Haus.* "She lives in the house."

- `im` → `in` (ADP) + `dem` (DET: `Case=Dat, Number=Sing, Gender=Masc,Neut, Definite=Def`).
- Feature line: `im` → `in.DEF.DAT.SG`. Gender is omitted because the Leipzig rules have no notation for ambiguous gender.
  - Writing `DAT.SG.M/N` would be a local convention. Leipzig Rule 4B's semicolon means two distinguishable meanings, not ambiguity [R].
  - **Decision needed:** how to show ambiguous features (omit, use a local `/` form, or show both in the card detail).
- Compare `unser-n` in Rule 4's example: `-n` is `DAT.PL` with no gender in the Leipzig gloss.

### 3.3 Greek contraction `στο` (σε + το)

Sentence (fabricated): *Είμαι στο σπίτι.* "I am at the house."

- `στο` → `σε` (ADP) + `το` (DET: `Case=Acc, Gender=Neut, Number=Sing, Definite=Def`) [I].
- Feature line: `στο` → `at.DEF.ACC.SG.N`.
  - Shown under Rule 4's period separation.
  - The preposition `σε` governs the accusative here; treating `στο` as "at" with an accusative article is a Leipzig-style choice, not a parser fact.
- **[U]** The Greek GDT treebank's Tense on the particle `θα` (future) was not checked. Do not assume a `Tense` value without looking at the treebank.

### 3.4 Italian `dell'` (di + la, elided)

Sentence (fabricated): *Bevo dell'acqua.* "I drink (some) water."

- Surface `dell'` has an apostrophe. Stanza's regex for sub-word offsets does not match it, so both sub-words share the token span (§4.3) [V].
- The `_clean_surface` step removes the apostrophe from sub-word surfaces, so the persisted sub-word surface is `di` or `il` without the elision [V].
- **[U]** The exact tokenization of `dell'` in the Italian Stanza model was not confirmed live.

## 4. Local contract

### 4.1 What is persisted

- `corpus_tokens` has `surface`, `raw_lemma`, `canonical_lemma`, `upos`, `dependency`, `head`, `morphology jsonb`, `start_offset`, `end_offset`. There is no misc column and no group column [V, `migrations/000001_initialize.up.sql`].
- Offsets are rune-based, and jobs relocate them per chunk [V, `internal/analysis/jobs.go`].
- Morphology is stored as a JSON map of UD feature names to values, copied from the analyzer token [V, `internal/analysis/jobs.go:1634`].

### 4.2 What the sentence-study view reads and shows

- `GetVocabularySentenceStudy` (`sqlc/queries/concordance.sql`) does not select `t.morphology` [V].
- `domain.SentenceStudyToken` has no morphology or offset fields [V, `internal/domain/domain.go`].
- `concordance.templ` (lines ~135–151) renders surface, UPOS, lemma, dependency, and head only [V].

So morphology is persisted but not shown in the sentence study. Adding it needs a query change, a domain field, and a template change; none is a schema change.

### 4.3 Shared-span contractions

- Stanza creates MWT sub-words as `Word(sentence, {ID, TEXT})` with **no offsets** [V, `doc.py` `set_mwt_expansions`].
- Sub-word spans are set only when the component words, joined by `\s*`, match the token text by regex `^(w1)\s*(w2)$` [V, `doc.py`].
- For `zum` (zu + dem), `dell'`, and `στο`, the regex fails, so the producer falls back to the shared token span [V, `producer.py` `_map_sentence`; checked against a Python mirror of the regex].
- Consequence: two sub-words with the same span cannot be told apart by span alone. Grouping by identical span is fragile, because an unrelated pair of tokens could share a span after re-analysis.
- **Recommendation for a future design (not decided):** store an explicit group key (token ordinal of the MWT range plus a sub-word index) in `corpus_tokens.morphology` or a new column. Store that key rather than inferring grouping from spans.
- The parent MWT range line has FORM and MISC only in UD [R]. Any group key must therefore be the producer's own, not a UD field.

### 4.4 Surface cleaning

- `_clean_surface` strips punctuation, and sub-word surfaces lose apostrophes [V].
- Recovering the source form needs `sentence.text` plus the token offsets. That is possible for the parent token; for sub-words it is only an inference from the shared span [I].

## 5. Unknowns and checks needed before a design decision

| # | Unknown | Why it matters | How to settle it |
|---|---|---|---|
| U1 | Does Stanza emit `morphemes` or MSeg/MGloss for these models? | Decides whether a true morphemic gloss is possible without new analysis | Run a short fabricated sentence through the de/it/el models in a scratch venv |
| U2 | Italian `dell'` tokenization in the Stanza model | Determines sub-word text and group key | Same probe as U1 |
| U3 | Greek GDT `θα` Tense value | Affects whether Tense is shown on the particle | Check the treebank, not the parser |
| U4 | Ambiguous features (`dem` gender) | No Leipzig notation | Product decision (see §3.2) |
| U5 | Whether `SBJ`/`OBJ` abbreviations should be used when dependency roles are shown | Label collision | Product/UI decision |
| U6 | Whether to keep the feature line and the tree separate or merge them | The learner's "replace or companion" question | Out of scope; not decided here |

## 6. Terminology hazard in the codebase

"Morphology" means two things here:

- **Analyzer features** (UD feature map): stored per token in `corpus_tokens.morphology` [V]. Copied into the candidate map through `enrichment/local.go` `StanzaMorphology` [V] and `analysis/jobs.go` [V].
- **Dictionary-derived** gender, article, plural, IPA, and principal parts: merged into the same JSON by `internal/cardexport/cardexport.go` (around line 1170) [V].

These are different provenance sources. A feature line for sentence study should label which source each feature comes from, or the card and the sentence view will disagree silently.

## 7. Provenance

Each feature in a glossing line should carry its source: analyzer (Stanza), dictionary, or LLM. The current design already separates these for cards [V, ADR 0067]. The sentence view has no provenance today. Contextual gloss is per target, one per card (ADR 0079); sentence translation has no per-token alignment (ADR 0080 gives LLM alignment for the target only) [V]. A glossing line for an entire sentence therefore has no LLM-sourced gloss to draw on.

## 8. Recommendation (findings only)

- Do not claim Leipzig compliance for a feature line. Call it a **word-aligned feature line** and state which Leipzig rules it follows (1, 3, 4, 5, 6, 7).
- Treat a true morphemic gloss as a separate, later option that depends on U1 and U2.
- Add an explicit group key before any shared-span rendering. Do not infer groups from identical offsets.
- Decide ambiguous-feature rendering (U4) and label collisions (U5) before implementation.

## Appendix: Verification notes

- Stanza v1.15.0 source copies are in `~/.cache/scratch/sentence-study-leipzig/stanza-src/` (scratch, to be deleted at the end of the task).
- The Leipzig rules page was re-fetched during this write-up; quotations above are from that fetch.
- No Stanza models were run and no provider or API calls were made.
