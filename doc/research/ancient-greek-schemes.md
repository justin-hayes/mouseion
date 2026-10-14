# Ancient Greek annotation schemes: comparison and candidate reading units

Research for #2032 under map #2021. Date: 2026-10-14. Baseline: `b262ed0a`. This version supersedes `a7a12f7d`; §0 lists what changed.

This document records evidence. It does not choose an architecture, a priority, a corpus, an MVP, or a normalization policy; those decisions stay with the human reviewer (#2021).

## 0. Corrections to `a7a12f7d`

- **XPOS (Perseus):** UD Perseus XPOS keeps the native 9-character AGDT postag, including `e` (middle or passive). Verified (§2.2).
- **XPOS (PROIEL):** UD PROIEL XPOS holds two-character POS codes (observed values `V-`, `Nb`, `S-`, `Df`, `R-`), not the native morphology string.
- **Worked example:** the earlier PROIEL example ἦν was imperfect, not aorist. The aorist example is now ἐγέννησεν (MATT 1.2, `3saia----i`), and ἦν (MATT 1.18, `3siia----i`) is the imperfect (§2.3).
- **AGDT citation fields:** `subdoc` and `document_id` are sentence attributes. Words carry no citation field (§4).
- **Converter reproduction:** the claim that converter rules reproduce UD PROIEL FEATS for 213,053 token pairs is withdrawn. It was not replicated. The converter rules are quoted as read (§2.3).
- **Counts:** AGDT has 95 distinct relations (not 96). PROIEL native has 29 distinct `#` lemmas on 3,475 tokens (not 7,230). Zero native tokens carry `slash` (not 19,939). The UD PROIEL `LId` count (6,501) is not reconciled with the native `#` count.
- **Crosswalk FORM mismatches:** the 911 raw mismatches all disappear after Unicode NFC normalization (§2.2).
- **Candidate units:** Lysias 1, 14, and 15 have speech-level subdoc ranges, so their reference check is trivial and their evidence is word-level only. Thucydides 1, Herodotus 1, Plutarch *Alcibiades*, and Gorman *Aeschines* 1 are not complete. Plutarch *Lycurgus*, Lysias 23, Polybius 1, and Athenaeus 12 and 13 are complete at leaf level (§6.1). The earlier scope rule could hide missing sections; it now scores the full span of claimed references.
- **Herodotus, PROIEL:** book 1 has 212 distinct cited chapters, max 216; chapters 127–130 are absent (§6.3).
- **Pedalion:** the earlier §4 probe used double-quoted attributes, but Pedalion XML uses single quotes. Its figures are withdrawn (§5).
- **NT completeness:** "other NT books are contiguous by chapter" is withdrawn. A per-book table is in §6.2. Titus is the only book checked against an external verse total.
- **Removed claims:** Trachiniae's 46-line gap, Odyssey book 10's one-line shortfall, and the Aeschylus identification were not re-checked in this pass and are removed. Hebrews is not claimed complete: the file stops at chapter 12.

## 1. Sources and pins

Every claim cites a pinned source. Full SHAs were read from the clones with `git rev-parse HEAD`.

| Source | Repository | Pinned commit |
| --- | --- | --- |
| AGDT (Perseus Dependency Treebank), v2.1 | https://github.com/PerseusDL/treebank_data | `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` |
| UD Ancient Greek-Perseus | https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus | `331ddef91411d0e6549744ee889e05549e6da77d` |
| UD Ancient Greek-PROIEL | https://github.com/UniversalDependencies/UD_Ancient_Greek-PROIEL | `a4ab8d436de97d4598d410d91ea20b4127d04a5f` |
| PROIEL native data (syntacticus) | https://github.com/syntacticus/syntacticus-treebank-data | `525cee4fb40590d7d514376c11acaed1bdd91c15` |
| proiel-cli (PROIEL→UD converter) | https://github.com/proiel/proiel-cli | `385e59f052b9a08755376253aad199129a4afc9e` |
| Gorman, Greek Dependency Trees | https://github.com/vgorman1/Greek-Dependency-Trees | `d823e96d253277bc1872b83ff5ac8c393a0f830a` |
| Pedalion trees | https://github.com/perseids-publications/pedalion-trees | `112c106b86b865ddde4be7e5f3e409d9c42689f9` |
| Canonical Greek literature (reference texts) | https://github.com/PerseusDL/canonical-greekLit | `01b725d835e6e733062ffd79e0efdbae1ba06e5c` |

Pinned URL form: `https://github.com/<org>/<repo>/blob/<full-sha>/<path>`.

All numbers below come from the probes in §10, run once against these pins on 2026-10-14. The probes are Python 3 scripts run with `python3 -I`.

## 2. Scheme comparison

| Dimension | AGDT v2.1 (native) | UD Perseus (converted from AGDT) | PROIEL native | UD PROIEL (converted from PROIEL) | Gorman | Pedalion |
| --- | --- | --- | --- | --- | --- | --- |
| Morphology | 9-char `postag` (positions pos, person, number, tense, mood, voice, gender, case, degree; [TAGSETS.xml](https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/v2.1/Greek/TAGSETS.xml)) | UPOS and FEATS (converted); XPOS keeps the native 9-char postag | 10-char `morphology` (person, number, tense, mood, voice, gender, case, degree, strength, inflection) | UPOS and FEATS (converted); XPOS holds two-character POS codes | not inspected beyond file count (§6.4) | `aldt` v1.5 (sample file, §5) |
| Relations | `relation`; 95 distinct values, 66 `_` compounds (§2.1) | UD labels; DEPS `_` on every token | `relation` plus `head-id`; no `slash` attribute in the file | UD labels; DEPS `_` on every token | not inspected | not inspected |
| Root | `head="0"`, `relation="PRED"` (ξυνέγραψε, first sentence of [tlg0003.tlg001.perseus-grc1.1.tb.xml](https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/v2.1/Greek/texts/tlg0003.tlg001.perseus-grc1.1.tb.xml)) | root relation not re-checked | `head-id` absent on 12,630 tokens (roots and unannotated tokens, not separated) | not re-checked | not inspected | not inspected |
| Empty nodes | `artificial` words (excluded from the crosswalk, §2.2) | not checked in this pass | 8,407 tokens with `empty-token-sort` (§2.3) | converter removes sort `P`, `V`, `C` empties (as read, §2.3) | not inspected | not inspected |
| Lemma homographs | no `#` and no digits in any lemma (0 each) | plain lemma | `#` on 3,475 tokens, 29 distinct lemmas | plain lemma; MISC `LId` on 6,501 tokens | not inspected | not inspected |
| Sentence identity | sentence `id` with file-level `document_id` and `subdoc` (§4) | `sent_id` = `<AGDT file>@<sentence id>` | `id` (0 duplicates in the file) | `sent_id` from source | not inspected | `ref` (§5) |

### 2.1 AGDT relations and morphology

Probe `final_counts.py`, output:

```
AGDT words: 557918 | distinct relations: 95 | with _ compound: 66 | empty relation: 196
AGDT non-ASCII relations: ['AuxΓ', 'AuxΚ']
AGDT lemmas with #: 0 | lemmas with a digit: 0
```

The 557,918 count includes `artificial` words. The 196 empty-relation words are included in that total.

### 2.2 Crosswalk: AGDT → UD Perseus

Probe `final_crosswalk.py`, output:

```
UD Perseus sentences indexed: 13919 | DEPS non-_ tokens: 0
paired sentences: 13919 | paired tokens: 202989 | FORM equal raw: 202078 | FORM equal after NFC: 202989 | MISC SpaceAfter tokens: 24950
first raw FORM mismatches (AGDT form, UD form): [('tlg0003.tlg001.perseus-grc1.1.tb.xml', '10', 'παῤ', 'παῤ'), ('tlg0003.tlg001.perseus-grc1.1.tb.xml', '17', 'δἰ', 'δἰ'), ('tlg0003.tlg001.perseus-grc1.1.tb.xml', '65', 'δἰ', 'δἰ'), ('tlg0003.tlg001.perseus-grc1.1.tb.xml', '113', 'παῤ', 'παῤ'), ('tlg0003.tlg001.perseus-grc1.1.tb.xml', '198', 'παῤ', 'παῤ')]
AGDT tense -> UD Tense (count): [(('-', '-'), 165278), (('a', 'Past'), 14562), (('p', 'Pres'), 13731), (('i', 'Past'), 5309), (('r', 'Past'), 2143), (('f', 'Fut'), 1545), (('l', 'Pqp'), 400), (('t', 'Fut'), 21)]
AGDT voice -> UD Voice (count): [(('-', '-'), 165365), (('a', 'Act'), 26287), (('e', 'Mid'), 6479), (('m', 'Mid'), 3429), (('p', 'Pass'), 1425), (('d', '-'), 4)]
AGDT ADV_CO -> UD deprel: [('conj', 1384), ('advcl', 742), ('obl', 458), ('advmod', 91), ('cop', 46)]
AGDT ADV_AP_CO tokens: 40 | ATR_ExD0_OBJ_CO tokens: 1
```

Findings:

- Pairing: 13,919 sentences and 202,989 tokens pair. That equals the total UD Perseus token count, so every UD Perseus token is in a pair.
- FORM: the 911 raw mismatches all disappear after NFC normalization (202,989 of 202,989 equal). They are encoding variants, not pairing errors.
- Tense `a` (aorist) → `Tense=Past` with no `Aspect` feature in UD Perseus (14,562 tokens).
- Voice `e` (middle or passive) → `Voice=Mid` only (6,479 tokens). The passive reading is not carried into UD Perseus.
- The `ADV_CO` relation spreads across four UD relations (`conj`, `advcl`, `obl`, `advmod`, plus `cop`). The compound's detail is not preserved in UD.
- DEPS is `_` on every UD Perseus token.

Worked example, ἐγένετο in UD Perseus dev ([line 341](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/grc_perseus-ud-dev.conllu#L341)): XPOS `v3saim---` (native postag kept), FEATS `Mood=Ind|Number=Sing|Person=3|Tense=Past|VerbForm=Fin|Voice=Mid`.

### 2.3 PROIEL native and the PROIEL → UD converter

Probe `final_native.py`, output:

```
native tokens: 140773 | empty-token-sort: 8407 | non-empty: 132366
non-empty tokens with a space in form: 31
UD PROIEL tokens by # source: {'Histories': 82000, 'The Greek New Testament': 132005} | total: 214005
```

Probe `final_counts.py`, PROIEL lines, output:

```
UD PROIEL tokens: 214005 | DEPS non-_: 0 | MISC Ref=: 214005 | MISC LId=: 6501
UD PROIEL XPOS most common: [('V-', 43609), ('Nb', 35809), ('S-', 28468), ('Df', 21994), ('R-', 15619)]
PROIEL native tokens: 140773 | sentences: 11506 | duplicate sentence ids: 0
PROIEL native lemmas with #: 3475 | tokens with no head-id: 12630 | tokens with slash: 0
```

- UD PROIEL is two treebanks: the NT (132,005 tokens) and Herodotus's *Histories* (82,000 tokens). It is not a conversion of `greek-nt.xml` alone.
- The native NT file has 140,773 tokens, 8,407 of them empty. Removing the empties leaves 132,366, against 132,005 converted NT tokens. The 361-token difference is **not reconciled**.
- Native morphology has 10 positions: person, number, tense, mood, voice, gender, case, degree, strength, inflection.

The converter, as read in [`morphology.rb`](https://github.com/proiel/proiel-cli/blob/385e59f052b9a08755376253aad199129a4afc9e/lib/proiel/cli/converters/conll-u/morphology.rb) (tense map at lines 276–288):

- `'i' => 'Tense=Past|Aspect=Imp'`
- `'a' => 'Tense=Past|Aspect=Perf'`
- voice `'a' => 'Voice=Act'`

These rules are quoted as read. They were not run against the whole corpus in this pass, so no agreement rate is claimed.

Worked examples (native morphology position order: person, number, tense, mood, voice):

- ἐγέννησεν, MATT 1.2, `3saia----i`: aorist indicative active. The converter's tense rule gives `Aspect=Perf|Tense=Past` for the aorist, while UD Perseus gives `Tense=Past` only (§2.2). The UD PROIEL CoNLL-U line for this token was read in an earlier pass and is not re-cited here.
- ἦν, MATT 1.18, `3siia----i`: imperfect indicative active, converted to `Aspect=Imp`.

The same aorist therefore receives different aspect features in the two UD treebanks. Both come from one tense value in their source, so the difference is in the conversion, not in the source morphology.

### 2.4 Empty nodes

UD CoNLL-U supports empty nodes. The AGDT `artificial` words are excluded from the Perseus crosswalk (§2.2). The PROIEL native empties (8,407 tokens, §2.3) are removed by the converter as read. Whether the pinned UD Perseus release contains empty nodes was not checked.

## 3. Greek morphology: preserved, normalized, lost

| Feature | AGDT native | UD Perseus | UD PROIEL | Status |
| --- | --- | --- | --- | --- |
| Full morphology string | 9-char postag | kept in XPOS; features normalized to FEATS | 10 native positions, not kept in XPOS | preserved in XPOS for Perseus only |
| Aorist | tense `a` | `Tense=Past`, no Aspect | `Aspect=Perf|Tense=Past` (converter rule, §2.3) | normalized differently across UD treebanks |
| Middle/passive `e` | voice `e` | `Voice=Mid` (6,479 tokens) | converted by rule | passive reading not in UD Perseus FEATS |
| Strength, inflection (PROIEL) | not in AGDT | not in UD | native present; converter does not map them (as read) | lost in UD |
| Relation compounds | 66 `_` compounds | collapsed to UD labels | native `relation` | lost in UD |
| Presentation and information status (PROIEL) | not in AGDT | not in UD | native present | lost in UD PROIEL (as read from converter) |

Examples:

- Preserved: AGDT ἐγένετο `v3saim---` → UD Perseus XPOS keeps it, FEATS `Voice=Mid|Tense=Past` ([line 341](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/grc_perseus-ud-dev.conllu#L341)).
- Normalized: AGDT `ADV_CO` → several UD relations (§2.2).
- Lost: AGDT `ADV_AP_CO` (40 tokens) and `ATR_ExD0_OBJ_CO` (1 token) have no UD counterpart.

## 4. Citations and sentence identity

- AGDT sentence attributes are only `id`, `document_id`, and `subdoc`. Word attributes are `id`, `form`, `lemma`, `postag`, `relation`, `head`, `artificial`, and `insertion_id`. (Checked by parsing `tlg0003.tlg001.perseus-grc1.1.tb.xml`.) The citation sits in sentence-level `subdoc`, for example `1.1.1`. Lysias speeches carry a speech-level range (`1-50`).
- UD Perseus `sent_id` is `<AGDT file>@<sentence id>` (for example `tlg0003.tlg001.perseus-grc1.1.tb.xml@3`).
- PROIEL citations are `citation-part` (for example `MATT 1.18`), carried into UD PROIEL as MISC `Ref` on every token (214,005 of 214,005).
- PROIEL sentence `id` is unique in the native file (0 duplicates, §2.3).
- Homographs: AGDT lemmas have no `#` and no digits (§2.1). PROIEL native lemmas carry `#` on 3,475 tokens (29 distinct lemmas). UD PROIEL carries the variant in MISC `LId` on 6,501 tokens. The two counts do not reconcile.

## 5. Pedalion identity

Pedalion XML uses single-quoted attributes. Sample file `src/pedalion/public/xml/epicurus1.xml` (in the pinned tree) has `format="aldt" version="1.5"` on its root and 1,523 `ref` attributes. Word `ref` values look like `Leuven|0537-012|1|1`. No `ref` value repeats within that file.

Not verified in this pass: uniqueness of `ref` across files, the per-file inventory (41 XML files under `public/xml`), and relation labels. The earlier counts (281,253 words, 7,195 repeats) came from a probe that used double quotes and is withdrawn.

## 6. Candidate complete reading units

"Complete" here means every canonical leaf of a unit (a section or a chapter at the leaf level of its canonical text) is covered by an AGDT sentence reference, and the word sequence matches. It does not mean identical text: AGDT files are based on different editions (grc1) from the canonical texts (grc2 or grc4). These are inventories of evidence, not a recommended MVP.

### 6.1 AGDT v2.1 units

Probe `final_units.py` per unit, plus `final_bounds.py` for the canonical book lengths. "Words" are normalized canonical words in scope matched against AGDT words.

| Unit (AGDT file) | Canonical text | Canonical leaves in scope | References covered | Word match | Status |
| --- | --- | --- | --- | --- | --- |
| Lysias 1 (`tlg0540.tlg001`) | `tlg0540.tlg001.perseus-grc2.xml` | 50 (1–50) | 50/50 | 2,402/2,415 (99.46%) | complete at speech level; reference check trivial (speech-level range `1-50`) |
| Lysias 14 (`tlg0540.tlg014`) | `tlg0540.tlg014.perseus-grc2.xml` | 47 (1–47) | 47/47 | 2,422/2,426 (99.84%) | complete at speech level; reference check trivial |
| Lysias 15 (`tlg0540.tlg015`) | `tlg0540.tlg015.perseus-grc2.xml` | 12 (1–12) | 12/12 | 601/602 (99.83%) | complete at speech level; reference check trivial; residual difference is crasis/elision (καν vs κ αν) |
| Lysias 23 (`tlg0540.tlg023`) | `tlg0540.tlg023.perseus-grc2.xml` | 16 (1–16) | 16/16 | 776/777 (99.87%) | complete; section-level subdocs, so the reference check is meaningful |
| Plutarch *Lycurgus* (`tlg0007.tlg004`; title Λυκοῦργος) | `tlg0007.tlg004.perseus-grc2.xml` | 153 (1.1–31.5) | 153/153 | 9,377/9,391 (99.85%) | complete; 426 plain subdocs and 8 ranges |
| Plutarch *Alcibiades* (`tlg0007.tlg015`) | `tlg0007.tlg015.perseus-grc2.xml` | 187 (1.1–39.5) | 181/187 | 10,028/10,043 (99.85%) | **not complete**; uncovered 10.3, 23.5, 31.3, 32.3, 32.5, 36.5 |
| Thucydides 1 (`tlg0003.tlg001`) | `tlg0003.tlg001.perseus-grc2.xml` | 580 (1.1.1–1.146.1) | 572/580 | 22,276/22,307 (99.86%) | **not complete**; uncovered 1.11.3, 1.26.1, 1.38.6, 1.61.2, 1.65.3, 1.100.3, 1.108.3, 1.110.5 |
| Herodotus 1 (`tlg0016.tlg001`) | `tlg0016.tlg001.perseus-grc2.xml` | 711 (1.1.0–1.216.4) | 709/711 | 29,043/29,068 (99.91%) | **not complete**; uncovered 1.96.3, 1.129.4 (one ambiguous fragment `1.82,3`) |
| Athenaeus 12 (`tlg0008.tlg001.perseus-grc1.12.tb.xml`) | `tlg0008.tlg001.perseus-grc4.xml` | 81 (12.1–12.81) | 81/81 | 17,860/17,906 (99.74%) | complete at leaf level (book 12 has 81 leaves); 5 non-reference subdoc values; 117 ambiguous fragments |
| Athenaeus 13 (`tlg0008.tlg001.perseus-grc1.13.tb.xml`) | `tlg0008.tlg001.perseus-grc4.xml` | 95 (13.1–13.95) | 95/95 | 21,694/21,745 (99.77%) | complete at leaf level (book 13 has 95 leaves); 1 non-reference subdoc value |
| Polybius 1 (`tlg0543.tlg001`; title Ἱστορίαι) | `tlg0543.tlg001.perseus-grc2.xml` | 943 (1.1.1–1.88.12) | 943/943 | 25,612/25,657 (99.82%) | complete for book 1 (book 1 has 943 leaves, last 1.88.12) |
| Aeschines 1 (Gorman; four files; title Κατὰ Τιμάρχου) | `tlg0026.tlg001.perseus-grc2.xml` | 196 (1–196) | 192/196 | 13,399/13,563 (98.79%) | **not complete**; sections 59, 114, 172, 174 have no sentence |

Notes:

- The Athenaeus and Polybius units are complete at leaf level only; their word match rates are cross-edition.
- Word match is cross-edition and is the only evidence for Lysias 1, 14, and 15. The residual unmatched words were not inspected individually, except the Lysias 15 crasis case.
- The reference check skips non-reference subdoc values and ambiguous fragments (counts in the table). Their words are still in the word match.

### 6.2 PROIEL NT (verse coverage)

Probe `final_proiel_verses.py` over `greek-nt.xml`. "No gaps" means every chapter has verses 1..N without holes. The probe checks chapters only up to each book's highest cited chapter, so a missing final chapter is not visible in the probe. Book-level totals are external only where a source is pinned (Titus, Philemon).

| Book | Verses | Result in file |
| --- | --- | --- |
| TIT | 46 | no gaps; **total matches Wikipedia** ([Epistle to Titus](https://en.wikipedia.org/w/index.php?oldid=1354169853), "46 verses in total") |
| PHILEM | 25 | one chapter, no gaps; final verse 25 agrees with the page's section heading "Conclusion and greetings (21–25)" ([oldid 1361421780](https://en.wikipedia.org/w/index.php?oldid=1361421780)); total not otherwise checked |
| PHIL 104, COL 95, EPH 155, GAL 149, 1THESS 89, 2THESS 47, 1TIM 113, 2TIM 83, JAS 108, 1COR 437, 2COR 256 | as listed | no gaps; no external total checked |
| REV | 405 | no gaps; no external total checked |
| HEB | 278 | chapters 1–12 only; no chapter 13 tokens in the file; **not complete** |
| 1PET | 54 | chapters 1–3 only; the file ends at 3:4; **not complete** |
| JUDE | 3 | verses 2, 3, 23 present; **not complete** |
| 3JOHN | 2 | verses 1 and 11 present; **not complete** |
| 1JOHN, 2JOHN, 2PET | 0 | absent from the file (checked by name) |
| MATT 1067 | | gaps 17:21, 18:11, 21:44, 23:14 |
| MARK 673 | | gaps 7:16, 9:44, 9:46, 11:26, 15:28 |
| LUKE 1147 | | gaps 17:36, 23:17, 24:12, 24:40 |
| JOHN 878 | | gap 5:4 |
| ACTS 1003 | | gaps 8:37, 15:34, 24:7, 28:29 |
| ROM 432 | | gap 16:24 |

The gaps in the Gospels, Acts, and Romans are not verified as textual-variant omissions; the edition's apparatus was not checked.

### 6.3 Herodotus (PROIEL)

Probe `final_hdt.py` over `syntacticus/proiel/hdt.xml`:

```
book 1 distinct chapters: 212 | min 1 max 216
missing chapters in 1..max: [127, 128, 129, 130]
distinct chapters per book: {1: 212, 3: 1, 4: 61, 5: 119, 6: 139, 7: 238, 8: 7}
```

Book 1 is not complete in PROIEL (chapters 127–130 absent). Books 2 and 9 are absent. Book 3 has one chapter. The coverage is selective.

### 6.4 Gorman and Pedalion

- Gorman: 166 XML files in `src/gorman` (all versions, counted by `find`). Aeschines 1 is split across four files. The `bu` versions were not inspected. The licence files were not checked in this pass, so no licence statement is made.
- Pedalion: see §5. Not inventoried beyond the sample.

## 7. Other open treebanks

- **UD Ancient Greek-PTNK** (listed in UD 2.18; Septuagint Genesis 1–50 and Ruth 1–4, CC BY-SA 4.0, projected from Hebrew and corrected; per the UD listing): **not verified**. The user declined the clone. No PTNK data was accessed, and no SHA, file, licence, or count is given here. Every PTNK statement remains unverified.
- **AthDGC** (arXiv 2606.15510; Zenodo DOI 10.5281/zenodo.20439182, v0.4.0): automatic Stanza `grc_proiel` annotation across classical periods. **Not human-reviewed**, so it is not a candidate for human-reviewed units.

## 8. New precise questions

1. Which 361 tokens separate the native NT non-empty count (132,366) from the UD PROIEL NT count (132,005)?
2. Are the NT verse gaps listed in §6.2 textual variants in the edition's apparatus?
3. Why are chapters 127–130 of Herodotus book 1 absent from `hdt.xml`?
4. Why does the UD PROIEL `LId` count (6,501) differ from the native `#`-lemma token count (3,475)?
5. Does UD PTNK use the same aorist and middle conventions as UD Perseus or UD PROIEL? This needs a pinned clone, which the user has declined, so it stays open.

## 9. Limitations

- Word matches are cross-edition (AGDT grc1 against canonical grc2 and grc4). A match rate is evidence of alignment, not identity.
- "Complete" is judged by leaf coverage within each canonical text, so it inherits the canonical leaf structure. The probe counts leaves, not sentences.
- Non-reference subdoc values and ambiguous fragments are skipped by the reference check (counts in §6.1).
- The UD Perseus conversion was not reproduced, so source-version effects are not separated from conversion effects for the Perseus pairs. No claim is made about the absence of version effects.
- The crosswalk pairs sentences only when non-elliptic token counts are equal. The 13,919 pairs include every UD Perseus token, but the pairing is not checked against the source beyond counts and FORM.
- PTNK is unverified. Pedalion coverage is not inventoried. Gorman `bu` versions and licence files were not checked. UD release notes were not checked for changes after the pinned releases.
- External verse totals: only Titus (Wikipedia, pinned) is verified, and Philemon is partly verified by a heading.
- No legal conclusions are drawn. Licence evidence is not reviewed in this pass.

## 10. Probes

Run each probe with `python3 -I` against the pinned clones under `src/` in the scratch area. The scripts are the exact files that produced the numbers above.

Probe `final_units.py`, run as `python3 -I final_units.py <canonical.xml> <agdt.xml> [<agdt.xml> ...]`:

```python
# AGDT coverage of a canonical Perseus text (PerseusDL/canonical-greekLit).
# Usage: python3 -I final_units.py <canonical.xml> <agdt.xml> [<agdt.xml> ...]
# Several AGDT files may be given for one work (for example Gorman's split speeches); their
# sentences are pooled. Two checks, reported separately:
#  (1) reference coverage: each canonical leaf <div type="textpart"> (refs nested by n, joined
#      with '.') is covered when a sentence subdoc equals it, is a prefix of it, lies inside it,
#      or lies in a range "lo-hi". A short upper bound such as "1.8.2-3" means "1.8.3".
#      Only leaves between the lowest and highest claimed reference are scored, so a section
#      with no sentence at all counts as uncovered.
#  (2) word sequence: normalized canonical words (apparatus, notes, page breaks excluded)
#      against AGDT word forms (artificial words excluded), aligned in order with difflib.
import difflib, os, re, sys, unicodedata, xml.etree.ElementTree as ET

CANON, AGDT_FILES = sys.argv[1], sys.argv[2:]
SKIP = {'app', 'rdg', 'note', 'bibl', 'pb', 'wit', 'sic', 'corr'}


def local(tag):
    return tag.rsplit('}', 1)[-1]


def leaves(el, prefix=()):
    for c in el:
        if local(c.tag) == 'div' and c.get('type') == 'textpart':
            ref = prefix + (c.get('n') or '?',)
            if any(local(d.tag) == 'div' and d.get('type') == 'textpart' for d in c.iter() if d is not c):
                yield from leaves(c, ref)
            else:
                yield ref, c
        else:
            yield from leaves(c, prefix)


def key(ref):
    return tuple((0, int(x)) if x.isdigit() else (1, x) for x in re.split(r'[.\-]', ref))


def expand_hi(lo, hi):
    lp, hp = lo.split('.'), hi.split('.')
    if len(hp) < len(lp):
        hp = lp[:-len(hp)] + hp
    return '.'.join(lp), '.'.join(hp)


def covered(ref, subs, ranges):
    dotted = '.'.join(ref)
    if any('.'.join(ref[:k]) in subs for k in range(1, len(ref) + 1)):
        return True
    if any(s.startswith(dotted + '.') for s in subs):
        return True
    return any(key(lo) <= key(dotted) <= key(hi) for lo, hi in ranges)


def body_text(el):
    out = []

    def walk(n):
        if local(n.tag) in SKIP:
            return
        if n.text:
            out.append(n.text)
        for c in n:
            walk(c)
            if c.tail:
                out.append(c.tail)
    walk(el)
    return ' '.join(out)


def norm(text):
    t = unicodedata.normalize('NFD', text)
    t = ''.join(c for c in t if not unicodedata.combining(c))
    t = re.sub("[ʼ’']", ' ', t)  # elision marks are word breaks
    return re.sub(r'[^\w\s]', ' ', t).lower().split()


croot = ET.parse(CANON).getroot()
title = next((e.text.strip() for e in croot.iter() if local(e.tag) == 'title' and e.text and e.text.strip()), '-')

sents = []
for f in AGDT_FILES:
    sents += list(ET.parse(f).getroot().iter('sentence'))

REF = re.compile(r'\d+(?:\.\d+)*')
subs, ranges, ambiguous, other = set(), [], 0, 0
for s in sents:
    vals = [x.strip() for x in (s.get('subdoc') or '').split(',') if x.strip()]
    for i, v in enumerate(vals):
        if i > 0 and '.' not in v and '-' not in v:
            ambiguous += 1  # "1.82,3": the comma sits inside the reference, not between two refs
            continue
        if '-' in v:
            lo, hi = v.split('-', 1)
            if REF.fullmatch(lo) and REF.fullmatch(hi):
                ranges.append(expand_hi(lo, hi))
            else:
                other += 1  # non-reference text such as "book=13:chapter=50"
        elif REF.fullmatch(v):
            subs.add(v)
        else:
            other += 1

# scope: every canonical leaf between the lowest and highest reference the AGDT files claim,
# so a section with no sentence at all is counted as uncovered, not silently dropped
claimed = [key(v) for v in subs] + [key(x) for lo_hi in ranges for x in lo_hi]
lo_k, hi_k = min(claimed), max(claimed)
scope = [(r, el) for r, el in leaves(croot) if lo_k <= key('.'.join(r)) <= hi_k or covered(r, subs, ranges)]
uncovered = ['.'.join(r) for r, _ in scope if not covered(r, subs, ranges)]

cw = [w for _, el in scope for w in norm(body_text(el))]
aw = [w for s in sents for x in s.iter('word') if not x.get('artificial') for w in norm(x.get('form') or '')]
matched = sum(b.size for b in difflib.SequenceMatcher(None, cw, aw, autojunk=False).get_matching_blocks())

all_refs = ['.'.join(r) for r, _ in leaves(croot)]
print('canonical:', os.path.basename(CANON), '| title:', title)
print(f'canonical leaves: {len(all_refs)} ({all_refs[0]}..{all_refs[-1]}) | scored in scope: {len(scope)} ({".".join(scope[0][0])}..{".".join(scope[-1][0])})')
print('agdt files:', len(AGDT_FILES), '| sentences:', len(sents), '| words:', len(aw),
      '| ambiguous comma fragments:', ambiguous, '| non-reference subdoc values:', other)
print(f'references: {len(scope) - len(uncovered)}/{len(scope)} covered in scope; uncovered: {uncovered[:8]}')
print(f'words: canonical in scope {len(cw)}, matched {matched} ({matched / max(len(cw), 1):.2%} of canonical, {matched / max(len(aw), 1):.2%} of AGDT)')
```

Probe `final_bounds.py`, run as `python3 -I final_bounds.py <canonical.xml> <top-level n> [...]`:

```python
# Leaf count of one top-level division of a canonical Perseus text, to bound an AGDT unit.
# Usage: python3 -I final_bounds.py <canonical.xml> <top-level n> [<top-level n> ...]
# A leaf is a <div type="textpart"> with no textpart descendants; its reference is the n
# attributes joined with '.'. Leaves are listed in document order.
import sys, xml.etree.ElementTree as ET


def local(tag):
    return tag.rsplit('}', 1)[-1]


def leaves(el, prefix=()):
    for c in el:
        if local(c.tag) == 'div' and c.get('type') == 'textpart':
            ref = prefix + (c.get('n') or '?',)
            if any(local(d.tag) == 'div' and d.get('type') == 'textpart' for d in c.iter() if d is not c):
                yield from leaves(c, ref)
            else:
                yield '.'.join(ref)
        else:
            yield from leaves(c, prefix)


refs = list(leaves(ET.parse(sys.argv[1]).getroot()))
print('canonical leaves total:', len(refs))
for top in sys.argv[2:]:
    part = [r for r in refs if r.split('.')[0] == top]
    print(f'top {top}: {len(part)} leaves, first {part[0] if part else None}, last {part[-1] if part else None}')
```

Probe `final_crosswalk.py`, run as `python3 -I final_crosswalk.py <src-dir>`:

```python
# Token-level crosswalk of AGDT v2.1 Greek to UD Ancient Greek-Perseus.
# Usage: python3 -I final_crosswalk.py <src-dir>
#   <src-dir>/treebank_data/v2.1/Greek/texts/*.xml  (PerseusDL/treebank_data, bf4334f0)
#   <src-dir>/UD_grc_Perseus/*.conllu               (UniversalDependencies, 331ddef9)
# A sentence pairs when UD sent_id "<AGDT file>@<sentence id>" exists and the non-elliptic
# token counts are equal. Within a pair, tokens are zipped by position. FORM is compared raw
# and after Unicode NFC normalization, so encoding variants are separated from real mismatches.
import collections, glob, os, sys, unicodedata, xml.etree.ElementTree as ET

SRC = sys.argv[1]
ud = collections.OrderedDict()
for p in sorted(glob.glob(os.path.join(SRC, 'UD_grc_Perseus', '*.conllu'))):
    sid = None
    for line in open(p, encoding='utf-8'):
        line = line.rstrip('\n')
        if line.startswith('# sent_id'):
            sid = line.split('=', 1)[1].strip()
            ud[sid] = []
        elif line[:1].isdigit():
            cols = line.split('\t')
            if '-' not in cols[0] and '.' not in cols[0]:
                ud[sid].append(cols)


def feats(s):
    return dict(x.split('=', 1) for x in s.split('|') if '=' in x)


def nfc(s):
    return unicodedata.normalize('NFC', s)


sentences = tokens = form_ok = nfc_ok = spacing = 0
mismatch_examples = []
tense, voice = collections.Counter(), collections.Counter()
rel_count = collections.Counter()
rel_target = collections.defaultdict(collections.Counter)
for p in sorted(glob.glob(os.path.join(SRC, 'treebank_data', 'v2.1', 'Greek', 'texts', '*.xml'))):
    fname = os.path.basename(p)
    for s in ET.parse(p).getroot().iter('sentence'):
        rows = ud.get(f"{fname}@{s.get('id')}")
        words = [w for w in s.iter('word') if not w.get('artificial')]
        if rows is None or len(rows) != len(words):
            continue
        sentences += 1
        for w, r in zip(words, rows):
            tokens += 1
            form_ok += w.get('form') == r[1]
            nfc_ok += nfc(w.get('form') or '') == nfc(r[1])
            if w.get('form') != r[1] and len(mismatch_examples) < 5:
                mismatch_examples.append((fname, s.get('id'), w.get('form'), r[1]))
            spacing += 'SpaceAfter' in r[9]
            pos = w.get('postag') or ''
            f = feats(r[5])
            if len(pos) == 9:
                tense[(pos[3], f.get('Tense', '-'))] += 1
                voice[(pos[5], f.get('Voice', '-'))] += 1
            rel_count[w.get('relation')] += 1
            rel_target[w.get('relation')][r[7]] += 1

deps = sum(r[8] != '_' for rows in ud.values() for r in rows)
print('UD Perseus sentences indexed:', len(ud), '| DEPS non-_ tokens:', deps)
print('paired sentences:', sentences, '| paired tokens:', tokens, '| FORM equal raw:', form_ok,
      '| FORM equal after NFC:', nfc_ok, '| MISC SpaceAfter tokens:', spacing)
print('first raw FORM mismatches (AGDT form, UD form):', mismatch_examples)
print('AGDT tense -> UD Tense (count):', tense.most_common(8))
print('AGDT voice -> UD Voice (count):', voice.most_common(8))
print('AGDT ADV_CO -> UD deprel:', rel_target['ADV_CO'].most_common(5))
print('AGDT ADV_AP_CO tokens:', rel_count['ADV_AP_CO'], '| ATR_ExD0_OBJ_CO tokens:', rel_count['ATR_ExD0_OBJ_CO'])
```

Probe `final_counts.py`, run as `python3 -I final_counts.py <src-dir>`:

```python
# Counts quoted in the scheme comparison; each count names the rule that produced it.
# Usage: python3 -I final_counts.py <src-dir>
#   <src-dir>/treebank_data/v2.1/Greek/texts/*.xml, <src-dir>/UD_grc_Perseus/*.conllu,
#   <src-dir>/UD_grc_PROIEL/*.conllu, <src-dir>/syntacticus/proiel/greek-nt.xml
import collections, glob, os, sys, xml.etree.ElementTree as ET

SRC = sys.argv[1]


def conllu_rows(pattern):
    for p in sorted(glob.glob(pattern)):
        for line in open(p, encoding='utf-8'):
            if line[:1].isdigit():
                c = line.rstrip('\n').split('\t')
                if '-' not in c[0] and '.' not in c[0]:
                    yield c


# AGDT v2.1: all <word> elements, artificial included
rel = collections.Counter()
n = empty = lemma_hash = lemma_digit = 0
for p in sorted(glob.glob(os.path.join(SRC, 'treebank_data', 'v2.1', 'Greek', 'texts', '*.xml'))):
    for w in ET.parse(p).getroot().iter('word'):
        n += 1
        r = w.get('relation')
        if r:
            rel[r] += 1
        else:
            empty += 1
        lem = w.get('lemma') or ''
        lemma_hash += '#' in lem
        lemma_digit += any(ch.isdigit() for ch in lem)
print('AGDT words:', n, '| distinct relations:', len(rel), '| with _ compound:', sum('_' in r for r in rel),
      '| empty relation:', empty)
print('AGDT non-ASCII relations:', sorted(r for r in rel if not r.isascii()))
print('AGDT lemmas with #:', lemma_hash, '| lemmas with a digit:', lemma_digit)

# UD Perseus: non-elliptic rows
pers = list(conllu_rows(os.path.join(SRC, 'UD_grc_Perseus', '*.conllu')))
print('UD Perseus tokens:', len(pers), '| DEPS non-_:', sum(c[8] != '_' for c in pers),
      '| MISC mentions SpaceAfter:', sum('SpaceAfter' in c[9] for c in pers))

# UD PROIEL
prom = list(conllu_rows(os.path.join(SRC, 'UD_grc_PROIEL', '*.conllu')))
print('UD PROIEL tokens:', len(prom), '| DEPS non-_:', sum(c[8] != '_' for c in prom),
      '| MISC Ref=:', sum('Ref=' in c[9] for c in prom), '| MISC LId=:', sum('LId=' in c[9] for c in prom))
print('UD PROIEL XPOS most common:', collections.Counter(c[4] for c in prom).most_common(5))

# PROIEL native
pn = ET.parse(os.path.join(SRC, 'syntacticus', 'proiel', 'greek-nt.xml')).getroot()
toks = list(pn.iter('token'))
ids = [s.get('id') for s in pn.iter('sentence')]
print('PROIEL native tokens:', len(toks), '| sentences:', len(ids), '| duplicate sentence ids:', len(ids) - len(set(ids)))
print('PROIEL native lemmas with #:', sum('#' in (t.get('lemma') or '') for t in toks),
      '| tokens with no head-id:', sum(t.get('head-id') is None for t in toks),
      '| tokens with slash:', sum(t.get('slash') is not None for t in toks))
```

Probe `final_native.py`, run as `python3 -I final_native.py <greek-nt.xml> <UD_grc_PROIEL dir>`:

```python
# PROIEL native NT tokens: empty tokens, spaces in forms, and the UD PROIEL token split by source.
# Usage: python3 -I final_native.py <greek-nt.xml> <UD_grc_PROIEL dir>
#   <greek-nt.xml>: syntacticus/syntacticus-treebank-data proiel/greek-nt.xml (525cee4f)
#   <UD dir>: UniversalDependencies/UD_Ancient_Greek-PROIEL *.conllu (a4ab8d43)
import collections, glob, sys, xml.etree.ElementTree as ET

toks = list(ET.parse(sys.argv[1]).getroot().iter('token'))
empty = [t for t in toks if t.get('empty-token-sort') is not None]
spaced = [t for t in toks if t.get('empty-token-sort') is None and ' ' in (t.get('form') or '')]
print('native tokens:', len(toks), '| empty-token-sort:', len(empty), '| non-empty:', len(toks) - len(empty))
print('non-empty tokens with a space in form:', len(spaced))

src = collections.Counter()
cur = None
for p in sorted(glob.glob(sys.argv[2] + '/*.conllu')):
    for line in open(p, encoding='utf-8'):
        if line.startswith('# source ='):
            cur = line.split('=', 1)[1].split(',')[0].strip()
        elif line[:1].isdigit() and '-' not in line.split('\t')[0] and '.' not in line.split('\t')[0]:
            src[cur] += 1
print('UD PROIEL tokens by # source:', dict(src), '| total:', sum(src.values()))
```

Probe `final_proiel_verses.py`, run as `python3 -I final_proiel_verses.py <greek-nt.xml> [BOOK[=EXPECTED] ...]`:

```python
# Verse coverage per NT book in the PROIEL native file (syntacticus proiel/greek-nt.xml).
# Usage: python3 -I final_proiel_verses.py <greek-nt.xml> [BOOK[=EXPECTED] ...]
# A verse is present when some token's citation-part is "BOOK C.V", or "BOOK V" for one-chapter books.
# A book has no gaps when every chapter 1..C has verses 1..N without holes and chapter numbers
# are contiguous. If EXPECTED is given, the verse total must also equal it.
import collections, re, sys, xml.etree.ElementTree as ET

path, specs = sys.argv[1], sys.argv[2:]
want = {}
for spec in specs:
    book, _, exp = spec.partition('=')
    want[book] = int(exp) if exp else None

found = collections.defaultdict(set)
tokens = collections.Counter()
for t in ET.parse(path).getroot().iter('token'):
    cp = t.get('citation-part') or ''
    m = re.fullmatch(r'(\S+) (\d+)\.(\d+)', cp) or re.fullmatch(r'(\S+) (\d+)', cp)
    if not m:
        continue
    book = m.group(1)
    ch, vs = (int(m.group(2)), int(m.group(3))) if m.lastindex == 3 else (1, int(m.group(2)))
    found[book].add((ch, vs))
    tokens[book] += 1

for book in (list(want) or sorted(found)):
    if book not in found:
        print(f'{book}: absent')
        continue
    chapters = collections.defaultdict(set)
    for ch, vs in found[book]:
        chapters[ch].add(vs)
    gaps = []
    for ch in sorted(chapters):
        missing = [v for v in range(1, max(chapters[ch]) + 1) if v not in chapters[ch]]
        if missing:
            gaps.append(f'{ch}:{missing[:5]}')
    absent_ch = [c for c in range(1, max(chapters) + 1) if c not in chapters]
    total = len(found[book])
    exp = want.get(book)
    if exp is None:
        status = 'no expected total given'
    elif not gaps and not absent_ch and total == exp:
        status = 'COMPLETE'
    else:
        status = 'INCOMPLETE'
    spans = ' '.join(f'{c}:{min(v)}-{max(v)}({len(v)})' for c, v in sorted(chapters.items()))
    print(f'{book:8s} tokens={tokens[book]:5d} verses={total:4d} expected={exp} {status} '
          f'gaps={gaps[:4]} absent_chapters={absent_ch} | {spans}')
```

Probe `final_hdt.py`, run as `python3 -I final_hdt.py <hdt.xml>`:

```python
# Herodotus in the PROIEL native file: which book-1 chapters carry tokens.
# Usage: python3 -I final_hdt.py <hdt.xml>   (syntacticus/syntacticus-treebank-data, 525cee4f)
# citation-part is "BOOK.CHAPTER.SECTION" (for example "1.1.0"), so the chapter is the second field.
import collections, sys, xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
books = collections.defaultdict(set)
for t in root.iter('token'):
    parts = (t.get('citation-part') or '').split('.')
    if len(parts) >= 2 and parts[0].isdigit() and parts[1].isdigit():
        books[int(parts[0])].add(int(parts[1]))
b1 = books.get(1, set())
print('book 1 distinct chapters:', len(b1), '| min', min(b1), 'max', max(b1))
print('missing chapters in 1..max:', [c for c in range(1, max(b1) + 1) if c not in b1])
print('distinct chapters per book:', {b: len(c) for b, c in sorted(books.items())})
```
