# Can the ablative absolute meet the construction shipping bar on OLA?

**Research date:** 2026-11-25

**Scope:** Optional ticket of the map *classical Latin in Mouseion*. On the source the corpus grilling chose
(Opera Latina Adnotata v0.2.0, AGDT scheme, stored `native_agdt`), can a detector for the ablative absolute meet
the bar of [ADR 0123](../adr/0123-constructions-are-a-derived-reading.md) (precision ≥ 0.90, point estimate, with a
95% Wilson lower bound ≥ 0.84), and what does the adapter cost? The Greek genitive-absolute detector
(`internal/constructions/detectors.go:209-235`, feature doc
`doc/features/ancient-greek-syntax-and-constructions.md`) is the model.

Primary sources only: the pinned OLA CoNLL-U files (read by range request from the Zenodo archive), UD
Latin-CIRCSE `r2.19`, LDT 2.1 at the pinned `treebank_data` commit, the Greek detector code at `a2317215`, and the
Allen & Greenough section texts (by number only). Only aggregates, IDs, counts and digests are committed. Scripts
are in [`latin-ablative-absolute/`](latin-ablative-absolute/), results in
[`results/`](latin-ablative-absolute/results/).

## Answer

**Not demonstrated; plausible, and likely to straddle the bar the way the Greek genitive absolute did.** No
analyser run by Mouseion is involved; the rule reads OLA's stored AGDT parse.

- **Rule agreement with gold is below the bar when taken strictly** (rule hit on OLA = same rule hit on gold):
  0.82 (32/39, Wilson 0.67–0.91) on LDT prose sentences the OLA model never trained on, and 0.83 (24/29,
  0.65–0.92) on CIRCSE *Germania*. Pooled: **56/68 = 0.82 (0.72–0.90)**.
- **Most of that gap is not parser error.** Read against Allen & Greenough §419, 6 of the 12 non-agreeing hits in
  those two samples are real ablative absolutes that gold annotates differently (a differently attached variant in
  LDT, coordinated second conjuncts in CIRCSE). With the 4 clear false hits and the 2 ambiguous ones read as false
  (agent reading, not Justin's): **62/68 = 0.91 (0.82–0.96)**; reading the ambiguous ones as true: 64/68 = 0.94
  (0.86–0.98). That is the Greek genitive absolute's position (0.91, 0.84–0.95): the point estimate clears 0.90 and
  the lower bound does not yet.
- **Recall** is 0.82 against the gold rule hits in the unseen LDT sentences and 0.75 (24/32) against CIRCSE's
  `advcl:abs` (0.89, 24/27, for the participial ones); three of CIRCSE's eight misses are verbless absolutes
  (A&G §419.a) that the rule does not target.
- **Training-text agreement is overstated, as the Greek OGA figures were**: 0.97 (291/300, 0.94–0.98) over all
  LDT prose sentences, 0.99 (259/261) over the sentences the model trained on.
- **Adapter cost is small once the Latin OLA adapter of [the scheme ticket](latin-scheme-route.md) exists**:
  one detector of about 30 lines plus tests, the `_CO`/`_AP`/`_ExD` reader (without it 18.8% of hits are lost), a
  Latin morphology mapping for `Mood=p`/`Case=b`, and a per-language construction registry. No migration.
- **What would settle it**: a human-checked sample of about 120 hits (see below). A sample of 120 hits
  tolerates up to 11 false hits.

Recommendation for the closing grilling: the ablative absolute does not block a go, and slice 1 should ship
Latin Constructions **fail-closed**. Treat it as the first Latin construction to measure after slice 1, with a
Greek-2607-style sample drawn from the actual prose Works, and keep the rule refinements below as hypotheses
to test, not decisions.

## The rule

Modelled on `detectGenitiveAbsolute`, one rule, no subtypes:

> **Ablative absolute.** Anchor: a printed participle (`Mood=p`) in the ablative (`Case=b`), whose base relation
> (suffixes `_CO`, `_AP`, `_ExD…` stripped) is `ADV`, with a printed SBJ child in the ablative, directly or through
> COORD (the SBJ of a coordinated participle hangs on the COORD node). Extent: the participle's printed subtree.
> No lexical exclusion.

Differences from Greek: the case is ablative; there is no perception-verb exclusion, because Latin perception verbs
take an accusative participle, not an ablative one; the subject can be any part of speech with `Case=b`.
Not detected by design (accepted recall loss, A&G §419.a, §419.b, §419.c): an absolute with a noun or adjective
instead of a participle (*consule* / *invito*), one whose subject is a clause, and one with no subject; an
elided *esse* has no node in OLA's CoNLL-U (23 `-` ellipsis tokens occur in the 15 files used), so the rule cannot see it.
Uses of the construction (time, cause, condition, concession) are A&G §420 and are not subtyped.

### Mapping across schemes

| | AGDT (OLA, LDT) | UD (CIRCSE) |
|---|---|---|
| Anchor | participle, `Mood=p`, `Case=b` | `VerbForm=Part`, `Case=Abl` |
| Role | base `ADV` | `advcl` (gold: `advcl:abs`) |
| Subject | SBJ child with `Case=b` | `nsubj` child with `Case=Abl` |
| Coordination | participle `ADV_CO` under COORD | `conj` of the first member |

CIRCSE has an explicit label, `advcl:abs` (137 across its five Works), which gives gold independent of the rule.
It labels only the first of two coordinated participles; the later ones are `conj`.

## Measurement

### Pins and what each set is

| Input | Pin |
|---|---|
| OLA v0.2.0 CoNLL-U | Zenodo `10.5281/zenodo.15183688`; per-file SHA-256 equals the sibling manifest for all 15 files used (e.g. *Germania* `phi1351.phi002.perseus-lat1` `b644e270…`) |
| CIRCSE | `UD_Latin-CIRCSE` tag `r2.19` = `5fff3a77c4386006e7e0f0db8dd6fd5c0f4d7523`; test, dev, train SHA-256 `f492948f…`, `b9d9b609…`, `56d92508…` |
| LDT 2.1 | `PerseusDL/treebank_data` `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`; six prose files (`phi0448.phi001` Caesar, `phi0474.phi013` Cicero, `phi0631.phi001` Sallust, `phi0972.phi001` Petronius, `phi1348.abo012` Suetonius, `phi1351.phi005` Tacitus *Hist.* 1.1–21) |
| OLA model's held-out sentence lists | `bowphs/latinpipe-evalatin` `conllus/{dev,test}.120.conllu`, SHA-256 `a8d1cebc…`, `129844da…` |

**The HF files are model output, not gold.** Over 2,815 matched non-punctuation tokens from the test file, their
heads and relations equal OLA's released parse exactly (UAS = LAS = 1.0). They are used only as the list of
sentences the model did not train on (the dev list was used for model selection, so test alone is the cleanest
held-out set), matched to LDT sentences by normalised form, as `latin-oga-equivalent` did.

Alignment is a global form-sequence alignment (`difflib`) as in the sibling tickets. In the LDT prose range 300 OLA hits and 299 gold hits align; 6 further OLA hits and 4 gold hits do not, and are
reported in `results/agreement.json` but not counted. *Germania*: all 29 align.

### Rule agreement with LDT gold (AGDT, same rule on gold)

A true hit = the gold tree yields the same rule hit at the aligned anchor.

| Subset (prose) | OLA hits | Agree | Precision (Wilson) | Gold hits | Found | Recall (Wilson) |
|---|---:|---:|---|---:|---:|---|
| All LDT sentences (mostly training text) | 300 | 291 | 0.970 (0.944–0.984) | 299 | 291 | 0.973 (0.948–0.986) |
| Sentences the model trained on | 261 | 259 | 0.992 (0.973–0.998) | 260 | 259 | 0.996 |
| **Held-out, test list** | 18 | 14 | 0.778 (0.548–0.910) | 17 | 14 | 0.824 (0.590–0.938) |
| Held-out, dev list | 21 | 18 | 0.857 (0.654–0.950) | 22 | 18 | 0.818 (0.615–0.927) |
| **Held-out, test + dev** | 39 | 32 | **0.821 (0.673–0.910)** | 39 | 32 | **0.821 (0.673–0.910)** |

Per Work (all LDT sentences): Caesar 18/18, Cicero 9/9, Tacitus 28/28, Sallust 62/66, Petronius 54/57,
Suetonius 120/122. Verse (Virgil, Ovid, Propertius, Phaedrus; not slice 1): 86/92 = 0.93 overall; 10/12 held-out.

### Rule agreement with CIRCSE gold, *Germania* (prose, UD)

OLA (5,733 words) against CIRCSE (5,674 words); 5,486 gold words align.

| Measure | Value |
|---|---|
| OLA rule hits | 29 (all align) |
| Gold `advcl:abs` anchors | 32 (29 participles; 1 noun, 1 determiner, 1 adjective) |
| Precision against `advcl:abs`, strict | 24/29 = 0.828 (0.655–0.924) |
| Same rule applied to CIRCSE's own tree (UD form) | 29 hits; 27 labelled `advcl:abs`, 2 are `conj` second members (both real absolutes) → ceiling ≈ 1.00 on this text |
| Recall, all gold `advcl:abs` | 24/32 = 0.750 (0.579–0.868) |
| Recall, participial gold absolutes (27) | 24/27 = 0.889 |

Germania is not among the model's held-out sentences (neither dev nor test list contains it) and is not in LDT. OLA's
training set is unpublished (`la_giuseppe`), so "unseen" is likely rather than proved.

### False-positive classes and the adjudicated figure

The strict figures count every disagreement as an error, but many disagreements are annotation variants in the
gold. The 12 non-agreeing hits in the two unseen samples (and 2 training-text ones in LDT prose) are
listed by ID in [`results/adjudication.tsv`](latin-ablative-absolute/results/adjudication.tsv). The verdicts are an
agent reading against A&G §419 and are **not** Justin's judgment.

| Class (OLA hit not confirmed by the gold rule) | Held-out LDT | *Germania* | Verdict |
|---|---:|---:|---|
| Real absolute, gold annotates it as a noun-headed or otherwise differently attached variant | 3 | 0 | true hit |
| Real absolute, second member of a coordination that CIRCSE does not label `advcl:abs` | 0 | 3 | true hit |
| Dative plural read as ablative (*-ibus*/*-is* syncretism); the participle is a dative attribute | 2 | 0 | false hit (parser) |
| Preposition + noun + attributive participle (*in* + ablative) | 1 | 0 | false hit (parser, ADV/SBJ for ATR) |
| Attributive participle on an ablative of means (A&G §409) | 0 | 1 | false hit (parser) |
| Ablative of means or manner with an attributive participle (A&G §409, §412), arguably an absolute | 1 | 1 | ambiguous |

| Sample | Strict | Adjudicated, ambiguous = false | Adjudicated, ambiguous = true |
|---|---|---|---|
| Held-out LDT prose (39) | 0.821 (0.673–0.910) | 35/39 = 0.897 (0.764–0.959) | 36/39 = 0.923 (0.797–0.973) |
| *Germania* (29) | 0.828 (0.655–0.924) | 27/29 = 0.931 (0.780–0.981) | 28/29 = 0.966 (0.828–0.994) |
| **Pooled (68)** | **0.824 (0.716–0.896)** | **62/68 = 0.912 (0.821–0.959)** | **64/68 = 0.941 (0.858–0.977)** |

I also read all 32 + 24 agreeing hits' participle, subject, preceding word and governing verb in the two samples; none
is a false hit. This reading is of the hit, not of the whole sentence.

Two of the three clear false-hit classes are parser errors (case syncretism; ATR labelled ADV), so, as in Greek, the
ceiling of the rule on perfect trees is high; the ambiguous class is a property of the construction (the ablative
of means or manner with an attributive participle is the same tree shape as an absolute) and no rule will separate it.

Rule refinement worth testing, not applied: a preposition immediately before the leftmost extent token removes
the *in deficiente porticu* false hit and no true hit in the 68. It would affect 24 of 7,746 hits (0.3%) in the
ten OLA prose files fetched, so it can cost at most 0.3% recall.

### Recall misses

*Germania*, 8 gold absolutes not found: 3 are verbless (noun or adjective anchor, A&G §419.a), 1 has a genitive
participle anchor in the gold (a gold oddity), 4 are participial parse misses (OLA has the participle as ATR, with the
wrong case, or with no ablative SBJ). Held-out LDT, 7 misses: 6 are an ablative participle labelled ADV with no
ablative SBJ child, 1 is a participle under `AuxP`.

### Training-text bias and what this does not measure

- OLA's training lineage is unpublished and contains the whole LDT, so the 0.97 / 0.99 LDT figures overstate
  accuracy, as the OGA figures did. Use only the held-out and *Germania* rows.
- The unseen sample is 39 LDT hits from five Works plus 29 hits from one Work: no Livy, no Cicero speeches, no
  *Metamorphoses*. Per-Work quality varies (all LDT sentences: Sallust 62/66, Petronius 54/57, Suetonius 120/122).
- OLA hit density differs strongly by Work, from 0.5 per 100 words (*Germania*) to 1.9 (Caesar *BG*) and 1.1 (Livy,
  5,533 hits in 518,260 words), so a Work-stratified sample matters.
- A&G citation numbers: §419 (definition and the three notes a–c), §420 (uses), §409 (means), §412 (manner); the
  text of §419, §420, §409 and §412 was read on the Dickinson College Commentaries copy; none is reproduced.

## What a human-checked sample (Greek ticket 2607) would add

- **A judged precision with an interval that can reach the bar.** The strict and adjudicated figures differ by 0.09,
  and 3 ambiguous items alone move the point estimate by 0.03. Only a blind, reviewed judgment settles those,
  with the Greek protocol's blind spot-check for anchoring.
- **Sample size.** At a true point estimate of 0.92 a sample of about 77 hits has Wilson lower bound 0.84; at 0.90
  about 134. 120 hits (the Greek size) tolerate 11 false hits. Draw them stratified by Work from the 80 BY-SA prose
  Works (Caesar, Livy, the speeches, Tacitus, Suetonius), because hit density runs 0.5 to 1.9 per 100 words.
- **Recall on a random sentence read in full**, which agreement against a single gold cannot give (verbless
  absolutes, the coordinated cases, and absolutes OLA fails to label ADV).
- **Separation of parser from rule errors.** Agreement cannot say whether the ambiguous means/manner class is a
  rule or a parse error; a judge can, and can say whether the *in* + ablative refinement is worth shipping.
- **Coverage the gold lacks**: Livy, Cicero and Caesar's other books have no gold in this ticket; a sample of OLA's own
  output on them is the only way to see the Work-level variation.

## Adapter cost

Assuming the Latin OLA adapter of the scheme ticket (CoNLL-U dialect parser, Citation join, suffix reader, `la`
normalisation) is built for the catalogue anyway, the construction-specific increment is:

| Item | Size | Note |
|---|---|---|
| Relation reader: strip `_CO`, `_AP`, `_ExD…` to the base category, keep a conjunct flag | in the scheme ticket's adapter; **required** for this rule | Without it the exact-match rule loses 1,480 of 7,884 hits (18.8%) on ten OLA prose files, mostly `ADV_CO` participles and `SBJ_CO` subjects |
| Latin morphology mapping to `VerbForm=Part`, `Case=Abl` from `Mood=p`, `Case=b` | tens of lines | the detector reads these names |
| `detectAblativeAbsolute` (copy of `detectGenitiveAbsolute`: case `Abl`, no lexical exclusion) | about 30 lines + unit tests | no lexicon, no authored data |
| Per-language construction registry: `detectors`, `All`, `ShippingGate`, `AGDTDetectorVersion`, `ConcordanceConstructionLanguage`, concept note | a few hundred lines of plumbing | Greek constants are global today; the map's charting finding 2 already counts this as catalogue generalisation, not detector cost |
| Golden fixtures (Mouseion-authored Latin trees), release-check counts and judgments | a day of work | mirror of `releasecheck/` |
| Concept note and A&G grammar reference | shape only (ADR 0124) | A&G §419, §419.a–c, §420 |
| Release check | like #2622 over the pinned prose Works | gate decides |

No migration. The detector is a pure reading of `syntax.Tree`, so a failed gate means Latin Constructions simply
stay off.

## Reproduction

- `fetch_ola.py <outdir> <regex>`: pulls one Work's CoNLL-U from the archive by range request (needs
  `rangezip.py` from `doc/research/latin-scheme-route/`; set `RANGEZIP_DIR`).
- `measure.py <data_dir> <out.json> [dump.txt]`: agreement tables above. The optional dump holds sentence forms and is
  scratch-only; never commit it.
- `suffix.py <data_dir> <out.json>`: hit counts with and without the suffix reader, and density.
- `lib.py`: readers (OLA, LDT XML), the AGDT and UD forms of the rule, alignment, Wilson.
- Results: `results/agreement.json`, `results/suffix_and_prevalence.json`, `results/adjudication.tsv`.
