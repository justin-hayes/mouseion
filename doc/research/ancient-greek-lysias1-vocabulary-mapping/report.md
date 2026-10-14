# Lysias I: AGDT → canonical vocabulary mapping (#2049), completion pass

Scope: interpreting the vocabulary of AGDT v2.1 Lysias I (`tlg0540.tlg001.perseus-grc1`, `treebank_data@bf4334f0`, sha256 `44ecf3a2…a568`). This covers producer component representation, UPOS, citation headwords, canonical keys, discriminators and lemma/POS review lineage.

Not covered here: text authority, independent witnesses, source variants and occurrence binding. Those belong to #2048. The component appendix below reports producer conventions only. It does not assign source coordinates.

This pass supersedes the conditional figures at [9531fc98](https://github.com/justin-hayes/mouseion/commit/9531fc98c1d7a301ef2128115717e0f471259ea0), including the 498-resolved-identity and 1,432-resolved-occurrence aggregates.

- Rule set: `mapping-spec.md`.
- Per-node manifest: `manifest/lysias1-nodes.tsv`.
- Evidence registry: `manifest/lysias1-evidence.tsv`, with pinned revision, path, line and sha256 for each item.

## Result

Every one of the 2,834 nodes has a rule. That is 2,802 printed nodes plus 32 `artificial="elliptic"` nodes. Full node classification is **not** the same as identity readiness. Statuses propagate across UPOS, headword, eligibility and discriminator together. A node's identity is `resolved` only when all four are resolved.

| Printed nodes (2,802) | Count |
| --- | --- |
| Eligible: every UPOS candidate is in `AllowedPOS` | 1,163 (NOUN 311, ADJ 243, ADV 92, VERB 517) |
| Eligibility conditional: candidates span vocabulary and non-vocabulary UPOS | 232 |
| Non-vocabulary: no candidate is in `AllowedPOS` | 1,407 |

Vocabulary nodes by identity status (eligible + conditional = 1,395):

- **1,145 resolved.**
- **249 conditional:**
  - 232 with conditional eligibility.
  - 15 nodes from homographs that v2.1 merged under one key.
  - 2 with a conditional headword.
- **1 unresolved.**

Identities: **465 resolved** (each has at least one resolved occurrence), **33 conditional-only** and **1 unresolved**. Two resolved identities also have conditional occurrences: `καί|ADV` (10 resolved, 13 conditional) and `νῦν|ADV` (2 resolved, 1 conditional). Their conditional rows are counted separately in `lysias1-identities.tsv`.

Source scope: the 5 rubric label words (sentences 78, 82, 84, 90, 121: Νόμος ×3, Μάρτυρες ×2) are kept and labelled `rubric_label` (rule R01R). They are not excluded. Whether rubric text is authoritative is #2048's question.

## 1. Producer representation of components (appendix: `manifest/lysias1-components.tsv`)

| Convention | Nodes | Finding | Status |
| --- | --- | --- | --- |
| C-ELISION | 50 | At each of these node ids, the v1.6 file has U+02BC and v2.0/v2.1 have U+0313. No other character changes. The v2.0 README records a "checked/normalized" pass on Greek characters, and the v2.1 header names v1.6 as the pre-normalization text. NFC composes U+0313 with the preceding letter in 5 forms (e.g. `παῤ`), so after NFC the elision mark can no longer be told apart from a breathing. All 50 lemmas are full citation forms. | Mark mapping supported; no vocabulary effect |
| C-CRASIS | 22 = 11 pairs | Consonant fragment + `-` (κ-, τ-), then a host carrying the remainder (ἀγὼ, ἀληθῆ, ἆλλα, ἀκεῖνος, οὔπισθεν). The pinned v2.1 corpus has 91 such fragments in 11 files, and the v1.6 forms are identical. Neither the 1.1 guidelines (English text), the 2.0 nor the 2.5 guidelines mention crasis. Five host nodes are eligible vocabulary (ἀληθής ×3, ἄλλος, ὄπισθεν), and their identities do not depend on the split. | Attested practice, undocumented |
| C-ENCLITIC | 4 (s011 w025/26, w028/29) | `-τε` then `μή` for μήτε. Elsewhere in the pinned corpus, hyphen-initial enclitics come *after* their host (6 cases in 2 files, e.g. οὔ + -τ̓). Lysias I is the only file where the enclitic comes first. | Unsupported order exception; no documented rule |

String concatenation is not used as proof of component spans anywhere here.

## 2. Compound lemmas (7 nodes, 5 identities)

Morpheus prints a lexicon analysis as `preverb-lemma_of` (`prntanal.c`). Its compound-lemma builder is disabled ("no longer called as of 6/7/88", `checkpreverb.c`). A `rpb:` key marks a preverb inside the lexeme's own stem (`morphkeys.c`). Decoding uses Morpheus's own stemlib, and the raw lemma is kept in every case.

| Raw lemma (nodes) | Headword | Rule | Status | Evidence |
| --- | --- | --- | --- | --- |
| κατά-καθεύδω (2) | καθεύδω | H1 | decoded | `vbs.simp.ml:9786` `:vs:kaqeud … rpb:kata/` under `:le:kaqeu/dw` |
| διά-διαιτάω (1) | διαιτάω | H1 | decoded | `vbs.simp.ml:5240` `rpb:dia/` under `:le:diaita/w` |
| ἀμφί-ἀμφισβητέω (1) | ἀμφισβητέω | H1 | decoded | `vbs.simp.ml:1660` `pb:a)mfi/` under `:le:a)mfisbhte/w` |
| ἀντί-βολέω (2) | ἀντιβολέω | H3 | conditional | `βολέω` has no preverb. The only entry with `pb:a)nti/` + stem `bol ew_denom` is `:le:a)ntibole/w` (`vbs.simp.ml:1966`). This is a decomposition match, not a printed or tabled decoding. |
| σύν-λέγω (1) | none | H4 | unresolved | The v1.6 lemma is `σύν-λέγω1`. Morpheus's compound table has only `su/n-le/gw2 sulle/gw` (`vbs.cmp.ml:6459`). The homograph numbers disagree, so συλλέγω is recorded as a candidate, not decoded. |

## 3. UPOS

- **Native letters.** AGDT 1.7's README and the v2.1 TAGSETS both define `d` = adverb and `g` = particle as separate letters. This closes the earlier "1.1 letter table unverified" gap. Lysias I uses `d` on all 259 AuxY/AuxZ particles.
- **Copula (εἰμί, 52 printed nodes).** The rule applies when εἰμί has an *effective* PNOM dependent. "Effective" means looking through COORD/APOS members and AuxP/AuxC, as HamleDT's `detect_prague_copula` does.
  - The rule rests on AGDT 1.1/2.0 PNOM ("dependent on a copulative verb"), UD `cop`/`AUX` ("verbal copulas are tagged AUX") and the validator data (`grc` εἰμί `cop.AUX`, status documented).
  - 49 nodes → **AUX, resolved, non-vocabulary**. The old heuristic found 41; the traversal adds 8.
  - 1 AuxV → AUX.
  - 2 have no PNOM (2:22, 119:6) → **VERB|AUX conditional**. UD allows existential "be" to be a copula, and UD Perseus tags εἰμί AUX in almost every use. That is cross-corpus practice.
  - Other verbs with a PNOM (γίγνομαι, ἡγέομαι, τυγχάνω) stay VERB, because UD says "become" verbs are not copulas and the `grc` aux list contains only εἰμί and ἔχω.
- **Particles (259).**
  - 10 καί + AuxZ → ADV, resolved. AGDT AuxZ means "even/also", and UD grc PART excludes "adverbs such as καί 'even'".
  - 27 repeated-coordinator correlatives → CCONJ, resolved: καί…καί, οὔτε…οὔτε, ἤ…ἤ, τε…τε, and the first `-τε` of μήτε…μήτε. Basis: AGDT 1.1 §3.11, under which non-head coordinators take AuxY under the final coordinator.
  - The remaining 222 are conditional, with candidates recorded per group:
    - R03N negators (32): ADV|PART.
    - R03C AGDT conjunction particles not used as correlatives (70): ADV|CCONJ.
    - R03A γάρ/οὖν/τοίνυν (46): ADV|CCONJ|PART. AGDT 2.0 §2.8/§3.9 treats these as sentence adverbs.
    - R03S ὡς (2): ADV|PART|SCONJ.
    - R03P others (72): ADV|PART.
  - Why they stay conditional: UD grc defines PART lexically, from Smyth's list. Smyth §2769 says particles "fluctuated in function, being now adverbial, now conjunctional". AGDT annotates them as adverbs. Existing converters (Interset, UD Perseus) map `d` → ADV tag-literally. No documented rule maps these native records to a UD class.
  - Grave lemmas (καὶ, μὴ) do not match lists that spell the lemma with an acute, and get no acute/grave folding.
- **Conjunctions.**
  - COORD → CCONJ (121).
  - AuxC with an effective verbal dependent → SCONJ (80).
  - 11 AuxC without one → SCONJ|CCONJ|ADP conditional. Most of these head an unprinted elliptic clause node that has no POS.
  - 3 other relations (ATR, AuxP) → CCONJ|SCONJ unresolved.
  - None of these affect vocabulary eligibility.
- **Names (n with a capitalized lemma).**
  - 9 nodes (Ἐρατοσθένης ×6, Σώστρατος ×2, Ἁρμόδιος) are `:le:` entries in Morpheus's proper-name stem file `nom.proper`. They map to **PROPN, resolved, non-vocabulary**: PROPN is not in `AllowedPOS`, which is a fact about the repo, not a policy choice.
  - Ἑλλάς, Θεσμοφόρια and Ἄρειας ×2 are not in `nom.proper`: NOUN|PROPN conditional. Πάγος/πᾶγος in Ἀρείου πάγου is a possible part of a multiword name: NOUN|PROPN conditional.
  - Ἀθηναῖος ×2: UD grc NOUN names it as a substantivized adjective, and AGDT 2.0 §4.1.2 says the same of an adjective lemma used as a noun. That gives ADJ|NOUN|PROPN, conditional.
  - The 5 rubric nouns are NOUN. UD: words in phrases used as names keep their original tags.
  - Ὀῆθεν is native `d` → ADV. Its Morpheus `nom.proper` entry is a `:wd: adverb`.
  - HamleDT's capitalization heuristic (capitalized noun lemma → PROPN) is recorded as converter practice and **not adopted**. Morpheus's entity list classes Νόμος as a person, which shows that lexicon membership alone does not classify a token.

## 4. Identity fields

Each node carries these fields separately:

- native lemma
- citation headword and its status and rule
- canonical key (#2033 profile: NFC → lowercase → σ → NFC; diacritics kept; Python unicodedata version in the summary)
- native letter and POS name
- UPOS candidates and status
- discriminator scheme, value and status
- rule and evidence IDs

**Discriminator.**

- v2.1 lemmas carry no discriminator. The v1.6 file named in the v2.1 header as the pre-normalization text has Morpheus homograph numbers on 83 lexical lemmas, and the v2.0 normalization stripped them, following AGDT 2.0's own instruction ("they should be deleted").
- Statuses: `absent_in_source` (2,345), `stripped_in_v2.1_witness_v1.6` (68) and `conflated` (15).
- `conflated` means the v2.1 key|UPOS merges different v1.6 entries:
  - δέω|VERB: δέω1 ×1, δέω2 ×2
  - λέγω|VERB: λέγω2 ×5, λέγω3 ×1
  - ἄπειμι|VERB: ἄπειμι1 ×1, ἄπειμι2 ×5, i.e. "be away" versus "go away"
- These 15 nodes are conditional, not resolved. A numbered value is recorded as witness evidence and is not written into the key automatically.

Headword cases:

- `εἴσειμι` (6) and `συνδειπνέω` (4) are source-plain headwords. A missing dictionary page is not a blocker.
- Adverbs lemmatized under their adjective (καλός, οἰκεῖος) follow AGDT 2.0 §2.6 ("the lemma for an adverb should be the corresponding adjective lemma").

## 5. Review lineage of the consumed lemma/POS layers (`manifest/lysias1-lineage.json`)

- **File-level claims.** "Vanessa Gorman: annotator of the text" (annotation date 2014-08-21). "Celano: release editor: post-annotation normalization and harmonization." The v2.0 release describes the annotation as semi-automatic, with Morpheus assisting lemmas and morphology.
- **v1.6 → v2.1 changes.**
  - POS letters: none changed.
  - Lexical lemmas: 83 homograph numbers stripped, no other lexical lemma changes.
  - Punctuation lemmas: 375 levelled.
  - Postag features: 138 position changes, 126 of them in position 9.
  - Forms: 50 elision-mark changes and 9 NFC-only changes.
- **What this shows.** The consumed POS letters and lemma strings are the annotator's, apart from documented normalization.
- **What it does not show.** No source statement says every lemma/POS was human-verified per token. Lysias I is not among the double-annotated AGDT2 texts (only Aesop and Pseudo-Apollodorus are). Syntax review is not lemma/POS review.
- **UD.** UD Perseus's "converted from manual" applies only to its own subset. That subset contains no Lysias I sentence (verified by sent_id), even though its README lists the speech.
- **Bottom line.** Lineage is human annotation, Morpheus-assisted, at file scope. Whether that meets the MVP's human-reviewed bar is the parent's call (#2030). This pass does not certify it.

## Remaining gaps (exact)

1. **222 particles, conditional.**
   - Eligibility depends on whether ADV or PART/CCONJ/SCONJ applies.
   - There is no documented native→UD rule. UD grc PART is defined against Smyth's list, and the pass inspected only §2769.
   - Full Smyth §2769–3003 membership was not checked.
2. **Copula εἰμί without PNOM: 2 nodes,** VERB|AUX.
3. **Names: 10 nodes.** Ἑλλάς, Θεσμοφόρια, Ἄρειας ×2, πάγος/πᾶγος, Ἀθηναῖος ×2; no producer name evidence. Not in Morpheus `nom.proper`. Other Morpheus noun files were not checked.
4. **Headwords.** ἀντί-βολέω ×2 is conditional, decoded by decomposition only. σύν-λέγω ×1 is unresolved: the homograph number does not match the compound table.
5. **Discriminator conflation: 15 nodes / 3 identities** (δέω, λέγω, ἄπειμι). Applying v1.6 numbers would combine two releases, and needs an explicit selection rule (ADR 0094 #12).
6. **Components.** No documented crasis convention (22 nodes). The s011 enclitic order contradicts the attested practice (4 nodes).
7. **Conjunction UPOS: 14 non-vocabulary nodes** (11 conditional, 3 unresolved). No effect on eligibility.

## Reproducibility

- **Run:** `scripts/reproduce.sh <new-dir>` fetches every input at its pinned commit and checks it against a sha256 list. That covers the AGDT v1.6/v2.0/v2.1 Lysias files and 32 Morpheus/UD/HamleDT/Interset files.
- **Regenerate:** `scripts/derive_evidence.py` rebuilds `manifest/`.
- **Tests:** `scripts/test_mapping.py` runs 23 tests: unit tests, source-backed fixtures in `fixtures/lysias1-expected.tsv`, status-propagation invariants, and byte-identical regeneration.
- **Smyth §2769:** cited from the Perseus website (fetched 2026-10-14, not pinned). No code uses it.
