# Third-party data register

This register lists every third-party data layer Mouseion uses today, under the
posture of [ADR 0111](adr/0111-third-party-licence-posture.md). Each layer is an
**Analysis resource** or **Source content** ([`CONTEXT.md`](../CONTEXT.md)). One
artifact can hold both, so it gets one row per layer.

This is a product record, not legal advice.

## What a deployer should know

- **The default warm set provisions NC-derived models.** Compose provisions
  `MOUSEION_NLP_WARM_LANGUAGES=de,it,el,fr` by default. Every one of those
  Stanza packages depends on the non-commercial (NC) conll17 word embeddings.
  The Italian and Modern Greek packages are also trained on NC treebanks.
- **The AGPL covers Mouseion's code, not these resources.** Mouseion is
  `AGPL-3.0-only`. The models, treebanks, dictionaries and texts below keep
  their own terms.
- **A commercial deployment is the deployer's responsibility.** NC, unclear and
  research-only Analysis resources are allowed only because the deployer fetches
  them at deploy time or supplies them, they never ship in the repository, an
  image or a release, and the deployment serves the deployer's personal,
  self-hosted, non-commercial purposes. Mouseion offers no configuration free of
  NC resources. A commercial deployer must replace them, starting from this
  register.
- **Exported decks** carry Analysis-resource output (the `Lemma` and `POS`
  fields) and Source content under the attribution notices named below. A deck
  analysed by the NLP service also carries a generic notice that its annotations
  come from deployer-provisioned models with their own terms. An imported
  Ancient Greek deck carries `AncientGreekSourceNotice` instead.
- **Adding a third-party layer adds a row here in the same change.** Classify it
  first. Source content must permit redistribution and be no more restrictive
  than share-alike. When it reaches decks, it also needs a deck notice.
  Reviewers check this register; no CI check enforces it.

## Columns

- **Kind**: Analysis resource or Source content.
- **Terms**: the licence instrument in force, quoted, with the revision it was
  read at. Model-card or README prose does not override a `LICENSE` file.
- **Restriction**: none, attribution, share-alike, NC, research-only or unclear.
  When a layer's only terms are research or academic use, it counts as unclear.
- **Delivery**: image, deploy time (`nlp-init` running
  `python -m mouseion_nlp.provision`), operator-built, or operator-supplied.
- **Reaches a deck**: whether the layer's expression, or only output derived
  from it, is copied into an exported deck.
- **Flag**: **NC** when an NC term applies to the layer. **Unverified** when its
  terms or lineage could not be confirmed from a primary source.

The terms come from the provenance research for
[#2307](https://github.com/justin-hayes/mouseion/issues/2307)
([findings at `b4568038`](https://github.com/justin-hayes/mouseion/blob/b4568038956f50069f225cfab3005e92cc553f8c/doc/research/third-party-licence-provenance.md)),
read on 2026-10-25. The findings record the sources, pins and SHA-256 digests.
Stanza is `stanza==1.15.0` (`nlp/uv.lock`). Its packages resolve through
`stanfordnlp/stanza-resources@569db9d` (`resources_1.15.0.json`).

## Analysis resources

| Layer | Kind | Terms | Restriction | Delivery | Reaches a deck | Flag |
|---|---|---|---|---|---|---|
| Stanza trained models for `de`, `it`, `el` and `fr` (HF `stanfordnlp/stanza-{de,it,el,fr}` at `v1.15.0`; `fr` at `caeb7742`) | Analysis resource | Each model card declares `apache-2.0`. Stanza's performance page (unpinned) says "License information for models built from the UD data is unclear" and offers the packs under the Open Data Commons Attribution License v1.0 to the extent Stanford holds rights. The training data rows below apply too. | unclear | Deploy time (`stanza-data` volume) | Output only (`Lemma`, `POS`), with the generic annotations notice | **NC** (through conll17 and the treebanks below) |
| conll17 word embeddings (`pretrain/conll17.pt` for `de`, `it`, `el`, `fr`) | Analysis resource | LINDAT `11234/1-1989`: "the CC BY-SA-NC 4.0 license applies to the automatically generated annotations and word embeddings, not to the underlying data, which may have different license and impose additional restrictions." The record's file is CC BY-NC-SA 4.0. | NC | Deploy time, inside the Stanza download | Output only | **NC**; underlying text **unverified** |
| Stanza `newswiki` character models (`de` `pos` and `depparse`; `fr`) | Analysis resource | No licence statement found; training corpus unidentified (`stanza@v1.15.0`, `default_packages.py`). | unclear | Deploy time | Output only | **Unverified** |
| Stanza German lemma data composed from German Wiktionary (`de` combined package) | Analysis resource | Stanza's combined-models page (unpinned) says it is "composed from German Wiktionary". No Stanza-side licence statement; Wiktionary text is CC BY-SA 4.0 / GFDL under the Wikimedia Terms of Use §7. | share-alike | Deploy time, inside the `de` package | Output only | **Unverified** |
| UD German-GSD (`UD_German-GSD@ce54dbe9`), trains the `de` package | Analysis resource | `LICENSE.txt`: CC BY-SA 4.0. README: "Google gave permission to drop the NC restriction … This applies to the UD annotations (not the underlying content …)"; "Portions of the German data were sampled from the CoNLL 2006 Tiger Treebank". | share-alike | Inside the `de` package weights | Output only | Tiger-sampled sentences **unverified** |
| UD Italian-ISDT (`UD_Italian-ISDT@ff2447f6`), trains the `it` package | Analysis resource | `LICENSE.txt`: "released … under the Creative Commons Attribution-NonCommercial-ShareAlike 3.0 Unported licence". The README's "research purposes" wording does not override it. | NC | Inside the `it` package weights | Output only | **NC** |
| UD Italian-VIT (`UD_Italian-VIT@12fc5f68`), trains the `it` package | Analysis resource | `LICENSE.txt`: CC BY-NC-SA 3.0. | NC | Inside the `it` package weights | Output only | **NC** |
| UD Italian-PoSTWITA (`UD_Italian-PoSTWITA@b8237631`), trains the `it` package | Analysis resource | `LICENSE.txt`: CC BY-NC-SA 4.0. | NC | Inside the `it` package weights | Output only | **NC** |
| UD Italian-TWITTIRO (`UD_Italian-TWITTIRO@ff4c6073`), trains the `it` package | Analysis resource | `LICENSE.txt`: CC BY-SA 4.0. | share-alike | Inside the `it` package weights | Output only | — |
| UD Greek-GDT (`UD_Greek-GDT@005bb3e0`), trains the `el` `gdt` package | Analysis resource | `LICENSE.txt`: "distributed under a CreativeCommons Attribution-NonCommercial-ShareAlike 3.0 license". | NC | Inside the `el` package weights | Output only | **NC** |
| French treebanks GSD, ParisStories, Rhapsodie and Sequoia, train the `fr` combined package | Analysis resource | GSD, ParisStories and Rhapsodie: CC BY-SA 4.0. Sequoia: LGPL-LR. Pins are in the French provenance note at [`c90b50de`](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md). | share-alike | Inside the `fr` package weights | Output only | — |
| GreekBERT (`nlpaueb/bert-base-greek-uncased-v1` at `ec2b8f88`), used by the `el` dependency parser | Analysis resource | The HF card has no licence field. `nlpaueb/greek-bert` `LICENSE.md` is MIT, "Copyright (c) 2020 NLP AUEB Group" (read on `master` at `2f0d84b6`, not a tag). | attribution | Deploy time (`huggingface-data` volume) | No: it shapes dependency parses, not card fields | Training corpora **unverified** |
| OGA `oga_src` v0.2.0 annotations (Zenodo `10.5281/zenodo.15199546`): lemmas, parts of speech, morphology and syntax | Analysis resource | Zenodo record: `cc-by-sa-4.0`. The GitHub repository has no `LICENSE` file. AGDT version and lineage not confirmed ([#2306](https://github.com/justin-hayes/mouseion/issues/2306)). | share-alike | Operator-supplied CoNLL-U file, size and SHA-256 pinned, imported in the app | Output only (`Lemma`, `POS`), with `AncientGreekSourceNotice` | AGDT lineage **unverified** |

## Source content

| Layer | Kind | Terms | Restriction | Delivery | Reaches a deck | Flag |
|---|---|---|---|---|---|---|
| Wiktionary entries, extracted by Wiktextract and published by Kaikki.org, for `de`, `it`, `el`, `fr` and `grc`: definitions, glosses and morphology (article, gender, plural, IPA, principal parts) | Source content | Wiktionary text: CC BY-SA 4.0 or GFDL under the Wikimedia Terms of Use §7. Wiktextract code: MIT (`05f0d933`). Kaikki publishes no resource-specific statement for the raw dump. The index metadata records the dump date, Wiktextract commit and source digest. | share-alike | Operator-built (`make dictionary-index`), mounted read-only | **Yes**, with `dictionary.AttributionNotice` | — |
| LSJ, Liddell-Scott-Jones *A Greek-English Lexicon* (`PerseusDL/lexica@56061ca1`) | Source content | README: "licensed under a Creative Commons Attribution-ShareAlike 4.0 International License". | share-alike | Operator-supplied directory (`DICTIONARY_LSJ_DIR`), compiled into the dictionary index | **Yes**, with `dictionary.AncientGreekLSJNotice` and the pin | — |
| OGA `oga_src` v0.2.0 Greek text, derived from Perseus `canonical-greekLit` | Source content | Zenodo record: `cc-by-sa-4.0`. `PerseusDL/canonical-greekLit` README: "licensed under a Creative Commons Attribution-ShareAlike 4.0 International License". | share-alike | Operator-supplied CoNLL-U file, size and SHA-256 pinned, imported in the app | **Yes**, with `dictionary.AncientGreekSourceNotice` | — |
| Xenophon, *Anabasis*, translated by Carleton L. Brownson (1921–22), Perseus `perseus-eng2` digitisation (`PerseusDL/canonical-greekLit@0189b9c8`, SHA-256 `f4c88203…6ea7`) | Source content | `PerseusDL/canonical-greekLit` README: "licensed under a Creative Commons Attribution-ShareAlike 4.0 International License". | share-alike | Operator-supplied file, digest-pinned, imported in the app | **Yes**, as passage context, with `dictionary.BrownsonSourceNotice` | Print copyright of the translation **unverified** |

No Source content layer in use is NC, research-only, unlicensed or unclear.

## Not covered here

- **LLM provider output**, such as Glosses and translations, falls under each
  provider's terms. ADR 0111 leaves this open.
- **Books a learner imports** are the learner's own material, not a layer
  Mouseion supplies.
- **Software dependencies** keep their own licences. These include Go modules,
  Python packages (Stanza's code is Apache-2.0) and the web assets bundled under
  [`internal/webapp/static/vendor/`](../internal/webapp/static/vendor/).
