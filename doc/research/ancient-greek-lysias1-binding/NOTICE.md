# Rights and provenance notice (#2048)

This notice records what each pinned source declares and where each declaration applies. It is not legal advice. Each row below states what the source says, which layer it covers, and what is left uncertain.

## Sources and declarations

| Layer | Pinned source | Declaration read | Applies to | Qualification |
|---|---|---|---|---|
| Native annotation, v2.1 (primary) | `PerseusDL/treebank_data` @ `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`, root `README.md` | "Contents of this repository are licensed under" CC BY-SA 3.0 United States. The root copyright line says material to which third parties hold copyright is excluded from the Perseus/Tufts copyright statement. | Repository contents, including `v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml` | The v2.1 file header has no licence element. `v2.1/Greek/README.MD` has no licence statement. Third-party qualifications on individual files were not reviewed. |
| Native annotation, v2.0 (witness) | same repository and revision, `v2.0/Greek/nonArethusaCompliant/...` | as above | as above | The v2.0 file header and `v2.0/Greek/README.MD` have no licence statement. |
| Native annotation, v1.6 (witness) | same repository and revision, `v1.6/greek/data/...` | as above | as above | The v1.6 file header and `v1.6/README.md` have no licence statement. The file is covered by the root declaration only if no narrower declaration applies. See the uncertainty below. |
| AGDT 1.7 documentation | same repository and revision, `v1/greek/docs/README.txt` | "AGDT 1.7 is licensed under a Creative Commons Attribution-NonCommercial-ShareAlike 2.5 License" (`by-nc-sa/2.5`) | Documentation for AGDT 1.7 | Scoped to the 1.7 documentation. Whether it governs the v1.6 data file is **not established**. |
| Canonical running text, grc2 | `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c`, `data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml` | TEI `availability/licence`: "Available under a Creative Commons Attribution-ShareAlike 4.0 International License" (`by-sa/4.0`) | This file | The header's `sourceDesc` cites a print source (William Heinemann Ltd.; Harvard University Press; an Internet Archive scan). The print edition's own rights status was not determined. |
| Canonical repository licence | same, `license.md` | CC BY-SA 4.0 | Repository contents | None found. |
| AGDT2 and GSAAGDT guidelines | Guideline text from the same repository's AGDT2 and v1 documentation (scratch copies, not committed) | As declared in those documents. | Search only | Used only to search for conventions. Not reproduced in committed artifacts. |

## Operations

- **Private analysis.** Reading and computing on the pinned inputs locally does not publish or redistribute them. The declared licences above permit that use.
- **Committed derived manifests.** `manifest/` contains hashes, code-point offsets, ASCII native and canonical keys, classes, reasons and counts. It contains no Greek text. Some of its content is derived from the sources. Whether the derived manifests are adaptations that carry the share-alike terms of CC BY-SA 3.0 US and 4.0 is **not determined** here. It is recorded as an open question, with attribution (below) provided in the meantime.
- **v1.6-derived columns.** `release_stability.tsv` (rows for `agdt-v1.6`), the `witness_v16_equal_canonical` column, and the `conditional` status of the 50 fold nodes all depend on the v1.6 data file. The NC clause in the 1.7 documentation is the only declaration that could govern that file. Its applicability is **genuinely uncertain**. Removing these columns would not change any node key, span or accepted status, but it would remove the only convention evidence for the 50 conditional folds.

## Attribution for output derived from these inputs

- Native annotation: Ancient Greek Dependency Treebank, `PerseusDL/treebank_data` @ `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`, Perseus Digital Library, Tufts University. Root README: CC BY-SA 3.0 United States.
- Canonical text: `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c`, Perseus Digital Library, Tufts University, `tlg0540.tlg001.perseus-grc2`. TEI header: CC BY-SA 4.0.

## What is committed

- Hashes, code-point spans, ASCII keys (native sentence and node IDs; `cg2:` token IDs), classes, reasons and counts.
- Scripts that read the pinned inputs by URL and hash. The scripts contain no Greek text.
- Greek surfaces are **not** committed. Per-occurrence surfaces go only to a scratch `detail/` directory.

## Unknowns recorded explicitly

- Whether the NC clause of the AGDT 1.7 documentation governs the v1.6 data file, and so whether the v1.6-derived committed columns are affected.
- Whether the derived manifests are adaptations under CC BY-SA.
- Whether any individual AGDT file carries a third-party qualification.
- The rights status of the print source named in the grc2 header.
- The licence of this repository's own research outputs. That is a human decision and is not made here.
