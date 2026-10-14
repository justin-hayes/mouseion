# Ancient Greek public source gates: correction (supersedes 580e834d)

**Research date:** 2026-10-14

**Issue:** [#2044](https://github.com/justin-hayes/mouseion/issues/2044) (reopened; map [#2021](https://github.com/justin-hayes/mouseion/issues/2021), parent [#2030](https://github.com/justin-hayes/mouseion/issues/2030)).

**Supersedes:** [`ancient-greek-public-source-gates.md`](ancient-greek-public-source-gates.md) at commit `580e834d`. That report is kept unchanged as history. Where this report disagrees with it, this report is correct.

**Scope:** Public, self-service evidence only, at the pins in section 7. Three candidate texts: Xenophon *Anabasis* Book 1 (preferred), Lysias 1 and Plato *Crito* (fallbacks). No outreach, no people contact, no accounts, no upstream posts, no provider or LLM calls, no paid or gated sources, no retries of the denied Gorman guideline PDF, and no use of the Gorman "Treebanking Tips" PDF. Steadman readers are out of scope. This report accepts or rejects no source; the MVP decision is human-owned.

**Status:** The earlier negative result was overstated. The text-authority gate it treated as blocking was measured with a token-strict method that counts diacritic and clitic-split differences as failures. Measured at character level, two candidates now pass the rights, review and text-authority gates on the rules in section 2, subject to two human confirmations. Anabasis Book 1 remains blocked on rights and review, not text.

## Executive findings

1. **Lysias 1 (AGDT v2.1) passes all three gates on the stated rules**, pending two human confirmations (section 5). The previous report said it was blocked on a CTS mismatch and on lineage. Neither is a blocker: an unidentified edition may be accepted when its bounds and text are verified, and shared lineage with the Gorman file is not a second review.
2. **Plato *Crito* has two candidates, and neither is complete on its own.**
   - *Gorman/Kahl* text matches canonical grc2 at 99.97% of letters and covers all 59 Stephanus sections. It is blocked only on rights (the Gorman NC/BY-SA conflict).
   - *Pedalion* covers all 59 sections, but it omits the 33-token passage in section 54b. The previous report called this "partial section coverage". That was wrong: every section is present, and the defect is one contiguous omission.
3. **Anabasis Book 1 (Gorman) is near-identical to the declared edition, not token-identical.** At character level it matches 99.97% of canonical letters. Nine blocks differ: three word-level (two omissions, one insertion), two lexical variant forms, and four accent or diacritic variants. It is blocked on rights (conflict unresolved) and on review scope (unspecified). Text is not the primary blocker. The previous "98.0% units exact" figure was a token-level number that counted diacritic and clitic differences as mismatches.
4. **No other inventoried public source carries Anabasis Book 1 with human review.** OGA was not checked at file level in the previous report. It is now checked: it carries `tlg0032.tlg006.perseus-grc2`, but its annotations are declared automatic.
5. **The Gorman licence conflict is still unresolved.** README BY-NC-SA 4.0 conflicts with the bundled `TREEBANK LICENSE` (BY-SA 4.0). Nothing in the public record resolves it, and the stricter reading is not adopted as a fix. It blocks Anabasis Book 1 and Crito/Kahl.

## 1. Gate ledger

Gates: **R** rights (a licence that permits the intended use for the component actually used, with any conflict resolved), **V** review (documented human involvement in the annotation layer used), **A** text authority (the annotated tokens match a pinned, bounded text).

"Validated" means measured at the pins in section 7. "Provisional" means a lead that needs a human decision.

| Route | Component | R | V | A | Outcome |
|---|---|---|---|---|---|
| Lysias 1 | AGDT v2.1 `tlg0540.tlg001.perseus-grc1.tb.xml` | Pass per release (v2.1 Greek under root BY-SA 3.0 US; v1.7 NC docs do not apply). **Provisional** | Pass: manual annotation (Gorman), Morpheus morphology assistance | Pass as unidentified edition: 50/50 sections, bounds match, 99.97% letters match grc2. **Provisional** on accepting the unidentified-edition rule | **Candidate: passes on stated rules, two human confirmations** |
| Lysias 1 | Gorman `Lysias 1 bu1.xml` | Blocked (README NC conflict) | Same as AGDT | Same as AGDT | Not needed: same lineage as AGDT |
| *Crito* | Gorman `Plato_Crito_Travis_Kahl.xml` (`crito_gorman_kahl.xml`) | **Blocked**: README BY-NC-SA 4.0 vs file BY-SA 4.0 | Pass: single named annotator (Kahl); the README "one person" statement and post-2020 pre-parse statement are a layer ambiguity, not a human-involvement conflict | Pass: 99.97% letters match grc2; all 59 sections labelled (two range labels, `51a-c` and `52d-e`, resolve correctly) | **Blocked on R** |
| *Crito* | Pedalion `public/xml/crit.xml` | Pass on the BY-SA 4.0 data licence (`TREEBANK_LICENSE`). MIT root `LICENSE` covers code and does not conflict. **Provisional**: README line is tentative | Pass if partial correction counts: Joosten, partially corrected by Van Hal from a Keersmaekers pre-parse. **Provisional**: correction scope is partial | **Fails**: 54b omission (section 2.4); all 59 sections present otherwise | **Blocked on A** |
| *Anabasis* 1 | Gorman `Xen_Anab_book_1.*` | **Blocked**: README NC vs file BY-SA | Unspecified review scope (the data article describes a lemma-selection workflow, not the Book 1 upload) | Near-pass: 99.97% letters match grc2; nine differing blocks (section 2.1) | **Blocked on R and V** |
| *Anabasis* 1 | AGDT v2.1, UD Perseus, PROIEL | n/a | n/a | n/a | **No Book 1 data** |
| *Anabasis* 1 | Pedalion Trees | n/a | n/a | n/a | **No Book 1 data** (*Memorabilia* only) |
| *Anabasis* 1 | GLAUx `0032-006.xml` | Not assessed | Automatic (`analysis="auto"`) | Not assessed | **Insufficient** |
| *Anabasis* 1 | OGA `tlg0032.tlg006.perseus-grc2` | Not assessed | Automatic (README: "automatically generated annotations") | Not assessed | **Insufficient** |

Anabasis Book 1 has no route that passes R and V. The preferred reading therefore stays blocked, and it is blocked on documents, not on text.

## 2. Evidence

Method for all text figures: canonical and annotation tokens were extracted from the pinned XML. Punctuation tokens and elliptic (`artificial`) nodes were excluded from annotation tokens. Two measures were taken:

- **Token-strict:** the share of canonical tokens inside exact token blocks, and section-level coverage.
- **Letter-level:** all punctuation, hyphens, apostrophes and combining marks are removed, and the letter sequence is compared with the canonical sequence. This is the decisive measure for text authority, because it ignores clitic splits (`οὔ` + `-τε` for `οὔτε`) and tokenisation of elided or crasis forms.

### 2.1 Anabasis Book 1 (Gorman)

- **Pin:** `vgorman1/Greek-Dependency-Trees` `d823e96d`. Files: `xml versions/Xen_Anab_book_1.1-5.xml` (342 sentences) and `Xen_Anab_book_1.6-9.xml` (269 sentences), 611 in total.
- **Canonical:** `tlg0032.tlg006.perseus-grc2` at canonical-greekLit `01b725d8`. Book 1 cut from the seven-book file, 9,325 tokens and 47,869 letters.
- **Letter-level:** 47,853 letters matched. **99.97%** of canonical letters; 99.98% of Gorman letters. Nine non-equal blocks:
  - Word-level (three): `ἀνέβη` present in canonical, absent in Gorman. `ἐπτὰ` present in Gorman, absent in canonical. One `ῆν` form present in canonical, absent in Gorman.
  - Lexical variants (two): `Τισσαφέρνους` (canonical) vs `Τισσαφέρνης` (Gorman); `τῇ ἀρχῇ` (canonical) vs `ἡ ἀρχὴ` (Gorman).
  - Accent or diacritic variants (four): `ᾶ`/`ά` (twice), `ῇ`/`ὴ`, `ού`/`έ`.
- **Conclusion:** Gorman Book 1 does not match the declared edition token-for-token. It is near-identical, with a small set of real variants. Under the rule that no cross-edition substitution is allowed, it fails a strict identity gate. This is a finding for a human to weigh, not a route to rescue by substitution.
- **Section 1.7.20:** no Gorman label. Sentence labels (204 of 207 unique labels) map to canonical Book 1 sections; the three non-canonical labels are `1.5.171`, `19.31` and `3.4.1`.
- **Token-strict (for comparison with the earlier report):** the earlier 98.0% exact unit figure remains arithmetically correct as a token-level number. It is not a text-authority verdict.

### 2.2 Anabasis Book 1: other inventoried sources

- **AGDT v2.1:** no `tlg0032`.
- **UD Perseus and PROIEL:** no `tlg0032`.
- **Pedalion Trees:** Xenophon *Memorabilia* only.
- **GLAUx:** 3,837 sentences, 65,763 words, all `analysis="auto"`.
- **OGA** (`a69d917d`): `urn_cts/texts/urn_cts.xml` lists `tlg0032.tlg006.perseus-grc2`. The README says the annotations are automatically generated, with a parser and lemmatiser trained on AGDT. Automatic, so it does not meet V.

### 2.3 Lysias 1 (AGDT v2.1)

- **Pin:** AGDT `bf4334f0`. File `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml`.
- **Canonical:** `tlg0540.tlg001.perseus-grc2` at `01b725d8` (Lamb 1930). No `grc1` canonical exists at the pin.
- **Bounds:** 50 textpart sections, all present, 0 with no exact tokens. The canonical start (1.1) and end (1.50) align with the first and last AGDT tokens.
- **Token-strict:** 2,409 canonical tokens and 2,428 annotation tokens. 99.17% of canonical tokens inside exact blocks. 35 sections fully exact, 15 partial, 0 with none.
- **Letter-level:** 12,281 of 12,285 canonical letters matched, **99.97%**; 99.72% of AGDT letters. Differences:
  - Clitic and crasis splits (`κ-ἀγὼ` for `κἀγὼ`, `τ-ἀληθῆ` for `τἀληθῆ`, `παῤ` vs `παρʼ`, `οὔτε` vs `οὕτε`).
  - **Five rubric tokens** present in AGDT and absent in grc2: `Νόμος` (three times) and `Μάρτυρες` (twice), in sections 29, 30, 32 and 43. These are inserted headings inside quoted documents.
- **Lineage:** Gorman `Lysias 1 bu1.xml` has identical statistics to the AGDT file, so the two share lineage. That is not a second review and not a blocker.
- **Licence:** the v1.7 docs (`v1/greek/docs/README.txt`) say CC BY-NC-SA 2.5. The v2.1 Greek directory has no licence statement, so the root README (CC BY-SA 3.0 US) is the operative statement for that release. The NC statement belongs to v1.7 and does not attach to v2.1 by inference.
- **Review:** the v2.1 Greek README states the texts were "semi-automatically annotated" and "annotated manually", with Morpheus assisting on morphology. Native manual annotation is accepted under the review rule, so no second review statement is needed.

### 2.4 Plato *Crito*

- **Canonical:** `tlg0059.tlg003.perseus-grc2` at `01b725d8` (Burnet 1905). 4,172 canonical tokens, 19,680 letters, 59 Stephanus section milestones (43a–54e).

**Gorman/Kahl** (`Plato_Crito_Travis_Kahl.xml`, file added 2022-03-03):
- 4,248 annotation tokens after exclusions. 28 sections fully exact and 31 partial at token level, because the file splits clitics that the canonical keeps whole.
- Letter-level: **19,675 of 19,680 letters, 99.97%** of canonical; 100.00% of Gorman letters. The one canonical-only block is the editorial citation `ηομ. ιλ. 9.363` in section 44b, which is not running text.
- Labels: 60 distinct `subdoc` labels cover all 59 sections once the two range labels are expanded.
- Annotator named in the header: TravisKahl. No review statement in the file.

**Pedalion** (`public/xml/crit.xml`, document `0059-003`, first release 2020, Joosten with partial correction by Van Hal from a Keersmaekers pre-parse):
- 4,142 annotation tokens. 51 sections fully exact, 8 partial at token level, 0 with no exact tokens.
- Letter-level: 19,505 of 19,680 letters, **99.11%** of canonical. Non-equal blocks:
  - The citation `ηομ. ιλ.` in 44b (editorial, as above).
  - **Section 54b omission.** The canonical passage beginning `ἀλλʼ, ὦ Σώκρατες, πειθόμενος ἡμῖν τοῖς σοῖς τροφεῦσι μήτε παῖδας περὶ πλείονος ποιοῦ …` and ending `… ἔχῃς πάντα ταῦτα ἀπολογήσασθαι τοῖς ἐκεῖ ἄρχουσιν· … οὐδὲ ὁσιώτερον, οὐδὲ ἄλλῳ τῶν σῶν οὐδενί, …` is 33 canonical tokens. It is absent from Pedalion: `ἀπολογήσασθαι` has zero Pedalion tokens, and `πειθόμενος` appears nowhere in the file. Words either side (`ἀφικομένῳ`, `ἠδικημένος`, `φασκόντων`) do match, so the gap is one contiguous block.
  - One transposition: `ἐξελθών` appears in 53a in Pedalion but not at the canonical position.
  - Remaining blocks are em-dash joins (`φοβοῦ—καὶ`) and punctuation tokenisation, which the letter-level measure ignores.
- Identifier `0059-003` is not a CTS URN. The decision in section 3 means this is a provenance note, not a gate failure.
- **Licence history** (`pedalion-trees`): root `LICENSE` is MIT (code). CC0 at `8c39f8f` (2018-12-12). `TREEBANK_LICENSE` is BY-SA 4.0 from `14598e0` (2019-04-29). README line "We will assign… probably BY-SA 4.0" added tentatively in `3e57923` (2019-08-25). `crit.xml` added in `bf3bcab` (2020-12-21). The README does not mention Crito; only an XML comment does.
- About 170 word lines have `postag=''` (carried from the previous report, not re-measured here).

## 3. Rules applied

- **Shared lineage needs no independent second annotator.** Human-annotated, or human-reviewed automatic, is accepted. Native manual annotation needs no second "review" statement.
- **Licences are assessed per release and per component.** AGDT v1.7 NC does not conflict with v2.1 BY-SA 3.0 US by inference. MIT code does not conflict with BY-SA data.
- **CTS is not mandatory.** A missing `grc1` canonical does not exclude a pinned, annotation-carried text as an explicitly unidentified edition, provided bounds, text and rights are verified. No cross-edition substitution: the AGDT file is not re-labelled as grc2.
- **A CTS declaration is an attributed claim**, not proof of the source revision. The AGDT `grc1` URN is recorded as declared.
- **Crito's Stephanus 43–54 may be the entire dialogue.** Internal boundaries were verified rather than assumed.

## 4. Corrections to 580e834d

1. **Lysias 1:** "blocked on lineage and CTS mapping" is withdrawn. Lineage is not a second-review problem. The CTS mismatch is an attributed claim and does not block the unidentified-edition route. Letter-level text match is 99.97%.
2. **Crito/Pedalion:** "partial section coverage" is withdrawn. All 59 sections are present. The blocking defect is the 54b omission, a 33-token contiguous passage.
3. **Crito/Pedalion licence:** "MIT/BY-SA licence conflict at repo level" is withdrawn. MIT code does not conflict with BY-SA data; the operative data licence is `TREEBANK_LICENSE` (BY-SA 4.0).
4. **Anabasis Book 1 text:** "token correspondence unverified" is now measured: 99.97% of letters match, in nine differing blocks, three of them word-level.
5. **OGA:** "not checked" is now checked. It is automatic and does not meet V.
6. **Crito/Kahl:** the 580e834d report said Kahl had "no review statement" and listed review as blocked. The file names one annotator, which meets V under the rule in section 3. Kahl is blocked on R only.

Unchanged from 580e834d: the Gorman licence conflict, the AGDT licence statements, and the Anabasis route being blocked on rights and review (now also with a measured text picture).

## 5. Human frontier

Ordered by how many routes each one unblocks:

1. **Gorman NC vs BY-SA (unresolved).** Blocks Anabasis Book 1 (with review scope) and Crito/Kahl. No public document resolves it. The stricter NC reading is not applied as a fix. This needs a documentary answer from the rights holder, which is outside the scope of this task, so it is a human decision.
2. **Lysias confirmation, two parts.**
   - (a) Accept the v2.1 Greek release under the root BY-SA 3.0 US statement, with v1.7 NC not attaching. This is the per-release reading the rules set. A human should confirm it.
   - (b) Accept an unidentified-edition text authority for AGDT Lysias 1: the declared `grc1` URN has no pinned canonical, the text matches grc2 at 99.97%, and five rubric tokens are AGDT-only. A human should decide whether those five tokens are acceptable.
3. **Pedalion 54b omission.** Pedalion cannot be used as a complete Crito text at its pin. Options are to accept a documented gap, or to take a different source. Contact with upstream is out of scope for this task.
4. **Anabasis Book 1 has no route that passes R and V.** The only candidate (Gorman) is blocked on item 1, and its text differs from the declared edition in nine places. A human should decide whether an Anabasis route is worth pursuing before any further search.

## 6. Limits

- Public, self-service files at the pins only. No outreach, no people contact, no access retries, no paid or gated sources.
- The Gorman guideline PDF and the Treebanking Tips PDF were not used. Lemma and homograph-digit review definitions remain unestablished, which is why Anabasis review scope stays unspecified.
- Steadman readers were not opened.
- The letter-level measure is a comparison of letter sequences. It does not verify word boundaries, morphology or lemma correctness.
- The AGDT `grc1` URN is recorded as declared and not validated against any upstream edition.
- These are gate outcomes, not acceptance decisions. The MVP decision is human-owned.

## 7. Pins and reproducibility

- AGDT `bf4334f0af5e`; canonical-greekLit `01b725d835e6`; pedalion-trees `112c106b86b8`; Gorman upstream `d823e96d2532`; Gorman mirror `83d93a554a46`; OGA `a69d917dbe11`.
- Raw upstream files live in `~/.cache/scratch/greek-2044-raw/` and are **not committed**. They contain upstream treebank and TEI text under unresolved licences. Only counts, percentages and identifiers are committed here.
- Scripts (token alignment, letter-level comparison, Book 1 cut) live in `~/.cache/scratch/greek-2044-scripts/`, also not committed. The method is described in section 2.
- Both scratch directories and the worktree are removed after the push.
