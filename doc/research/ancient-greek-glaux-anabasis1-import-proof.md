# GLAUx *Anabasis* Book 1 import proof (#2050)

Status: bounded executable evidence for [#2030](https://github.com/justin-hayes/mouseion/issues/2030). The agent accepts no source, MVP, edition or mapping; human acceptance and independent verification are still to come.

Companion files: [`ancient-greek-glaux-anabasis1-import-proof/`](ancient-greek-glaux-anabasis1-import-proof/README.md). Rights: [`NOTICE.md`](ancient-greek-glaux-anabasis1-import-proof/NOTICE.md).

## Result

Complete *Anabasis* Book 1 imports end to end, from pinned GLAUx annotation bound to the exact Perseus text of Marchant's 1904 edition:

- All 10,645 native nodes are accounted for. There are 0 binding exceptions, 0 uncovered source characters, 0 section-label mismatches and 0 word-boundary violations.
- Every printed node maps to a lemma and UPOS through a documented crosswalk, with 0 unresolved printed nodes.
- Mouseion's real Go selection code, run with empty Known/Reserved, selects **373 of 1,294 identities**. Its output is byte-identical to the Python model.
- **372 of the 373 selected identities** have a deterministic representative-sentence input under the #2034 caps.

One required-identity condition needs a human decision. Marchant secludes **63 lexical words** (8 bracketed deletions). GLAUx deliberately leaves them unannotated. This proof classifies them as non-vocabulary edition-secluded text, which is supported by both the edition and the producer. Under a stricter reading, those 63 nodes would block vocabulary-ready publication. The verdicts are in [§12](#12-verdict-ledger).

Practical quality is usable but has visible, measurable problems, set out in [§10](#10-quality-findings):

- 34 selected identities are names.
- οὐ and μή appear as adverb vocabulary.
- GLAUx splits οὐδέ/οὔτε into components.
- Some adverbs carry adjective headwords.
- εἰμί is never vocabulary.
- A Perseus typo propagates into the annotation.
- Sentence strings show secluded text without brackets.

## 1. Command and pinned inputs

```sh
doc/research/ancient-greek-glaux-anabasis1-import-proof/scripts/reproduce.sh
```

The command fetches the 27 inputs in [`inputs/pinned-inputs.tsv`](ancient-greek-glaux-anabasis1-import-proof/inputs/pinned-inputs.tsv) by commit URL and verifies their sha256. It then imports, runs the Go selection probe, checks parity, requires byte-identical regeneration of `manifest/`, and runs 25 tests. All of these steps passed on 2026-10-14 from a freshly fetched input directory.

| Role | Pin |
| --- | --- |
| Annotation | `alekkeersmaekers/glaux` @ `b077d8f6` (still `main` HEAD on 2026-10-14), `xml/0032-006.xml`, blob `ad9a1c77`, sha256 `19588704…3244` |
| Text | `PerseusDL/canonical-greekLit` @ `bd7b1c7a`, `data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`, blob `6fc2f2a1`, sha256 `e666c4a6…7434` |
| Revision evidence | All 15 historical revisions of that Perseus file, plus `license.md` at HEAD `0189b9c8` |
| UPOS evidence | `UD_Ancient_Greek-Perseus` @ `331ddef9` (train/dev/test) |
| Edition point checks | Internet Archive `operaomniarecogn03xenouoft` OCR (sha256 pinned; no upstream revision id) |

## 2. Scope, edition and full unit

- **Unit.** Book 1 runs from 1.1.1 to 1.10.19: 10 chapters and 205 sections. The extracted text has 58,547 code points. GLAUx encodes 610 Book 1 segments, and none crosses into another book; Books 2–7 are present in the file but not imported. The book heading `Κύρου Ἀναβάσεως Α` is excluded as ancillary, and GLAUx does not annotate it.
- **Edition.** The Perseus header names E. C. Marchant, *Xenophontis Opera Omnia* vol. 3 (Oxford 1904, reprinted 1961). Twelve point checks against the scan OCR find Marchant's bracketed deletions, angle-bracket supplements, cruces, the book heading and the apparatus note "ἔνθα … ἔπεισιν del." where Perseus encodes `<del>`, `<add>` and `<sic>`. GLAUx `metadata.txt` names only "Perseus".
- **Text revision accompanying the annotation.** After applying the documented decoding (§3), GLAUx's 10,488 printed tokens match the `bd7b1c7a` revision (2019-09-09) with **0** differences ([`text_revisions.tsv`](ancient-greek-glaux-anabasis1-import-proof/manifest/text_revisions.tsv)). Earlier revisions leave 12–32 GLAUx tokens and 6–22 source tokens unmatched, mostly beta-code parenthesis errors fixed in 2018–2019. Every later revision differs by exactly one token: from `84a6f3e5` (2022) on, Perseus encodes `<choice><sic>ἦν</sic><corr>ἢν</corr></choice>` at 1.3.14. The binding therefore uses blob `6fc2f2a1`. Adopting a later revision would be a new text revision that needs its own binding (ADR 0093). It must not be a silent substitution.
- **Interior coverage.** Every non-space character of the extracted text is covered by a node span. Every printed node falls inside its declared section.
- **Editorial material.**
  - 8 deletions (`del`) cover 73 GLAUx nodes, 63 of them lexical. GLAUx gives these nodes an empty `form` and a brace-wrapped `form_original` and no lemma.
  - 2 supplements (`add`, 4 nodes) are annotated.
  - 7 obelized passages (`sic`, 19 words) are annotated, and their 14 `†` nodes are bound as zero-width anchors at the `sic` boundaries.
  - 96 `artificial="elliptic"` nodes are unprinted.

## 3. Producer decoding and binding (`glaux-perseus-bind/1`)

The rules are inferred from the file and each one is validated against the exact text. A mismatch raises an error; nothing is guessed.

| Rule | Nodes | Evidence |
| --- | --- | --- |
| B-EXACT: NFC(form) equals the text at the cursor | 10,411 | GLAUx is NFD (its README says so). Accents, breathings and iota subscript are kept. |
| Elision mark: GLAUx U+2019 corresponds to source U+02BC | 193 | All 193 U+02BC marks in Book 1 correspond. |
| B-SUFFIX-COMPONENT: `-X` binds immediately after its host | 40 | `οὐ`+`-δὲ`, `οὔ`+`-τε`, `μη`+`-δὲ`: each pair concatenates to the printed word. Offsets come from source substrings, not from lemma lengths. |
| B-CRASIS-SHARED: `X-` shares the following host's span | 7 | `κα-`+`κἂν`, `τα-`+`ταὐτὸν`, `το-`+`τοὔμπαλιν`. The fragment is not a substring, so no component offset is invented. |
| B-SECLUDED: `{…}` binds inside a `del` region | 73 | The span is the deleted words. |
| B-SUPPLIED: `<…>` binds inside an `add` region | 4 | — |
| B-CRUX-ANCHOR: `†` is a zero-width anchor at a `sic` start or end | 14 | Seven pairs. |
| B-UNPRINTED | 96 | No span. |

Mouseion's current occurrence identity is offset-only (`selection.OccurrenceIdentity`), so the 7 crasis pairs share a span with no other distinguishing key. This does not affect counting. It does confirm ADR 0094's requirement for node identifiers before learner occurrence decisions could target these nodes.

## 4. Complete sentences (`oct-grc-terminal/1`)

Complete sentences are segmented over the exact text, separately from GLAUx segments. `.`, `;`, U+037E, `!` and `?` end a sentence, together with any following closers. Ano teleia is medial (#2034). The unit ends on a terminal.

- The text has **382 complete sentences**: 373 end with `.` and 9 with `;`.
- GLAUx's 610 native segments end on `.` (373), `;` (9) and `·` (228). Every complete sentence is an exact union of 1–7 native segments, and no native segment crosses a complete-sentence boundary. This independently corroborates the boundaries without relying on the producer's labels.
- Lengths: the median is 21 whitespace units and the maximum 100. 13 sentences have at most 5 units. **22** exceed 50 units, and 4 of those also exceed 400 code points. No sentence exceeds only the code-point cap. 360 of the 382 are within both caps.
- **Boundary problem.** Perseus carries no quotation marks. At 1.6.7, `…ὅ τι ἐδύνω; ἔφη Ὀρόντας.` therefore leaves `ἔφη Ὀρόντας.` (2 units) as its own "sentence". This is the only such case among the nine `;` boundaries, and no representative sentence uses it.

## 5. Canonical mapping (`glaux-agdt-upos-crosswalk/1`, profile `ancient-greek/1`)

GLAUx uses AGDT-style 9-position postags. UD Ancient Greek-Perseus is a token-paired conversion of AGDT that keeps the native postag as XPOS. The parent-reviewed scheme survey paired all 202,989 tokens. Its observed output is the crosswalk ([`crosswalk_letters.tsv`](ancient-greek-glaux-anabasis1-import-proof/manifest/crosswalk_letters.tsv)):

| AGDT letter | UPOS observed in UD Perseus | Rule |
| --- | --- | --- |
| a, d, g, i, l, m, n, p, r, u | ADJ, ADV, PART, INTJ, DET, NUM, NOUN, PRON, ADP, PUNCT. Each letter maps to one UPOS for every token, and the code rejects the rule if that ever changes. | M-LETTER |
| v | VERB 35,836; AUX 1,875. Only εἰμί (1,864 AUX, almost all `cop`) and ἔχω (11 AUX, all `aux`) receive AUX. | εἰμί → AUX (180 occurrences); relation `AuxV` → AUX (none in Book 1); otherwise VERB |
| c | CCONJ or SCONJ by lemma, and single-valued for every attested Book 1 lemma | Lemma rule. ἐπειδή is not attested with `c` in UD Perseus, so its 6 occurrences stay ambiguous between CCONJ and SCONJ. Both are outside `AllowedPOS`, so the vocabulary status is determinate. The ambiguity is shown, not resolved. |

- **Headword** is GLAUx's NFC lemma, unchanged.
- **Canonical key** is the #2033 profile: NFC, lowercase, `ς`→`σ`, NFC.
- **Lexical homograph discriminators** are absent in the source. GLAUx lemmas carry no numbers, so none can be preserved or invented.
- **Native evidence.** Morphology (postag positions 2–9), heads, relations, animacy and WordNet senses are kept in `nodes.tsv`. They are not mapped or validated.

Vocabulary eligibility depends only on the letter rules and the εἰμί rule. The CCONJ/SCONJ split never changes eligibility, so a production adapter would not need the UD-Perseus-derived lemma table at runtime.

## 6. Occurrence denominators

| Count | Value |
| --- | --- |
| Native Book 1 nodes | 10,645 |
| Unprinted (elliptic) | 96 |
| Edition-secluded, unannotated | 73 (63 lexical) |
| Printed punctuation and crux signs | 1,171 |
| Printed lexical annotated nodes | 9,305 |
| Non-vocabulary by UPOS | 4,655: CCONJ 1,163; DET 1,048; PRON 925; ADP 585; PART 307; SCONJ 269; AUX 180; NUM 165; INTJ 7; CCONJ\|SCONJ 6 |
| **Vocabulary-eligible occurrences** (analyzable-token denominator) | **4,650** |
| Unresolved printed nodes | 0 |

## 7. Vocabulary selection and Known comparison

The scenario is one analyzed Book, empty Known and Reserved, no corrections or exclusions, and no external frequencies. The across-Books total therefore equals the in-Book count, so 3/15 reduces to at least three occurrences in the Book. That reduction is checked, not assumed:

- `_selectionprobe` calls `selection.Project(…, DefaultConfig)`, `NewEligibility(nil, nil)` and `AllowsBookDeckCandidate`.
- It supplies the across-Books total exactly as `persistence.currentReadingCandidateFrequency` does.
- Its output is byte-identical to the Python model in [`selection_model.tsv`](ancient-greek-glaux-anabasis1-import-proof/manifest/selection_model.tsv).

Results:

- **Identities:** 1,294 (NOUN 470, VERB 492, ADJ 223, ADV 109). Real `AnalyzableStatistics` gives 4,650 analyzable tokens and 1,294 distinct identities.
- **Selected:** 373 identities (NOUN 138, VERB 144, ADJ 50, ADV 41), covering 3,517 occurrences, 75.6% of the eligible occurrences. The full list with frequencies is in [`identities.tsv`](ancient-greek-glaux-anabasis1-import-proof/manifest/identities.tsv).
- **Known comparison:** 0 Known and 0 Reserved, so known-token coverage is 0.0% and the Book falls in the "below 95%" coverage band. All 1,294 identities are Unknown, and the snapshot would freeze all 373 selected identities. Generated cards would not mark any of them Known (ADR 0036).
- **Most frequent:** Κῦρος 170, οὐ 103, βασιλεύς 73, λέγω 73, ἔχω 70, πολύς 67, Ἕλλην 54, ποιέω 52, στράτευμα 44, γίγνομαι 39.

## 8. Deterministic card inputs (#2034)

For each selected identity, the earliest occurrence qualifies if:

- its complete sentence has a terminal boundary and is within 50 whitespace units and 400 code points;
- the target has a verified span; and
- the target has a usable headword.

Nothing is truncated.

- **372 eligible** and **1 omitted**: οἴχομαι, whose 3 occurrences all fall in sentences over 50 units.
- 27 identities use a later occurrence because an earlier sentence exceeded a cap. Example: φημί's first occurrence is in a 59-unit sentence at 1.2.25; 1.2.26 is chosen.
- Flags on chosen inputs: target identity repeated in the same sentence 30 (the chosen span is node-specific); capitalized noun headword 34; sentence containing editorial markup 4.
- Dictionary coverage, Gloss/translation and Anki rendering are not measured. No external calls were made.

The [`review_set.tsv`](ancient-greek-glaux-anabasis1-import-proof/manifest/review_set.tsv) cases are each chosen by a stated rule:

- ordinary sentences;
- a 5-unit complete sentence (`ὧδε οὖν ἐποιεῖτο τὴν συλλογήν.`);
- a cap rejection followed by the next qualifying occurrence;
- the omission;
- a repeated target;
- NFD versus NFC polytonic text (`ᾐσθάνετο`);
- a suffix component;
- crasis with a shared span;
- elision decoding;
- the 1.3.14 typo;
- a target inside a crux;
- a secluded node;
- a name as NOUN;
- an adverb under an adjective headword;
- the attribution fragment.

The lexical-homograph case is reported as **absent**: there are no source discriminators, and εἶμι does not occur in Book 1.

## 9. Product journey

[`catalog_example.json`](ancient-greek-glaux-anabasis1-import-proof/manifest/catalog_example.json) is a minimal metadata-only offering. It records the Work (CTS work and GLAUx id as aliases) and the Edition (Marchant 1904, `perseus-grc2`). It offers two Reading units: Book 1, whose annotation is evidenced by this proof, and the whole work, which is not assessed and must not be presented as analyzed on Book 1 evidence. It also records pinned text and annotation sources, layer and scope provenance (`analysis=auto`), component rights, capabilities, and `requires_stanza: false`.

Trace under ADRs 0093 and 0096:

1. **Sync.** Sync stores only this metadata. The Book 1 offering becomes an Inbox Book and contributes no personal counts.
2. **To Read.** Choosing To Read records intent and queues acquisition of the two pinned files. The acquisition verifies their hashes, then runs §2–§5 and publishes atomically. Any failure is reported as blocked, with no fallback edition or annotation.
3. **Analysis.** The published analysis supplies the §6 denominators and the §7 identities. Once the count projection is ready, the Book contributes 4,650 occurrences to Personal analyzed-Book occurrences for `grc`, separate from `el`.
4. **Current reading.** If #2030 accepts the seclusion classification, the Book is eligible. Starting a Current reading then freezes the 373 selected identities and reserves them.
5. **Prepared deck.** Deck preparation uses the 372 frozen sentence and target inputs and reports one omission. Omissions never shrink the frozen selection.
6. **Finish.** Finishing makes the frozen 373 Known, independent of deck readiness.

None of this was built, and no UI, database or queue code was changed.

## 10. Quality findings

These problems are measured; the proof does not certify the automatic predictions. GLAUx reports lemma accuracy of 98.8% and morphology of 97.2% on its own evaluation. Book 1's labels are all `auto`.

| Finding | Kind | Effect |
| --- | --- | --- |
| UD Perseus has no PROPN, so names are NOUN | mapping (faithful to the conversion) | 34 of 373 selected identities are capitalized nouns (444 occurrences): Κῦρος, Κλέαρχος, Τισσαφέρνης, place names, and ethnics such as Ἕλλην. A capitalization rule would remove them, but it has no native-tag evidence and is not adopted. |
| AGDT `d` is ADV, including negation | mapping | οὐ (103) and μή (32) are selected vocabulary. Deictic and connective adverbs (ἐνταῦθα, ἐντεῦθεν, ἔτι, τότε) are selected too. |
| GLAUx splits οὐδέ, οὔτε and μηδέ | producer convention | 37 οὐ+δέ/τε and 3 μή+δέ/τε splits. οὐ absorbs them, so οὐδέ and οὔτε never appear as their own words. |
| Adverbs lemmatized under the adjective | producer convention | `καλός|ADV` (καλῶς), `κακός|ADV`, `πρότερος|ADV`. These are distinct identities, but the headword misleads. |
| Suppletive and rare citation forms | producer lemma choice | κρατύς (7) for κράτιστος and κρείττων; μείς (6) for "month"; `οἰκουμένην` is split between `οἰκουμένη|NOUN` and `οἰκέω|VERB`. |
| εἰμί is always AUX | mapping | 180 occurrences are never vocabulary, existential uses included. Mapping them to VERB would add one selected identity; that alternative is not adopted. |
| Text error propagates | source text | At 1.3.14, Perseus 2019 reads `ἦν` and GLAUx lemmatizes it as εἰμί. Perseus later encoded `sic ἦν / corr ἢν`, and the OCR shows a grave accent. Here the error is non-vocabulary (AUX), but the same mechanism could create a wrong vocabulary identity. |
| Secluded text appears unbracketed | presentation | Plain sentence strings show `[καὶ φύλακας]` as `καὶ φύλακας`. Of the chosen inputs, 2 contain deletions and 2 contain supplements. |
| Attribution fragment | segmentation | One 2-unit pseudo-sentence (§4). It is not chosen. |

Mapping errors were not found by these checks: every rule's output matches its stated evidence, and the tests pin each convention. Upstream prediction errors were not systematically audited; sampling them would be a separate scholarly review.

## 11. What remains for #2030

These are the smallest open questions. None of them needs a new survey.

1. **Seclusion.** Is edition-secluded text (63 lexical nodes, which neither the edition nor the producer treats as authentic text) an accepted non-vocabulary classification for Book 1? If not, the Book is not vocabulary-ready from GLAUx alone.
2. **Names and function adverbs.** Accept the UD-Perseus-faithful NOUN/ADV mapping, which makes names and negations vocabulary, or decide a reviewed alternative.
3. **Editorial markup and segmentation presentation.** Decide how card sentences show deletions, supplements and cruces, and whether the one attribution fragment needs a rule.
4. **Text revision.** Decide whether to keep the annotated 2019 blob, which contains the typo, or to require a later revision with its own binding.
5. **Still to be done:** human review of the examples, Gloss and translation acceptance, Anki polytonic rendering, dictionary coverage, and independent parent verification of this proof.

## 12. Verdict ledger

The machine-readable version is [`ledger.json`](ancient-greek-glaux-anabasis1-import-proof/manifest/ledger.json).

| Area | Verdict |
| --- | --- |
| Scope and edition | **pass**. Complete Book 1, Marchant 1904, exact annotated revision identified. Qualifications: the 1.3.14 typo; the heading is excluded. |
| Rights, local private analysis | evidence suffices (explicit BY-SA declarations for text and annotation) |
| Rights, direct acquisition | evidence suffices (public pinned commits) |
| Rights, public research outputs | evidence suffices with notices (BY-SA; crosswalk counts BY-NC-SA 2.5) |
| Rights, bundled distribution | evidence suffices for #2035 exact-component review; acceptance is human. The Perseus README's general Tufts statement is recorded. |
| Binding and segmentation | **pass**: 0 exceptions, 0 uncovered characters. Qualifications: the one fragment; the terminal convention is a research version. |
| Required lexical identities | **conditional** on the seclusion classification. 0 unresolved printed nodes. |
| Optional capabilities | unassessed: morphology FEATS, syntax validation, dictionary coverage. Discriminators are absent in the source. |
| Measured output quality | **conditional**, see §10 |
| Deterministic card inputs | **pass** (372 of 373), with the editorial-markup presentation qualification |
| Semantic and artifact checks | **unassessed**: Gloss, translation, Anki, dictionary, human example review |

No production code, schema, dependency, accepted ADR or glossary changed. The Greek NLP service was not touched, no analyzer was run, and no external provider was called.
