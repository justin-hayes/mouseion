# Ancient Greek: CTS identity and cross-corpus text-to-annotation matching

- Date: 2026-10-14
- Issue: justin-hayes/mouseion#2024 (under map #2021)
- Baseline: `b262ed0a`
- Scope: research only. No importer, schema, or production code changed.

## Question

Can a CTS URN identify the same text across the open Ancient Greek corpora, and can a passage in one corpus be matched to the same passage's tokens and annotations in another? What does reconciling them cost?

## Method and limits

- Upstream repositories were cloned into a scratch directory outside the repository. Versions are given as HEAD short SHAs with dates.
- Matching was done with small Python probes (standard library only). They are not committed. Commands are given in the Reproduction section so they can be rerun.
- Counts are exact for the files and keys named. Overlap figures are sampled where stated. No corpus-wide claim is made from a sample.
- Labels: **verified** = reproduced by a command in this document; **inferred** = reasoned from verified facts, not directly measured; **unknown** = not established.

## Sources

| Source | Version inspected | License (as found) |
| --- | --- | --- |
| Perseus / OGL `canonical-greekLit` | HEAD `01b725d` (2026-10-06) | OGL-style; see `license.md` |
| First1KGreek | HEAD `03776b39` (2026-09-30) | CC BY-SA 4.0 (inferred from repo convention; not re-checked) |
| AGDT (`treebank_data`) | v2.1 Greek, v2.0, v1.6 | CC BY-NC-SA 3.0 (per repo) |
| PROIEL treebank (`proiel-treebank`) | `8e38896` (2023-05-02) | CC BY-NC-SA 3.0 (verified) |
| Gorman Greek treebank (`Greek-Dependency-Trees`) | `d823e96` (2022) | **Conflict**: README says CC BY-NC-SA 4.0; `TREEBANK LICENSE` says CC BY-SA 4.0 (verified) |
| Pedalion (`treebanks/pedalion`) | 2019 snapshot; current repo `perseids-publications/pedalion-trees`, pushed 2023-01-24 | Current repo MIT (verified); old README said "probably" CC BY-SA 4.0 (verified) |
| Diorisis | 2018 release, `annoCorpora.zip` (820 XML files) | Not re-checked |

## Findings

### 1. CTS work and edition identity

| Claim | Status | Evidence |
| --- | --- | --- |
| Perseus has 1,836 greekLit work URNs (plus one `hebrew` URN) in `inventory.json` | verified | `scripts/inventory.py` over `canonical-greekLit` |
| 93 work URNs appear in both Perseus and First1KGreek | verified | `scripts/overlap.py` |
| Of those 93, 41 have a Perseus `grc` edition and a 1st1K `grc` edition | verified | `scripts/overlap.py` |
| The same edition URN is used in both repos, e.g. `urn:cts:greekLit:tlg0090.tlg001.opp-grc1` and `opp-lat1` | verified | `scripts/overlap.py` |
| The same work can carry different editions and citation schemes: `tlg0627.tlg001` has Perseus `grc2` (Jones 1923, `section`) and 1st1K `grc1` (Littré 1839, `chapter`) | verified | `scripts/headers.py` |
| Same URN does not imply same text. AGDT line join against 1st1K `grc1` (Sidgwick 1902) shows real wording differences (e.g. `ἀνθονομούσας` vs `ἀνθονόμον τᾶς`) | verified | `scripts/line_join.py` |

**Work / edition / version / exemplar / passage.** CTS keys identity on the URN, so `work` (`tlg0085.tlg001`), `edition` (`perseus-grc2`), and `passage` (`1.1`) are three levels of one key. A *version* or *exemplar* is not in the URN. Two editions of one work with different wording must therefore get different edition URNs. The data is not consistent about that: the same edition URN appears in two repos (above), and AGDT `cite` values are work-level URNs with no edition, so the edition only survives in `document_id` (verified).

### 2. Registration and tracking drift

| Claim | Status | Evidence |
| --- | --- | --- |
| `canonical-greekLit.tracking.json` has 1,561 keys: 1,499 `migrated` and 62 `not migrated` (greekLit) | verified | Probe below |
| Of 1,499 `migrated` entries, 1,462 point to a file absent at HEAD; 37 are present | verified | Probe below |
| 748 of the absent entries are `…-grc1` editions | verified | Probe below |
| `data/` contains 2,538 XML files, 7 of them `perseus-grc1` | verified | `find` |

So the tracking file is not a reliable index of what exists at HEAD. Matching must check the filesystem, not the tracking file.

### 3. Treebanks: edition and citation identity

**AGDT** (Aeschylus, Suppliants, `tlg0085.tlg001`)
- AGDT v2.1 Greek has 33 files. 9 edition URNs are registered at HEAD and 24 are not. v2.0 has the same split. v1.6 has 9 of 77 registered. Most unregistered ones refer to `perseus-grc1` (verified).
- Line join against Perseus `grc2` at HEAD: 6,071 words. 1,102 have no citation (mostly punctuation and `[0]`) and 16 are multi-cite. Of 1,060 keyed lines, 166 are identical at L0 (exact), 989 at L1 (punctuation stripped), and 1,013 at L2 (punctuation and elision normalised) (verified).
- Residual at L2: 41 elision and tokenization count differences. A line-boundary drift at line 155 vs 156 (`τὸν γάιον`) (verified).

**Gorman** (Greek Dependency Trees, 166 XML files)
- 219 distinct CTS IDs. Only 9 are registered at HEAD. 25 end in `perseus-grc1`, and none of those files exist at HEAD (verified).
- 28 legacy IDs such as `0014-007` (= `tlg0014.tlg007`, no edition) (verified).
- Per file: 133 use CTS only, 21 legacy only, 1 mixed, 11 neither (verified).
- Gorman is described as hand-corrected in the project's documentation (inferred from the decision record; not re-checked against the README).

**PROIEL** (`hdt.xml`, `greek-nt.xml`)
- `hdt.xml`: 5,660 sentences, all `status="reviewed"`; 85,080 tokens; 1,345 null-form tokens; 617 tokens without `citation-part` (verified).
- Citation is `B.C.S` (e.g. `1.1.0`), not CTS (verified). `greek-nt.xml`: 11,505 reviewed sentences, citation `MATT 1.1` (verified).
- Herodotus join (PROIEL `hdt` vs Perseus `tlg0016.tlg001` `grc2`): 2,055 of 4,338 Perseus section keys covered, about 47% (verified). Token-identical: 0 of 2,055 at L0, 1,687 at L1, 1,713 at L2 (verified). Residuals: 316 token-count differences (e.g. crasis `τἄλλα` vs `τὰ ἄλλα`) and 52 letter differences (verified).

**Pedalion**
- The 2019 clone is a snapshot. The project moved to `perseids-publications/pedalion-trees` (MIT, pushed 2023-01-24) (verified).
- Edition and citation checks against the current repo were not completed (unknown).

### 4. Machine-analysed corpus: Diorisis

- 820 XML files. Filenames are `Author - Title (tlgNNN).xml` with the work number only. 71 work-number tokens are shared by more than one author. 76 names do not match the pattern (verified).
- The header `tlgAuthor` holds the title, not the TLG author code (verified).
- Word forms are Beta Code. `location` is a dotted sentence-start key, reproduced 492 of 492 against TEI ancestry (verified).
- Span alignment against Perseus `grc2` at HEAD (Aeschylus): 515 sentences, of which 275 span more than one line. Identical tokens: L1 275 of 515, L2 325 of 515. Against 1st1K: L2 252 of 510 (verified).
- Part of the residual is converter error. The Beta converter used is minimal (verified by reading `scripts/beta.py`). The split between converter error and real text difference is unknown.

### 5. Normalisation

- Apostrophe code points differ by source: Perseus U+02BC; 1st1K U+1FBD; AGDT U+0313; PROIEL and Gorman U+2019 (verified).
- Elision and crasis change token counts across sources, so token-level equality is only reachable after normalisation (verified by the residual counts above).

## Overlap matrix

Rows are sources; columns are the matching key. Cells: V = verified, I = inferred, U = unknown.

| Source pair | Work URN | Edition URN | Passage (citation) | Token text |
| --- | --- | --- | --- | --- |
| Perseus ↔ 1st1K | V (93 shared) | V (same URN for some `opp-grc1`) | V (`section` vs `chapter`) | V differs (Littré vs Jones) |
| Perseus ↔ AGDT | V (AGDT carries work URN) | V (`document_id` only; `cite` is work-level) | V (line join, 1,060 keys) | V L2 1,013 of 1,060 |
| Perseus ↔ PROIEL | V (Herodotus only) | U | V (`B.C.S`, not CTS; 47% of sections) | V L2 1,713 of 2,055 covered |
| Perseus ↔ Gorman | U (not sampled) | U | V (mixed citation schemes; 9 registered IDs) | U |
| Perseus ↔ Diorisis | I (work number only) | U | V (sentence location, sampled) | V L2 325 of 515 (Aeschylus) |
| Perseus ↔ Pedalion | U | U | U | U |

## Reconciliation cost

- Identity: edition URNs must be checked against the file system, not `tracking.json`. Expect about 1,462 stale entries before any matching (verified count).
- Citation: at least three schemes (CTS `section`/`chapter`/`line`, PROIEL `B.C.S`, Diorisis dotted location, plus legacy `0014-007`). A per-source mapping table is needed for each.
- Text: L2 normalisation (punctuation, elision, apostrophe code point) gets most tokens to match in the sampled AGDT pair (1,013 of 1,060 lines, about 4% residual). It does far worse for the sampled Diorisis pair (325 of 515 sentences, about 37% residual), part of which is converter error. Either residual needs human review or a tolerance rule.
- Licence: at least three licence positions appear for one corpus (Gorman) and PROIEL and AGDT are NC. Any downstream use needs a licence decision before import.

## Decision questions

1. Should Mouseion treat the CTS edition URN as the matching key, or should it key on work plus checked file path? (The URN is not consistent across sources.)
2. For Gorman, which licence applies: CC BY-NC-SA 4.0 (README) or CC BY-SA 4.0 (`TREEBANK LICENSE`)? Is the NC term relevant to the product?
3. Is an L2-normalised token match with a stated residual acceptable for learner-facing annotations, or must matches be exact?
4. Should PROIEL and AGDT be used only as human-reviewed reference, with Diorisis limited to machine-analysed annotation and labelled as such?

## Reproduction

The probe scripts are not committed. Rerun the tracking count from a `canonical-greekLit` checkout at `01b725d`:

```sh
python3 -I -c "
import json,os,collections
d=json.load(open('canonical-greekLit.tracking.json'))
c=collections.Counter()
for k,v in d.items():
    if 'greekLit' not in k or v.get('status')!='migrated': continue
    t=v.get('target') or ''
    p=t.split('/',1)[1] if t.startswith('canonical-greekLit/') else t
    c['present' if os.path.exists(p) else 'absent']+=1
print(dict(c))
"
```

Expected: `{'absent': 1462, 'present': 37}`.

The overlap, line-join, Herodotus, and Diorisis probes were run as `scripts/overlap.py`, `scripts/line_join.py`, `scripts/herodotus_join.py`, and `scripts/diorisis_join.py` against the HEADs above. Their outputs are summarised in §1–§4.

## Not done

- Full corpus-wide overlap (only sampled: Aeschylus, Herodotus, `tlg0627.tlg001` headers, and the 93-work URN set).
- Pedalion edition and citation checks against the current repo.
- Gorman edition resolution beyond the 9 registered IDs.
- Diorisis licence check.
