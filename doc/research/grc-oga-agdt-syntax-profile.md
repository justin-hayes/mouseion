# AGDT syntax profile of the pinned OGA v0.2.0 files

**Research date:** 2026-11-23

**Ticket:** #2598 (map #2597, Ancient Greek syntax and construction identification)

**Scope:** Is the AGDT syntax in the pinned `oga_src` v0.2.0 files complete and
regular enough to be Mouseion's Ancient Greek syntax source, and what would
storing it faithfully require? This is a structural profile of the files. It
makes no claim about parse accuracy, which is #2600.

## Sources (pinned)

- Data: `mgbilby/oga_src` tag `v0.2.0` = commit
  `223d2258745ecf35e7659191080f93640fe900c1`, files under
  `https://raw.githubusercontent.com/mgbilby/oga_src/v0.2.0/data/`. Every file
  was downloaded and checked: its size and SHA-256 equal `internal/classical/layers.json`.
- oga_src README (same tag): `https://github.com/mgbilby/oga_src/blob/v0.2.0/README.md`.
  It documents the MISC dialect (`<own id>|<head id>|ref=...`, ellipsis ids
  `e_#`) and links the AGDT guidelines.
- AGDT 2.0 guidelines: `PerseusDL/treebank_data` at
  `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`,
  `AGDT2/guidelines/Greek_guidelines.md`
  (`https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/AGDT2/guidelines/Greek_guidelines.md`).
  Sections used: 2 (postag), 3.1-3.19 (labels), 3.20 (suffixes).
- UD practice for AGDT postags: `UniversalDependencies/UD_Ancient_Greek-Perseus`
  tag `r2.19` = `84daf01a020539d6e1e44bb286b8f6352719d869`,
  `grc_perseus-ud-test.conllu`. Its XPOS carries the AGDT postag, so it shows how
  UD maps each postag letter to FEATS.
- Mouseion code: origin/main `45ffbcfdfc4cfada440934d3b735abbfc2aed8a7`:
  `internal/ancientgreek/ogasrc.go`, `proto/mouseion/v1/normalized_corpus.proto`,
  `migrations/000001_initialize.up.sql`, `internal/ancientgreek/testdata/*.conllu`.
- Measurement: `doc/research/grc-oga-agdt-syntax-profile/measure.py` and
  `tables.py` (both ASCII-only). Run with
  `python3 -I measure.py out.json <files...>` then `python3 -I tables.py out.json`.
  The scripts print counts, labels, POS letters and sentence ids only.

## Answer

- **The corpus is 7 Works, not 8.** `layers.json` and `manifest.jsonl` name 7
  distinct OGA files: Xenophon *Anabasis* plus 6 Plato dialogues (*Apology*,
  *Crito*, *Phaedo*, *Symposium*, *Meno*, *Republic*). The 22 Reading units
  belong to these 7 Works. The map (and this ticket) wrongly said 8 Works and
  "Plato x7".
- **Verdict: native AGDT is usable with stated normalisation.** No work is
  unusable. The trees are structurally clean in all 7 works:
  - every head resolves within its sentence, with no self-heads and no cycles;
  - token ids are gap-free;
  - the MISC head reference agrees with HEAD on all 251,406 tokens;
  - 14,975 of 14,978 sentences have exactly one root.
- **No `_CO` or `_AP` suffixes** occur in any work, which confirms the
  charting finding. APOS sits mostly on punctuation (358 of 420), which is the
  guidelines' second (AGDT 1.0) apposition style but without its `_AP` suffix.
- **A conjunct's function is on its own label.** The members of a COORD carry
  their role directly (an OBJ under a COORD is an object conjunct). A nested
  COORD takes its members' label. Without `_CO`, conjuncts can't be told from
  dependents shared by the whole coordination:
  - uniform member labels: 7,108 COORDs (59%);
  - a majority label plus others, probably shared dependents: 2,470 (20%);
  - ambiguous: 1,272 (10%);
  - fewer than 2 members (for example a sentence-linking conjunction): 1,298 (11%).

  A construction rule that needs "the conjuncts" must define this rule and
  accept its ambiguity, or abstain on mixed COORDs (which fits precision-first).
- **The normalisation required:**
  1. Repair labels outside AGDT 2.0 sections 3.1-3.19. There are 23 tokens in
     all:
     - `AuxΖ` (capital Greek Zeta for Z): 4 tokens, Anabasis only, maps to `AuxZ`;
     - `AuxV` (auxiliary or periphrastic copula, an AGDT 1.0 / Latin label): 18
       tokens in Crito, Phaedo, Symposium and Republic, kept or mapped;
     - `_`: 1 token, Phaedo, stored as unknown.
  2. Treat a comma as a transparent coordinator. 1,906 sentences (12.7%) have a
     comma (AuxX) as root, and 98% of those sit directly over a COORD. Across
     the corpus, 2,152 AuxX tokens head content.
  3. Accept multi-root sentences. There are 3 in the source (Phaedo sent_id
     747, Symposium 216, Republic 3654, each over 190 tokens), and many more if
     ellipsis is dropped (item 3 under Ellipsis).
  4. Remap morphology (see Morphology below).
  5. Decide how ellipsis is stored (below). This is the only choice that isn't
     mechanical.
- **Deviations from the guidelines, all corpus-wide:**
  - AuxK never hangs on the virtual root; it attaches to the root token. The
    guidelines (3.16) say it is "always appended to the ROOT node".
  - Ellipsis nodes use POS `-`, where the guidelines say `x`.
  - 2,788 sentences have more than one PRED. 93% of the non-root PREDs sit
    under a COORD, as coordinated main clauses.
- **Per-work deviations are of degree, not kind.**
  - Meno (25%), Crito (28%) and Phaedo (7%) have many PRED-less sentences
    rooted in ExD, from speaker labels and vocatives.
  - Ellipsis density ranges from 2.2% of kept tokens headed by an ellipsis node
    (Anabasis) to 12.1% (Meno).
  - Only Anabasis has `AuxΖ`; only Phaedo has `_`.
- **Charting finding 3 is partly wrong.**
  - Participle and infinitive *are* distinguishable in today's stored
    morphology. A participle has no Mood plus Case; an infinitive has neither
    Mood nor Case (nor Person). Over all 44,548 verbs this rule separates them
    exactly, apart from 4 anomalous postags.
  - What is dropped is **VerbForm** (never mapped), **Mood for non-finite
    forms** (`p` participle 10,186, `n` infinitive 9,067), **perfect tense**
    (`r`, 2,562 verbs), future perfect (`t`, 2) and **common gender** (`c`,
    1,298 tokens). So constructions that need the perfect tense, or an explicit
    VerbForm, still need a new interpretation that remaps morphology.
- **Non-projectivity is high.** 13.4% of arcs are non-projective ignoring
  punctuation, and 61.7% of sentences have at least one such arc.
  Constructions must be defined over heads, not contiguous spans, and learner
  highlighting must allow discontinuous spans.

## 1. Relation inventory

- The corpus uses 19 of the 20 labels in AGDT 2.0 sections 3.1-3.19. `MWE` is
  never used. With the 3 non-guideline strings that makes 22 distinct DEPREL
  values. The full per-work table is below.
- The charting count for Crito ("21 labels") is 20: the 19 guideline labels in
  Crito plus `AuxV`.
- The relative frequencies are stable across works. ATR, ADV and OBJ lead
  everywhere.

## 2. Head completeness

- Every head resolves within its sentence, with no self-heads, no cycles and no
  id gaps, in all 7 works.
- 14,975 of 14,978 sentences have exactly one root (HEAD 0); 3 sentences have
  two. Root labels:
  - PRED: 11,823
  - AuxX: 1,906
  - ExD: 597
  - COORD: 594
  - other: 61
- **Non-projective arcs:**
  - 17.4% counting all nodes;
  - 13.4% ignoring punctuation (both as dependent and as an intervening node);
  - 16.1% over kept tokens after the climb rule.

  By work, the rate ignoring punctuation runs from 10.6% (Meno) to 14.9%
  (Republic).
- **Contract mismatch.** `Token.head` in `normalized_corpus.proto` (lines 80-84
  on main) is a 0-based kept-token ordinal with self-headed roots. Its
  `dependency` field is documented as "the token's basic Universal
  Dependencies relation". Storing AGDT labels there either contradicts that
  comment or needs a label-scheme marker; the persistence decision must choose
  one. `corpus_tokens` (`head bigint >= 0`, `dependency <> ''`) can hold AGDT
  labels and multiple self-headed roots as it stands.
- **Ordinal shift.** 1,015 ellipsis nodes sit inside a sentence rather than at
  its end, so dropping them renumbers the kept tokens after them. The
  conversion must map source id to kept ordinal per sentence and never reuse
  the 1-based HEAD directly.

## 3. Ellipsis (`[0]`, POS `-`) and ExD

- There are 6,595 ellipsis nodes (2.6% of tokens). As PRED: 3,810; as root:
  2,797. 5,580 sit at the sentence end, before the final punctuation, so their
  linear position is artificial. Their postag is always `---------`.
- **17,255 kept tokens (7.0%) have an ellipsis node as head.** In 121 cases the
  chain is two ellipsis nodes deep.
- Kept children per ellipsis node: 0 kept children for 484 nodes, 1 for 1,383,
  2 or more for 4,728.
- Every ellipsis node has a stable document-wide id (`e_#`) in MISC.
- **Re-attachment rules measured:**
  - *Climb rule* (attach each kept token to its nearest kept ancestor, root if
    none) keeps every kept label and gives a tree over kept tokens. It yields
    **2,793 sentences (18.6%) with more than one root**, which the self-headed
    root contract can represent. It loses the information that the tokens
    were co-dependents of an unprinted verb, so a construction over an elided
    predicate (for example a clause whose verb is elided under an AuxC) is
    misread.
  - *Promotion rule* (UD style: the best-ranked kept non-Aux child takes the
    ellipsis node's place) gives a unique choice for 5,391 ellipsis nodes. 360
    nodes give a tie and 360 have only Aux children. It also rewrites the
    promoted token's relation, so it is not faithful.
  - **Recommendation:** store the ellipsis nodes as non-occurrence syntax nodes
    keyed by `e_#`, so the source tree is kept exactly and the climb rule is a
    derived view. If the schema must stay token-only, use the climb rule and
    mark the re-attached arcs "via elided node", then abstain on constructions
    that run through one. This feeds the "Ellipsis and artificial nodes"
    open question on the map.
- **ExD:** 2,150 tokens in all: 2,148 kept and 2 ellipsis nodes. They are
  vocatives, parentheticals and the speaker labels of the dialogues, which are
  sentences made of a speaker abbreviation plus punctuation. 597 sentences are
  rooted in ExD. Speaker-label sentences carry no construction and should be
  excluded explicitly.

## 4. COORD / APOS / AuxC / AuxP shapes

- **COORD** (12,148 tokens):
  - Head POS: conjunction 11,068, adverb 693, particle 270, punctuation 116.
  - Content children: 2 for 6,701 COORDs, 3 for 2,767, 4 or more for 1,382, 1
    for 1,080, none for 218.
  - Member labels: PRED 5,937, OBJ 5,867, ADV 5,470, SBJ 3,942, ATR 2,702, PNOM
    1,701, COORD 1,264 (nested), AuxP 871, AuxC 756.
  - Recursive function recovery: see the Answer section.
- **APOS** (420 tokens): on punctuation 358, noun 42, ellipsis 9. Content
  children: 2 for 226, 1 for 98, none for 53. This is the comma-headed
  apposition style without `_AP`, so the noun and its appositive are the two
  children, each with its sentence function.
- **AuxC** (7,312 tokens):
  - Head POS: conjunction 7,185, adverb 77, pronoun 27, preposition 20.
  - 6,325 have exactly one content child; 630 have none; 357 have 2 or more.
  - Child labels: ADV 4,831, OBJ 1,040, COORD 906, SBJ 195.
  - The child is a finite verb in 4,547 cases: indicative 2,612, subjunctive
    1,148, optative 784, imperative 3. It is an infinitive in 280 and a
    participle in 36. It is an **ellipsis node in 1,198 (16%)** and a COORD
    over coordinated clauses in 831.
  - Most frequent lemmas, transliterated: ei, hoti, epei, hos, ean.

  So a subordinate-clause construction needs:
  - the head mood, read through COORD;
  - an ellipsis policy (one AuxC clause in six has no printed verb);
  - the AuxC lemma.

  The AuxC/SCONJ mapping already uses the AuxC label (`mapUPOS`).
- **AuxP** (9,899 tokens):
  - Head POS: preposition 9,857, adverb 39.
  - 9,599 have exactly one content child.
  - Child labels: ADV 6,562, OBJ 1,577, ATR 712, COORD 516, PNOM 440.
  - Child case: accusative 3,551, genitive 3,144, dative 2,392, none 842
    (articular infinitives, adverbs and the like).

  The prepositional phrase therefore has a clean shape: the AuxP heads its
  object, and the phrase's function sits on that object.

## 5. Morphology needed by constructions

- Postag mood position (5th character) on verbs:
  - indicative `i` 20,028
  - participle `p` 10,186
  - infinitive `n` 9,067
  - subjunctive `s` 2,236
  - optative `o` 2,224
  - imperative `m` 803
  - anomalous: `g` 1, `-` 3
- Tense position (4th character):
  - present `p` 25,016
  - aorist `a` 8,720
  - imperfect `i` 5,933
  - perfect `r` 2,562
  - future `f` 2,150
  - pluperfect `l` 157
  - future perfect `t` 2
- **The current mapping `oga-agdt-feats/1`** (`ogasrc.go`, `featureMappings`)
  drops these values (they are counted, not refused):
  - Mood: `p`, `n`
  - Tense: `r`, `t`
  - Gender: `c`

  It never emits VerbForm or Aspect. Overall it drops mood `p` 10,188 times,
  mood `n` 9,067, tense `r` 2,563, tense `t` 2 and gender `c` 1,298. These
  counts include 3 non-verb tokens.
- **Derivable FEATS, following UD Perseus r2.19 practice:**
  - VerbForm: `Fin` for mood i/s/o/m, `Part` for p, `Inf` for n.
  - Aspect: `Perf` for tense r and t (UD gives `Tense=Past|Aspect=Perf` for r,
    `Tense=Fut|Aspect=Perf` for t), and `Imp` for i (`Tense=Past|Aspect=Imp`).
  - UD Perseus gives aorist `Tense=Past` with no Aspect. The current mapping
    already writes `Past` for aorist and `Imp` as Tense for the imperfect, so
    the remap should also settle UD Tense/Aspect alignment.
- **Participle vs infinitive in stored morphology today:**
  - AGDT `p` (10,186 verbs): no Mood, has Case, no Person.
  - AGDT `n` (9,067 verbs): no Mood, no Case, no Person.
  - Finite verbs: Mood and Person present, no Case.

  The anomalies are 4 tokens (`-` or `g` mood). The distinction is therefore
  recoverable by rule, but it is implicit, and the perfect tense is not.

## 6. Fixture drift

The committed `internal/ancientgreek/testdata/oga-src-*.conllu` fixtures use
`DET` (78 tokens) and `PUNCT` (41 tokens) as DEPREL. These are UD labels and
appear in no pinned file. The real inventory never has `DET`: articles are ATR.
The fixtures also have 2 to 8 roots per sentence in 5 of 9 files, while the
real data has one root (3 exceptions). Any syntax fixture should be re-cut from
the real shapes: a single root, AuxK under the root token, a comma root over a
COORD, ellipsis nodes with kept dependents, and AuxC over an ellipsis node.

## Per-work tables

The tables are generated by `tables.py` from `measure.py` output and contain
counts only. `AuxΖ` is the label written with capital Greek Zeta.

### Size and tree integrity

| Work | Sentences | Tokens | Kept | Ellipsis [0] | Unresolved/self head | Cycle sents | MISC head mismatch | ID gaps | 1-root sents | Other-root sents | Max sent len |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Xen. Anab. | 3813 | 65022 | 64400 | 622 | 0 | 0 | 0 | 0 | 3813 | 0 | 116 |
| Pl. Ap. | 483 | 10417 | 10235 | 182 | 0 | 0 | 0 | 0 | 483 | 0 | 145 |
| Pl. Crito | 373 | 5256 | 5130 | 126 | 0 | 0 | 0 | 0 | 373 | 0 | 133 |
| Pl. Phd. | 1435 | 27071 | 26368 | 703 | 0 | 0 | 0 | 0 | 1434 | 1 | 259 |
| Pl. Symp. | 1021 | 20850 | 20414 | 436 | 0 | 0 | 0 | 0 | 1020 | 1 | 198 |
| Pl. Meno | 1492 | 13456 | 12812 | 644 | 0 | 0 | 0 | 0 | 1492 | 0 | 81 |
| Pl. Rep. | 6361 | 109334 | 105452 | 3882 | 0 | 0 | 0 | 0 | 6360 | 1 | 249 |
| All | 14978 | 251406 | 244811 | 6595 | 0 | 0 | 0 | 0 | 14975 | 3 | 259 |

### Root label and PRED placement

| Work | Root=PRED | Root=AuxX | Root=COORD | Root=ExD | Root=other | Sents 0 PRED | Sents >1 PRED | PRED not root | ...of which under COORD | AuxK on virtual root |
|---|---|---|---|---|---|---|---|---|---|---|
| Xen. Anab. | 2884 | 682 | 233 | 0 | 14 | 8 | 935 | 2167 | 2114 | 0 |
| Pl. Ap. | 368 | 85 | 18 | 5 | 7 | 9 | 127 | 287 | 263 | 0 |
| Pl. Crito | 203 | 53 | 12 | 104 | 1 | 104 | 69 | 159 | 155 | 0 |
| Pl. Phd. | 1124 | 162 | 51 | 96 | 3 | 104 | 220 | 530 | 490 | 0 |
| Pl. Symp. | 820 | 156 | 34 | 4 | 8 | 11 | 236 | 536 | 493 | 0 |
| Pl. Meno | 1019 | 83 | 18 | 369 | 3 | 371 | 105 | 236 | 217 | 0 |
| Pl. Rep. | 5405 | 685 | 228 | 19 | 25 | 46 | 1096 | 2444 | 2205 | 0 |
| All | 11823 | 1906 | 594 | 597 | 61 | 653 | 2788 | 6359 | 5937 | 0 |

### Projectivity (arcs)

| Work | Non-proj arcs (all nodes) | Sents with any | Non-proj arcs ignoring punct | Non-proj arcs, kept tokens after climb |
|---|---|---|---|---|
| Xen. Anab. | 15.9% | 70.3% | 11.6% | 15.3% |
| Pl. Ap. | 18.0% | 72.0% | 13.6% | 16.6% |
| Pl. Crito | 16.9% | 42.1% | 12.5% | 15.8% |
| Pl. Phd. | 16.9% | 59.2% | 13.0% | 15.5% |
| Pl. Symp. | 18.5% | 73.1% | 13.6% | 17.2% |
| Pl. Meno | 13.4% | 30.6% | 10.6% | 12.5% |
| Pl. Rep. | 18.7% | 63.0% | 14.9% | 16.8% |
| All | 17.4% | 61.7% | 13.4% | 16.1% |

### Ellipsis

| Work | Ellipsis | % tokens | as PRED | as root | Kept tokens headed by ellipsis | % kept | 2-deep chains | Ell. 0 kept kids | 1 kid | >=2 kids | Sents >1 root after climb | Promotion unique | Promotion tie | Only Aux kids | At sentence end |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Xen. Anab. | 622 | 1.0% | 311 | 130 | 1445 | 2.2% | 2 | 40 | 183 | 399 | 129 | 514 | 53 | 15 | 539 |
| Pl. Ap. | 182 | 1.7% | 86 | 44 | 408 | 4.0% | 3 | 23 | 62 | 97 | 44 | 138 | 9 | 12 | 147 |
| Pl. Crito | 126 | 2.4% | 75 | 51 | 325 | 6.3% | 2 | 10 | 32 | 84 | 51 | 91 | 8 | 17 | 104 |
| Pl. Phd. | 703 | 2.6% | 403 | 305 | 2006 | 7.6% | 16 | 59 | 128 | 516 | 305 | 530 | 55 | 59 | 593 |
| Pl. Symp. | 436 | 2.1% | 225 | 152 | 1229 | 6.0% | 14 | 35 | 99 | 302 | 151 | 358 | 28 | 15 | 367 |
| Pl. Meno | 644 | 4.8% | 510 | 468 | 1556 | 12.1% | 1 | 24 | 76 | 544 | 468 | 555 | 14 | 51 | 582 |
| Pl. Rep. | 3882 | 3.6% | 2200 | 1647 | 10286 | 9.8% | 83 | 293 | 803 | 2786 | 1645 | 3205 | 193 | 191 | 3248 |
| All | 6595 | 2.6% | 3810 | 2797 | 17255 | 7.0% | 121 | 484 | 1383 | 4728 | 2793 | 5391 | 360 | 360 | 5580 |

### Relation inventory (kept tokens; ellipsis in brackets)

| DEPREL | Xen. Anab. | Pl. Ap. | Pl. Crito | Pl. Phd. | Pl. Symp. | Pl. Meno | Pl. Rep. | All |
|---|---|---|---|---|---|---|---|---|
| ATR | 10860 [20] | 1424 [4] | 681 [2] | 3973 [13] | 3070 [10] | 1475 [4] | 15714 [70] | 37197 [123] |
| ADV | 9376 [204] | 1368 [65] | 709 [30] | 3823 [151] | 2940 [126] | 1406 [92] | 14379 [664] | 34001 [1332] |
| OBJ | 8681 [69] | 1400 [23] | 628 [11] | 2933 [103] | 2381 [54] | 1527 [26] | 11985 [810] | 29535 [1096] |
| AuxY | 5717 | 948 | 443 | 2608 | 1925 | 1161 | 11091 | 23893 |
| SBJ | 5382 [14] | 841 [4] | 349 [8] | 2207 [30] | 1631 [19] | 910 [11] | 7663 [116] | 18983 [202] |
| AuxX | 3586 | 764 | 393 | 2202 | 1705 | 855 | 9540 | 19045 |
| PRED | 4740 [311] | 569 [86] | 287 [75] | 1251 [403] | 1131 [225] | 745 [510] | 5649 [2200] | 14372 [3810] |
| AuxK | 3813 | 483 | 373 | 1436 | 1021 | 1492 | 6361 | 14979 |
| COORD | 2839 | 521 | 271 | 1277 | 1066 | 467 | 5707 | 12148 |
| AuxZ | 1927 | 596 | 339 | 1325 | 917 | 766 | 4956 | 10826 |
| AuxP | 3205 | 300 | 142 | 1007 | 801 | 328 | 4116 | 9899 |
| PNOM | 1697 [1] | 372 | 140 | 972 | 821 [2] | 585 | 4014 [18] | 8601 [21] |
| AuxC | 2054 | 384 | 162 | 832 | 637 | 433 | 2810 | 7312 |
| ExD | 135 | 129 | 175 | 338 [2] | 153 | 556 | 662 | 2148 [2] |
| OCOMP | 104 | 34 | 5 | 33 | 44 | 32 | 312 | 564 |
| AuxG | 62 | 52 | 9 | 62 | 97 | 38 | 191 | 511 |
| APOS | 58 [3] | 34 | 16 | 64 [1] | 32 | 21 [1] | 186 [4] | 411 [9] |
| ATV | 98 | 8 | 2 | 11 | 17 | 6 | 55 | 197 |
| AtvV | 62 | 8 | 4 | 10 | 23 | 9 | 50 | 166 |
| AuxV | 0 | 0 | 2 | 3 | 2 | 0 | 11 | 18 |
| Aux\u0396 | 4 | 0 | 0 | 0 | 0 | 0 | 0 | 4 |
| _ | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 1 |

### Suffixes and non-guideline labels

| Work | _CO | _AP | Labels outside AGDT 2.0 3.1-3.19 |
|---|---|---|---|
| Xen. Anab. | 0 | 0 | Aux\u0396 4 |
| Pl. Ap. | 0 | 0 | none |
| Pl. Crito | 0 | 0 | AuxV 2 |
| Pl. Phd. | 0 | 0 | AuxV 3, _ 1 |
| Pl. Symp. | 0 | 0 | AuxV 2 |
| Pl. Meno | 0 | 0 | none |
| Pl. Rep. | 0 | 0 | AuxV 11 |

### COORD / APOS / AuxC / AuxP shapes (all works)

- coord_upos: c 11068, d 693, g 270, u 116, v 1
- coord_content_children: 2 6701, 3 2767, 4 1382, 1 1080, 0 218
- coord_child_rel: PRED 5937, OBJ 5867, ADV 5470, SBJ 3942, ATR 2702, PNOM 1701, COORD 1264, AuxP 871, AuxC 756, ExD 186, OCOMP 137, APOS 58, ATV 8, AtvV 7
- coord_conjunct_labels: same 8013, mixed 4135
- apos_upos: u 358, n 42, - 9, a 6, p 2, v 2, l 1
- apos_content_children: 2 226, 1 98, 0 53, 3 43
- auxc_upos: c 7185, d 77, p 27, r 20, a 3
- auxc_content_children: 1 6325, 0 630, 2 312, 3 45
- auxc_child_rel: ADV 4831, OBJ 1040, COORD 906, SBJ 195, ExD 47, ATR 26, AuxC 22, AuxP 11, PNOM 8, PRED 1
- auxc_child_mood: i 2612, pos:- 1198, s 1148, pos:c 831, o 784, n 280, pos:d 97, pos:n 40, p 36, pos:a 30, pos:r 11, pos:p 8, pos:u 4, m 3, pos:g 3, pos:l 1, pos:m 1
- auxp_upos: r 9857, d 39, c 3
- auxp_content_children: 1 9599, 2 169, 0 126, 3 5
- auxp_child_rel: ADV 6562, OBJ 1577, ATR 712, COORD 516, PNOM 440, ExD 122, APOS 16, AuxC 5, OCOMP 4, AuxP 1
- auxp_child_case: a 3551, g 3144, d 2392, - 842, n 25, v 1
- verb_mood: i 20028, p 10186, n 9067, s 2236, o 2224, m 803, - 3, g 1
- verb_tense: p 25016, a 8720, i 5933, r 2562, f 2150, l 157, - 8, t 2
- unmapped_today: mood=p 10188, mood=n 9067, tense=r 2563, tense=t 2, mood=g 1
- postag_voice: - 206855, a 32066, e 8465, m 3051, p 969
- postag_gender: - 149855, m 48457, n 30557, f 21239, c 1298
- postag_case: - 149556, a 37023, n 30900, g 18640, d 13744, v 1543
- postag_degree: - 248733, c 1517, s 1156
- upos_letters: v 44548, d 40164, u 35007, n 29000, a 21544, p 21251, c 21075, l 20100, r 9896, - 6595, g 1088, i 936, m 201, x 1
- coord_mixed_patterns: PRED*2+SBJ*1 174, COORD*1+PRED*1 173, ADV*1+OBJ*1 110, OBJ*1+SBJ*1 94, ATR*1+OBJ*2 88, AuxC*1+PRED*2 83, COORD*1+OBJ*1 69, OBJ*2+SBJ*1 68, ADV*1+COORD*1 59, ADV*1+PRED*2 41, ADV*1+OBJ*2 39, COORD*1+PRED*2 37, ADV*2+ATR*1 37, ADV*1+ATR*1 37, ADV*1+PRED*2+SBJ*1 32, ATR*1+OBJ*1 31, ATR*1+SBJ*2 30, PNOM*1+SBJ*1 25, AuxC*1+PRED*2+SBJ*1 23, ExD*1+PRED*2 22, PRED*3+SBJ*1 20, AuxP*1+PRED*2 19, ADV*2+SBJ*1 19, ADV*1+PNOM*1 12, ADV*2+OBJ*1 10, ADV*1+SBJ*2 9, ADV*1+SBJ*1 6, COORD*1+SBJ*1 3, OBJ*3+SBJ*1 3, ADV*1+OBJ*3 3, ADV*2+COORD*1 3, ATR*2+SBJ*1 3, PNOM*2+SBJ*1 3, ADV*1+ExD*1+PRED*2 2, AuxC*1+COORD*1+SBJ*1 2
- coord_resolved: uniform 7108, majority+others(shared-dependent?) 2470, fewer-than-2-members 1298, ambiguous 1272
- coord_nested: 1264

### Verb mood and tense per work (postag positions 5 and 4)

| Work | ind i | subj s | opt o | imp m | inf n | ptcp p | pres p | impf i | fut f | plpf l | perf r | aor a | futpf t |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Xen. Anab. | 6327 | 440 | 750 | 157 | 2302 | 3251 | 5792 | 2586 | 610 | 80 | 508 | 3650 | 1 |
| Pl. Ap. | 870 | 75 | 100 | 46 | 430 | 397 | 1209 | 101 | 103 | 5 | 164 | 336 | 0 |
| Pl. Crito | 340 | 92 | 35 | 47 | 211 | 184 | 583 | 47 | 73 | 1 | 51 | 153 | 0 |
| Pl. Phd. | 1991 | 273 | 230 | 71 | 1098 | 1038 | 2821 | 467 | 169 | 22 | 370 | 851 | 0 |
| Pl. Symp. | 1544 | 177 | 184 | 94 | 896 | 812 | 2237 | 326 | 136 | 20 | 212 | 775 | 0 |
| Pl. Meno | 1044 | 97 | 118 | 68 | 422 | 297 | 1397 | 99 | 65 | 7 | 186 | 292 | 0 |
| Pl. Rep. | 7912 | 1082 | 807 | 320 | 3708 | 4207 | 10977 | 2307 | 994 | 22 | 1071 | 2663 | 1 |
| All | 20028 | 2236 | 2224 | 803 | 9067 | 10186 | 25016 | 5933 | 2150 | 157 | 2562 | 8720 | 2 |

