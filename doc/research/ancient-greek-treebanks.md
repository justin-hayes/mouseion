# Ancient Greek Treebanks: Annotation Provenance and Conversion Fidelity

**Research date:** 2026-10-14

**Scope:** Inventory the Ancient Greek dependency treebanks that could feed a
reading tool, record per subset how annotation was produced and reviewed, and
trace what the Universal Dependencies (UD) releases keep or drop from their
originals. Licenses are recorded for the source data and for the converted
data separately. This document records evidence. It does not choose an
architecture, a first corpus, or an MVP.

**Tracking:** #2023 (under parent #2021).

## Summary

- **Map gist:** In the files checked, PROIEL is the only corpus where every
  sentence carries a `reviewed` status, but the UD PROIEL release is a 2018 conversion that drops empty
  nodes and the chronicle subset. AGDT is semi-automatically annotated, and UD
  Perseus contains no elliptic sentences, so its elliptic structure is absent
  from UD. Gorman and Pedalion are mixed-provenance corpora whose license files
  conflict or are unresolved.
- **PROIEL and UD PROIEL diverge in both sentences and tokens.** Against the
  2023 Syntacticus export, the UD release has 89 fewer NT+Herodotus sentences,
  2,180 fewer non-empty tokens, and no empty nodes. The P-empty drop is
  confirmed in a sample (MATT 1.18). The cause of the sentence and token gap
  beyond the empty drop is not established (see Open Questions).
- **UD Perseus excludes every AGDT sentence with an elliptic node.** Of 21,261
  AGDT sentences in the 18 UD-included files, 4,955 contain elliptic nodes and
  none of them appear in UD. A further 2,387 elliptic-free sentences in those
  files are also absent from UD, and no cause is documented upstream.
- **Gorman's and Pedalion's license files conflict or are unresolved.** The
  Gorman upstream README says BY-NC-SA 4.0, its own `TREEBANK LICENSE` says
  BY-SA 4.0, and the perseids mirror README says CC0 1.0. Pedalion's
  `TREEBANK_LICENSE` says BY-SA 4.0.
- **Sentence ids are not global identifiers in Pedalion.** 378 `(document_id,
  subdoc, id)` keys recur across different text files. Counting
  `external_examplesentences.xml` (266 sentences) raises that to 401 shared keys.

## Pinned Sources

Each source was obtained as a shallow clone at the commit below. Re-run the
commands in [Reproduction](#reproduction) against these SHAs.

| Source | Upstream | Commit (short) | License evidence |
| --- | --- | --- | --- |
| AGDT v2.1 Greek | https://github.com/PerseusDL/treebank_data (`v2.1/Greek/texts/`) | `bf4334f0af5e` | `README.md` line 15: CC BY-SA 3.0 US |
| UD Ancient Greek Perseus | https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus | `331ddef91411` | `README.md` line 116 and `LICENSE.txt`: CC BY-NC-SA 2.5 |
| PROIEL/Syntacticus Greek | https://github.com/syntacticus/syntacticus-treebank-data (`proiel/`) | `525cee4fb405` | `<license>` element and `proiel/README.md`: CC BY-NC-SA 4.0 |
| UD Ancient Greek PROIEL | https://github.com/UniversalDependencies/UD_Ancient_Greek-PROIEL | `a4ab8d436de9` | `README.md` line 33: CC BY-NC-SA 3.0 |
| PROIEL→UD converter | https://github.com/proiel/proiel-cli (`lib/proiel/cli/converters/conll-u.rb`) | `385e59f052b9` | Not checked in this document |
| Gorman Greek Dependency Trees | https://github.com/vgorman1/Greek-Dependency-Trees | `d823e96d2532` | `README.md` line 8: BY-NC-SA 4.0; `TREEBANK LICENSE`: BY-SA 4.0 |
| Gorman mirror (Perseids) | https://github.com/perseids-publications/gorman-trees | `83d93a554a46` | `README.md` line 144: CC0 1.0; `TREEBANK_LICENSE`: BY-SA 4.0 |
| Pedalion Greek trees | https://github.com/perseids-publications/pedalion-trees | `112c106b86b8` | `TREEBANK_LICENSE`: BY-SA 4.0 |

Release identifiers: UD PROIEL states that it was converted from the
`20180408` PROIEL release (`UD_Ancient_Greek-PROIEL/README.md` line 7). The
Gorman Zenodo deposit is version 1.0.1 (31 December 2019, DOI
10.5281/zenodo.3596076, per upstream `README.md` line 12). The PROIEL and
Gorman sources here are later than those releases.

## Corpus Inventory

Counts come from `scripts/treebank_probe.py` (see Reproduction). "Tokens" means
XML `<token>` or `<word>` elements; empties are counted separately where the
format marks them.

| Corpus | Files | Sentences | Tokens | Elliptic / empty nodes | Notes |
| --- | --- | --- | --- | --- | --- |
| AGDT v2.1 Greek (source) | 33 | 33,555 | 557,918 | 9,137 elliptic | `texts/*.xml`, AGDT/Perseus format |
| UD Perseus (derived) | 18 | 13,919 | 202,989 (UD README; not re-counted) | 0 elliptic | Subset of AGDT files; ids `file@AGDT-sentence-id` |
| PROIEL NT (source) | 1 | 11,506 | 140,773 | 8,407 empty | `greek-nt.xml`, all `status="reviewed"` |
| PROIEL Herodotus (source) | 1 | 5,665 | 85,166 | 1,347 empty | `hdt.xml`, all `status="reviewed"` |
| PROIEL Chronicle (source) | 1 | 1,008 | 24,612 | 323 empty | `chron.xml`, all `status="reviewed"`; not in UD |
| UD PROIEL (derived) | 3 | 17,082 | 214,005 | 0 empty | NT + Herodotus only |
| Gorman (source) | 166 | 29,547 | 692,547 | 9,564 elliptic | 103 `bu`-suffixed files (18,531 sentences) + 63 others (11,016) |
| Pedalion (source) | 40 | 18,791 | 281,253 | 4,892 elliptic | 39 text files (18,525 sentences) + `external_examplesentences.xml` (266) |

Notes on the rows:

- The AGDT row counts the 33 XML files under `texts/`. `TAGSETS.xml` sits
  outside that directory and is not counted.
- The UD Perseus token count is taken from the UD README and was not
  reproduced by the probe. The probe confirms all 13,919 UD sentence ids appear
  in the AGDT source.
- Gorman's `bu`-suffixed files and the others share no `(document_id, subdoc,
  id)` keys (0 shared keys). Whether the `bu` files are alternative annotation
  layers of the same text is **not established**.

## Annotation Provenance by Subset

### AGDT v2.1 / Perseus

- The data README says: "The data have been semi-automatically annotated. They
  have been annotated manually, but many corrections have been performed
  automatically in order to improve consistency." (`treebank_data/v2.1/Greek/README.MD`
  line 43.)
- The same README line says: "Morphology has been annotated with the help of
  the Morpheus tagger." Lemma sourcing is not separately checked.
- Elliptic nodes are marked `artificial="elliptic"` with `insertion_id`. Some
  carry placeholder forms (`[0]`, `[1]`, `[2]`) and some carry supplied words
  (for example `ἐστιν`, `εἶναι`, `,`). Counted: 9,137 elliptic nodes in 33 files.

### PROIEL / Syntacticus

- Every sentence in `greek-nt.xml`, `hdt.xml`, and `chron.xml` carries
  `status="reviewed"`. The schema allows `annotated`, `reviewed`, and
  `unannotated`, so the status field does mark review.
- The file headers list annotators with per-sentence counts. For example, NT
  lists Dagmar Wodtko (7,570 sentences) as the largest contributor, and
  Herodotus lists Þorsteinn Vilhjálmsson (2,127 sentences) and Bergthora
  Aegisdottir (2,058 sentences).
- Empty tokens carry `empty-token-sort` values `P`, `V`, and `C`. Counts are in
  the inventory table.

### Gorman (Greek Dependency Trees)

- Upstream README, lines 4–6: trees uploaded after January 2020, including all
  Demosthenes trees except 4 and 18, "were pre-parsed at the Pedalion Project
  by Toon Van Hal and Alek Keersmaekers and hand-corrected by V. Gorman."
- The Zenodo 1.0.1 release (31 December 2019) predates those additions. A
  citation of 1.0.1 therefore describes a different provenance state than the
  current repository.
- The current repository is therefore mixed-provenance: post-2020 trees are
  pre-parsed and then hand-corrected, and older trees are not described by the
  same statement.

### Pedalion

- The `pedalion-trees` README (Greek section) says: "Most sentences were
  annotated by Toon Van Hal, all of which are based on a first automated parsing
  by Alek Keersmaekers. In addition, there are student contributions…"
- Other sections of the same README describe other texts with different
  provenance (for example, one text "based on a preparsed version by Alek
  Keersmaekers", partly corrected by Toon Van Hal and Alek Keersmaekers).
- Labels that are not valid relation names appear in the data. The probe
  counts 126 `<word>` elements with an empty relation or a `!!!`/`%%%` error
  marker (77 empty; 49 with markers such as `!!!congruentiefout geslacht?!!!ADV`).
  These are **residue of error annotations** and need a filter before any use.
- Sentence ids are file-scoped. 401 `(document_id, subdoc, id)` keys occur in
  more than one file. Of the 401 shared keys, 400 have different text in each
  file. Identify a sentence by file plus id.

## Original Versus UD Conversions

### UD Perseus (from AGDT v2.1)

- The probe verified that all 13,919 UD sentence ids resolve to AGDT
  sentences in the 18 UD-included files.
- In those files, AGDT has 21,261 sentences. 4,955 contain elliptic nodes; 16,306
  do not.
- UD contains no sentence with an elliptic node. Therefore all 4,955 elliptic
  sentences in those files are absent from UD (count verified).
- UD contains 13,919 sentences. That leaves 2,387 elliptic-free AGDT sentences
  in the same files that are absent from UD (arithmetic: 16,306 − 13,919).
  **Cause not established.** The probe did not find an attribute that separates
  them from the included sentences.
- UD Perseus has no `LId` or `Ref` annotations, and its DEPS column is `_` on
  every token. The AGDT ids survive only in `# sent_id`.

### UD PROIEL (from PROIEL NT and Herodotus)

Verified this session:

- **Scope.** UD README line 3: the release "consists of the New Testament plus
  selections from Herodotus." The chronicle is excluded. Excluding it gives 17,171
  NT+Herodotus source sentences.
- **Version.** The UD README (line 7) says the conversion is based on the
  20180408 PROIEL release. The source here is the 2023 Syntacticus export. Any
  difference between them is a version effect, not necessarily conversion loss.
- **Sentences.** 89 NT+Herodotus source sentences are absent from UD (16 NT, 73
  Herodotus). No UD sentence is absent from the source.
- **Tokens.** Non-empty source tokens: 216,185. UD tokens: 214,005. Difference:
  2,180. The probe lists 44 sentences whose token counts differ. The first five
  printed all have more tokens in UD (for example, sentence 70149: 15 source vs
  17 UD).
- **Empty nodes.** UD has zero empty nodes. Source NT+Herodotus has 9,754 empty
  nodes (8,407 + 1,347).

Converter behaviour (`proiel-cli`, `lib/proiel/cli/converters/conll-u.rb`):

- Line 54 and line 212 remove `empty_token_sort == 'P'` tokens before output.
- Lines 219–224 comment that the remaining empties (V and C) are removed by
  attaching their dependents to a grandmother node with the `remnant` relation.
  The UD PROIEL output contains no `remnant` relation (0 occurrences, checked).
- Line 57 splits forms on whitespace into subtokens. Lines 64–65 take lemmas by
  position. Line 250 splits `lemma#variant` and line 489 writes the variant
  as `LId`. Line 259 writes the citation part as `Ref=`. Around line 502 the
  slashes column (DEPS) is written as `_`.
- Line 639 promotes a node with the `orphan` relation.

Concrete samples (both verified in the UD file this session):

- **MATT 1.18 (PROIEL sentence 14677).** PROIEL has an empty P node (id 862390,
  relation `sub`, head 267030 = `εὑρέθη`). The empty is absent from UD. In UD,
  `εὑρέθη` (id 12, `root`) has no `nsubj` or `nsubj:pass` dependent. The
  dependent `αὐτοὺς` (id 11) is attached as `nsubj` to `συνελθεῖν` (id 10).
  The PROIEL `ἢ` (id 267027, relation `aux`) appears in UD as `advmod`.
- **Herodotus 1.10 (PROIEL sentence 64428).** In UD, `ἦν` (id 8) is `AUX`
  with relation `cop`, and `ἕτοιμος` (id 9) is the `root`. UD `ὡς` (id 4) is
  `mark` on `ἐδύνατο` (id 6). The PROIEL side of this example was not re-checked
  in this pass.

### What is not known yet

- The 2,387 AGDT elliptic-free sentences absent from UD.
- Whether the 89 missing PROIEL sentences and the 44 token-count differences are
  due to the 2018→2023 release change, the converter, or both.
- Whether the two `Ref` mismatches noted earlier (`ACTS 7.15` vs `ACTS_7.14`,
  `REV 11.17` vs `REV_11.16`) are a conversion error or a source citation
  error. Not checked.

## Licenses

Source and converted licenses are listed separately. "Converted" means the UD
release built from that source.

| Data | Source license | Converted license | Evidence |
| --- | --- | --- | --- |
| AGDT v2.1 Greek → UD Perseus | CC BY-SA 3.0 US (AGDT) | CC BY-NC-SA 2.5 (UD Perseus) | `treebank_data/README.md` line 15; `UD_Ancient_Greek-Perseus/README.md` line 116 |
| PROIEL → UD PROIEL | CC BY-NC-SA 4.0 (Syntacticus file metadata, current export) | CC BY-NC-SA 3.0 (UD PROIEL) | Syntacticus `<license>`; `UD_Ancient_Greek-PROIEL/README.md` line 33 |
| Gorman | BY-NC-SA 4.0 (upstream README); BY-SA 4.0 (`TREEBANK LICENSE`) | Mirror: CC0 1.0 (README), BY-SA 4.0 (`TREEBANK_LICENSE`) | Upstream README line 8; perseids `README.md` line 144 |
| Pedalion | BY-SA 4.0 (`TREEBANK_LICENSE`) | Not converted here | `pedalion-trees/TREEBANK_LICENSE` |

Points to keep separate:

- **PROIEL and UD PROIEL versions differ.** CC BY-NC-SA 3.0 (UD release) and
  4.0 (current source) are different versions of the same family. The
  difference is a version effect, not a same-release conflict.
- **Gorman has a same-release conflict** in the current repository: the README
  says BY-NC-SA 4.0 while `TREEBANK LICENSE` says BY-SA 4.0. The perseids mirror
  README says CC0 1.0. The explicit `TREEBANK_LICENSE` file and the README
  disagree, and this document does not decide which governs. The resolution is
  for the rights holder or a human reviewer.
- **Derived tables.** This document does not analyse whether a derived
  frequency table or count inherits a share-alike obligation from its source.
  That needs a legal read and is outside this research.
- **Text versus annotation.** The file headers give a separate text license:
  `greek-nt.xml` has `<electronic-text-license>Public domain</electronic-text-license>`,
  and `hdt.xml` has `Free, non-commercial`. The file-level `<license>` element
  (CC BY-NC-SA 4.0) is recorded separately. This document did not determine how
  those header statements apply to converted data.

## Candidate Passages for a First Slice

These are examples of where the sources differ. They are not a recommendation.

- MATT 1.18 (PROIEL 14677): empty subject absent from UD; shows the P-drop and
  its effect on `εὑρέθη`.
- Herodotus 1.10 (PROIEL 64428): copula and mark structure differ in UD.
- Pedalion error-residue relation labels (containing `!!!`), which any parser
  over these files has to handle.

## Open Questions

Sharper than before this research:

1. **Which annotation layer is authoritative for PROIEL?** The 2023 export has
   empty nodes and 2,180 more non-empty tokens than UD. Should the 2023 export be
   the baseline, or the UD release?
2. **Does the empty-node drop change any learner-visible analysis?** MATT 1.18
   shows a subject that exists in PROIEL and is absent from UD. Whether this
   matters depends on the feature, not on the corpus.
3. **Why are 2,387 elliptic-free AGDT sentences absent from UD Perseus?** No
   documented criterion has been found.
4. **Which Gorman license governs?** Three explicit statements conflict.
5. **How should Pedalion sentences be identified?** Sentence ids recur across
   files, so identity must include the file name.

Still fog:

- Whether the Gorman `bu`-suffixed files are distinct annotation layers.
- Whether the 44 PROIEL token-count differences are all subtoken splits.
- Whether the converter's `remnant` reattachment changes head attachment
  beyond the examples checked.
- Alternatives beyond the sources listed above were not surveyed.

## Limitations

- **Upstream history.** The commits pinned above are the versions this document
  describes. Later upstream changes are not reflected.
- **Converter version.** The `proiel-cli` commit is from the clone, not from the
  version that produced UD 2.x. Its behaviour in the UD release is inferred from
  the UD README and the converter source, not from re-running it.
- **Counts outside the probe.** The UD Perseus token count (202,989) is taken
  from the UD README and was not re-counted by the probe.
- **Not checked.** The two `Ref` mismatches noted in an earlier pass
  (`ACTS 7.15` vs `ACTS_7.14`, `REV 11.17` vs `REV_11.16`), the Pedalion
  relation semantics, and the Gorman `bu` file content were not resolved.
- **No licensing advice.** License evidence is recorded from the files; no legal
  conclusion is drawn.

## Reproduction

Work in a scratch directory outside the repository. Clone each source, then
check out the commit in [Pinned Sources](#pinned-sources) if the clone's HEAD
differs. The probe expects `src/` (the clones) and `scripts/` in the same
directory.

```sh
mkdir -p ~/.cache/scratch/ancient-greek-treebanks/src && cd ~/.cache/scratch/ancient-greek-treebanks
git -C src clone --depth 1 https://github.com/PerseusDL/treebank_data.git
git -C src clone --depth 1 https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus.git
git -C src clone --depth 1 https://github.com/UniversalDependencies/UD_Ancient_Greek-PROIEL.git
git -C src clone --depth 1 https://github.com/syntacticus/syntacticus-treebank-data.git
git -C src clone --depth 1 https://github.com/proiel/proiel-cli.git
git -C src clone --depth 1 https://github.com/vgorman1/Greek-Dependency-Trees.git
git -C src clone --depth 1 https://github.com/perseids-publications/gorman-trees.git
git -C src clone --depth 1 https://github.com/perseids-publications/pedalion-trees.git
```

The probe script is `scripts/treebank_probe.py`, kept in the scratch directory.
Its contents are reproduced here so the counts can be checked.

```python
"""Count the Ancient Greek treebank facts cited in doc/research/ancient-greek-treebanks.md."""
import glob
import os
import sys
import xml.etree.ElementTree as ET
from collections import Counter

SRC = "src"


def agdt(xml_dir="treebank_data/v2.1/Greek/texts"):
    files = sorted(glob.glob(f"{SRC}/{xml_dir}/*.xml"))
    sents = words = elliptic = 0
    for p in files:
        for s in ET.parse(p).getroot().iter("sentence"):
            sents += 1
            for w in s.iter("word"):
                words += 1
                if w.get("artificial"):
                    elliptic += 1
    print("AGDT v2.1 Greek files", len(files), "sentences", sents, "words", words, "elliptic nodes", elliptic)


def agdt_ud_overlap():
    ud_ids = set()
    for p in glob.glob(f"{SRC}/UD_Ancient_Greek-Perseus/*.conllu"):
        for line in open(p, encoding="utf-8"):
            if line.startswith("# sent_id"):
                ud_ids.add(line.split("=", 1)[1].strip())
    # UD sent_id = "<AGDT file name incl. .xml>@<AGDT sentence id>"
    files = {i.split("@")[0] for i in ud_ids}
    agdt_sents = {}
    for name in files:
        path = f"{SRC}/treebank_data/v2.1/Greek/texts/{name}"
        if not os.path.exists(path):
            print("no AGDT file for UD id", name)
            continue
        for s in ET.parse(path).getroot().iter("sentence"):
            key = f"{name}@{s.get('id')}"
            agdt_sents[key] = any(w.get("artificial") for w in s.iter("word"))
    print("UD Perseus sent_ids", len(ud_ids), "UD files", len(files))
    print("AGDT sentences in UD stems", len(agdt_sents), "with elliptic nodes", sum(agdt_sents.values()))
    print("UD ids found in AGDT", len(ud_ids & set(agdt_sents)), "UD ids missing from AGDT", len(ud_ids - set(agdt_sents)))
    print("AGDT elliptic sentences present in UD", sum(1 for k in ud_ids if agdt_sents.get(k)))


def proiel():
    src = {}
    for name in ("greek-nt", "hdt", "chron"):
        r = ET.parse(f"{SRC}/syntacticus-treebank-data/proiel/{name}.xml").getroot()
        toks = list(r.iter("token"))
        empty = sum(1 for t in toks if t.get("empty-token-sort"))
        sents = list(r.iter("sentence"))
        print(name, "sentences", len(sents), "tokens", len(toks), "empty", empty, "non-empty", len(toks) - empty)
        if name != "chron":
            for s in sents:
                src[s.get("id")] = sum(1 for t in s.iter("token") if not t.get("empty-token-sort"))
    ud = {}
    sid = None
    for p in glob.glob(f"{SRC}/UD_Ancient_Greek-PROIEL/*.conllu"):
        for line in open(p, encoding="utf-8"):
            if line.startswith("# sent_id"):
                sid = line.split("=", 1)[1].strip()
                ud[sid] = 0
            elif line[:1].isdigit() and "." not in line.split("\t")[0] and "-" not in line.split("\t")[0]:
                ud[sid] += 1
    print("UD PROIEL sentences", len(ud), "tokens", sum(ud.values()))
    print("NT+Hdt source sentences", len(src), "non-empty tokens", sum(src.values()))
    print("source not in UD", sum(1 for k in src if k not in ud), "UD not in source", sum(1 for k in ud if k not in src))
    print("sentences with differing token counts", sum(1 for k in ud if k in src and ud[k] != src[k]))


def _gorman_like(xml_glob, skip=()):
    files = sorted(p for p in glob.glob(xml_glob, recursive=True) if os.path.basename(p) not in skip)
    sents = words = elliptic = 0
    bu_files = 0
    keys = {}
    for p in files:
        bu_files += " bu" in os.path.basename(p)
        for s in ET.parse(p).getroot().iter("sentence"):
            sents += 1
            keys.setdefault((s.get("document_id"), s.get("subdoc"), s.get("id")), set()).add(p)
            for w in s.iter("word"):
                words += 1
                elliptic += bool(w.get("artificial"))
    shared = sum(1 for v in keys.values() if len(v) > 1)
    print(f"files {len(files)} (bu-suffixed {bu_files}) sentences {sents} words {words} elliptic {elliptic} keys shared across files {shared}")


def gorman():
    _gorman_like(f"{SRC}/Greek-Dependency-Trees/**/*.xml")


def pedalion():
    _gorman_like(f"{SRC}/pedalion-trees/**/*.xml")
    _gorman_like(f"{SRC}/pedalion-trees/**/*.xml", skip=("external_examplesentences.xml",))


if __name__ == "__main__":
    {"agdt": agdt, "overlap": agdt_ud_overlap, "proiel": proiel, "gorman": gorman, "pedalion": pedalion}[sys.argv[1]]()
```

Run it from the directory that contains `src/` with `-I` (isolated mode, because
the clones are untrusted):

```sh
python3 -I scripts/treebank_probe.py agdt      # AGDT v2.1: 33 files, 33,555 sentences, 557,918 words, 9,137 elliptic
python3 -I scripts/treebank_probe.py overlap   # UD Perseus vs AGDT: 13,919 ids, 4,955 elliptic sentences, 0 in UD
python3 -I scripts/treebank_probe.py proiel    # PROIEL vs UD PROIEL: 89 missing sentences, 44 differing token counts
python3 -I scripts/treebank_probe.py gorman    # Gorman: 166 files, 29,547 sentences, 0 shared keys
python3 -I scripts/treebank_probe.py pedalion  # Pedalion: 40 files (401 shared keys), 39 text files without the examples file (378 shared)
```

Two spot checks, run inline:

```sh
# Relation labels that are error residue or empty in Pedalion (expect 126)
python3 -I -c "
import glob, xml.etree.ElementTree as ET, collections
c = collections.Counter()
for p in glob.glob('src/pedalion-trees/public/xml/*.xml'):
    for w in ET.parse(p).getroot().iter('word'):
        r = w.get('relation') or ''
        if '!!!' in r or '%%%' in r or r.strip() == '':
            c[r] += 1
print(sum(c.values()), c.most_common(4))"

# UD dependents of εὑρέθη (id 12) in MATT 1.18 (expect no nsubj)
awk '$0=="# sent_id = 14677"{f=1} f&&/^$/{exit} f' src/UD_Ancient_Greek-PROIEL/*.conllu \
  | grep -P '^\d+\t' | awk -F'\t' '$7==12 || $8~/^nsubj/ {print $1, $2, "head="$7, $8}'
```

Delete the scratch directory when the work ends. The repository keeps only this
file.
