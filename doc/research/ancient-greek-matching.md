# Ancient Greek: CTS identity and cross-corpus text-to-annotation matching

- Date: 2026-10-14
- Issue: justin-hayes/mouseion#2024 (under map #2021)
- Baseline: `b262ed0a`
- Supersedes: this file at `e13ce04e`, which the parent reopened for factual and reproducibility errors. Its quantitative claims are withdrawn or replaced below.
- Scope: research only. No importer, schema, or production code changed.

## Summary

- The CTS specification defines a four-part work component (`TEXTGROUP.WORK.VERSION.EXEMPLAR`), so an edition is part of a CTS URN. The earlier statement that "version or exemplar" is not in the URN was wrong.
- At canonical-greekLit `01b725d8`, the counts are: 2,538 XML under `data/`, of which 926 are `__cts__.xml` metadata and 1,612 are non-metadata edition files. There are 810 `perseus-grc` editions across 769 work keys, and 5 exact `perseus-grc1.xml` files.
- Of 1,499 `migrated` greekLit tracking entries, 1,462 point to files absent at HEAD. Three independent checks reproduce this. Its cause (stale tracking) is inferred, not established.
- Gorman's treebanks use 217 distinct CTS URN strings (24,943 occurrences). Six name an edition present at HEAD; 211 name a `perseus-grc1` edition that is absent while the work is present.
- AGDT v2.1 Greek: of 33 files, 9 name an edition present at HEAD, 17 name an absent one, and 7 are partial-range files whose work component breaks the spec grammar.
- One line-level join was rebuilt (AGDT v2.1 Suppliants vs Perseus `grc2`). Of 1,052 lines in both, 1,017 agree at L2 (punctuation, whitespace and apostrophe-like marks removed). The 35 residual lines are not all classified; four are shown with exact text in §6.
- Licences were checked file by file. The AGDT licence is CC BY-SA 3.0 US per its root README (not NC). Gorman and Pedalion have conflicting or unassigned treebank licence statements. PROIEL is CC BY-NC-SA 3.0 US per its README.
- Diorisis was not re-verified in this revision. No Diorisis match ratio is reported.
- GLAUx (1,421 rows) and OGA (1,999 work entries) were checked at identifier and file level (§5.6). GLAUx `TLG` IDs are work-level, with no edition. OGA `urn_cts` values resolve to 1,656 present editions at the release pins the README names. Three GLAUx XML files are partly or mainly `analysis="manual"`, which qualifies the source survey's description of GLAUx as automatic.

## Corrections to the previous revision (e13ce04e)

| Earlier claim | Status | Corrected statement |
| --- | --- | --- |
| The CTS URN has no version or exemplar | **False** | The spec defines VERSION (edition or translation) and EXEMPLAR as URN parts (§3). |
| The same edition URN in two repos shows an identity inconsistency | **Not shown** | A shared URN is consistent with the spec. Whether the text at a shared URN matches across repos was not tested here (unknown). |
| 2,538 XML files; 7 `perseus-grc1` files | **Partly wrong** | 2,538 includes 926 `__cts__.xml` metadata files. Non-metadata is 1,612. The exact `perseus-grc1.xml` count is 5. The 7 came from a substring glob. |
| 1,462 stale tracking targets | **Reproduced; cause not established** | See §4. Path normalisation is correct; the result holds under three checks. |
| AGDT `cite` is work-level, edition only in `document_id` | **Partly right** | Word `cite` is work-level (verified). The edition also appears on each sentence `document_id` and on the root `cts` attribute (§5.1). |
| AGDT licence is CC BY-NC-SA 3.0 | **False** | Root README says CC BY-SA 3.0 US (§7). |
| Gorman README says CC BY-NC-SA 4.0; `TREEBANK LICENSE` says SA 4.0 | **Withdrawn** | The source repo used for that claim was not identified. The current `perseids-publications/gorman-trees` README says CC0 1.0 (§7). |
| Pedalion moved to an MIT repo, so MIT applies | **Incomplete** | The MIT `LICENSE` covers code. The README says treebank licence is not yet assigned ("probably" CC BY-SA 4.0) (§7). |
| Gorman: 219 IDs, 9 registered, 25 `perseus-grc1` | **Replaced** | 217 distinct CTS URNs; 6 present; 211 absent (§5.2). |
| AGDT vs Perseus: 1,013 of 1,060 lines; PROIEL vs Perseus 47% coverage; Diorisis 325 of 515 | **Withdrawn** | The scripts were lost and the numbers cannot be reproduced. AGDT was rebuilt (§6). PROIEL-Perseus and Diorisis joins were not rebuilt; no ratio is reported. |
| Perseus `grc2` vs 1st1K `grc1` example for `tlg0627.tlg001` | **Withdrawn** | Not re-verified in this revision. |
| Diorisis converter is minimal | **Withdrawn** | Not inspected in this revision. The converter is unverified (§5.5). |
| AGDT registry counts 9 / 17 / 7 (§5.1) | **Previously ad hoc; now reproducible** | `cts_probe.py agdt-registry` reproduces the same counts (§5.1, §11). |

## 1. Question and scope

Can a CTS URN identify the same text across open Ancient Greek corpora, and can a passage in one corpus be matched to the same passage's tokens and annotations in another? What evidence supports each answer, and what remains unknown?

Method: each claim cites an inspected file, header, record, or licence at a pinned commit. The probes read git objects at pinned SHAs (`git ls-tree`, `git show`, `git log`), not the working tree. The full probe source is in §11.

Status labels: **V** = verified by a probe or a direct file read in this revision. **I** = inferred from verified facts. **U** = unknown.

## 2. Pinned inputs

| Input | Pin | Date (commit) | Remote | Notes |
| --- | --- | --- | --- | --- |
| canonical-greekLit | `01b725d835e6e733062ffd79e0efdbae1ba06e5c` | 2026-10-06 | https://github.com/PerseusDL/canonical-greekLit | Blobless clone. Working tree not used. |
| treebank_data (AGDT) | `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f` | 2024-06-20 | https://github.com/PerseusDL/treebank_data | GitHub API `license: null`. |
| gorman-trees | `83d93a554a46ff6e9a34a15a9e0be2dff270a612` | 2022-01-20 (pushed 2023-02-10) | https://github.com/perseids-publications/gorman-trees | Description: "Treebanks by Vanessa Gorman". |
| Gorman_Greek_Dependency_Trees | no commits | pushed 2019-10-27 | https://github.com/NinaCengic/Gorman_Greek_Dependency_Trees | Empty (`size: 0`, no commits). Lineage relation to gorman-trees: U. |
| pedalion-trees | `112c106b86b865ddde4be7e5f3e409d9c42689f9` | 2022-01-20 (pushed 2023-01-24) | https://github.com/perseids-publications/pedalion-trees | Current repo (`master`). |
| proiel-treebank | `8e388967a1335ed12335ddc655fe46993ee7d57a` | 2023-05-02 | https://github.com/proiel/proiel-treebank | `hdt.xml` sha256 `1d853bc5d34fbb2d05c56198b55fb08a667126e3be8bc5a6199083bb0104b92c`. |
| CTS URN specification | 2.0.rc.1 (Feb 2014) | fetched 2026-10-14 | https://cite-architecture.github.io/ctsurn_spec/ | Editors Blackwell and Smith. Later revisions not checked (U). |
| GLAUx | `b077d8f6ff429a5c7245954bc16bb7d1d7948823` | 2026-02-19 | https://github.com/alekkeersmaekers/glaux | `metadata.txt` fetched at this SHA, plus three XML heads. Repo not cloned. |
| OGA | `a69d917dbe117a307829977dd556e22f05ce0715` | 2025-03-27 | https://github.com/OperaGraecaAdnotata/OGA | `urn_cts/texts/urn_cts.xml` fetched at this SHA. Zenodo archive (about 8.6 GB per the source survey) not downloaded. |
| canonical-greekLit release `0.0.11376425141` | `476fb6133498e5710de7eec0b492791da86be4b4` | 2024-10-17 | as above | Release named in the OGA README. Used for the OGA check. |
| First1KGreek release `1.1.11352615003` | `9e9592d4a1c1df6d1849780fd1ccd3491a880d89` | 2024-10-15 | https://github.com/OpenGreekAndLatin/First1KGreek | Release named in the OGA README. Blobless clone, trees only. |
| AGDT sample file | `v2.1/Greek/texts/tlg0085.tlg001.perseus-grc2.tb.xml` at AGDT pin | | | sha256 `cc0172386ce2b43f6867b6158339b931f56207fbd954e0ea5ddc4cbc9cdd10a3`. |
| Perseus sample file | `data/tlg0085/tlg001/tlg0085.tlg001.perseus-grc2.xml` at canonical pin | | | sha256 `5e18ab5b96c03ada774c21ea772fe98de262d3287aaf67de482883d1119fc079`. |

Reproducible fetch (run in an empty scratch directory):

```sh
git clone --filter=blob:none https://github.com/PerseusDL/canonical-greekLit.git canonical-greekLit
git clone --filter=blob:none https://github.com/PerseusDL/treebank_data.git treebank_data
git clone --filter=blob:none https://github.com/perseids-publications/gorman-trees.git gorman-trees
git clone --filter=blob:none https://github.com/perseids-publications/pedalion-trees.git pedalion-trees
git clone --filter=blob:none https://github.com/proiel/proiel-treebank.git proiel-treebank
git -C canonical-greekLit checkout -q 01b725d835e6e733062ffd79e0efdbae1ba06e5c
git -C treebank_data checkout -q bf4334f0af5e13d16b04c1cccd6237e683ac6f5f
git -C gorman-trees checkout -q 83d93a554a46ff6e9a34a15a9e0be2dff270a612
git -C pedalion-trees checkout -q 112c106b86b865ddde4be7e5f3e409d9c42689f9
git -C proiel-treebank checkout -q 8e388967a1335ed12335ddc655fe46993ee7d57a
```

## 3. CTS specification: grammar and implications

Source: CTS URN specification 2.0.rc.1, https://cite-architecture.github.io/ctsurn_spec/ (V, read 2026-10-14). Quoted phrases are exact.

Grammar:

- Overall: "urn:cts:CTSNAMESPACE:WORK:PASSAGE" (section *Overall structure of a CTS URN*). The passage is separated from the work by a colon.
- Work component: "TEXTGROUP.WORK.VERSION.EXEMPLAR", where "TEXTGROUP is a mandatory value, WORK, VERSION and EXEMPLAR are optional values" (section *The work component*).
- The work component "consists of one to four parts."
- VERSION: "The third part of the work component is allowed if the URN also includes a value for a work, and must be a valid value for a version (edition or translation) in the parent notional work."
- EXEMPLAR: "The fourth part of the work component is allowed if the URN also includes a value for a version, and must be a valid value for an exemplar in the parent version."
- Passage: "The passage component is a hierarchy of one or more levels expressing a logical citation scheme applying to all versions of a text." It "may have a non-null value only if the work component includes two or more parts."
- Substrings: "Indexed substring references are permitted only with URNs at the version or exemplar level."
- Semantic note in *Semantics of a CTS citation*: a version is an identifier "corresponding to the expression level of FRBR" (in the spec, "This in turn may be followed with an identifier for a specific version of that work ..., corresponding to the expression level of FRBR"). The spec also says "(Note that there is no level of a CTS URN correponding to the FRBR manifestation.)" (sic).

Implications:

1. **Edition is in the URN [V].** `urn:cts:greekLit:tlg0085.tlg001.perseus-grc2` has three levels: textgroup `tlg0085`, work `tlg001`, version `perseus-grc2`. A URN with no third part does not identify an edition.
2. **Passage keys are version-independent by design [V].** The spec says the passage scheme applies to all versions. A line key such as `urn:cts:greekLit:tlg0085.tlg001:1` is spec-valid and joins across editions. It does not identify which edition's text is at that line. The text join must therefore check the version explicitly.
3. **Token addresses need a version URN [V].** Substring addressing (`@word`) is valid only at version or exemplar level. A work-level passage key (AGDT `cite`) cannot address a token within a line. Any token-level join must go through a version-level URN.
4. **AGDT root `cts` has an extra fourth part [V for the string; I for its meaning].** `urn:cts:greekLit:tlg0085.tlg001.perseus-grc2.tb` parses under the grammar as an exemplar `tb` of version `perseus-grc2`. The spec allows this. The treebank's intent is not documented in the files inspected (U).
5. **Partial-range AGDT files break the grammar [V].** The root `cts` of `tlg0548.tlg001.perseus-grc1.1.1.1-1.4.1.tb.xml` is `urn:cts:greekLit:tlg0548.tlg001.perseus-grc1.1.1.1-1.4.1.tb`. It has no colon, so the whole string is the work component, with nine dot-separated segments. The spec allows at most four work parts, and the passage must follow a colon. These URNs are non-conformant.

## 4. Canonical counts and tracking at `01b725d8`

### 4.1 File counts [V]

Probe: `cts_probe.py counts` (§11). Output:

```
xml under data/: 2538 (__cts__ metadata: 926; non-metadata: 1612)
non-metadata matching tlgNNNN/tlgNNN/<urn>.xml: 1612 of 1612
perseus-grc editions: 810; distinct work keys: 769
exact suffix perseus-grc1.xml files: 5
```

The earlier "2538 XML" conflated metadata with editions. The earlier "7" came from a substring glob. The 1,612 / 810 / 769 figures match the independent count from the source agent (reconciled here, not accepted on trust).

### 4.2 Tracking file: format and sample record [V]

`canonical-greekLit.tracking.json` is at the repo root. It is a JSON object keyed by URN. The `target` field is repo-relative with a `canonical-greekLit/` prefix; all 1,561 entries use it. There are no absolute URLs or basename-only targets. Stripping that prefix once is the correct normalisation.

Sample record (exact from the pinned file):

```json
"urn:cts:greekLit:tlg0001.tlg001.perseus-grc2": {
  "epidoc_compliant": false,
  "fully_unicode": true,
  "git_repo": "canonical-greekLit",
  "has_cts_metadata": true,
  "has_cts_refsDecl": true,
  "last_editor": "Stella Dee",
  "status": "migrated",
  "target": "canonical-greekLit/data/tlg0001/tlg001/tlg0001.tlg001.perseus-grc2.xml",
  "valid_xml": true
}
```

Statuses: 1,499 `migrated`, 62 `not migrated`.

### 4.3 Absent targets: three checks [V]

For the 1,499 `migrated` greekLit entries, after stripping the prefix:

| Result | Count |
| --- | --- |
| target in HEAD tree | 37 |
| target absent at HEAD; deleted in history | 1,300 |
| target absent at HEAD; added, not at HEAD | 162 |
| target absent at HEAD; never in history | 0 |

The three checks are: (a) membership in `git ls-tree -r` at the pin; (b) a basename search over the tree, which finds 0 of the 1,462 absent targets under another path; (c) `git log --all --name-status` on the clone, showing each absent target was once added or deleted. Check (c) covers only history reachable from the clone's refs.

Interpretation: tracking drift is the most likely explanation (I). The 162 "added, not at HEAD" targets were not traced to a rename or removal commit (U).

The claim is reproduced. The probe is correct: the prefix is handled, and the result does not depend on the working copy.

## 5. Treebank identifiers and mapping

### 5.1 AGDT v2.1 Greek [V]

Source: `v2.1/Greek/` at `bf4334f0` (35 files in the tree, 33 of them `.tb.xml`; the other two are `README.MD` and `TAGSETS.xml`, not treebanks, and were not examined). Sample: `texts/tlg0085.tlg001.perseus-grc2.tb.xml`.

Identifiers in the sample file (exact):

```xml
<treebank version="2.1" xml:lang="grc" oldId="Perseus:text:9999.01.0010" cts="urn:cts:greekLit:tlg0085.tlg001.perseus-grc2.tb">
...
<sentence subdoc="1-4" id="2897945" document_id="urn:cts:greekLit:tlg0085.tlg001.perseus-grc2">
...
<word id="1" form="Ζεὺς" lemma="Ζεύς" postag="n-s---mn-" head="4" relation="SBJ" cite="urn:cts:greekLit:tlg0085.tlg001:1"/>
```

Sample counts: 529 sentences; 6,071 words, of which 1,102 have an empty or missing `cite` (the join probe's "without cite" count). The `document_id` on each sentence and the root `cts` both carry the edition. The word `cite` carries no edition.

Edition registration across the 33 v2.1 Greek `.tb.xml` files (`cts_probe.py agdt-registry`, §11; verbatim output):

```
v2.1/Greek .tb.xml files: 33: {'name does not match edition pattern': 7, 'edition file absent at canonical pin': 17, 'edition file present at canonical pin': 9}
absent, by edition label: {'perseus-grc1': 17}
present: ['tlg0011.tlg001.perseus-grc2.tb.xml', 'tlg0011.tlg002.perseus-grc2.tb.xml', 'tlg0011.tlg005.perseus-grc2.tb.xml', 'tlg0059.tlg001.perseus-grc1.tb.xml', 'tlg0085.tlg001.perseus-grc2.tb.xml', 'tlg0085.tlg002.perseus-grc2.tb.xml', 'tlg0085.tlg003.perseus-grc2.tb.xml', 'tlg0085.tlg004.perseus-grc2.tb.xml', 'tlg0085.tlg006.perseus-grc2.tb.xml']
pattern mismatches: ['tlg0003.tlg001.perseus-grc1.1.tb.xml', 'tlg0008.tlg001.perseus-grc1.12.tb.xml', 'tlg0008.tlg001.perseus-grc1.13.tb.xml', 'tlg0016.tlg001.perseus-grc1.1.tb.xml', 'tlg0060.tlg001.perseus-grc3.11.tb.xml', 'tlg0096.tlg002.opp-grc2.1-53.tb.xml', 'tlg0548.tlg001.perseus-grc1.1.1.1-1.4.1.tb.xml']
```

Summary of the same output:

- 9 name an edition file present at HEAD.
- 17 name an edition file absent at HEAD (for example `tlg0007.tlg004.perseus-grc1.tb.xml`, `tlg0007.tlg015.perseus-grc1.tb.xml`, `tlg0011.tlg003.perseus-grc1.tb.xml`).
- 7 are partial-range files that do not match the edition-file pattern (see §3, item 5).

All 33 root `cts` attributes end in `.tb`.

### 5.2 Gorman (`perseids-publications/gorman-trees`) [V]

Probe output (`cts_probe.py ids`):

```
public/xml: files 158; files with CTS URN 149; files with Leuven|NNNN-NNN 10
distinct CTS URNs 217, occurrences 24943: {'edition file absent; work present': 211, 'edition file present at HEAD': 6}
distinct legacy IDs 9: {'work present': 9} absent works: []
```

Sample (exact, `public/xml/aeschines-1-1-50-bu1.xml`, line 33, on the `<sentence id="1">` element):

```xml
<sentence id="1"
          document_id="urn:cts:greekLit:tlg0026.tlg001.perseus-grc1"
```

Distinct CTS URN strings (217) that name an edition absent at HEAD: 211, of which 209 are `perseus-grc1`, one is `perseus-grc2`, and one is `perseus-grc3`. Six name an edition present at HEAD (for example `tlg0003.tlg001.perseus-grc2`). Some URN strings carry a passage suffix (`…perseus-grc1:1.1`); the probe strips it before checking the edition.

Legacy IDs: 9 distinct `Leuven|NNNN-NNN` values; all 9 work keys exist at HEAD. The mapping `NNNN-NNN` → `tlgNNNN.tlgNNN` is inferred from the file pattern (I), not stated in the repo.

Lineage: the repo `NinaCengic/Gorman_Greek_Dependency_Trees` exists with no commits (V). Whether gorman-trees derives from it is unknown (U). The earlier "Greek-Dependency-Trees" clone at `d823e96` could not be re-identified and is not used.

### 5.3 Pedalion (`perseids-publications/pedalion-trees`, current) [V]

- 40 XML files in `public/xml`. No `urn:cts:` string in any of them (0 matches).
- 29 distinct legacy IDs `Leuven|NNNN-NNN` (for example `0006-003`, `0009-001`, `0010-020`).
- Under the inferred mapping (§5.2), 18 IDs have a work present at HEAD and 11 have a work absent: `0009-001`, `0009-002`, `0041-001`, `0096-002`, `0343-001`, `0537-012`, `0541-007`, `0544-002`, `0559-002`, `0585-001`, `1220-001`.
- So Pedalion needs a work-level mapping and an edition decision for 18 IDs, and a source decision for 11.

### 5.4 PROIEL (`proiel/proiel-treebank`) [V]

`hdt.xml` (sha256 `1d853bc5d34fbb2d05c56198b55fb08a667126e3be8bc5a6199083bb0104b92c`):

```
sentences 5660 status {'reviewed': 5660}
tokens 85080 tokens without citation-part 617
example citation-part ['1.1.0', '1.1.0']
```

PROIEL uses `citation-part` values such as `1.1.0`, not CTS. `hdt.xml` and `greek-nt.xml` contain no `urn:cts` string (0 matches each). A PROIEL-to-CTS mapping would be needed for every token. The number of distinct citation values was not counted (U).

`status="reviewed"` is an attribute value in the file. What it means for the editorial process is not documented in the files inspected (U).

### 5.5 Diorisis [U]

Not re-verified in this revision. The earlier figures, converter description, and match ratios are withdrawn. A Diorisis join would need a Beta Code conversion that is validated against known forms before any mismatch is attributed to source or converter. That validation has not been done.

### 5.6 GLAUx and OGA (identifier and file checks only)

Scope: `metadata.txt` and one URN file per producer, plus three GLAUx XML files. The OGA archive was not downloaded. Licences were not re-checked here; the source survey covers them. No text join was run.

**GLAUx** (`alekkeersmaekers/glaux` @ `b077d8f6`; `metadata.txt` sha256 `40ed4a63fb57520ce6bc11823cf805472017495c1e3ad933174bc0291be7345b`)

- `metadata.txt` has 1,421 data rows (1,422 lines with the header). `TLG` is unique per row, and 23 rows carry a lowercase letter suffix (for example `0007-051a`, `0632-002b`). [V]
- Probe output (`cts_probe.py glaux`, canonical-greekLit @ `01b725d8`):

```
GLAUx rows 1421; distinct TLG 1421; with letter suffix 23
  ('First1K', 'exact work present') 29
  ('First1K', 'work absent') 257
  ('Perseus', 'base work present; letter suffix absent') 4
  ('Perseus', 'exact work present') 624
  ('Perseus', 'work absent') 81
  ('other', 'exact work present') 33
  ('other', 'work absent') 193
  ('wikisource', 'exact work present') 105
  ('wikisource', 'work absent') 95
```

- `TLG` is a work-level legacy ID (`NNNN-NNN`). The metadata has no edition column, so GLAUx does not record which edition text it used. [V for the columns; U for the edition.] Mapping `NNNN-NNN` to `tlgNNNN/tlgNNN` is inferred (I), as in §5.2 and §5.3.
- These are work-level presence checks only. For the 709 Perseus rows: 624 exact, 4 base-only, 81 absent. The cause of the 81 is not established (U).
- Sample XML, fetched at the pin. Counts are occurrences of `analysis=` (sentence elements) and lines containing `<word ` (one word per line):

| File | `analysis="auto"` | `analysis="manual"` | `<word ` lines | `document_id` |
| --- | ---: | ---: | ---: | --- |
| `0001-001` | 3,232 | 0 | 46,337 | `0001-001` |
| `0003-001` | 4,782 | 1,269 | 169,992 | `0003-001` |
| `0012-002` | 11 | 6,719 | 105,474 | `0012-002` |

- Verbatim head of `xml/0012-001.xml` (fetched at the pin; the first two lines of the `<sentence>` and the first `<word>`):

```xml
<treebank version="2" xml:lang="grc">
  <sentence struct_id="411" id="1" document_id="0012-001" analysis="manual">
    <word id="100004219" form="μῆνιν" line="1.1" div_book="1" lemma="μῆνις" postag="n-s---fa-" head="100004220" relation="OBJ" animacy="nonconc" sense="anger.n.01"/>
```

- Identifier reading: the sentence `document_id` is the legacy `NNNN-NNN` form, not a CTS URN. Word `line="1.1"` is a line reference with no namespace. A CTS URN would need `TLG` plus an edition choice, which GLAUx does not record (U).
- Annotation reading: two of the three sampled files are not purely automatic. `0012-002` is 6,719 of 6,730 `analysis=` occurrences manual; `0003-001` is 1,269 of 6,051 manual. The source survey's "automatically annotated" describes its 10-file sample accurately (I did not re-examine that sample), but it is not a uniform description of the corpus. The manual layer is probably imported from treebanks (I): the `0012-001` row lists the Perseus AGDT as its treebank. This is for the survey to qualify, not corrected here.

**OGA** (`OperaGraecaAdnotata/OGA` @ `a69d917d`; `urn_cts/texts/urn_cts.xml` sha256 `1b136f752ab29e9c705745e72940b75557335a1dfcdd100825f6a77fd4e9d6cc`). The OGA README names canonical-greekLit `0.0.11376425141` and First1KGreek `1.1.11352615003`; §2 gives their commits.

- The file has 1,999 `<urn_cts>` work entries, all distinct. Verbatim record:

```xml
  <work>
    <urn_cts>ggm0001.ggm001.1st1K-grc1</urn_cts>
    <title_labels>Anametresis Pontou</title_labels>
    <title_from_print_edition>Anametresis Pontou</title_from_print_edition>
    <author>Anonymous</author>
```

- 188 entries are not `tlgNNNN.tlgNNN.edition` (for example `ggm0001.ggm001.1st1K-grc1`, `ogl0001.ogl001.1st1K-grc1`). They were not matched against canonical-greekLit. [V]
- Probe output at the release pins, and at canonical HEAD for comparison (identical):

```
OGA urn_cts work entries 1999 (distinct 1999)
  other namespace or not 3-part 188
  ('1st1K', 'present') 927
  ('INTF', 'absent') 1
  ('cypria', 'present') 1
  ('digicorpus', 'present') 1
  ('opp', 'absent') 139
  ('opp', 'present') 1
  ('perseus', 'absent') 15
  ('perseus', 'present') 726
perseus entries absent at canonical pin: ['tlg0610.perseus002.perseus-grc1', 'tlg0645.tlg002.perseus-grc2', 'tlg0645.tlg003.perseus-grc2', 'tlg0658.tlg001.perseus-grc1', 'tlg1205.tlg001.perseus-grc1', 'tlg1205.tlg002.perseus-grc1', 'tlg1725.tlg001.perseus-grc1', 'tlg1766.tlg001.perseus-grc1', 'tlg2042.tlg001.perseus-grc1', 'tlg2042.tlg007.perseus-grc1', 'tlg2042.tlg008.perseus-grc1', 'tlg5034.tlg001a.perseus-grc1', 'tlg5034.tlg001b.perseus-grc1', 'tlg5034.tlg001c.perseus-grc1', 'tlg5034.tlg001d.perseus-grc1']
```

- `1st1K`: all 927 editions are present at the First1KGreek release tag. [V] An earlier check at an arbitrary First1KGreek commit (`03776b39`) gave 926 of 927; that is not the pin OGA names, so it is not used.
- `perseus`: 726 present and 15 absent, with the same 15 at both canonical pins. Post-release drift is therefore not the cause (V). The cause is not established (U).
- `opp`: 1 present, 139 absent. `INTF`: 1 absent. The source repositories for these labels were not inspected (U).
- OGA README lines 59 and 60 list "CTS URN for work, author, and edition" and "CTS passage for each work" as annotation layers. The passage layer and the `annotation_example/` files were not sampled (U).

**Crosslink to the source survey** (`doc/research/ancient-greek-sources.md` @ `b1598e77`):

- Confirmed here: 1,421 GLAUx rows and 1,999 OGA work entries.
- Not checked here: token totals (GLAUx 19,498,849; OGA 40,105,221) and per-source token totals.
- Qualification candidate: "automatically annotated" for GLAUx (see the annotation reading above).

## 6. Line join: AGDT v2.1 vs Perseus `grc2` (one text)

Probe: `cts_probe.py agdt-join` (§11). Units are citation lines, not words. Key: AGDT word `cite` last segment vs TEI `<l n>`.

Output (exact, from `probes/cts_probe.out`):

```
AGDT words 6071; without cite 1102; keyed lines 1055
TEI <l> keys 1079; duplicate keys skipped 0
lines in both 1052; AGDT-only 3; TEI-only 27
L0 exact: 159 of 1052
L1 token sequence (punctuation removed): 618 of 1052
L2 character stream (punctuation, whitespace, apostrophes removed): 1017 of 1052
```

Normalisation rationale:

- L0: NFC-normalised string equality of the joined AGDT forms and the TEI line text. This keeps punctuation and TEI spacing, so it is strict by design.
- L1: removes Unicode punctuation (category P*) except apostrophe-like marks, then compares token sequences. Apostrophe-like marks are kept because they differ by source encoding (below), so L1 is stricter than L2.
- L2: removes the apostrophe-like marks and whitespace, then compares character streams. This absorbs tokenisation differences (elision split or joined).

Encoding difference for the same elision [V]: AGDT `ἀρθέντ̓` uses U+0313 (COMBINING COMMA ABOVE). TEI `ἀρθέντʼ` uses U+02BC (MODIFIER LETTER APOSTROPHE). Both are first-party XML, so this is a source convention, not a converter error.

Exact originals for the first L1 residuals:

```
n=2
  AGDT: στόλον ἡμέτερον νάιον ἀρθέντ̓
  TEI : στόλον ἡμέτερον νάιον ἀρθέντʼ
n=6
  AGDT: οὔτιν̓ ἐφ̓ αἵματι δημηλασίαν
  TEI : οὔτινʼ ἐφʼ αἵματι δημηλασίαν
```

Exact originals for the L2 residuals (the four in the probe output):

```
n=42
  AGDT: πόντιον τιμάοῤ ἶνίν τ̓
  TEI : πόντιον τιμάορʼ, ἶνίν τʼ
n=80
  AGDT: δόντες ἔχειν παῤ αἶσαν
  TEI : δόντες ἔχειν παρʼ αἶσαν,
n=106
  AGDT: πυθμὴν δἰ ἁ
  TEI : πυθμὴν διʼ ἁ-
n=122
  AGDT: Σιδονίᾳ καλύπτρᾳ
  TEI : Σιδονία  καλύπτρᾳ.
```

Classification of residuals (V for the examples; I for the rest):

- n=42 and n=80 are elision-encoding differences (AGDT rho-with-psili U+1FE4 `ῤ` vs TEI `ρʼ`). They are source encoding, not converter error.
- n=106 is a hyphenated line break in TEI (`ἁ-`).
- n=122 is a genuine wording difference in the first word (`Σιδονίᾳ` vs `Σιδονία`). Its status as a witness difference (edition vs annotation) is U.
- The remaining 31 L2 residuals were not classified (U).

Residual: 35 of 1,052 common lines (3.3%); 4 classified above, 31 not. Lines present only on one side: 3 AGDT-only, 27 TEI-only (cause U).

What this does not show: the join uses one text, one edition, and one file. It does not show the match rate for other works, other editions, or the grc1 editions (absent at HEAD).

## 7. Licences [V unless marked]

Only the files cited here were read. No bulk survey was done.

| Source | Evidence (exact) | Status |
| --- | --- | --- |
| AGDT (`treebank_data` @ `bf4334f0`) | Root README line 15: "Contents of this repository are licensed under a Creative Commons Attribution-ShareAlike 3.0 United States License" (link to `by-sa/3.0/us`). No `LICENSE` file at root. No AGDT v2.1 Greek file contains "licen" (0 of 33). API `license: null`. | Root README states CC BY-SA 3.0 US. Not NC. Per-file licence: none found. |
| PROIEL (`proiel-treebank` @ `8e388967`) | README lines 8–10: "freely available under a Creative Commons Attribution-NonCommercial-ShareAlike 3.0 License" (link `by-nc-sa/3.0/us`). | CC BY-NC-SA 3.0 US per README. |
| Gorman (`gorman-trees` @ `83d93a55`) | README line 144: "The treebanks are licensed under the CC0 1.0 license (see `TREEBANK_LICENSE` file)." `TREEBANK_LICENSE` header: "Attribution-ShareAlike 4.0 International". README line 133 (Zenodo recipe): "License: Creative Commons Attribution 4.0 International". `LICENSE` (MIT, "Copyright (c) 2018 The Perseids Project") covers code. | **Conflict in the record.** README names CC0 1.0 and points to a file whose text is CC BY-SA 4.0; a separate section says CC BY 4.0. The repo does not settle which applies. |
| Pedalion (`pedalion-trees` @ `112c106b`) | README line 239: "We will assign a Creative-Commons licence to our treebanks, probably the following one: https://creativecommons.org/licenses/by-sa/4.0/." `TREEBANK_LICENSE` header: "Attribution-ShareAlike 4.0 International". `LICENSE` (MIT). | Not assigned by the README (future tense). `TREEBANK_LICENSE` text is CC BY-SA 4.0. Whether it governs the data is unresolved. |
| CTS spec | Spec front matter: "available under the terms of the Creative Commons Attribution-ShareAlike 4.0 International License." | Applies to the specification text, not to any corpus. |

Factual questions that the sources do not settle, reported as-is: which licence applies to Gorman's treebanks (CC0 1.0, CC BY-SA 4.0, or CC BY 4.0 for the Zenodo deposit), and whether Pedalion's `TREEBANK_LICENSE` governs its data. These are legal determinations; no source here is authoritative for them.

## 8. Text authority

Two sources of the text can be used to match annotations to passages:

**A. Annotation-carried text** (the treebank's own `form` attributes are the text).

- Where it is safe: for annotation-only features (lemma, morphology, dependency). No external text is needed, and the annotation carries its own identity via `document_id`.
- Why a text match against an edition can fail: annotations use different elision and apostrophe encodings, tokenise differently, and sometimes differ from the edition's wording (§6, n=122).
- What would make a text check safe: a stated L2 agreement criterion per line; disclosure of residual lines; and a recorded source hash. The threshold is a policy decision (§9, P2).

**B. Externally sourced edition** (the canonical edition is the text, and annotations map to it by citation key).

- Where it is safe: only when the annotation's version URN resolves to a file present at HEAD, and the citation key exists in that file.
- Why it fails here: 211 of 217 Gorman URN strings name an edition file absent at HEAD (209 of them `perseus-grc1`), and 17 of 33 AGDT v2.1 Greek files name a `perseus-grc1` edition absent at HEAD. For those, there is no text to match. Pedalion's 11 absent works have no canonical text at HEAD at all.
- What would make it safe: an exact version URN resolved against present content, and a per-line check as in A.

**Mapping workload (V, counts only):**

| Source | Identifiers needing an edition or work decision |
| --- | --- |
| Gorman | 217 distinct CTS URN strings (24,943 occurrences), of which 211 need a new edition target; 9 legacy IDs (work present) |
| AGDT v2.1 Greek | 33 files: 9 resolved, 17 need a new target, 7 need regrammaring (partial range) |
| Pedalion | 29 legacy IDs: 18 need work-level mapping, 11 have no work at HEAD |
| PROIEL | 5,660 sentences with `citation-part` values; no CTS mapping in the files; distinct value count not computed |
| GLAUx | 1,421 rows keyed by work-level `TLG` (no edition): 791 exact work present, 4 base-only, 626 absent (§5.6) |
| OGA | 1,999 `urn_cts` entries: 1,656 present, 155 absent (3-part `tlg` namespace, at the release pins), 188 non-`tlg` namespaces not matched (§5.6) |

No per-token mapping counts are given; they were not computed.

## 9. Decision questions

**Factual research (answerable from sources; reported above or still open):**

- F1. Which licence text governs Gorman's treebanks? The sources conflict (§7). Not decided here.
- F2. Does Pedalion's `TREEBANK_LICENSE` govern its data? Not decided here.
- F3. Is there a later revision of the CTS URN specification than 2.0.rc.1? (U)
- F4. What does the AGDT fourth URN part `.tb` mean in the treebank's own documentation? (U)
- F5. Is `NNNN-NNN` (Pedalion and Gorman legacy IDs) defined as `tlgNNNN.tlgNNN` in a source document? (U; inferred)

**Human policy (requires a human decision):**

- P1. Use CTS edition URN as the matching key, or work plus a checked file path?
- P2. Accept an L2-normalised text match for learner-facing annotations, and at what residual level?
- P3. Use PROIEL (NC) and AGDT (SA) data at all, given the licence terms in §7?
- P4. Which text authority applies when an annotation's edition is absent at HEAD (§8)?

## 10. Remaining uncertainty

- Only one AGDT file, one Perseus text, and no other corpus were joined. No corpus-wide rate is claimed.
- The 162 "added, not at HEAD" tracking targets are not traced (U).
- Diorisis, PROIEL-to-Perseus text, and Pedalion text matching were not run in this revision.
- The text-level comparison of a shared URN across repos (e.g. `opp-grc1`) was not run (U).
- The relation between Gorman's two repos is unknown (U).
- The licence conflicts in §7 are not resolved by any source inspected.
- GLAUx and OGA: identifier and file checks only (§5.6). No text join was run for either. The OGA archive was not downloaded. The source repositories for the OGA `opp` and `INTF` labels were not inspected. GLAUx records no edition.

## 11. Reproduction

Inputs: the pins in §2. Scratch: an empty directory of your choice, not the shared scratch root.

Probe source (`probes/cts_probe.py`, stdlib only; reads git objects at the given SHA):

```python
#!/usr/bin/env python3
"""CTS matching probe (stdlib only). Reads git objects at pinned SHAs, never the working tree.

  cts_probe.py counts     CANON_REPO CANON_SHA
  cts_probe.py tracking   CANON_REPO CANON_SHA
  cts_probe.py ids        SRC_REPO SRC_SHA CANON_REPO CANON_SHA SRC_DIR
  cts_probe.py proiel     PROIEL_REPO PROIEL_SHA HDT_PATH
  cts_probe.py agdt-join  AGDT_TB_XML PERSEUS_TEI_XML
  cts_probe.py agdt-registry AGDT_REPO AGDT_SHA CANON_REPO CANON_SHA
  cts_probe.py glaux      CANON_REPO CANON_SHA GLAUX_METADATA_TSV
  cts_probe.py oga        CANON_REPO CANON_SHA OGA_URN_XML F1K_REPO F1K_SHA
"""
import collections, csv, json, re, subprocess, sys, unicodedata
import xml.etree.ElementTree as ET

EDITION = re.compile(r'^data/(tlg\d{4})/(tlg\d{3}[a-z]?)/\1\.\2\.(.+)\.xml$')
CTS_URN = re.compile(r'urn:cts:[A-Za-z0-9:.\-]+')
LEGACY = re.compile(r'Leuven\|(\d{4}-\d{3})')
APOS = "'’ʼ᾽̓̔ι"


def git(repo, *args):
    return subprocess.run(['git', '-C', repo, *args], capture_output=True, check=True).stdout.decode('utf-8')


def tree(repo, sha):
    return set(git(repo, 'ls-tree', '-r', '--name-only', sha).split('\n')) - {''}


def works_at(paths):
    return {m.group(1) for p in paths if (m := re.match(r'data/(tlg\d{4}/tlg\d{3}[a-z]?)/', p))}


def cmd_counts(repo, sha):
    paths = tree(repo, sha)
    xml = [p for p in paths if p.startswith('data/') and p.endswith('.xml')]
    meta = [p for p in xml if p.endswith('/__cts__.xml')]
    non = [p for p in xml if not p.endswith('/__cts__.xml')]
    ed = [(p, EDITION.match(p)) for p in non]
    ok = [(p, m) for p, m in ed if m]
    grc = [(p, m) for p, m in ok if m.group(3).startswith('perseus-grc') and re.search(r'perseus-grc\d+$', m.group(3))]
    print(f'xml under data/: {len(xml)} (__cts__ metadata: {len(meta)}; non-metadata: {len(non)})')
    print(f'non-metadata matching tlgNNNN/tlgNNN/<urn>.xml: {len(ok)} of {len(non)}')
    print(f'perseus-grc editions: {len(grc)}; distinct work keys: {len({m.group(1, 2) for _, m in grc})}')
    print(f'exact suffix perseus-grc1.xml files: {sum(1 for p in paths if p.endswith("perseus-grc1.xml"))}')


def cmd_tracking(repo, sha):
    paths = tree(repo, sha)
    trk = json.loads(git(repo, 'show', f'{sha}:canonical-greekLit.tracking.json'))
    print('sample record', json.dumps({'urn:cts:greekLit:tlg0001.tlg001.perseus-grc2': trk['urn:cts:greekLit:tlg0001.tlg001.perseus-grc2']}, ensure_ascii=False, indent=1))
    forms = collections.Counter(('canonical-greekLit/' if v.get('target', '').startswith('canonical-greekLit/') else 'other') for v in trk.values())
    print('target prefix forms:', dict(forms))
    hist = {}
    for line in git(repo, 'log', '--all', '--name-status', '--format=', '--diff-filter=AD', '--', 'data').split('\n'):
        if line[:2] in ('A\t', 'D\t'):
            hist.setdefault(line[2:], set()).add(line[0])
    c = collections.Counter()
    for k, v in trk.items():
        if v.get('status') != 'migrated' or 'greekLit' not in k:
            continue
        rel = v['target'][len('canonical-greekLit/'):]
        if rel in paths:
            c['target in HEAD tree'] += 1
        elif 'D' in hist.get(rel, ()):
            c['target absent at HEAD; deleted in history'] += 1
        elif 'A' in hist.get(rel, ()):
            c['target absent at HEAD; added, not at HEAD'] += 1
        else:
            c['target absent at HEAD; never in history'] += 1
    print('migrated greekLit entries:', sum(c.values()), dict(c))


def cmd_ids(src_repo, src_sha, canon, csha, src_dir):
    cpaths = tree(canon, csha)
    works = works_at(cpaths)
    files = [p for p in git(src_repo, 'ls-tree', '-r', '--name-only', src_sha, src_dir).split('\n') if p.endswith('.xml')]
    cts, legacy, cts_files, legacy_files = collections.Counter(), set(), 0, 0
    for f in files:
        s = git(src_repo, 'show', f'{src_sha}:{f}')
        u = set(CTS_URN.findall(s))
        l = set(LEGACY.findall(s))
        cts_files += bool(u)
        legacy_files += bool(l)
        cts.update(CTS_URN.findall(s))
        legacy |= l
    print(f'{src_dir}: files {len(files)}; files with CTS URN {cts_files}; files with Leuven|NNNN-NNN {legacy_files}')
    cls = collections.Counter()
    for u in cts:
        body = u[len('urn:cts:greekLit:'):] if u.startswith('urn:cts:greekLit:') else None
        if body is None:
            cls['non-greekLit namespace'] += 1
            continue
        head = body.partition(':')[0].split('.')
        if len(head) < 3:
            cls['work-level URN'] += 1
            continue
        w = f'{head[0]}/{head[1]}'
        path = f'data/{w}/{".".join(head[:3])}.xml'
        cls['edition file present at HEAD' if path in cpaths else ('edition file absent; work present' if w in works else 'edition file absent; work absent')] += 1
    print(f'distinct CTS URNs {len(cts)}, occurrences {sum(cts.values())}:', dict(cls))
    lc = collections.Counter()
    for i in legacy:
        a, b = i.split('-')
        lc['work present' if f'tlg{a}/tlg{b}' in works else 'work absent'] += 1
    print(f'distinct legacy IDs {len(legacy)}:', dict(lc), 'absent works:', sorted(i for i in legacy if f"tlg{i[:4]}/tlg{i[5:]}" not in works))


def local(tag):
    return tag.rsplit('}', 1)[-1]


def cmd_proiel(repo, sha, path):
    root = ET.fromstring(git(repo, 'show', f'{sha}:{path}'))
    sents = [e for e in root.iter() if local(e.tag) == 'sentence']
    toks = [e for e in root.iter() if local(e.tag) == 'token']
    print('sentences', len(sents), 'status', dict(collections.Counter(s.get('status') for s in sents)))
    print('tokens', len(toks), 'tokens without citation-part', sum(1 for t in toks if not t.get('citation-part')))
    print('example citation-part', [t.get('citation-part') for t in toks[:2]])


def nfc(s):
    return unicodedata.normalize('NFC', s)


def l1_tokens(s):
    s = nfc(s)
    s = ''.join(' ' if ch.isspace() else ('' if unicodedata.category(ch).startswith('P') and ch not in APOS else ch) for ch in s)
    return s.split()


def l2_stream(s):
    return ''.join(ch for ch in ''.join(l1_tokens(s)) if ch not in APOS)


def cmd_agdt_join(agdt, tei):
    lines, words, unkeyed = collections.OrderedDict(), 0, 0
    for _, el in ET.iterparse(agdt, events=('end',)):
        if local(el.tag) != 'word':
            continue
        words += 1
        cite = el.get('cite', '')
        if not cite:
            unkeyed += 1
            continue
        lines.setdefault(cite.rsplit(':', 1)[-1], []).append(el.get('form', ''))
    a = {k: ' '.join(v) for k, v in lines.items()}
    t, dup = collections.OrderedDict(), 0
    for el in ET.parse(tei).getroot().iter():
        if local(el.tag) == 'l' and 'n' in el.attrib:
            if el.get('n') in t:
                dup += 1
            else:
                t[el.get('n')] = ''.join(el.itertext())
    common = [k for k in a if k in t]
    print(f'AGDT words {words}; without cite {unkeyed}; keyed lines {len(a)}')
    print(f'TEI <l> keys {len(t)}; duplicate keys skipped {dup}')
    print(f'lines in both {len(common)}; AGDT-only {len(a) - len(common)}; TEI-only {len(t) - len(common)}')
    c = collections.Counter()
    l1_ex, l2_ex = [], []
    for k in common:
        x, y = a[k], t[k]
        c['L0 exact'] += nfc(x) == nfc(y)
        if l1_tokens(x) == l1_tokens(y):
            c['L1 tokens'] += 1
        elif len(l1_ex) < 2:
            l1_ex.append((k, x, y))
        if l2_stream(x) == l2_stream(y):
            c['L2 stream'] += 1
        elif len(l2_ex) < 4:
            l2_ex.append((k, x, y))
    for name, key in [('L0 exact', 'L0 exact'), ('L1 token sequence (punctuation removed)', 'L1 tokens'), ('L2 character stream (punctuation, whitespace, apostrophes removed)', 'L2 stream')]:
        print(f'{name}: {c[key]} of {len(common)}')
    for title, ex in [('L1 non-matching examples', l1_ex), ('L2 non-matching examples', l2_ex)]:
        print(title)
        for k, x, y in ex:
            print(f'  n={k}\n    AGDT: {x}\n    TEI : {y}')


AGDT_EDITION = re.compile(r'^(tlg\d{4}\.tlg\d{3}[a-z]?)\.((?:perseus|opp)-grc\d+)\.tb\.xml$')


def cmd_agdt_registry(agdt, asha, canon, csha):
    cpaths = tree(canon, csha)
    names = [p.rsplit('/', 1)[-1] for p in git(agdt, 'ls-tree', '-r', '--name-only', asha, 'v2.1/Greek').split('\n') if p.endswith('.tb.xml')]
    c, absent, present, mismatch = collections.Counter(), collections.Counter(), [], []
    for n in names:
        m = AGDT_EDITION.match(n)
        if not m:
            c['name does not match edition pattern'] += 1
            mismatch.append(n)
            continue
        t, w = m.group(1).split('.')
        if f'data/{t}/{w}/{m.group(1)}.{m.group(2)}.xml' in cpaths:
            c['edition file present at canonical pin'] += 1
            present.append(n)
        else:
            c['edition file absent at canonical pin'] += 1
            absent[m.group(2)] += 1
    print(f'v2.1/Greek .tb.xml files: {len(names)}:', dict(c))
    print('absent, by edition label:', dict(absent))
    print('present:', present)
    print('pattern mismatches:', mismatch)


def cmd_glaux(canon, csha, tsv):
    works = works_at(tree(canon, csha))
    rows = list(csv.DictReader(open(tsv, encoding='utf-8'), delimiter='\t'))
    c = collections.Counter()
    for r in rows:
        m = re.fullmatch(r'(\d{4})-(\d{3})([a-z]?)', r['TLG'].strip())
        if not m:
            c['TLG not NNNN-NNN[a-z]'] += 1
            continue
        a, b, s = m.groups()
        src = r['SOURCE'] if r['SOURCE'] in ('Perseus', 'First1K') else ('wikisource' if 'wikisource' in r['SOURCE'] else 'other')
        if f'tlg{a}/tlg{b}{s}' in works:
            status = 'exact work present'
        elif f'tlg{a}/tlg{b}' in works:
            status = 'base work present; letter suffix absent'
        else:
            status = 'work absent'
        c[(src, status)] += 1
    print(f"GLAUx rows {len(rows)}; distinct TLG {len({r['TLG'] for r in rows})}; with letter suffix {sum(bool(re.search(r'[a-z]$', r['TLG'])) for r in rows)}")
    for k, v in sorted(c.items(), key=str):
        print(' ', k, v)


def cmd_oga(canon, csha, xml, f1repo, f1sha):
    cpaths = tree(canon, csha)
    f1 = tree(f1repo, f1sha)
    urns = re.findall(r'<urn_cts>([^<]+)</urn_cts>', open(xml, encoding='utf-8').read())
    c, absent = collections.Counter(), []
    for u in urns:
        parts = u.split('.')
        if len(parts) != 3 or not re.fullmatch(r'tlg\d{4}', parts[0]):
            c['other namespace or not 3-part'] += 1
            continue
        label = re.sub(r'-.*$', '', parts[2])
        path = f'data/{parts[0]}/{parts[1]}/{u}.xml'
        present = path in (f1 if label == '1st1K' else cpaths)
        c[(label, 'present' if present else 'absent')] += 1
        if not present and label == 'perseus':
            absent.append(u)
    print(f'OGA urn_cts work entries {len(urns)} (distinct {len(set(urns))})')
    for k, v in sorted(c.items(), key=str):
        print(' ', k, v)
    print('perseus entries absent at canonical pin:', sorted(absent))


if __name__ == '__main__':
    cmd, args = sys.argv[1], sys.argv[2:]
    {'counts': cmd_counts, 'tracking': cmd_tracking, 'ids': cmd_ids, 'proiel': cmd_proiel, 'agdt-join': cmd_agdt_join,
     'agdt-registry': cmd_agdt_registry, 'glaux': cmd_glaux, 'oga': cmd_oga}[cmd](*args)
```

Run (from the directory holding the clones; SHAs as in §2):

```sh
# GLAUx and OGA are fetched as single files at their pins. The OGA archive is not downloaded.
curl -sL https://raw.githubusercontent.com/alekkeersmaekers/glaux/b077d8f6ff429a5c7245954bc16bb7d1d7948823/metadata.txt -o glaux/metadata.txt
curl -sL https://raw.githubusercontent.com/OperaGraecaAdnotata/OGA/a69d917dbe117a307829977dd556e22f05ce0715/urn_cts/texts/urn_cts.xml -o oga/urn_cts.xml
git clone --filter=blob:none --no-checkout https://github.com/OpenGreekAndLatin/First1KGreek.git first1k
P="python3 -I cts_probe.py"; C=01b725d835e6e733062ffd79e0efdbae1ba06e5c
CR=476fb6133498e5710de7eec0b492791da86be4b4; F1=9e9592d4a1c1df6d1849780fd1ccd3491a880d89
A=bf4334f0af5e13d16b04c1cccd6237e683ac6f5f
$P counts canonical-greekLit $C
$P tracking canonical-greekLit $C
$P ids gorman-trees 83d93a554a46ff6e9a34a15a9e0be2dff270a612 canonical-greekLit $C public/xml
$P ids pedalion-trees 112c106b86b865ddde4be7e5f3e409d9c42689f9 canonical-greekLit $C public/xml
$P proiel proiel-treebank 8e388967a1335ed12335ddc655fe46993ee7d57a hdt.xml
$P agdt-registry treebank_data $A canonical-greekLit $C
$P glaux canonical-greekLit $C glaux/metadata.txt
$P oga canonical-greekLit $CR oga/urn_cts.xml first1k $F1
$P oga canonical-greekLit $C oga/urn_cts.xml first1k $F1
# AGDT join: extract the two inputs first
git -C treebank_data show $A:v2.1/Greek/texts/tlg0085.tlg001.perseus-grc2.tb.xml > agdt.xml
git -C canonical-greekLit show $C:data/tlg0085/tlg001/tlg0085.tlg001.perseus-grc2.xml > perseus.xml
$P agdt-join agdt.xml perseus.xml
```

Probe source hash: `558d926730ac22ac3c46535e4dd1aa6bbbb83fe17672a6970aa34ded91ca9258` (sha256 of `cts_probe.py` as run). The `ids`, `tracking`, and `agdt-registry` commands require the blobless clones; `git show` and `git ls-tree` fetch missing data from the remote as needed.

Captured outputs: §4–§6 and §5.6 come from the final probe at the pins above. Each output is reproduced inline; the probe run logs are not committed.
