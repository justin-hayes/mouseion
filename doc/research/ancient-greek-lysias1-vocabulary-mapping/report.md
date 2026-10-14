# Lysias I: AGDT → canonical vocabulary mapping (#2049)

Scope: canonical vocabulary interpretation for Lysias I (`tlg0540.tlg001.perseus-grc1`, AGDT v2.1). Owned here: UPOS mapping, lemma/headword decoding, canonical keys, node accounting. Not covered: authority, source coordinates, rubric scope, and variant bindings (#2048). No cards, no annotation, no provider calls.

## Result

The mapping is established for 2,802 printed nodes. It is complete at the node level: every printed node has a rule, a UPOS status, and a vocabulary class. It is **not** complete at the identity level. The blockers are listed below.

- **Eligible (1,439 occurrences):** NOUN 328, ADJ 243, ADV 341, VERB 527.
  - 1,173 resolved with no conditions.
  - 259 resolved-conditional: `d` on AuxY/AuxZ particles → ADV (rule R03P).
  - 7 hyphenated compound-verb lemmas: the UPOS is resolved (VERB) but the headword is undecoded.
- **Non-vocabulary (1,322):** the UPOS is resolved for most classes. R08 (91 AuxC → SCONJ) is conditional on the relation-based split. R09 (3 conjunctions with a relation-only class) has its CCONJ/SCONJ split unresolved. Neither affects vocabulary eligibility.
- **Unresolved (41):** the copula εἰμί with a predicate-nominal dependent. VERB vs AUX is not established by the native scheme.
- Total: 1,439 + 1,322 + 41 = 2,802. Plus 32 `artificial="elliptic"` nodes, excluded as unprinted per ADR 0094. Node count 2,834.

### Node accounting by letter (printed)

v 569, u 374, d 341, n 328, p 287, l 284, a 243, c 215, r 136, i 25. No g, m, x, b, or k occur in Lysias I.

### Identities

- Eligible identities: 503 = 498 resolved + 5 unresolved-headword (7 nodes).
- Copula candidates: `candidate:εἰμί|VERB` (41) and `candidate:εἰμί|AUX` (41). These are separate from the 10 resolved non-copula `εἰμί|VERB` tokens.
- Identity key = canonical key + `|` + UPOS.

**Full identities are not established.** Blockers, with the number of affected nodes:

1. Copula εἰμί: 41 nodes. If VERB, eligible occurrences rise to 1,480 and `εἰμί|VERB` goes from 10 to 51. If AUX, they are non-vocabulary and the eligible count stays at 1,439. The native scheme has no copula label, so this cannot be resolved from Lysias evidence.
2. Hyphenated compound-verb headwords: 7 nodes, 5 identities. `κατά-καθεύδω` (2), `ἀντί-βολέω` (2), `διά-διαιτάω` (1), `ἀμφί-ἀμφισβητέω` (1), `σύν-λέγω` (1). The guidelines I checked do not define this notation. Raw lemmas are kept and the canonical key is not decoded.
3. Particle letter (`d` vs `g`): 259 nodes. See "Particle evidence" below.

## Headword cases (raw, not folded)

- **Grave accent:** 10 nodes (`καὶ` ×8, `μὴ` ×2) keep their grave. No acute/grave fold is applied (ADR 0094). These are separate identities from `καί` (99) and `μή` (12).
- **Accent-variant splits within one UPOS (not merged):**
  - `καί`/`καὶ` (ADV)
  - `μή`/`μὴ` (ADV)
  - `πάγος`/`πᾶγος` (NOUN, 1 each)
  - `παίδιον`/`παιδίον` (NOUN, 2 and 4)
  - Accent folding would also merge `εἰμί` ("be") and `εἶμι` ("go"), which is why it is not applied.
- **Capitalized lemmas:** 17 nodes, all NOUN: `Ἐρατοσθένης` ×6, `Ἀθηναῖος` ×2, `Σώστρατος` ×2, `Ἄρειας` ×2, `Ἑλλάς`, `Ὀῆθεν`, `Θεσμοφόρια`, `Νόμος`, `Ἁρμόδιος`. The key is lowercased per #2033. The native tagset has no PROPN, and UD v2.1 emits no PROPN for capitalized NOUN lemmas (11,323/11,323 NOUN). Whether names enter vocabulary is a policy question for the parent, not a mapping question.
- **Adjective/adverb lemma sharing:** `καλόσ` (ADJ 1, ADV 2) and `οἰκεῖοσ` (ADJ 1, ADV 1). The adverbs carry the adjective lemma. Two identities per key, one per UPOS.
- **Lemma examples checked in Lysias I:** `δέ` 65 and `δὲ` 4 (separate, accent-variant); `εἴσειμι` 6; `συνδειπνέω` 4. These are usable as raw headwords. I did not verify them against Kaikki or LSJ. They are attributed to the source lemma field only.

## Mapping rules (versioned)

Full table: `manifest/lysias1-rules.tsv`, rules version `lysias1-upos-rules/2026-10-14.1`.

- Letter rules: n→NOUN, a→ADJ, d→ADV, v→VERB, c→CCONJ/SCONJ, r→ADP, l→DET, p→PRON, i→INTJ, u→PUNCT, m→NUM, g→PART.
- Relation-specific rules:
  - v + AuxV → AUX (R04, 1 node).
  - v + copula candidate → VERB|AUX, unresolved (R05, 41 nodes).
  - c + COORD → CCONJ (R07, 121 nodes).
  - c + AuxC → SCONJ, conditional (R08, 91 nodes).
  - d + AuxY/AuxZ → ADV, conditional (R03P, 259 nodes).
- Copula candidate: εἰμί with a dependent whose relation starts with PNOM. This is a structural test on the native tree only.
- Eligibility (mirrors repo): UPOS ∈ {NOUN, VERB, ADJ, ADV}, lemma contains a letter, relation ≠ `compound:prt`. Source: `internal/selection/selection.go` `AllowedPOS` (lines 69–72); `internal/lexical/lemma.go` `IsLemma`. `internal/lexical` contentPOS is a context helper, not vocabulary selection. ADR 0073 Greek function words are a context stopword set, separate from eligibility.

## Particle evidence (d vs g)

- The release tagset `v2.1/Greek/TAGSETS.xml` defines `d` = adverb and `g` = particle.
- AGDT 2.5 guideline §4.1.12.1 defines particles as "those adverbs, typically sentence adverbs". Its morphology section (line ~464) says sentence adverbs are "annotated as adverbs at the morphological level". The 2.0 guideline §2.6 makes the same point.
- AGDT 1.1 §4.3 (pinned PDF) defines a particle as "a morphological category for uninflected function words", with no letter given.
- Lysias I header declares GSAAGDT 1.1. All 259 AuxY/AuxZ particles are `d`; none are `g`.
- Corpus (33 texts, native fields only): 15 files use no `g` on particles, 18 use `g`. Declared scheme does not predict the letter. Two 2.0 files use `d` only.
- UD v2.1 cross-corpus: `d` on AuxY/AuxZ → ADV 11,324/11,324.

Result: ADV is the tag-literal mapping and is recorded as conditional. I did not reclassify to PART. Choosing PART would move 259 nodes out of vocabulary, and the evidence does not show the source meant that. The 1.1 letter table is also unverified (see blockers).

## Review scope and evidence

- Native fields for all 2,834 Lysias I nodes: form, lemma, postag, relation, head, artificial.
- Native tagset and guidelines: TAGSETS.xml; 2.0 and 2.5 guideline markdown; 1.1 PDF §3.11–3.12 and §4.3 (extracted with `scripts/extract_pdf_text.py`). The PDF extractor recovers the English text. Greek in the PDF is not recoverable this way.
- Cross-corpus UD evidence: 13,919 sentences and 202,989 tokens joined to AGDT v2.1 by sentence id and exact form sequence. Aggregate counts only.
- Provenance: release-level human annotation with Morpheus assistance; annotator Vanessa Gorman; normalization Celano; annotation date 2014. The release README notes the adverb/particle distinction is unsettled and that noun POS errors remain for missing values.
- Not done: I did not review every lemma or POS against LSJ or a second annotator. The syntax-level review does not establish lemma or POS correctness. The UD release does not contain Lysias I (no tlg0540 sent ids). UD is cross-corpus evidence only.

## Remaining blockers

1. Copula εἰμί VERB vs AUX: 41 nodes. Needs a decision on whether copula is VERB or AUX under the Greek scheme.
2. Hyphenated compound-verb headwords: 7 nodes, 5 identities. Needs a headword-decoding rule or an explicit "raw headword" policy.
3. Particle `d` vs `g` (259 nodes): needs the 1.1 morphology letter table, which is not verified here, or an accepted repo policy.
4. Accent-variant splits (4 groups) and grave-accent keys (10 nodes): needs a policy on identity folding. ADR 0094 currently keeps them distinct.
5. Capitalized lemmas as vocabulary (17 nodes): a policy question about proper names.
6. R08/R09 conjunction UPOS (94 nodes): affects UPOS identity only, not vocabulary eligibility.

## Reproducibility

- `scripts/reproduce.sh <scratch-dir>` fetches the pinned inputs, verifies hashes and commits, and runs `scripts/test_mapping.py`.
- `scripts/derive_evidence.py` regenerates `manifest/` from pinned inputs. `test_mapping.py` checks byte-identical regeneration against the committed manifest.
- Results: see the test output recorded in the issue comment.
