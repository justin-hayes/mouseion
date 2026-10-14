# Rights, provenance and attribution notice (#2048 reconciliation)

Not legal advice. This records what each input declares, what the committed outputs contain, and which questions stay open. It adds no licence choice, approval or history rewrite.

## What is committed

- `manifest/` and `fixtures/` contain ASCII keys, hashes, statuses, counts, rule and evidence ids, and native strings as `U+XXXX` code-point sequences: AGDT forms and lemmas (v2.1 always; v1.6 where it differs), the 50f4ad6c headwords and canonical keys, and grc2 token code points on the 77 rows that are not exact single-token matches. The escaping exists so raw and NFC forms (for example `U+03C1 U+0313` against its NFC composition `U+1FE4`) stay visibly distinct. **It is not a rights mechanism**: escaped Greek is still Greek text and still derived from its sources.
- No witness text (Mnemosyne, Wikisource, Hude) is copied here. Per-node `collation_refs` point to rows of the dc3e5ed7 collation files by index only.
- Scripts and documents were written for this research.

## Inputs and declarations

| Component | Declaration read (pinned file) | Reach and open questions |
|---|---|---|
| AGDT v2.1 / v2.0 Lysias I | `treebank_data@bf4334f0` root `README.md`: contents under CC BY-SA 3.0 US; material to which third parties hold copyright is outside the Perseus/Tufts statement. No licence element in the file headers. | Root declarations can cover a file without a file-level notice. Third-party qualifications of individual files not reviewed. |
| AGDT v1.6 Lysias I | Same root `README.md`. `v1.6/README.md`: "Transformed versions of v1 treebank file for use in Arethusa (using 1.6 version of the treebank schema)", no licence statement. `v1/greek/docs/README.txt`: AGDT 1.7 under CC BY-NC-SA 2.5. | Whether the AGDT 1.7 declaration reaches this schema-1.6 Lysias file is **not established**; location alone does not settle it. Private research and private non-commercial use are permitted under either candidate declaration; public redistribution/adaptation of v1.6-derived values (the 83 discriminators, the 50 elision forms, the A2 primary route) stays unresolved. |
| grc2 text | `canonical-greekLit@01b725d8`: TEI `licence` and `license.md`, CC BY-SA 4.0. | Exact-component review under #2035 not performed here. The print source (Lamb 1930) rights were not determined. |
| grc1 @aa0dd9e6 | No licence element in the blob; no licence or README file in the tree at that commit (`aa0dd9e6-root-listing.txt`). | Whether the later repository licence reaches the historical blob is not reviewed. Used here only for lineage facts already published at dc3e5ed7. |
| dc3e5ed7 / 50f4ad6c outputs | Mouseion research outputs; their own NOTICE files record their sources' terms (MN all rights reserved; WS CC BY-SA 4.0; Hude unreviewed; Morpheus MPL-2.0; UD CC BY-NC-SA 2.5 counts only). | Consumed only through their committed manifests and collation files; no witness was re-fetched. |

Operations stay distinct: private research analysis, private non-commercial personal use through direct acquisition, and public redistribution or adaptation (bundles, mirrors, release assets, published decks or statistics) are separate questions under #2035. Witness research is not approval of witness text as product data.

## Attribution for derived content

- Ancient Greek Dependency Treebank, Lysias 1 (`tlg0540.tlg001.perseus-grc1`), annotator Vanessa Gorman; release editor Giuseppe G. A. Celano; Perseus Digital Library, Tufts University. `PerseusDL/treebank_data` @ `bf4334f0af5e13d16b04c1cccd6237e683ac6f5f`, files v2.1, v2.0 and v1.6. Root README: CC BY-SA 3.0 United States. Modified: fields re-encoded as code points and joined with derived statuses.
- `PerseusDL/canonical-greekLit` @ `01b725d835e6e733062ffd79e0efdbae1ba06e5c`, `tlg0540.tlg001.perseus-grc2` (Lamb 1930, Perseus encoding). CC BY-SA 4.0. Modified: token code points on 77 rows.

## Unknowns kept explicit

- Reach of the AGDT 1.7 NC declaration to the v1.6 Lysias file.
- Whether these derived manifests are adaptations carrying ShareAlike terms (treated as such for attribution).
- Rights of the historical grc1 blob and of the Lamb print.
- The licence of Mouseion research outputs themselves (a human decision, not made here).
