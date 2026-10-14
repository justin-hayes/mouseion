# Ancient Greek Anabasis: final bounded public-source pass (#2045)

Status: research findings only. Not legal advice, not a source acceptance, and not a decision on the MVP route (#2030). Baseline: parent-reviewed #2044 comment 6165876954 and the earlier Book 1 evidence report (`doc/research/ancient-greek-anabasis-book1.md`, #2042).

## Scope and constraints honored

- Public, unauthenticated sources only. No accounts, tokens, paid or gated access, and no retries of denied access.
- The Gorman guideline PDFs were not opened, downloaded, or extracted by any mechanism.
- No outreach, upstream posts, Steadman reader integration, raw-text NLP, provider calls, cards, or production, dependency, ADR, or glossary edits.
- Read-only range requests were used only to list archive central directories and fetch small metadata files.

## Summary

1. The Gorman Greek treebank carries two licence statements that do not agree. Its README declares CC BY-NC-SA 4.0. The `TREEBANK LICENSE` file it points to is the full CC BY-SA 4.0 legal code, with no NonCommercial clause. Neither statement can be chosen over the other from public evidence.
2. Under the CC legal codes, the two declarations lead to different permitted adaptations. A shared adaptation of BY-SA 4.0 material must be licensed BY-SA 4.0 or later (or a listed compatible licence). A shared adaptation of BY-NC-SA 4.0 material must be BY-NC-SA 4.0 or later. The "honour the NC declaration" route therefore cannot cover a shared adaptation if the BY-SA file governs.
3. The pinned Gorman tree has Books 1 and 3 of the Anabasis (plus Xenophon's *Hiero*, a different work). Books 2 and 4–7 are absent. Coverage is therefore partial even before licensing.
4. The alternative Anabasis sources found are either licensed under terms that do not match Gorman's, or have provenance that is automatic or unknown. None is validated here.
5. No operation is marked **supported** for the Gorman material. Several are **conditional** or **unresolved**. The bounded search did not establish a conservative non-commercial route for the Gorman Anabasis.

## Operation matrix

Status key: **supported** = the public record directly permits it on the stated terms. **conditional** = permitted only if a named condition holds, which is not yet established. **unresolved** = the public record conflicts or is silent, so no determination is made.

| # | Operation on Gorman Anabasis text (Books 1, 3) | Status | Basis and open condition |
|---|---|---|---|
| 1 | Reading the files locally for private study, with no copy shared | conditional | CC licences govern the exercise of licensed rights such as reproduction and distribution. Whether unshared local use needs a licence is not assessed here. |
| 2 | Self-hosted instance serving the text to a closed group, unmonetised | unresolved | CC FAQ says NonCommercial turns on intent and specifics, and that the absence of revenue is not dispositive. Serving copies to others is distribution, so both declarations apply. |
| 3 | Public redistribution of the Gorman XML verbatim | unresolved | The README (NC-SA) and the file (SA) conflict. Redistribution under BY-SA would not satisfy an NC declaration. The conflict cannot be resolved by choosing the more permissive grant. |
| 4 | Public release of a derived adaptation (transformed data or code built from the XML) under BY-SA 4.0 or later | conditional | Compatible with the BY-SA legal code if BY-SA is the operative grant. Still depends on resolving row 3's conflict. |
| 5 | Public release of a derived adaptation under BY-NC-SA 4.0 | unresolved | Only compatible with the legal code if the NC declaration governs. Under the BY-SA file, this is not a permitted licence for the adaptation. |
| 6 | Commercial use of any of the above | unresolved | Matters only if NC governs. This report does not treat "the app is free" as NC waived, nor "a paid provider" as commercial. |
| 7 | Using the Anabasis layer of an OGA-derived dataset | unresolved | See candidates A and B below. Declared licence is BY-SA 4.0, but per-text terms are deferred upstream and the Anabasis layer's provenance and review status are not established. |
| 8 | Using `cltk/greek_pos_edit_xenophon_anabasis` | conditional | Repo declares MIT; the text is declared as Marchant (OCT 1904), public domain. Tags are automatic (TnT) with crowd edits invited, not human-reviewed. Each declaration is only as good as the chain of custody, which is not verified. |
| 9 | Citing the upstream repositories and their declared licences (metadata only) | supported | Describing what a source declares is a factual statement, not a reuse of its content. |

## Search ledger

Primary sources (all fetched unauthenticated):

- Gorman, *Greek-Dependency-Trees*, pinned commit `d823e96d253277bc1872b83ff5ac8c393a0f830a`.
  - `README.md` (1225 bytes): "This work is licensed under a Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International License (see TREEBANK_LICENSE file)."
  - `TREEBANK LICENSE` (filename contains a space; the underscore form returns 404): 20131 bytes, sha256 `b42fac528cae505cd8673b7f0c6112dc7c73e9c04a75f71af4517830796bf208`. Content is the full CC BY-SA 4.0 legal code. No NonCommercial term.
  - Tree listing: `Xen_Anab_book_1.1-5.xml`, `Xen_Anab_book_1.6-9.xml`, `Xen_Anab_book_3.xml`, `Xenophon_Hiero.xml`. Books 2 and 4–7 absent.
- CC BY-SA 4.0 legal code, sha256 `28a9529c7d0bb4dc51f4bf5c116a3d16ef247a052f7591466768ddf563fd1cf5`. Section 3(b)(1): adaptations must be licensed BY-SA 4.0 or later, or a BY-SA Compatible License.
- CC BY-NC-SA 4.0 legal code, sha256 `e66c269d4819aaab34b49ef5220c4ddab6756f21bb5180761a4eb8561f2b7bbd`. Section 3(b)(1): adaptations must be licensed BY-NC-SA 4.0 or later, or a BY-NC-SA Compatible License.
- CC compatible-licenses page, sha256 `0a15d15f756863c218dabbd4434e29bd9734ed0538955c0cc5bbfa817c7f7ab1`. For BY-SA 4.0 material: BY-SA 4.0 or later, ported versions, Free Art 1.3, and GPLv3 (one-way). BY-NC-SA 4.0 is not listed. For BY-NC-SA 4.0 material, no non-CC compatible licences are listed.
- CC FAQ, sha256 `d4ffb015ad13ce12eb060979abf835caa5c2ce2ef5d928e6af1885845db99c02`. NonCommercial means "not primarily intended for or directed towards commercial advantage or monetary compensation." Whether a use is commercial depends on specifics and intent. A for-profit entity's use does not necessarily violate NC; a nonprofit's use can. A licensor may offer more than one set of terms, but that is the licensor's choice, not a licensee's right to pick between two third-party declarations.

Candidates (not validated; no full-text or sentence checks run):

- **A. `cltk/greek_pos_edit_xenophon_anabasis`.** Repo licence MIT (`LICENSE`, 2014 copyright). Text declared as Marchant OCT 1904, public domain. Book files are TnT-tagged automatically; crowd edits invited. Automatic provenance, not human-reviewed.
- **B. `nauarchus/AGDTmini`.** Licence field NOASSERTION on GitHub; `LICENSE.md` declares CC BY-SA 4.0, derived from OGA v0.2.0. Contains `data/tlg0032/tlg006/tlg0032.tlg006.agdtm-grc1.csv` (1,458 lines, locations such as "Xen. Anab._1.1.1"). Lemma@XPOS strings look automatic. Provenance statement is secondhand.
- **C. Opera Graeca Adnotata (OGA), Zenodo 14206061.** Top README declares CC BY-SA 4.0 and defers per-text terms to the original repositories. Describes "morphosyntactically annotated" layers with no review statement. The nested `workspace/oga.zip` (stored, 3,225,283,562 bytes) was not listed in this pass; its Xenophon entries and provenance metadata remain unread.
- **D. Diorisis, figshare 6187256.** CC BY 4.0 record; single 194 MB zip (md5 `f3a26efa7e7d2b93d1bcca26900d180a`). Annotation method and Xenophon coverage not confirmed; not inspected.

Ruled out for Anabasis coverage (checked, no Xenophon *Anabasis* files found): PerseusDL/treebank_data at `bf4334f0` (no tlg0032 paths); UD Ancient Greek Perseus and PROIEL READMEs (no Xenophon); pedalion/treebanks; grp-grc-treebanks; AristarchusTreebank-Lit (Thucydides only); GLAUx repositories; gAGDT; pAGDT; agldt-rs; daphne-trees.

Repository search for "tlg0032" returned no results.

## Open items that would settle the question

1. Read the OGA nested `workspace/oga.zip` central directory for tlg0032 entries and any per-text metadata; establish whether the Anabasis layer is automatic or reviewed.
2. Establish, from a primary statement by Gorman or the repository owner, which of the two Gorman declarations governs. This report did not and should not contact anyone; that decision is for the project owner.
3. Establish the Anabasis books for which any candidate has text, before any MVP route is selected. Books 2 and 4–7 have no public Gorman coverage in the pinned tree.

## Notes

- Gorman's README points to Perseids guidelines (Bamman and Crane; Bamman et al.) and a "Treebanking Tips" sheet. Those were not opened, per the ticket's constraint.
- The `TREEBANK LICENSE` file is the only text of the Gorman grant reviewed here. Its sha256 is recorded above so the pinned version can be re-checked.
