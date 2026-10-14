# Ancient Greek annotation schemes: comparison and candidate MVP reading units

Research for #2032 under map #2021. Date: 2026-10-14. Baseline: `b262ed0a`.

This document records evidence. It does not choose an architecture, a priority, a corpus, an MVP, or a normalization policy; those decisions stay with the human reviewer (#2021).

## Sources and pins

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

Pinned URL form: `https://github.com/<org>/<repo>/blob/<full-sha>/<path>`.

The prior treebank inventory (`doc/research/ancient-greek-treebanks.md`, commit `fe37aedd`) supplies coverage and count context and is not repeated here.

Version separation: AGDT v2.1 is dated 2015 and the PROIEL native export is dated 2023 (per the pinned sources). UD Perseus and UD PROIEL are later conversions. Where a difference could come from the source release or from the conversion, the sections below say which.

## 1. Scheme comparison

| Dimension | AGDT v2.1 (native) | UD Perseus (converted from AGDT) | PROIEL native | UD PROIEL (converted from PROIEL) | Gorman | Pedalion |
| --- | --- | --- | --- | --- | --- | --- |
| Morphology | 9-char `postag` ([TAGSETS](https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/v2.1/Greek/TAGSETS.xml)) | UPOS, FEATS, and full native string kept in XPOS | 10-char `morphology` (person, number, tense, mood, voice, gender, case, degree, strength, inflection) | UPOS and FEATS; native 3-char POS kept in XPOS | AGDT-style 9-char postag (`morphgrc` v1.5) | `aldt` v1.5 attributes |
| Relations | 14 base labels, 96 distinct values including `_` compounds | UD labels; DEPS is `_` everywhere | `relation` plus `head-id`; secondary `slash` edges | UD labels; DEPS is 0 non-empty | AGDT-style labels | AGDT-style labels |
| Root | `head="0"`, `relation="PRED"` ([example](https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/v2.1/Greek/texts/tlg0003.tlg001.perseus-grc1.1.tb.xml#L60))| `root` | no `head-id`; `relation="pred"` ([ἦν, MATT 1.18](https://github.com/syntacticus/syntacticus-treebank-data/blob/525cee4fb40590d7d514376c11acaed1bdd91c15/proiel/greek-nt.xml#L581)) | `root` | AGDT-style | AGDT-style |
| Empty nodes | `artificial` elliptic words, see §1.4 | excluded from the crosswalk; empty-node handling in the release not checked | empty tokens, sort `P`/`V`/`C` | Sort `P`/`V`/`C` empties removed by converter | not inspected | not inspected |
| Lemma homographs | none (no `#`) | plain lemma | `lemma#n` on 7,230 lemmas | plain lemma; MISC `LId` on 6,501 tokens | AGDT-style | not inspected |
| Sentence identity | file + sentence `id` | UD `sent_id` = `<file>@<id>` | sentence `id` (globally unique in the file) | `sent_id` from source | file + `id` + CTS `document_id` | `ref` (see §4) |

### 1.1 AGDT morphology and relations

The AGDT 9-character `postag` has positions pos, person, number, tense, mood, voice, gender, case, degree ([TAGSETS.xml](https://github.com/PerseusDL/treebank_data/blob/bf4334f0af5e13d16b04c1cccd6237e683ac6f5f/v2.1/Greek/TAGSETS.xml)). Voice `e` means middle or passive (mp). Tense `a` means aorist.

Probe: relation inventory and compound labels in the v2.1 texts.

```python
# python3 -I probe_agdt_rel.py <treebank_data>/v2.1/Greek/texts
import glob, sys, collections, xml.etree.ElementTree as ET
c = collections.Counter(); empty = 0
for p in glob.glob(sys.argv[1] + '/*.xml'):
    for w in ET.parse(p).getroot().iter('word'):
        r = w.get('relation')
        if not r: empty += 1; continue
        c[r] += 1
print('distinct relations:', len(c), '| with _ compound:', sum(1 for r in c if '_' in r), '| empty:', empty)
print('non-ASCII:', sorted(r for r in c if not r.isascii()))
```

Result: 96 distinct relations, 66 of them `_` compounds (for example `ATR_ExD0_OBJ_CO`), two non-ASCII labels (`AuxΓ`, `AuxΚ`), and 196 words with an empty relation. These figures come from the session's full-corpus probe; the 196 and the two non-ASCII labels were not re-run in this final pass.

### 1.2 Crosswalk: AGDT → UD Perseus

Probe (re-run in this session, 13,919 sentence pairs):

```python
# python3 -I crosswalk_agdt.py <repo-root>; pairs keyed by file@sentence-id, equal non-elliptic token counts
import glob, os, collections, xml.etree.ElementTree as ET
S = '<repo-root>/'
def conllu(paths):
    recs = collections.OrderedDict()
    for p in paths:
        for line in open(p, encoding='utf-8'):
            line = line.rstrip('\n')
            if line.startswith('# sent_id'): sid = line.split('=', 1)[1].strip(); recs[sid] = []
            elif line[:1].isdigit() and '-' not in line.split('\t')[0] and '.' not in line.split('\t')[0]:
                recs[sid].append(line.split('\t'))
    return recs
ud = conllu(glob.glob(S + 'UD_grc_Perseus/*.conllu'))
pairs = 0; tense = collections.Counter()
for p in glob.glob(S + 'treebank_data/v2.1/Greek/texts/*.xml'):
    fname = os.path.basename(p)
    for s in ET.parse(p).getroot().iter('sentence'):
        key = f"{fname}@{s.get('id')}"
        if key not in ud: continue
        words = [w for w in s.iter('word') if not w.get('artificial')]
        rows = ud[key]
        if len(rows) != len(words): continue
        pairs += 1
        for w, r in zip(words, rows):
            pos = w.get('postag') or ''
            if len(pos) >= 9:
                tense[(pos[3], dict(x.split('=', 1) for x in r[5].split('|') if '=' in x).get('Tense', '-'))] += 1
print('pairs:', pairs)
print(sorted(tense.items(), key=lambda kv: -kv[1])[:8])
```

Findings (all from the same crosswalk tables):

- Tense `a` (aorist) → `Tense=Past`, with no `Aspect` feature in UD Perseus. This is a conversion choice, not a source difference.
- Voice `e` (mp) → `Voice=Mid` only, for 6,479 tokens. The passive is not distinguished in UD Perseus.
- The native 9-char postag is kept in XPOS (`XPOS: manual native`).
- DEPS is `_` everywhere.
- SpaceAfter appears in MISC for 24,950 tokens.
- AGDT relation compounds collapse to a single UD relation with a spread: for example `ADV_CO` (2,723 tokens) maps to `conj` (1,384), `advcl` (742), `obl` (458), `advmod` (91). The compound's detail is not preserved in UD.

Worked example, ἐγένετο in UD Perseus dev ([line 341](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/grc_perseus-ud-dev.conllu#L341)): AGDT `v3saim---` becomes `Mood=Ind|Number=Sing|Person=3|Tense=Past|VerbForm=Fin|Voice=Mid`. The same lemma is `root` in another sentence ([line 1462](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/grc_perseus-ud-dev.conllu#L1462)) and `conj` in line 341, so the same form has different relations in different contexts, which is expected of an annotated corpus.

### 1.3 PROIEL native and the PROIEL → UD converter

Native morphology is 10 positions: person, number, tense, mood, voice, gender, case, degree, strength, inflection. Native tokens carry `citation-part`, `presentation-after`, `information-status`, `antecedent-id`, and `slash` (19,939 secondary edges; `xsub` is the most common label).

The converter ([`morphology.rb`](https://github.com/proiel/proiel-cli/blob/385e59f052b9a08755376253aad199129a4afc9e/lib/proiel/cli/converters/conll-u/morphology.rb), [`conll-u.rb`](https://github.com/proiel/proiel-cli/blob/385e59f052b9a08755376253aad199129a4afc9e/lib/proiel/cli/converters/conll-u.rb)):

- XPOS is the native 3-character POS.
- DEPS is always `_`.
- MISC carries only `Ref` (from `citation-part`) and `LId` (lemma variant).
- `presentation-after`, `information-status`, and `antecedent-id` are dropped. `slash` is dropped.
- Empties with sort `P` are removed; `V` and `C` empties are removed by remnant handling.

Reproduction (17,038 sentence pairs with equal non-empty token counts; 44 length mismatches). The probe applies the converter's `MORPHOLOGY_MAP` to the native 10-char string and compares the result to UD PROIEL FEATS, per feature. It was run in the session scratch area (`probes/crosswalk.py proiel`) and is not reproduced inline here; its per-feature output is: Aspect 23,125 agree; Case 122,850; Gender 121,948; Mood 26,773; Number 149,623; Person 41,358; Tense 43,443; VerbForm 43,453; Voice 43,443; Degree 11,989; `form` identical on all 213,053 tokens; lemma identical on 213,051, differing on 2.

The converter rules reproduce UD PROIEL FEATS for all 213,053 token pairs. So in these pairs there is no version effect; the difference from the source is entirely in the converter's rules. Lemma base agrees on 213,051 tokens; 2 differ and were not inspected.

Cross-treebank inconsistency: aorist maps to `Aspect=Perf|Tense=Past` in UD PROIEL, but to `Tense=Past` with no Aspect in UD Perseus (§1.2). This difference is conversion, not source: both come from the AGDT/PROIEL tense values, which are the same aorist.

UD PROIEL DEPS has 0 non-empty values. MISC has `Ref` on every token and `LId` on 6,501 tokens, while the native file has 7,230 `#`-lemmas. The gap between 7,230 and 6,501 is unexplained.

Worked example, PROIEL MATT 1.18 (sentence [14677](https://github.com/syntacticus/syntacticus-treebank-data/blob/525cee4fb40590d7d514376c11acaed1bdd91c15/proiel/greek-nt.xml#L583)): the native empty token on εὑρέθη is dropped in UD; μητρὸς is `nsubj:pass` on the participle; πρὶν is reattached as `mark` on the infinitive; ἢ is `advmod`.

### 1.4 Empty nodes

UD can represent empty nodes (CoNLL-U decimal IDs). Here the AGDT `artificial` elliptic words are excluded from the Perseus crosswalk (§1.2), and the PROIEL converter removes sort `P`/`V`/`C` empties (§1.3). Whether the pinned UD Perseus release contains empty nodes was not checked. Where a release omits empties, that is an omission in the release, not a limit of the format: CoNLL-U supports empty nodes.

## 2. Greek morphology: present, preserved, normalized, lost

| Feature | AGDT native | UD Perseus | UD PROIEL | Status |
| --- | --- | --- | --- | --- |
| Full morphology string | present (9 chars) | preserved in XPOS; features normalized to FEATS | present natively (10 chars); converted FEATS; native 3-char POS in XPOS | preserved in XPOS for both; FEATS normalized |
| Aorist | `a` | `Tense=Past`, no Aspect | `Aspect=Perf|Tense=Past` | normalized differently across UD treebanks |
| Middle/passive `e` (mp) | present | `Voice=Mid` only (6,479 tokens) | converted by rule | normalized; passive lost in UD Perseus |
| Strength / inflection (PROIEL) | not in AGDT | not in UD | native present; dropped by converter | lost in UD |
| Relation compounds | 66 `_` compounds | collapsed to one UD label | `slash` secondary edges, `xsub` largest | lost in UD |
| Presentation/information status | not in AGDT | not in UD | native present (`presentation-after`, `information-status`) | lost in UD PROIEL |

Examples:

- Preserved: AGDT ἐγένετο `v3saim---` → UD Perseus XPOS kept, FEATS `Voice=Mid|Tense=Past` ([line 341](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/grc_perseus-ud-dev.conllu#L341)).
- Normalized: AGDT tense `a` → `Tense=Past` without Aspect in Perseus; the PROIEL aorist ἦν `3siia----i` ([line 581](https://github.com/syntacticus/syntacticus-treebank-data/blob/525cee4fb40590d7d514376c11acaed1bdd91c15/proiel/greek-nt.xml#L581)) becomes `Aspect=Perf|Tense=Past` in UD PROIEL.
- Lost: AGDT `ADV_AP_CO` (40 tokens) and `ATR_ExD0_OBJ_CO` (1 token) — compound detail is not in UD. PROIEL `slash` secondary edges are not in UD.

## 3. Homographs, citations, sentence identity

- Homographs: PROIEL native `lemma` uses `#n` numbering (for example `εἰμί#1`; 7,230 lemmas). AGDT lemmas have no digits and no `#`, so homograph disambiguation in AGDT happens in the POS/morphology only. UD PROIEL keeps the plain lemma in LEMMA and the variant in MISC `LId` for 6,501 tokens; the mismatch with 7,230 is unexplained.
- Citations: PROIEL `citation-part` (for example `MATT 1.18`) becomes MISC `Ref` in UD PROIEL. AGDT citations are carried by the file name (`tlg0003.tlg001.perseus-grc1.1.tb.xml`) and sentence `id`, not by a citation field inside the sentence.
- Sentence identity:
  - AGDT: file name plus sentence `id`. UD Perseus `sent_id` is `<file>@<id>` (for example `tlg0008.tlg001.perseus-grc1.13.tb.xml@1144`).
  - PROIEL: sentence `id` (for example `14677`), unique in the native file.
  - Gorman: file + sentence `id` + CTS `document_id` (`morphgrc` v1.5).
  - Pedalion: `ref` of the form `Leuven|docid|sent|word`. Not globally unique (§4).

## 4. Pedalion identity probe

```python
# python3 -I pedalion_ref.py <pedalion-root>
import glob, sys, collections, xml.etree.ElementTree as ET
root = sys.argv[1]
refs = collections.Counter(); empty = 0; total = 0
for p in glob.glob(root + '/**/*.xml', recursive=True):
    for w in ET.parse(p).getroot().iter('word'):
        r = w.get('ref')
        if r is None: continue
        total += 1
        if r.startswith('|') or r == '': empty += 1
        refs[r] += 1
rep = sum(c - 1 for c in refs.values() if c > 1)
print('words with ref:', total, '| distinct refs:', len(refs), '| repeated extra occurrences:', rep, '| empty-prefix refs:', empty)
```

Result (run this session): 281,253 words with `ref`; 274,058 distinct; 7,195 extra occurrences; 276 with an empty prefix. Pedalion `ref` is therefore not a usable global key without a document/sentence scope.

## 5. Candidate complete reading units

"Complete" here means the source contains a full, human-reviewed unit (a work, book, or book-range), not single sentences. These are inventories of evidence, not a recommended MVP.

### 5.1 AGDT v2.1 (human-reviewed, Perseus)

- Per-file inventory: header, edition, scheme, and per-file line/section bounds for 33 files. Work identities were confirmed from opening words:
  - tlg0011.tlg001 Trachiniae; .002 Antigone; .003 Ajax; .004 Oedipus Tyrannus; .005 Electra (Sophocles)
  - tlg0012.tlg001 Iliad; tlg0012.tlg002 Odyssey
  - tlg0007.tlg004 Lycurgus; tlg0007.tlg015 Alcibiades
  - tlg0003.tlg001 Thucydides 1
  - tlg0016.tlg001 Herodotus 1
  - tlg0085.tlg007 Eumenides
- Shortfalls: Odyssey book 10 has 573 of 574 lines (one-line shortfall, checked against known book length). Sophocles Trachiniae has a 46-line gap that is unexplained.
- Aeschylus tlg0085.tlg001 is probably *Suppliants* and was not confirmed.

### 5.2 PROIEL NT (reviewed)

- 24 NT books present; 11,506 reviewed sentences.
- 2 Peter, 1 John, and 2 John are absent.
- 3 John (2 sentences) and Jude (4 sentences) are very partial.
- Other NT books are contiguous by chapter.

### 5.3 Herodotus (PROIEL)

- Book 1: 212 of 216 chapters have annotated tokens.
- Books 2 and 3 are essentially absent; book 8 is partial. The coverage is selective.

### 5.4 Gorman and Pedalion

- Gorman: 166 files, AGDT-style schema (§1). Versions labelled "bu" are unresolved and were not inspected. The repository contains both `LICENSE` and `TREEBANK LICENSE` files ([LICENSE](https://github.com/vgorman1/Greek-Dependency-Trees/blob/d823e96d253277bc1872b83ff5ac8c393a0f830a/LICENSE)), which are not reconciled here. This is file-level evidence only; no licence conclusion is drawn.
- Pedalion: `aldt` v1.5 XML; coverage and per-book inventory were not completed in this pass.

## 6. Additional open treebanks discovered

- **UD Ancient Greek-PTNK** (listed in UD 2.18, 2026-05-15, which lists three Ancient Greek treebanks: Perseus, PROIEL, PTNK). Reported contents: Septuagint Genesis 1–50 and Ruth 1–4; CC BY-SA 4.0; projected from Hebrew, then automatically and manually corrected; metadata "Relations: manual native"; the v2.17 changelog adds `Ref`. **Not verified.** A clone was attempted and rejected by the user; no SHA is pinned, no README, licence file, or record was inspected, and no counts are given. Treat every PTNK statement as unverified until someone verifies it.
- **AthDGC** (arXiv 2606.15510; Zenodo DOI 10.5281/zenodo.20439182, v0.4.0): automatic Stanza `grc_proiel` annotation across classical periods. **Not human-reviewed**; it is not a candidate for human-reviewed units.

## 7. New precise questions (not in the map's fog)

1. Does UD PTNK (Septuagint) use the same aorist and middle feature conventions as UD Perseus or UD PROIEL? This needs a pinned clone to answer.
2. Why do PROIEL native `#`-lemmas (7,230) and UD PROIEL MISC `LId` (6,501) disagree?
3. Why does the aorist receive `Aspect=Perf` in UD PROIEL but no Aspect in UD Perseus, when both come from the same tense value?
4. Are the Trachiniae 46-line gap and the Odyssey book 10 one-line shortfall errors in the source release or in the file bounds?
5. Which Gorman "bu" versions are final, and is the dual licence file pair resolved upstream?

## 8. Limitations and blockers

- PTNK is unverified (see §6); its clone was rejected.
- Pedalion's relation labels, coverage, and per-book inventory were not completed.
- Gorman "bu" versions and the licence pair were not resolved.
- UD Perseus and UD PROIEL release notes end at v2.11 and v2.12 respectively; later UD releases were not inspected for changes.
- Source-version effects (2015 AGDT, 2023 PROIEL export) are not separated from conversion effects for the UD Perseus pairs, because the UD Perseus conversion was not reproduced; only the PROIEL converter was reproduced (§1.3).
- Statistics in §1.1 and §5 were produced by probes in the session scratch area, not all re-run in this final pass; only the crosswalk (§1.2), PROIEL reproduction (§1.3), and Pedalion probe (§4) were re-run for this document.
- No legal conclusions are drawn; licence evidence is recorded at file level only.
