# OGA v0.2.0 parser accuracy and prior art for Greek construction detection

**Research date:** 2026-11-23

**Ticket:** [#2600](https://github.com/justin-hayes/mouseion/issues/2600), on the
map [#2597](https://github.com/justin-hayes/mouseion/issues/2597).

**Scope:** How accurate is the automatic syntax in OGA v0.2.0? What prior art
exists for rule-based identification of Greek constructions over a dependency
treebank, and what accuracy did it report? Where do rule-based detectors fail
on parser output, as opposed to gold trees? Primary sources are the papers,
the repositories at pinned commits, and a local measurement of the pinned
`oga_src` files against Gorman gold trees.

**Licence constraint (#2602):** this file and its script contain no copied OGA
relations, heads or Greek text. They hold aggregate figures, an ASCII-only
script and OGA occurrence IDs (`t_N`, the document-wide token IDs in the
`oga_src` MISC column). Greek grammatical words are transliterated.

**Evidence bar:** every detection figure below is **interim**. It measures
agreement between OGA's parser output and the Gorman gold trees, with the
same rule run on both. It is not a check against Smyth, and the gold trees are
themselves single-annotator work. All three measured texts are in the parser's
training data (section 2.1), so **the agreement overstates real accuracy** on
the catalog Works that have no gold trees (Phaedo, Symposium, Meno, Republic).

## Answer

- **Published parser figures bound detector accuracy at about UAS 0.83 and
  LAS 0.77.** OGA v0.2.0's syntax is Trankit trained on AGDT-scheme data, with
  test-set UAS 82.60 and LAS 77.10 for the production run (arXiv 2410.12055,
  Table 4; the OGA v0.2.0 README repeats them). The cross-validation mean is
  UAS 82.28 / LAS 76.67. The paper reports **no per-relation, per-genre or
  per-author figures**.
- **Plato is harder than Xenophon.** No OGA figure exists by author. For a
  comparable AGDT-scheme parser, Keersmaekers & Van Hal (2023, Table 8) report
  LAS 0.790 for philosophical dialogue, 0.751 for philosophy and science, and
  0.825 for history. Locally, OGA agrees with Gorman gold at LAS 0.722 on
  *Apology*, 0.814 on *Crito* and 0.857 on *Anabasis* 1 and 3 (all training
  texts; section 2).
- **Known weak spots:** coordination, heads through elliptic nodes, ADV vs OBJ,
  and SBJ vs OBJ for the accusative of an infinitive. Locally, coordination
  members and coordinators score LAS 0.69 pooled. Tokens whose gold head is
  elliptic get an elliptic OGA head only 0.69 of the time. ADV to OBJ is the
  most frequent confusion in *Crito* and *Anabasis*, and OBJ to ADV in
  *Apology*. Keersmaekers & Van Hal show the same confusions and add that
  shared modifiers of conjuncts attach correctly only 30.7% of the time.
- **Interim construction agreement, pooled over the three texts (OGA rule
  versus the same rule on gold):**

  | Construction | Gold | OGA | Both | Precision | Recall |
  | --- | ---: | ---: | ---: | ---: | ---: |
  | Conditional protasis (ei / ean) | 284 | 285 | 270 | 0.947 | 0.951 |
  | Result clause (hoste) | 38 | 38 | 36 | 0.947 | 0.947 |
  | Articular infinitive | 76 | 71 | 69 | 0.972 | 0.908 |
  | Purpose clause (hina / hopos / hos + subj./opt.) | 48 | 52 | 44 | 0.846 | 0.917 |
  | Genitive absolute | 55 | 66 | 50 | 0.758 | 0.909 |
  | Accusative + infinitive | 198 | 213 | 156 | 0.732 | 0.788 |

  The conditional, result and articular-infinitive rules clear the provisional
  0.90 bar on agreement. The genitive absolute clears it on *Crito* (0.909) and
  *Anabasis* (0.892, just under), but falls to 0.389 on *Apology*. The
  accusative + infinitive misses the bar on every text (0.62 to 0.82).
- **Reusable published definitions exist for the genitive absolute (Vierros &
  Yordanova 2022) and for complement vs adverbial clause typing over parser
  output (Keersmaekers 2021).** The AGDT 2.0 guidelines give the
  representation for every construction on the list, with Smyth sections
  (section 4). No published precision/recall exists for any of these
  constructions on Classical literary prose parsed by OGA.

### Findings that contradict or refine the map's assumptions

1. **The catalog's measurable texts are training texts.** *Apology* (10,457
   tokens), *Crito* (5,093) and *Anabasis* (18,737) are listed in arXiv
   2410.12055's Appendix A. Phaedo, Symposium, Meno and Republic are not, and
   have no public gold trees, so no agreement figure is available for 4 of
   the 7 catalog Plato Works. Because the production model was trained on a
   60% split and never retrained on train plus validation (section 4 of the
   paper), part of each training text was unseen, but which part is not
   published.
2. **The AGDT relation layer does not encode clause type.** A subordinate
   clause is an AuxC conjunction heading a verb labelled ADV (adverb clause)
   or OBJ/SBJ (substantive clause). Purpose, result and condition differ only
   in the conjunction's lemma and the verb's mood. The guidelines put the
   finer types in the Smyth (SG) layer, which OGA does not output. Detectors
   must therefore be lexical plus morphological plus relational, and stored
   morphology needs mood (finding 3 on the map already says VerbForm is
   missing).
3. **OGA elliptic nodes carry no part of speech.** They have form `[0]` and
   POS `-`, while Gorman's elliptic nodes are typed verbs. A conditional whose
   verb is elided is recognisable on OGA only through the ADV label on the
   elliptic node. Before the detector accepted elliptic ADV nodes, conditional
   recall on *Crito* was 0.79. The ellipsis handling therefore affects
   construction recall, not only display.
4. **Lemmas differ from gold.** OGA lemmatises contracted *ean* (AuxC) as
   *an*. A detector keyed on gold lemma lists silently loses those clauses.
5. **The genitive-absolute false positives in *Apology* are lexical.** All 11
   OGA-only hits there are genitive participles that gold treats as OBJ or
   PNOM. Their gold heads are verbs of hearing (*akouo* 3, *akroaomai* 1),
   *kai* coordinators (6), and *eimi* (1). This is the supplementary participle after verbs of hearing and
   perceiving (SG 2110-2115, which the AGDT guidelines cite for OBJ and OCOMP
   participles), not a parser accident. Excluding participles whose head, looked through COORD, is a
   verb of perception should recover most of the precision. That is a design
   hypothesis, not measured.
6. **The accusative + infinitive is limited by SBJ vs OBJ, not by heads.**
   Most disagreements are infinitives where one tree labels the accusative SBJ
   and the other OBJ. Keersmaekers & Van Hal (footnote k) name the same cause,
   because the subject of an infinitive is in the accusative.

## 1. OGA v0.2.0 parser: published figures

Sources:

- Celano, "A State-of-the-Art Morphosyntactic Parser and Lemmatizer for
  Ancient Greek", arXiv 2410.12055v1, 2024-10-15
  (<https://arxiv.org/abs/2410.12055>).
- The OGA README at tag `v0.2.0`, commit
  `ca3528bc65907014af88580c2793fe684caddcd7`
  (<https://github.com/OperaGraecaAdnotata/OGA/blob/ca3528bc65907014af88580c2793fe684caddcd7/README.md>).

Facts:

- **Training data:** AGDT, Gorman Trees and Pedalion Trees, 1,277,310 tokens
  before normalisation, split 60/20/20 for train, validation and test. Five-fold
  cross-validation, each split trained twice. Scored with the CoNLL 2018
  shared-task script.
- **Normalisation that matters downstream:** the `_CO` and `_AP` suffixes were
  deleted. Elliptic nodes were moved to the end of the sentence as
  placeholders `[0]`, `[1]` and so on, following Celano's 2023 ellipsis
  modelling. Crasis was left untouched. *oude* and *eite* are tokenised as two
  words.
- **Table 2 (mean F1 with SD over 10 runs):**

  | Model | POS | XPOS | Feats | UAS | LAS |
  | --- | ---: | ---: | ---: | ---: | ---: |
  | Dithrax | 95.55 | 90.65 | 94.40 | 77.70 | 70.81 |
  | Trankit | 96.18 | 91.55 | 94.61 | 82.28 | 76.67 |
  | GreBERTa | 94.12 | 89.16 | 93.21 | 58.85 | 53.41 |

- **Production run (Table 4; the README):** POS 96.41, XPOS 91.90, Feats
  94.77, UAS 82.60, LAS 77.10, Lemmas 91.41 (GreTa).
- **No per-relation, per-genre or per-author breakdown** appears in the paper
  or the README.
- **The parser predicts head and relation only.** The ellipsis nodes in OGA
  come from a separate model (footnote 10,
  `git.informatik.uni-leipzig.de/celano/ellipsis_Ancient_Greek`; Celano 2023,
  ICNLSP). I did not read that paper, so its accuracy is not recorded here.
- **The predecessor, OGA v0.1.0, was weaker.** It used COMBO trained on AGDT
  2.1, at UAS 71.47 and LAS 63.55 (arXiv 2404.00739, Table 1).

### Comparable parser: Keersmaekers & Van Hal (2023)

Source: Keersmaekers and Van Hal, "Creating a large-scale diachronic corpus
resource: Automated parsing in the Greek papyri (and beyond)", *Natural
Language Engineering* 30(5): 1035-1064, doi
[10.1017/S1351324923000384](https://doi.org/10.1017/S1351324923000384). The
parser is the Stanford graph-based parser on AGDT-scheme data, with
punctuation excluded.

- **Literary LAS by genre (Table 8):** religion 0.881; biography 0.832;
  history 0.825; oratory 0.822; narrative 0.804; non-philosophical dialogue
  0.798; **philosophical dialogue 0.790**; philosophy and science 0.751;
  poetry 0.740. The papyri reach 0.845.
- **Per-relation label accuracy (section 5.2, papyri):** ATR 0.94, AuxC 0.95,
  AuxP 0.99, AuxY 0.95, AuxZ 0.88, conjuncts 0.92, OBJ 0.92, PRED 0.96, SBJ
  0.87 (8% confused with OBJ), and **ExD 0.38**. 9% of adverbials were labelled
  OBJ.
- **Coordination:** LAS for coordination members was 0.684 under the Perseus
  style, and 0.833 after their transformation (section 5.1.7). Shared
  modifiers attach correctly 30.7% of the time.
- **Ellipsis:** tokens that depend on an elliptic node had label accuracy
  0.379 (section 5.1.8).
- **Error audit of 500 errors (Table 9):** 55% are true grammatical mistakes,
  26% are consistency issues in the training data, and 10% are errors in the
  gold annotation.
- **Inter-annotator agreement on AGDT** (Bamman et al. 2009, as cited in their
  footnote j): heads 0.874, relations 0.853, both 0.806. This is the ceiling
  that any "agreement with gold" figure sits under.
- Mambrini & Passarotti (2012) report a best AGDT LAS of 0.717. I could not
  fetch the paper, so this figure is taken from Keersmaekers & Van Hal and is
  secondary.

## 2. Local measurement: OGA versus Gorman gold

### 2.1 Inputs (pinned)

- **OGA:** `oga_src` tag `v0.2.0`, commit
  `223d2258745ecf35e7659191080f93640fe900c1`, files `tlg0059.tlg002`
  (*Apology*), `tlg0059.tlg003` (*Crito*) and `tlg0032.tlg006` (*Anabasis*).
  The SHA-256 of each file matches `internal/classical/layers.json`.
- **Gold:** `vgorman1/Greek-Dependency-Trees`, commit
  `d823e96d253277bc1872b83ff5ac8c393a0f830a`, `xml versions/`:
  `Plato_Crito_Travis_Kahl.xml`, `plato apology.xml`,
  `Xen_Anab_book_1.1-5.xml`, `Xen_Anab_book_1.6-9.xml` and
  `Xen_Anab_book_3.xml`.
- **Training overlap:** arXiv 2410.12055 Appendix A lists *Apology* 10,457,
  *Crito* 5,093 and *Anabasis* 18,737 tokens. Those counts are consistent
  with these Gorman files (for example, 18,490 real tokens plus elliptic nodes
  for *Anabasis*).
- **Method:** the script is
  [`compare_oga_gold.py`](grc-parser-accuracy-construction-prior-art/compare_oga_gold.py)
  (ASCII-only).
  - Real tokens are aligned by normalised form with `difflib`; diacritics,
    case and apostrophes are folded.
  - Elliptic nodes are excluded from scoring and from alignment.
  - Gold `_CO`, `_AP` and similar suffixes are stripped, because OGA was
    trained without them.
  - Punctuation is excluded from UAS, LA and LAS.
  - A token whose gold head is an elliptic node counts as head-correct when
    the OGA head is also elliptic. This is lenient, because elliptic nodes
    cannot be aligned to each other.
- **Run:** `python3 -I compare_oga_gold.py --ids OGA.conllu GOLD.xml...`

### 2.2 Tree agreement

| Text | Aligned (of gold) | Scored | UAS | LA | LAS | Coordination LAS | Elliptic-head agreement |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| *Crito* | 98.1% | 4,158 | 0.871 | 0.914 | 0.814 | 0.718 | 0.743 (208/280) |
| *Apology* | 98.3% | 8,730 | 0.787 | 0.863 | 0.722 | 0.578 | 0.601 (221/368) |
| *Anabasis* 1, 3 | 98.8% | 16,151 | 0.893 | 0.938 | 0.857 | 0.754 | 0.719 (337/469) |
| Pooled | | 29,039 | 0.858 | | 0.810 | 0.693 | 0.686 (766/1117) |

### 2.3 LAS per gold relation (relations with at least 20 tokens)

| Relation | *Crito* | *Apology* | *Anabasis* |
| --- | ---: | ---: | ---: |
| PRED | 0.934 | 0.794 | 0.930 |
| ATR | 0.901 | 0.791 | 0.934 |
| AuxP | 0.847 | 0.752 | 0.891 |
| SBJ | 0.852 | 0.729 | 0.852 |
| OBJ | 0.831 | 0.718 | 0.866 |
| AuxC | 0.812 | 0.633 | 0.840 |
| ExD | 0.813 | 0.727 | 0.739 |
| PNOM | 0.803 | 0.690 | 0.779 |
| AuxY | 0.897 | 0.783 | 0.876 |
| AuxZ | 0.754 | 0.666 | 0.734 |
| ADV | 0.724 | 0.706 | 0.828 |
| COORD | 0.605 | 0.541 | 0.617 |
| OCOMP | | 0.543 | 0.741 |
| AtvV / ATV | | 0.208 | 0.568 |

COORD labels are right 93% to 97% of the time. The low LAS comes from the
coordinator's head. AuxC labels are right 96% to 99% of the time, but the
conjunction's head is often wrong.

The most frequent label confusions (gold to OGA) are:

- *Crito*: ADV to OBJ 57, AuxZ to AuxY 31, ADV to ATR 30.
- *Apology*: OBJ to ADV 110, OBJ to SBJ 73, ADV to OBJ 66.
- *Anabasis*: ADV to OBJ 143, OBJ to ADV 69, SBJ to OBJ 64.

### 2.4 Construction agreement (interim)

Each detector is anchored on one token and runs identically on both trees. A
construction counts as "both" when the aligned anchor fires in both.
Precision is the share of OGA detections that gold also has, and recall is
the share of gold detections that OGA also has. The detector definitions are:

- **Genitive absolute:** a genitive participle labelled ADV, with a genitive
  SBJ child. The child may be found through COORD, or be the SBJ of a COORD
  that heads coordinated participles. This follows Vierros & Yordanova's
  types 1 to 3.
- **Accusative + infinitive:** an infinitive with an accusative SBJ child,
  found through COORD.
- **Articular infinitive:** an infinitive with an article child.
- **Conditional protasis:** an AuxC with lemma *ei*, *ean*, *en* or *an* that
  heads an ADV verb or an ADV elliptic node.
- **Purpose clause:** an AuxC with lemma *hina*, *hopos* or *hos* that heads
  an ADV subjunctive or optative.
- **Result clause:** an AuxC with lemma *hoste* that heads an ADV verb or an
  ADV elliptic node.

| Construction | *Crito* P / R | *Apology* P / R | *Anabasis* P / R |
| --- | --- | --- | --- |
| Genitive absolute | 0.909 (10/11) / 0.909 (10/11) | 0.389 (7/18) / 0.875 (7/8) | 0.892 (33/37) / 0.917 (33/36) |
| Accusative + infinitive | 0.758 (25/33) / 0.893 (25/28) | 0.622 (51/82) / 0.785 (51/65) | 0.816 (80/98) / 0.762 (80/105) |
| Articular infinitive | 1.000 (16/16) / 1.000 (16/16) | 1.000 (19/19) / 0.760 (19/25) | 0.944 (34/36) / 0.971 (34/35) |
| Conditional protasis | 0.911 (51/56) / 0.962 (51/53) | 0.935 (101/108) / 0.918 (101/110) | 0.975 (118/121) / 0.975 (118/121) |
| Purpose clause | 0.833 (5/6) / 1.000 (5/5) | 0.786 (11/14) / 0.917 (11/12) | 0.875 (28/32) / 0.903 (28/31) |
| Result clause | 0.800 (4/5) / 1.000 (4/4) | 0.938 (15/16) / 0.938 (15/16) | 1.000 (17/17) / 0.944 (17/18) |

The pooled figures are in the Answer. Small counts (fewer than 20 gold
occurrences) make per-text precision unstable. One disagreement on *Crito*'s 5
purpose clauses moves precision by 0.17.

**These are agreement figures, not correctness figures.** A disagreement can
be a gold error, since Keersmaekers & Van Hal found 10% of "errors" were gold
mistakes, and the detector can be wrong identically on both trees.
Agreement on training texts overstates accuracy on unseen Plato. The figure
the map's bar applies to comes from the Smyth-checked review of about 150
occurrences.

### 2.5 Occurrence IDs of disagreements

These are OGA `t_N` IDs in the pinned files, listed for the live review. FP
means OGA only, and FN means gold only.

- ***Crito*** (`tlg0059.tlg003`)
  - Genitive absolute: FP t_2054; FN t_3182.
  - Accusative + infinitive: FP t_819 t_1226 t_3114 t_3135 t_3190 t_3380
    t_4120 t_4243; FN t_1228 t_4242 t_4915.
  - Conditional: FP t_518 t_1473 t_2447 t_3511 t_4718; FN t_2268 t_3063.
  - Purpose: FP t_2843.
  - Result: FP t_3509.
- ***Apology*** (`tlg0059.tlg002`)
  - Genitive absolute: FP t_928 t_953 t_4064 t_4068 t_6067 t_6393 t_8483
    t_8900 t_8904 t_8907 t_8909; FN t_6350.
  - Accusative + infinitive: FP t_1427 t_2373 t_2376 t_2429 t_2493 t_3852
    t_3858 t_4374 t_4654 t_5172 t_5818 t_6351 t_7419 t_7459 t_7539 t_7676
    t_7716 t_7989 t_7997 t_8012 t_8142 t_8215 t_8274 t_8648 t_8723 t_9241
    t_9509 t_9562 t_9575 t_9942 t_10022; FN t_3724 t_3805 t_4164 t_4199
    t_4239 t_4415 t_4656 t_4939 t_7549 t_7643 t_7645 t_9122 t_9428 t_9671.
  - Articular infinitive: FN t_2272 t_4722 t_6110 t_6123 t_7676 t_9137.
  - Conditional: FP t_1008 t_4840 t_5390 t_6452 t_7360 t_7424 t_9876; FN
    t_940 t_1245 t_4272 t_6651 t_7056 t_7403 t_7433 t_7436 t_7476.
  - Purpose: FP t_324 t_2075 t_4966; FN t_1566.
  - Result: FP t_4723; FN t_3532.
- ***Anabasis*** (`tlg0032.tlg006`)
  - Genitive absolute: FP t_379 t_2536 t_18133 t_24608; FN t_7880 t_23908
    t_23959.
  - Accusative + infinitive: FP t_2938 t_4311 t_5020 t_5442 t_5714 t_6210
    t_8014 t_8454 t_9228 t_18195 t_19181 t_20090 t_20118 t_21836 t_21850
    t_22108 t_23947 t_23954; FN t_345 t_3109 t_4467 t_5721 t_5837 t_6695
    t_7199 t_9423 t_18245 t_18283 t_18740 t_18818 t_19410 t_19430 t_19887
    t_21572 t_21834 t_21935 t_22520 t_23257 t_23941 t_24632 t_24960 t_25481
    t_25803.
  - Articular infinitive: FP t_24798 t_25132; FN t_22188.
  - Conditional: FP t_3683 t_10380 t_21286; FN t_4364 t_24064 t_24551.
  - Purpose: FP t_103 t_20525 t_21670 t_21762; FN t_19939 t_20202 t_21763.
  - Result: FN t_24667.

## 3. Prior art for construction detection

### 3.1 Vierros & Yordanova (2022): the genitive absolute over AGDT, Gorman, PapyGreek, PROIEL and Duke-nlp

Source: Vierros and Yordanova, "Querying Syntactic Constructions in Ancient
Greek Parsed Corpora: A Case Study on the Genitive Absolute in Literature and
Documentary Papyri", *Classics@* 20
(<https://classics-at.chs.harvard.edu/querying-syntactic-constructions-in-ancient-greek-parsed-corpora-a-case-study-on-the-genitive-absolute-in-literature-and-documentary-papyri/>).

- **Definition (section 3):** the head is a participle in the genitive
  labelled ADV, and its required dependent is a genitive SBJ. "Valid paths"
  allow parent-child, ancestor-descendant, and coordinator-bridged sibling
  configurations, which they call types 1 to 4. The representation is the
  AGDT and Gorman XML, restructured into a hierarchy in Kiln/XSLT.
- **Known gap (note 33):** genitive absolutes without an expressed subject are
  not found, because "a query for a genitive participle marked with ADV
  returns too many results".
- **PROIEL INESS queries (note 51):** for example, type 1 is
  `#x:[pos!="C."] >adv #y:[morph="...p..g.*"] >sub #z:[morph="......g.*"]`.
  This is the UD-adjacent shape.
- **Accuracy:**
  - Gold corpora: a test suite was used, but no precision or recall is
    reported.
  - Automatically parsed Duke-nlp papyri: false positives occur, for example
    genitive attributes of a participle mislabelled SBJ (note 55).
  - Note 57 relays Keersmaekers' estimate of over 90% precision for
    genitive-subject labelling and head attachment, on 55 test-corpus cases.
    That figure is reported second-hand and is not reproducible.
- **Rates on gold (Table 4, Gorman):** genitive absolutes per sentence are
  4.16% in Plato's *Apology*, 1.17% in *Euthyphro* (AGDT) and 7.29% in
  Xenophon. Plato's dialogues are sparse in genitive absolutes, so a Plato
  Reading unit yields few occurrences to measure.

### 3.2 Keersmaekers (2021): complement vs adverbial clauses on parser output

Source: Keersmaekers, *A Computational Approach to the Greek Papyri:
Developing a Corpus to Study Variation and Change in the Post-Classical Greek
Complementation System*, PhD thesis, KU Leuven, 2021
(<https://lirias.kuleuven.be/retrieve/590983>), chapter 6, Tables 39 to 41
(printed pp. 155-157).

- **Extraction definition:** a verb whose head is a complementiser that itself
  depends on a verb. For infinitives and participles, every infinitive or
  participle that depends on a verb.
- **Head attachment on parser output (papyri, Table 40):**

  | Clause type | Precision | Recall |
  | --- | ---: | ---: |
  | Infinitive | 0.986 | 0.951 |
  | Participle | 0.970 | 0.957 |
  | *hoti* | 0.892 | 0.910 |
  | *hina* | 0.860 | 0.888 |
  | *hos* | 0.932 | 0.932 |
  | *hopos* | 0.902 | 0.925 |

  Errors cluster on long dependencies (8.1 words on average against 3.5) and
  on non-finite heads (35% of errors against 15% of correct cases).
- **Labelling complement vs adverbial (Table 41):** infinitive complements
  reach precision 0.979 and recall 0.994, and *hoti* complements 0.972 and
  0.878. Both are close to the always-complement baseline. Participle, *hina*,
  *hos* and *hopos* reach a combined precision of 0.651 and recall of 0.444.
  The ADV/OBJ label is a weak signal for clause type, so he trained a separate
  classifier on semantic and distance features.
- **Relevance:** the domain is the papyri, not Classical prose. This is the
  only published per-construction P/R on parser output found.

### 3.3 Others checked

- **Perseus/Arethusa and the AGDT 2.0 guidelines:** these give the
  representations, not detectors (section 4). No accuracy figures.
- **OGA ANNIS queries:** the README points to a `query` folder with ANNIS
  examples, which I did not inspect. Celano's papers report no construction
  detection figures.
- **GLAUx and DendroSearch** (Keersmaekers et al. 2019, *Creating, enriching
  and valorising treebanks of Ancient Greek*, SyntaxFest/TLT,
  <https://aclanthology.org/W19-7812.pdf>): these describe a query tool, with
  no construction precision/recall.
- **PROIEL:** queries for the genitive absolute only, via Vierros & Yordanova.
  No accuracy figures.
- **Harrington Trees:** these appear only as a treebank (c. 18K tokens of
  post-Classical prose, in Keersmaekers & Van Hal's Table 1). Keersmaekers &
  Van Hal suspect their automatic conversion explains the low LAS of 0.664 on
  Lucian. No construction work was found.
- **Pedalion Trees:** a treebank (c. 300K tokens), part of OGA's training
  data. No construction detection work was found.

## 4. Reusable definitions per construction (AGDT representation)

The source is the AGDT 2.0 guidelines, `PerseusDL/treebank_data` commit
`bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`, `AGDT2/guidelines/Greek_guidelines.md`
(last changed in `0ebc9289`, 2024-06-19). SG means Smyth's *Greek Grammar*.

| Construction | AGDT representation | SG | Published detector |
| --- | --- | --- | --- |
| Genitive absolute | Circumstantial participle labelled ADV. The genitive subject attaches to the participle as SBJ ("no difference in their annotation" from other circumstantial participles) | 2070-2075 | Vierros & Yordanova 2022, types 1-4 |
| Accusative + infinitive | Infinitive SBJ/OBJ of a verb. The accusative subject attaches to the infinitive as SBJ | 1972-1981 | Keersmaekers 2021 (infinitive complements) |
| Articular infinitive | Infinitive with the article as a dependent, labelled as a noun would be | 2025-2034 | none found |
| Conditional clause | AuxC (*ei*, *ean*) heading an ADV verb (adverb clause). *ei* heading OBJ is a substantive "whether" clause (SG 2247) and must be excluded | 2280-2368 | none found |
| Purpose clause | AuxC (*hina*, *hopos*, *hos*) heading an ADV verb in the subjunctive or optative. The infinitive of purpose is ADV | 2193-2206; 2008-2010 | Keersmaekers 2021 (*hina*, *hopos*, *hos*, papyri) |
| Result clause | AuxC *hoste* heading an ADV verb, either indicative or infinitive. The infinitive of result is ADV | 2249-2278; 2011 | none found |

## 5. Where rule-based detectors fail on parser output

This draws on the local measurement and the sources above.

1. **Relation labels that carry the construction.** ADV vs OBJ decides
   genitive absolute vs supplementary participle. SBJ vs OBJ decides whether
   an accusative is the subject of an infinitive. These are the most frequent
   confusions both locally and in Keersmaekers & Van Hal. Heads are more
   reliable than labels for these clauses (Keersmaekers 2021).
2. **Coordination.** Coordinator heads are right only about 60% of the time
   (COORD LAS 0.54 to 0.62). Detectors that walk through COORD inherit those
   errors, and shared subjects of coordinated participles are fragile.
3. **Ellipsis.** OGA's untyped elliptic nodes hide the elided verb's mood, so
   only label-based rules survive. About 31% of tokens with an elliptic gold
   head do not get an elliptic OGA head.
4. **Lexical divergence.** Lemma conventions differ (*an* for *ean*).
   Perception verbs take genitive participles that look like genitive
   absolutes.
5. **Genre.** Dialogue and philosophy parse worse (Keersmaekers & Van Hal,
   Table 8; locally, *Apology* scores 0.722 against 0.857 for *Anabasis*).
   Precision should be measured per Work, not only pooled.

## Unverifiable or open claims

1. **No OGA accuracy exists for unseen catalog Works.** Phaedo, Symposium,
   Meno and Republic have no public gold trees, and all measured texts are
   training texts. Only the live review can bound accuracy there.
2. **Which sentences of the training texts were unseen** (the 40% held out
   for validation and test) is not published.
3. **The ellipsis model's accuracy** (Celano 2023, ICNLSP) was not read.
4. **The 0.717 LAS from Mambrini & Passarotti (2012)** is taken from
   Keersmaekers & Van Hal. The original could not be fetched.
5. **The over-90% genitive-absolute precision** in Vierros & Yordanova (note
   57) is a personal communication relayed second-hand.
6. **The perception-verb exclusion** for the genitive absolute (finding 5) is
   a hypothesis that has not been measured.
7. **The OGA `query` folder** (ANNIS examples) was not inspected.
8. **Gold is not ground truth.** AGDT inter-annotator agreement on both head
   and relation is 0.806, and Gorman's files are single-annotator work.
