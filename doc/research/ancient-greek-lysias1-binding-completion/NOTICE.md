# Rights, provenance and attribution notice (#2048 binding and segmentation completion)

Not legal advice. This extends the [reconciliation NOTICE](../ancient-greek-lysias1-reconciliation/NOTICE.md) for the producer-tokenizer probe and the completion ledger. It adds no licence choice and no approval.

## What is committed

- `manifest/` contains ASCII keys, statuses, counts, blocker labels and code-point escapes (`U+XXXX`) for every Greek form, including the producer alignment and insertion tables. A check of the committed files finds no non-ASCII byte. The escaping is not a rights mechanism: the escaped forms are still derived from AGDT v1.6 and the historical grc1 text.
- No witness text is copied here. Witness evidence is cited by the ledger rows of the dc3e5ed7 collation, as in the reconciliation.
- `scripts/` (probe, ledger, tests, fetch script) was written for this research.
- The llt-tokenizer sources are not vendored. `scripts/fetch_tokenizer.sh` exports two Ruby files at pinned commits into the scratch folder; the probe reads them as data and writes no source text to `manifest/`.

## Inputs and declarations

| Component | Declaration | Reach and open questions |
|---|---|---|
| llt-tokenizer (`latin-language-toolkit/llt-tokenizer`), `c9b60a34` and `a4a810f` | `LICENSE.txt`: MIT License, "Copyright (c) 2013 LFDM", at the pinned commits. Read as data only. | Used to reproduce a historical Greek split. Nothing from it is copied into the outputs. |
| `llt/segtok` gem (the producer named in the AGDT v1.6 header) | Not reviewed. Its service URI (`services.perseids.org/llt/segtok`) returns HTTP 500, and no public source links it to these tokenizer commits. | **Producer identity is not established.** No rights statement is made about the deployed producer. |
| AGDT v1.6 Lysias I | As in the reconciliation NOTICE: the root `README.md` declares CC BY-SA 3.0 US. The v1 greek docs declare CC BY-NC-SA 2.5 for AGDT 1.7. | Whether the 1.7 declaration reaches the schema-1.6 file is **not established** (unchanged from the reconciliation). |
| grc1 @ `aa0dd9e6` (historical canonical-greekLit blob) | No licence element in the blob; no licence file at that commit. | Rights not reviewed. Used only for the lineage facts already published at dc3e5ed7. |
| grc2 (`canonical-greekLit@01b725d8`) | TEI `licence` and `license.md`: CC BY-SA 4.0. | Unchanged from the reconciliation. |
| Mnemosyne, Wikisource, Hude | Not fetched again in this work. Their evidence is read through the dc3e5ed7 collation files only. | Mnemosyne is declared all rights reserved in that collation; its text is not reproduced. |

## Attribution for derived content

- Ancient Greek Dependency Treebank, Lysias 1 (`tlg0540.tlg001.perseus-grc1`), annotator Vanessa Gorman; release editor Giuseppe G. A. Celano; Perseus Digital Library, Tufts University. `PerseusDL/treebank_data` @ `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`, file v1.6. Modified: fields re-encoded as code points and joined with derived statuses.
- `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c` (grc2) and `aa0dd9e6a25c848c04e75169e525ec413f033499` (grc1, historical). Modified: token code points and positions.
- `latin-language-toolkit/llt-tokenizer` @ `c9b60a345708af72c721ec1183c9ab928f7b8c03` and `a4a810f02f73bacf3f16e722425dfb80f3b2a5ab`, MIT. Read as data; reproduced behaviour is described, not copied.

## Unknowns kept explicit

- The identity and deployed revision of the producer `llt/segtok` named by the AGDT v1.6 header. The historical probe is a candidate only.
- Reach of the AGDT 1.7 NC declaration to the v1.6 Lysias file (unchanged).
- Rights of the historical grc1 blob (unchanged).
- The licence of these research outputs (a human decision, not made here).
