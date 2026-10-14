# Ancient Greek public source gates: Anabasis Book 1 and fallbacks

**Research date:** 2026-10-14

**Issue:** [#2044](https://github.com/justin-hayes/mouseion/issues/2044) (map [#2021](https://github.com/justin-hayes/mouseion/issues/2021), parent [#2030](https://github.com/justin-hayes/mouseion/issues/2030)).

**Scope:** Public and self-service evidence only, for three candidate texts: Xenophon *Anabasis* Book 1 (preferred), Lysias 1 and Plato *Crito* (fallbacks). The question was whether each route can pass its rights, review, and text-authority gates. No outreach of any kind, no provider or LLM calls, no paid acquisition, no gated or rejected access retries, no source acceptance, no source, dependency, ADR, or glossary changes. Steadman companion readers were not opened. Broad prior alignment was not rerun, and neither fallback was fully validated.

**Status:** Negative result for the preferred route. **No candidate passes all gates.** Anabasis Book 1 has a documented annotation with unresolved licence and no human-reviewed alternative in any inventoried public source. Lysias 1 and *Crito* each have a candidate annotation, but each has its own blocker. Nothing here is an acceptance decision.

This report reconciles with the earlier [#2042 measurement](ancient-greek-anabasis-book1.md) (commit `f012fc89`). Where this report corrects that one, it says so in section 3.

## Executive findings

1. **Anabasis Book 1 cannot pass the gates on current public evidence.** The only human-annotated candidate is the Gorman treebank. Its licence is documentarily contradictory, and its review scope is unspecified.
2. **The Gorman file declares its source edition.** Every Book 1 sentence carries `document_id="urn:cts:greekLit:tlg0032.tlg006.perseus-grc2"`. The earlier claim that the source edition is unknown is wrong. Token-level correspondence to that text is still unverified by this report.
3. **No other inventoried source carries Anabasis Book 1 with human review.** AGDT v2.1, UD Perseus, and PROIEL have no `tlg0032`. Pedalion Trees covers *Memorabilia* only. GLAUx has *Anabasis* Book 1 but is automatic only. OGA was not checked.
4. **Lysias 1 is not an independent second source.** The AGDT v2.1 file and Gorman's own `Lysias 1 bu1.xml` share lineage. Their CTS identifiers also do not match the canonical pin.
5. **Plato *Crito* has two candidates.** The Gorman-hosted Kahl file has full section-label coverage but no review statement. The Pedalion file has partial section coverage, a different review scope, and an MIT/BY-SA licence conflict at repo level.
6. **Documentary conflicts are recorded, not resolved.** No reading is chosen for any of them (section 4).

## 1. Gate ledger

Gates applied: **R** rights (a licence that permits the intended use, with the conflict resolved), **V** review (documented human review of the annotation layer used), **A** text authority (a stable canonical identity that matches the annotated tokens).

| Route | Annotation | R | V | A | Outcome |
|---|---|---|---|---|---|
| *Anabasis* 1 | Gorman `Xen_Anab_book_1.*` | Unresolved: README NC vs file BY-SA | Unspecified | Self-declared CTS URN; token match unverified | **Blocked** |
| *Anabasis* 1 | AGDT v2.1 | n/a | n/a | n/a | **No Book 1 data** |
| *Anabasis* 1 | UD Perseus / PROIEL | n/a | n/a | n/a | **No Book 1 data** |
| *Anabasis* 1 | Pedalion Trees | n/a | n/a | n/a | **No Book 1 data** (*Memorabilia* only) |
| *Anabasis* 1 | GLAUx `0032-006.xml` | Not assessed as passing | Automatic only | Not assessed | **Insufficient** |
| *Anabasis* 1 | OGA | Not checked | Automatic per #2022 inventory | Not checked | **Not checked** |
| Lysias 1 | AGDT v2.1 `tlg0540.tlg001.perseus-grc1.tb.xml` | Conflicting (see section 4) | Gorman as annotator; Celano release edit | CTS `perseus-grc1` not in canonical pin | **Blocked** |
| Lysias 1 | Gorman `Lysias 1 bu1.xml` | Same conflict as Gorman treebank | Not independent of AGDT | Same as AGDT | **Blocked (same lineage)** |
| Lysias 1 | GLAUx row 324 | Derived from Gorman | Automatic | n/a | **Not independent** |
| *Crito* | Gorman `Plato_Crito_Travis_Kahl.xml` | Not stated in file; README conflict | Single annotator (Kahl); no review statement | CTS `perseus-grc2`; all 59 section milestones covered | **Blocked on R and V** |
| *Crito* | Pedalion `public/xml/crit.xml` | MIT repo root vs BY-SA treebank licence | Joosten with partial correction by Van Hal | `document_id='0059-003'`, not CTS; Stephanus 43–54 only | **Blocked on A and scope** |

No row passes all three gates. The rows marked "not independent", "not checked", or "no data" are not counted as passing.

## 2. Evidence

### 2.1 Anabasis Book 1: Gorman treebank

- **Pin:** `vgorman1/Greek-Dependency-Trees`, commit `d823e96d`. Book 1 files: `xml versions/Xen_Anab_book_1.1-5.xml` (342 sentences) and `Xen_Anab_book_1.6-9.xml` (269 sentences), total **611** sentences. The file `<date>` is 2022-07-15.
- **Repo history:** Book 1 files were added 2022-07-24 (`7e4a012`) and edited 2022-08-14 (`3614dbc`).
- **Source identity:** All 611 sentences carry `document_id="urn:cts:greekLit:tlg0032.tlg006.perseus-grc2"`. Of 207 unique subdoc labels, 204 map to canonical Book 1 sections. Non-canonical labels are `1.5.171`, `19.31`, and `3.4.1`. Canonical section `1.7.20` has no Gorman label.
- **Annotators named in headers:** Arethusa, segtok, and Vanessa Gorman. Pedalion is not named for Book 1.
- **README:** Says trees uploaded after January 2020 were "pre-parsed at the Pedalion Project and hand-corrected by V. Gorman", without naming the layers covered.
- **Review scope:** The data article (JOHD, doi 10.5334/johd.13) describes a lemma-selection workflow for the 2014–2019 corpus. It does not describe review for the Book 1 upload. Review scope for Book 1 is therefore **unspecified**, not assumed to be lemma review.
- **Token correspondence:** Not verified against the canonical text in this run. The earlier #2042 measurement found 98.0% exact unit match on a diagnostic route, which is evidence of alignment, not a validated correspondence.

### 2.2 Anabasis Book 1: other inventoried sources

- **AGDT v2.1 (`PerseusDL/treebank_data`):** No `tlg0032` anywhere in the tree.
- **UD Perseus and PROIEL conllu:** No `tlg0032`.
- **Pedalion Trees:** Xenophon *Memorabilia* only.
- **GLAUx (`alekkeersmaekers/glaux`, `xml/0032-006.xml`):** 3,837 sentences, 65,763 words, all `analysis="auto"`. Automatic only. Insufficient for a human-reviewed gate.
- **OGA:** Not checked. The #2022 inventory lists it as automatic. A GitHub tree lookup on the `docs` branch returned 404, so no file-level check was made.

### 2.3 Lysias 1 (fallback)

- **AGDT v2.1 `tlg0540.tlg001.perseus-grc1.tb.xml`:** 134 sentences, 2,834 words. Header names Gorman as "annotator of the text", Celano as release editor for "post-annotation normalization", and Lamb (Loeb, 1930) as printed source.
- **Gorman `Lysias 1 bu1.xml`:** Same 134 sentences and 2,834 words. Tuple comparison matched **2,181 of 2,834** positions. Differences are in homograph digits, punctuation lemmas, and postag placeholders.
- **Conclusion:** AGDT is not an independent source from Gorman. The two files share lineage, so they are not a second review.
- **CTS:** AGDT and Gorman cite `perseus-grc1`. The canonical pin at `01b725d8` has `perseus-grc2` and `eng2` for this work, not `grc1`. The mapping must be checked before any use.
- **GLAUx row 324:** Derives from Gorman.
- **UD Perseus:** Its README lists Lysias rows, but its conllu contains no `tlg0540`.

### 2.4 Plato *Crito* (fallback)

- **Gorman `Plato_Crito_Travis_Kahl.xml`:** 276 sentences, 5,093 word elements, annotator TravisKahl, CTS `perseus-grc2`, added in commit `a481b45` on 2022-03-03. Section labels cover all **59** canonical Crito section milestones once the two range labels `51a-c` and `52d-e` are expanded. The file contains no review or pre-parse statement. The Gorman README still says all trees were "annotated by one person".
- **Pedalion Trees `public/xml/crit.xml`:** 267 sentences, 4,993 words. Joosten, with partial correction by Van Hal from a preparse; first release 2020. Subdoc labels cover Stephanus pages 43–54 only. `document_id='0059-003'`, which is not a CTS URN. About 170 word lines have `postag=''`.
- **Pedalion repo licence:** Root `LICENSE` is MIT (2018); `TREEBANK_LICENSE` is BY-SA 4.0.
- **AGDT v2.1 and UD:** No Crito.

## 3. Corrections to the #2042 report

- **"Gorman's source edition is unknown" is withdrawn.** The Book 1 sentences self-declare the CTS URN `tlg0032.tlg006.perseus-grc2`. Token correspondence to that edition is still unverified.
- The #2042 licence statement (README NC, file BY-SA, canonical BY-SA) is confirmed and is still unresolved here.

## 4. Documentary conflicts (recorded, not resolved)

No reading is chosen for any of these. Each one blocks its route until a human resolves it.

1. **Gorman licence.** README declares BY-NC-SA 4.0 (notice added in commit `25c5c0b`, 2020-11-24, replacing CC0 text). `TREEBANK LICENSE` (blob `117cd8de`, created `b690fad` on 2019-12-31) is BY-SA 4.0 legal text with no NC clause, unchanged since creation. The README said CC0 from 2019-12-31 (`16348c5`) while the file was already BY-SA, so the conflict dates from the start of the repo. Zenodo deposits 3596076 (v1.0.1) and 3596010 (v1.0.0) are CC0 and from 2019 only. Tags 1.0.0 and 1.0.1 contain no Anabasis files. The JOHD article states CC0 1.0 for the 2014–2019 corpus, while its own page shows CC BY 4.0.
2. **AGDT licence.** v2.1 root README says CC BY-SA 3.0 US. The v1.7 docs say CC BY-NC-SA 2.5.
3. **Pedalion licence.** Repo root `LICENSE` is MIT; `TREEBANK_LICENSE` is BY-SA 4.0.
4. **Gorman review statement.** README says "all annotated by one person" for the Crito file. README also says post-2020 uploads were pre-parsed and hand-corrected. These are not reconciled per file.
5. **Lysias CTS.** `perseus-grc1` in AGDT and Gorman vs `perseus-grc2` in canonical.

## 5. Decision frontier

- **Anabasis Book 1:** Blocked on rights (conflict 1) and review scope (unspecified). Token correspondence is unverified. There is no human-reviewed alternative in the public inventory, so this is not a rights-only blocker.
- **Lysias 1:** Blocked on lineage (no independent second source) and CTS mapping. Rights conflict 2 also applies.
- **Crito:** Two candidates, each with a different blocker. Kahl has full section coverage but no review statement. Pedalion has review but partial coverage and a non-CTS identifier. Rights for both are unresolved.
- **No automatic substitute.** GLAUx is automatic and is not used to fill a human-review gate.

## 6. Limits

- No human contact, no outreach, no access retries. Every finding is from public, self-service files at the pins above.
- The Gorman guideline PDF and the Treebanking Tips PDF were not text-extracted. Their homograph-digit and lemma-review definitions are **not established from available text**. This is why review scope for Book 1 is recorded as unspecified.
- OGA was not checked at file level.
- Token correspondence between Gorman and canonical Book 1 was not measured in this report. The #2042 report contains the earlier measurement.
- Steadman readers were not opened and are out of scope.
- No candidate was validated as a text. These findings are gate outcomes, not acceptance.

## 7. Scratch and reproducibility

Raw upstream files are kept in `~/.cache/scratch/greek-2044-public-gates/` and are **not committed**. They contain upstream treebank and TEI text under unresolved licences. Only counts, tuple-match numbers, and source identifiers are committed here. The scratch directory is removed after this report is pushed.
