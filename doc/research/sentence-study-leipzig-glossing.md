# Leipzig-style morphology glossing for sentence study

Research for [#1993](https://github.com/justin-hayes/mouseion/issues/1993) under the Sentence Study map [#1983](https://github.com/justin-hayes/mouseion/issues/1983). Throwaway research branch; no production code changed.

**Revision 2.** This revision corrects revision 1 (commit `9af7acdf`). §0 lists each correction. Where revision 1 and this document disagree, this document governs.

Related research, not repeated here (on their own branches):
- Sentence grammar and dependency views: `doc/research/sentence-study-grammar.md`, branch `research/sentence-study-grammar` ([#1987](https://github.com/justin-hayes/mouseion/issues/1987)).
- Visualization options: `doc/research/sentence-study-visualization.md`, branch `research/sentence-study-visualization` ([#1985](https://github.com/justin-hayes/mouseion/issues/1985)).

## Evidence labels

- **[V]** verified in a primary source or in code, with the location cited.
- **[R]** a statement made by an external specification, quoted and cited.
- **[I]** an inference from the cited facts. Not separately tested.
- **[U]** unknown. Settling it needs a decision, a test, or a model run, which this research did not do.

All German, Italian, and Greek sentences are **fabricated** for illustration. Feature sets in §3 are illustrative and are not Stanza output.

Retrieval date for every external source: **2026-10-13**.

## 0. Corrections from revision 1

| # | Revision 1 claim | Status | Correction and evidence |
|---|---|---|---|
| C1 | Stanza `morphemes` emission is unknown and needs a live probe. | Corrected | Stanza's public Word fields do not include morphemes [V, §5]. In v1.15.0 `doc.py`, `MORPHEMES = 'morphemes'` appears only as an output-field name [V]. The local protobuf `Token` has no morpheme field [V]. The local contract therefore cannot carry per-morpheme alignments, whatever Stanza does. No probe is needed for the decision. |
| C2 | A word-aligned feature line needs a schema addition. | Wrong | Morphology is already persisted per token in `corpus_tokens.morphology` [V]. Showing it needs query, domain, and template changes only [V, §4.2]. No schema change is required for the feature line. |
| C3 | An explicit group key is required for shared-span contractions. | Overstated | A group key is one possible design, not a verified prerequisite (§4.3). |
| C4 | Grouping by identical span fails "after re-analysis" because revisions could mix. | Wrong | The study query selects a single analysis run [V: `sqlc/queries/concordance.sql`, `current_analysis_identity` and `analysis_run` filter]. Mixing revisions is not a case the query produces. The real within-run hazard is the offset-keyed correction join (§4.5). |
| C5 | Apostrophes are dropped from MWT sub-words; `dell'` becomes `di`. | Wrong | `_clean_surface` removes only edge punctuation and symbols and explicitly keeps apostrophes [V: `nlp/src/mouseion_nlp/producer.py` lines 150–~160, comment "Apostrophes are lexical in elided forms"]. A mirror of that function gives `dell'` → `dell'` [V, run with `python3 -I`]. |
| C6 | Stanza's regex fails only for `zum`, `dell'`, and `στο`. | Overstated | The regex cannot match any fused contraction whose surface is not its components joined by optional whitespace [V, mirror run, §4.3]. This is a general property, not a three-case list. |
| C7 | The span fallback is part of Stanza. | Corrected | Stanza sets no word offsets when the match fails [V: `doc.py` v1.15.0 lines 728–737]. The fallback to the token span is in our producer [V: `producer.py` lines 249–254]. |
| C8 | Rule 5 concerns tense followed by person-number, so `PRS.1SG` has no period. | Wrong | Rule 5 says "Person and number are not separated by a period when they occur in this order" [R]. It says nothing about tense. Its own example is `go-PRS.1PL`, which has a period between tense and person-number [R]. |
| C9 | A bracketed `[1SG]` is justified for Italian `vado` because the subject is not written. | Unsupported | Rule 6 applies when the gloss contains an element that "does not correspond to an overt element in the example" [R]. Whether a verb ending expresses person is an analysis claim. A missing subject pronoun is not evidence of a zero exponent. Removed. |
| C10 | `go-PRS.[1SG]` is a features-only line. | Wrong | It adds segmentation and a lexical meaning (`go`). All examples are now features-only (§3). |
| C11 | Leipzig-compatible for word-level labels is forbidden, or the labels are "Leipzig rules 1, 3–7". | Overstated | Leipzig's rules cover morphemes; a word-level label is outside that stated scope [R]. But the rules include conventions for unsegmented elements (4A, 4B) [R]. A word-level feature line is therefore allowed as a labeled convention, not a Leipzig morphemic gloss (§7). |
| C12 | Rule 7 applies to nouns but not determiners. | Unsupported | Rule 7 says "Inherent, non-overt categories such as gender may be indicated in the gloss" [R]. The noun/determiner distinction is my inference, not the rules'. Removed. |
| C13 | `SBJ`/`OBJ` collide with dependency role names. | Overstated | The Leipzig list contains `SBJ` and `OBJ` [R]. UD relations are `nsubj` and `obj` [R]. The overlap is conceptual [I], not a literal collision. |
| C14 | `θα` carries a Tense question in the Greek GDT treebank. | Removed | Unsupported as written. Not carried forward. |
| C15 | UD MWT range lines carry only FORM and MISC. | Corrected | UD: range tokens "have an underscore in all the remaining fields except MISC" [R], with one exception for `Typo=Yes` in FEATS [R]. |

## 1. Sources

### 1.1 Leipzig Glossing Rules

- URL: https://www.eva.mpg.de/lingua/resources/glossing-rules.php
- Version on the page: "revised version of February 2008"; "Leipzig, last change: May 31, 2015" [R].
- Authors and institutions as given on the page: Comrie and Haspelmath (MPI-EVA), Bickel (Leipzig) [R].

Quotations used (exact):

- Scope: "the rules here only allow for information about morphemes" [R, Preamble]. Corpus tagging "may want to add information about larger units such as words or phrases" [R, Preamble].
- Analysis: "its purpose is not to state an analysis, but to give some further possibly relevant information on the structure of a text or an example, beyond the idiomatic translation" [R, Preamble].
- Data: "Glosses are part of the analysis, not part of the data." [R, Preamble]
- Rule 1: "Interlinear glosses are left-aligned vertically, word by word, with the example." [R]
- Rule 4: "When a single object-language element is rendered by several metalanguage elements (words or abbreviations), these are separated by periods." [R]
- Rule 4A: "If an object-language element is neither formally nor semantically segmentable and only the metalanguage happens to lack a single-word equivalent, the underscore may be used instead of the period." [R]
- Rule 4B: "If an object-language element is formally unsegmentable but has two clearly distinguishable meanings or grammatical properties, the semi-colon may be used." [R]
- Rule 5: "Person and number are not separated by a period when they occur in this order. E.g. go-PRS.1PL (not: go-PRS.1.PL)" [R]
- Rule 6: "If the morpheme-by-morpheme gloss contains an element that does not correspond to an overt element in the example, it can be enclosed in square brackets." [R]
- Rule 7: "Inherent, non-overt categories such as gender may be indicated in the gloss, but a special boundary symbol, the round parenthesis, is used." [R]

### 1.2 Universal Dependencies CoNLL-U format

- URL: https://universaldependencies.org/format.html. The page states it "pertains to UD version 2" [R].

Quotations used (exact):

- "Morphological annotation is only provided for words." [R, Morphological Annotation]
- Feature values: "It is possible to declare that a feature has two or more values for a given word: Case=Acc,Dat." [R]
- Multiword token lines: they "have an underscore in all the remaining fields except MISC (because the token represents multiple words, each with its own lemma, part-of-speech tag, syntactic head, and so on)" [R].
- MSeg: "morphemic segmentation as commonly used in interlinear glossed text in linguistic literature" [R, Other Miscellaneous Attributes].
- MGloss: "A gloss is either a lexical meaning in English, or a grammatical tag; ... There are no guidelines for the tags" [R].

### 1.3 Stanza v1.15.0

- Source: https://raw.githubusercontent.com/stanfordnlp/stanza/v1.15.0/stanza/models/common/doc.py (tag `v1.15.0`).
  - Line 50: `MORPHEMES = 'morphemes'` [V].
  - Lines 62–63: `morphemes` listed in `DEFAULT_OUTPUT_FIELDS` and `NO_OFFSETS_OUTPUT_FIELDS` [V].
  - Lines 651–737: `set_mwt_expansions`. Word objects for expansions are created as `Word(sentence, {ID: ..., TEXT: e_word})` [V]. Word offsets are set only in the multi-word branch when the regex matches (line 732: `re.compile(search_string).match(token.text)`; lines 735–736) [V]. For single-word tokens, `word.start_char = token.start_char` (line 728) [V].
  - I did not inspect other modules in the package. Whether any of them populates a `morphemes` value is [U], and the decision does not depend on it (C1).
- Public documentation: https://stanfordnlp.github.io/stanza/data_objects.html. The Word properties listed are: `id`, `text`, `lemma`, `upos` (`pos`), `xpos`, `feats`, `head`, `deprel`, `deps`, `misc`, and `parent` [R]. Token properties include `start_char` and `end_char` [R]. `morphemes` is not listed [R].

### 1.4 Local code and schema (read at branch base `e6f59628`)

- `nlp/src/mouseion_nlp/producer.py`: `_morphology` (line 144), `_clean_surface` (line 150), `_map_sentence` (line 245; per-word `Token` construction, offset fallback at lines 249–254, `morphology=_morphology(word.feats)` at line 289). The module does not read Stanza's `misc` [V: no `misc` reference in `nlp/src/mouseion_nlp`].
- `proto/mouseion/v1/normalized_corpus.proto`: `message Token` (line 61) carries `surface`, `raw_lemma`, `pos`, `canonical_lemma`, `morphology` (map), `named_entity`, `location`, `dependency`, `head`. No morpheme, group, or misc field [V].
- `migrations/000001_initialize.up.sql` lines 380–405: `corpus_tokens` has `morphology jsonb` with an object check, `start_offset`, `end_offset`, `dependency`, `head`, and no misc or group column [V].
- `internal/analysis/jobs.go` line 1809: `INSERT INTO corpus_tokens(...)`, one row per normalized token [V].
- `sqlc/queries/concordance.sql` `GetVocabularySentenceStudy` (lines 3–40): filters `ca.analysis_run_id = sqlc.arg('analysis_run')`; joins `occurrence_lemma_corrections` on `source_document_id`, `start_offset`, and `end_offset` [V]. Does not select `t.morphology` [V].
- `internal/domain/domain.go` lines 456–460: `SentenceStudyToken` has `Surface`, `RawLemma`, `EffectiveLemma`, `UPOS`, `Dependency`, `HeadSurface`, `Ordinal`, `HeadOrdinal`, `Corrected`, `Excluded`. No morphology, offsets, or group [V].
- `internal/webapp/concordance.templ` sentence-study token list (around lines 135–151): renders surface, UPOS, lemma evidence, effective lemma, dependency, and head [V].
- `internal/enrichment/local.go` lines 27–40: `StanzaMorphology.Morphology` copies the candidate's morphology map unchanged [V].
- `internal/cardexport/cardexport.go` lines 1165–1186: dictionary-derived `Gender`, `Article`, and `Plural` are merged into the same morphology map [V].
- `AGENTS.md` ("Schema changes"): "Settle consequential shape (new tables/columns, constraints, destructive changes, durable state machines) in an accepted issue/feature doc/ADR before writing SQL" [R, repo].

### 1.5 Repo decision records

- ADR 0079 (`doc/adr/0079-*.md`, dated 2026-09-27, scope revised 2026-09-28): contextual gloss for the target, "the target's meaning in its representative sentence" [V].
- ADR 0080 (`doc/adr/0080-*.md`, dated 2026-09-28): LLM alignment for "a contiguous target phrase", treated as "a proposal, never as markup" [V]. It does not provide per-token sentence alignment [I].

## 2. Representations and what each explains

| | Word-level feature line | Interlinear morphemic gloss | Dependency tree (existing) | Contextual gloss (ADR 0079) |
|---|---|---|---|---|
| Unit | Surface word or word-level range | Morpheme | Token and head | Target |
| Explains | Which features the analyzer attributes to a word | How a word is segmented and what each part means | Which token governs which | Meaning of the target in this sentence |
| Source today | `morphology` per token [V] | Not in the local contract [V, §1.4] | `dependency`, `head` per token [V] | Per-target LLM text [V] |
| Leipzig status | Labeled word-level convention; LGR's morpheme scope does not cover it [R], §7 | The Leipzig morphemic form; not produced locally [V] | Not a Leipzig form | Not a Leipzig form |
| Shown in sentence study today | No, although persisted [V] | — | Yes [V] | Card-level [V] |

The feature line and the tree answer different questions: the feature line describes a word's form; the tree describes its syntactic role. Whether they should be shown together or as alternatives is an open product decision (U6) and is not decided here.

## 3. Examples (fabricated, features-only)

Each example lists the features an analyzer could attribute to a word, using UD feature names. A feature line reports features. It does not claim which part of the word expresses which feature, and it does not add lexical meanings. Labels use Leipzig-style abbreviations from the standard list [R] as a display choice. These are not observed Stanza outputs.

### 3.1 Italian `Vado al mercato.` ("I go to the market.")

| Surface | Kind | Illustrative features | Notes |
|---|---|---|---|
| `Vado` | Word | `Mood=Ind`, `Number=Sing`, `Person=1`, `Tense=Pres`, `VerbForm=Fin` | Person is a feature of the verb; no subject pronoun is written. No bracket is used, because the rules bracket only elements with no overt correspondent, and whether `-o` expresses person is an analysis question (C9) [R]. |
| `al` | Multiword range | (none on the range line) | UD gives range lines no features [R]. Features must come from the sub-words (`a`, `il`), which is an attribution decision (U5). |

Feature-line display for `al`, if the features of the sub-word `il` are attributed to the whole range: `DEF.M.SG`. This display is a choice that depends on U5. It is not a morpheme alignment.

### 3.2 German `Sie wohnt im Haus.` ("She lives in the house.")

- `im` is a range that expands to `in` and `dem` [I; fabricated].
- Illustrative features on `dem`: `Case=Dat`, `Definite=Def`, `Gender=Masc,Neut`, `Number=Sing`, `PronType=Art` [I]. The UD comma list means "masculine or neuter", not one gender [R].
- A feature line can show `DAT.SG` and must decide how to show `Gender=Masc,Neut`. Leipzig Rule 4B's semicolon means two distinguishable meanings in one element, not ambiguity [R]. No Leipzig notation for ambiguity is given, so the choice is local (U4).

### 3.3 Greek `Είμαι στο σπίτι.` ("I am at the house.")

- `στο` is a range that expands to `σε` and `το` [I; fabricated].
- Illustrative features on `το`: `Case=Acc`, `Definite=Def`, `Gender=Neut`, `Number=Sing` [I].
- A feature line can show `DEF.ACC.SG.N`. The preposition `σε` has no features in this illustration. The lexical meaning "at" is not part of a feature line; it belongs to a gloss or translation.

### 3.4 Italian `Bevo dell'acqua.` ("I drink (some) water.")

- The surface `dell'` keeps its apostrophe at the token level [V: `_clean_surface`, C5].
- The sub-word forms, and whether Stanza splits `dell'` into `di` and a second word, are [U]. Settling this needs a model run, which is out of scope for this research.

## 4. Local contract

### 4.1 What is persisted

- One `corpus_tokens` row per normalized token, with its own `token_ordinal` [V: `jobs.go` line 1809; `producer.py` builds one `Token` per Word]. Each row has `surface`, `raw_lemma`, `canonical_lemma`, `upos`, `morphology`, offsets, `dependency`, and `head` [V].
- `morphology` is a JSON object of feature names to values, from `_morphology(word.feats)`, which splits on `|` and then on the first `=` [V, `producer.py` lines 144–147].
- Feature values with multiple values keep the analyzer's string form (`Masc,Neut`) [V, UD multi-value rule, R].

### 4.2 Projection: what it would take to show features

- The study query does not select `t.morphology` [V]. The domain struct and the template do not carry it [V].
- Showing word-level features needs changes to the query, `SentenceStudyToken`, and the template, plus a regeneration of `gen/sqlc` for the query change [R, AGENTS.md "Generated artifacts"].
- No schema change is needed for this projection [V].

### 4.3 Shared-span contractions: what is known and what is a design option

Known [V]:

- Stanza gives MWT sub-words no offsets unless the regex matches. The regex joins the component words with `\s*` and matches the token text. For a fused contraction the token text is not its components with optional whitespace, so it does not match [V, `doc.py` line 732; mirror run]. Sub-words therefore get no offsets [V].
- Our producer then copies the parent token's offsets to each sub-word [V: `producer.py` lines 249–254]. Each sub-word becomes its own `corpus_tokens` row with its own `token_ordinal` [V].
- Within one analysis run, sibling rows share `start_offset` and `end_offset` [I from the above].
- Corrections join on the same offsets (§4.5), which makes this important.

Design options (none decided, none verified as required):

- **Option A: infer groups from data.** Treat consecutive rows in one sentence with identical offsets as one range. This depends on the invariant that only MWT siblings share spans in a run. That invariant is not verified across all data, so it would need tests first.
- **Option B: store an explicit group key.** A new column or table. Under AGENTS.md this is a consequential schema shape, so it needs an accepted issue or ADR before any SQL [R, AGENTS.md "Schema changes"].
- **Not an option:** putting the group identity inside `morphology`. That map holds analyzer grammatical features [V]. A group key there would mix grouping with grammatical evidence and with the dictionary merge in `cardexport.go` (§6).

### 4.4 Surface cleaning

- `_clean_surface` strips leading and trailing characters whose Unicode category starts with `P` or `S`, but not apostrophes [V, `producer.py` ~lines 150–160]. Internal characters are kept.
- Sub-word surfaces come from Stanza's expansion of the token [V: `set_mwt_expansions` uses `expansions`]. Our code does not recompute them from the source text.
- The source form of a parent token is recoverable from `sentence.text` plus its offsets. For sub-words, that recovery is only as reliable as the shared span (§4.3).

### 4.5 Offset-keyed corrections (within-run hazard)

- `GetVocabularySentenceStudy` joins `occurrence_lemma_corrections` on `source_document_id`, `start_offset`, and `end_offset` [V, `concordance.sql`].
- Sibling rows of a shared-span contraction have the same offsets [V+I]. A correction keyed to one sibling's span can therefore match every sibling row.
- Effect in production: [U], not tested here. This is a concrete risk for any design that uses the same offsets for siblings.

## 5. Stanza fields and the morphemes question

- Public Word fields: `id`, `text`, `lemma`, `upos`, `xpos`, `feats`, `head`, `deprel`, `deps`, `misc`, `parent` [R]. No morpheme field [R].
- v1.15.0 `doc.py` names `morphemes` in output-field lists (lines 62–63) and defines `MORPHEMES` (line 50) [V]. It does not assign a value in that file [V]. Package-wide population is [U] (§1.3).
- Our protobuf `Token` has no morpheme field [V], and the producer does not read `misc` [V]. So even UD `MSeg`/`MGloss` in `misc`, which are optional and treebank-specific [R], would be dropped on the way to `corpus_tokens`.
- Conclusion: the local contract carries no per-morpheme alignment, regardless of what Stanza's library can emit [V].

## 6. Terminology hazard in the code

"Morphology" refers to two sources:

- **Analyzer features:** UD FEATS per word, stored in `corpus_tokens.morphology` [V] and copied into the candidate by `StanzaMorphology` [V].
- **Dictionary-derived values:** `Gender`, `Article`, and `Plural` merged into the same map by `cardexport.go` [V]. The merge overwrites any analyzer value with the same key (`morphology["Gender"] = result.Gender` when non-empty) [V].

A feature line should label the source of each value, or the card and the sentence view will disagree without notice.

## 7. LGR limits, stated without an absolute rule

- LGR states that its rules "only allow for information about morphemes" [R]. A word-level feature line is therefore outside the rules' stated scope, not forbidden by them.
- LGR's own conventions include unsegmented elements. Rule 4A allows an underscore for an element that is "neither formally nor semantically segmentable" [R]. Rule 4B allows a semicolon for an element that "is formally unsegmentable but has two clearly distinguishable meanings or grammatical properties" [R].
- So a word-level feature line may use standard abbreviations, as a labeled convention. It should not be called an interlinear morphemic gloss, and it must not imply morpheme-to-feature alignments the data does not support.

## 8. Unknowns and decisions

| # | Item | Kind | Why it matters |
|---|---|---|---|
| U1 | Whether any Stanza module populates `morphemes` | Unknown; not needed for the decision | Doesn't change the local contract (C1) |
| U2 | How the Italian model splits `dell'` | Unknown; needs a model run, out of scope here | Determines sub-word text and any grouping |
| U3 | Whether to show `Gender=Masc,Neut` as a set or omit it | Product decision | Ambiguous features have no Leipzig notation (§3.2) |
| U4 | Whether a range's features are attributed to its sub-words, and how | Design decision | UD range lines carry no features (§3.1) |
| U5 | Group identity: infer (Option A) or store (Option B) | Design decision; Option B needs an ADR | §4.3 |
| U6 | Feature line vs tree: companion or replacement | Product decision, out of scope | The learner's question |
| U7 | Effect of offset-keyed corrections on sibling rows | Unknown; needs a test | §4.5 |

## 9. Findings (summary)

1. The Leipzig rules define morphemic glosses. Word-level feature labels are outside their stated scope, not forbidden by it (§7).
2. Feature display needs no schema change. It needs query, domain, and template work (§4.2).
3. Fused contractions get no sub-word offsets from Stanza, by the shape of its regex. Our producer copies the parent span (§4.3).
4. Sibling rows share offsets, so grouping and offset-keyed corrections both need care (§4.3, §4.5). A group key is one option, not a verified requirement.
5. The local contract has no per-morpheme alignments, and the producer does not read Stanza's `misc` (§5).
