# Rights, provenance and attribution notice (#2050)

Not legal advice. This notice records what each input declares and what the committed files contain. It does not choose a licence or approve any component; acceptance is a human decision under [#2035](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026).

## What is committed

| Files | Contains | Derived from |
| --- | --- | --- |
| `manifest/book1.txt`, `sentences.tsv`, `sections.tsv`, `editorial_regions.tsv`, `review_set.tsv`, `card_inputs.tsv` | Greek text of *Anabasis* Book 1, as extracted | Perseus TEI text |
| `manifest/nodes.tsv`, `identities.tsv`, `selection_*.tsv`, `summary.json` | GLAUx forms, lemmas, postags, heads, relations, animacy and sense labels, plus derived spans, UPOS, keys and statuses | GLAUx annotation and the Perseus text |
| `manifest/crosswalk_letters.tsv`, `crosswalk_lemma_c_v.tsv` | UPOS counts per AGDT letter and per conjunction/verb lemma | UD Ancient Greek-Perseus |
| `fixtures/marchant-point-checks.tsv`, `manifest/marchant_point_checks.tsv` | Short OCR line excerpts | Internet Archive scan of Marchant 1904 |
| `scripts/`, `_selectionprobe/`, `inputs/`, documents | Research code, registry and prose written for this probe | — |

Raw upstream files are not committed. `scripts/fetch_inputs.sh` retrieves them from their origins by commit and verifies their sha256.

## Inputs and declarations

| Component | Declaration read | Notes |
| --- | --- | --- |
| Perseus `tlg0032.tlg006.perseus-grc2.xml` at `bd7b1c7a` (blob `6fc2f2a1`) | Repository `README.md` at that commit: "Unless otherwise indicated, all contents of this repository are licensed under a Creative Commons Attribution-ShareAlike 3.0 United States License", alongside a general statement that Tufts holds overall copyright and a request to offer Perseus any modifications. The file header carries no licence element. | The same Book 1 text, apart from the 1.3.14 correction, is declared CC BY-SA 4.0 in the file header and `license.md` from the 2026 revisions (`license.md` at `0189b9c8`). The underlying print, Marchant's 1904 Oxford text, is marked `NOT_IN_COPYRIGHT` on the Internet Archive item. |
| GLAUx `xml/0032-006.xml` and `metadata.txt` at `b077d8f6` | `README.md`: "GLAUx is generally available under a CC BY-SA 4.0 license. Please note though that some of the texts and/or the manual annotations may have a more restrictive license." Metadata row 405: `SOURCE_LICENSE` CC BY-SA 4.0, `TREEBANK_ANNOTATIONS` NA. | Every Book 1 segment is `analysis="auto"`, so no manually annotated treebank with a restrictive licence is imported into this file. The Zenodo archive's CC BY 4.0 record describes a different release and is not used here. Rights in the training data or models behind the automatic annotation are not conflated with rights in this output. |
| UD Ancient Greek-Perseus at `331ddef9` | `LICENSE.txt`: CC BY-NC-SA 2.5. | Used only to derive UPOS counts. A production adapter's vocabulary decisions need only the letter rules and the εἰμί rule, not the lemma table (see the report). |
| Internet Archive `operaomniarecogn03xenouoft` OCR | Item metadata `possible-copyright-status`: `NOT_IN_COPYRIGHT`. | Used for twelve point checks only. The archive has no immutable revision; the file is pinned by sha256. |

Operations remain distinct: private analysis, direct acquisition, public research outputs such as this branch, and bundled or distributed Mouseion data are separate questions under #2035.

## Attribution for derived content

- Xenophon, *Anabasis*, Book 1, ed. E. C. Marchant, *Xenophontis Opera Omnia* vol. 3 (Oxford: Clarendon Press, 1904). Digital text: Perseus Digital Library, Tufts University, `PerseusDL/canonical-greekLit` @ `bd7b1c7a36a031f5b320f522ca0e7b260109349c`, CC BY-SA 3.0 US (later revisions CC BY-SA 4.0). Modified: TEI markup removed with markup labels kept separately; XML whitespace collapsed; book heading excluded.
- GLAUx corpus, text 405 (`0032-006`), Alek Keersmaekers, `alekkeersmaekers/glaux` @ `b077d8f6ff429a5c7245954bc16bb7d1d7948823`, CC BY-SA 4.0. Cite: Keersmaekers, A. (2021), "The GLAUx corpus: methodological issues in designing a long-term, diverse, multi-layered corpus of Ancient Greek", *Proceedings of LChange 2021*, 39–50, doi:10.18653/v1/2021.lchange-1.6. Modified: forms and lemmas NFC-normalized; spans, UPOS, keys and statuses added.
- UD Ancient Greek-Perseus, `UniversalDependencies/UD_Ancient_Greek-Perseus` @ `331ddef91411d0e6549744ee889e05549e6da77d`, CC BY-NC-SA 2.5. Only aggregate counts are reproduced.

Derived manifests are offered under CC BY-SA 4.0 where they contain Perseus or GLAUx material, and under CC BY-NC-SA 2.5 for the two crosswalk tables. Mouseion's application code licence (AGPL-3.0) is not applied to these data.
