# French Stanza 1.15.0 Resource Rights Basis

**Research date:** 2026-10-17 (all sources accessed 2026-10-17 unless noted).
**Ticket:** [#2155 Obtain a documented French resource rights basis](https://github.com/justin-hayes/mouseion/issues/2155) (wayfinder).
**Predecessor note:** [`doc/research/french-resource-provenance.md`](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md) at `c90b50de` (read via `gh api`; treated as a claim set to verify, not as a source).

**Scope.** Only the Stanza 1.15.0 French default package (`fr`, `combined` processors) and its artifact-specific rights: the eight runtime artifacts, the conll17 pretrain matrix, and the forward/backward `newswiki` character models. Wiktionary/Kaikki, NER (`ner`), and coreference (`coref`) are out of scope.

**Intended scope.** Personal self-hosted copying and execution, and automated operator provisioning of those copies. Not redistribution, not commercial hosting.

**Not an adoption decision.** This note records evidence and how the grants read on that scope. It does not accept any use, does not speak for the project owner or counsel, and is not legal advice. Labels: **[V]** verified in this pass from the primary source; **[M]** registry/API metadata; **[I]** inference from the sources; **[L]** legal interpretation for counsel or human review; **[G]** gap.

No model, vector, or corpus bytes were downloaded. Scratch work lived under `~/.cache/scratch/french-stanza-rights/` and was deleted at the end.

---

## 1. Summary

### Resolved or narrowed by this pass

1. **Stanza code and tag.** Tag `v1.15.0` → commit `c9a2158a87502dd63f757ab1aa933230b0acbce7` [V, R1]. Code is Apache-2.0 [V, R9].
2. **stanza-resources has no `v1.15.0` tag.** Tags list is empty and the ref returns 404 [V, R2]. The manifest is reached only by commit pin.
3. **Manifest pin.** `resources_1.15.0.json` at `569db9d983a47931673d7d265361ca93af85d176` has sha256 `ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463` and md5 `462ea52f652d309152a102ca15ffd942` [V, R3]. These match the predecessor note. The `fr` block in that revision is the current one; the French `default_md5` and `pos/combined_charlm` md5 changed in the 2026-09-29 rebuild commit `e8941fa8`, and the `conll17` and `newswiki` md5 values did not change across the nine commits that touch the file [V, R3].
4. **HF Apache-2.0 card is a template, not a per-artifact determination.** The `license: apache-2.0` line is a fixed string in the `stanfordnlp/huggingface-models` script `hugging_stanza.py`, applied to every language's model card, and the script contains no per-language or per-processor license input [V, R6]. The `it`, `de`, `el`, and `en` cards carry the same value [V, R7].
5. **The project also publishes an ODC-By 1.0 statement, qualified "to the extent" Stanford holds rights.** The currently served performance page says so [V, R8], consistently with historical maintainer explanations [R10, R11]. The HF card remains affirmative publisher evidence for Apache-2.0, despite being generated. No explicit reconciliation or file-specific exception was found. Both license texts supply private-copying routes within their respective scope; uncertainty over which applies is not itself evidence that private copying is prohibited.
6. **conll17 has a direct first-party license statement.** The LINDAT deposit that Stanza's `download_vectors.sh` fetches states that CC BY-NC-SA 4.0 "applies to the automatically generated annotations and word embeddings, not to the underlying data" [V, R18]. So the conll17 matrix's license does not depend solely on inheritance from processor weights.
7. **Treebank licenses confirmed.** GSD, ParisStories, and Rhapsodie: CC BY-SA 4.0. Sequoia: LGPL-LR [V, R20–R22].
8. **Grant scope under CC licenses, read from the legal codes.** CC BY-NC-SA §2(a)(1)(a) grants a right to "reproduce and Share the Licensed Material ... for NonCommercial purposes only" [V, R23]. Where that license covers the input, it supplies an affirmative private, NonCommercial reproduction/adaptation route. CC §3 attribution and ShareAlike duties have public-Sharing triggers [R23, R24]; the NC purpose limit does not. The predecessor already recognized this route. Neither private use nor a missing file-specific notice alone establishes or defeats coverage.

### Still unresolved (precise list in §6)

- **Which license governs the trained `.pt` weights.** Apache-2.0 (HF field) vs ODC-By "to the extent" (project statements). No source reconciles them. The maintainers disclaim legal positions [V, R10].
- **`newswiki` character models.** The corpus and its terms remain unidentified. A maintainer explicitly encourages reuse of the French model [R12], and publisher-level Apache/qualified ODC statements exist. Their artifact-specific coverage and any separate third-party rights are not reconciled; this is not an absence of all grant evidence.
- **conll17 conversion lineage.** Which deposit file, which `convert_pretrain.py` options (in particular `max_vocab`), and what checksum the hosted `pretrain/conll17.pt` came from [G].
- **Per-processor training revisions and notices.** The combined-models page gives treebank names but no revisions or per-processor notices [V, R14] [G].
- **Third-party underlying text.** GSD expressly distinguishes annotations from underlying sentences [R20]. Sequoia lists several external sources [R22]. Do not assume an annotation license supplies every underlying-text right, or that those rights necessarily attach to inference weights. Neither question is settled by these source notices alone.
- **LGPL-LR reach over a trained model** (Sequoia-derived weights). This is a legal question [L].

---

## 2. Verified pins and artifact mapping

| Item | Verified value | Source | Label |
|---|---|---|---|
| Stanza `v1.15.0` | commit `c9a2158a87502dd63f757ab1aa933230b0acbce7` (2026-10-01T05:38:55Z) | R1 | V |
| Stanza tag object | Lightweight; `git/tags/c9a2158…` returns 404 | R1 | V |
| `__resources_version__` | `'1.15.0'` | [stanza `_version.py` at c9a2158](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/_version.py) | V-prior (predecessor S7) |
| stanza-resources tags | `[]`; `git/ref/tags/v1.15.0` → 404 | R2 | V |
| stanza-resources manifest | `569db9d983a47931673d7d265361ca93af85d176`; sha256 `ae9741f2…4463`; md5 `462ea52f…ffd942` | R3 | V |
| HF tag resolution (`resolve/v1.15.0/`) | `X-Repo-Commit: caeb7742148cb0368e4a0f2c68a0f7b09bc22880` | R4 | V |
| HF refs API (`/api/models/…/refs`) | tag `v1.15.0` → `8fe630b3494cb98b6eb4374ede125b241d74cd94`; `main` → `caeb7742…` | R4 | M |
| HF `8fe630b3` revision | `resolve/8fe630b3…/README.md` returns the same bytes as caeb (sha256 `96d1a3b77a6de20d484e7b139d07e53ce291a5ecab3a083a2154c16907ac60c7`); `conll17.pt`, `combined_charlm.pt`, `newswiki` LFS etags identical to caeb | R4 | V |
| HF tag mutability | `hugging_stanza.py` deletes an existing `v{version}` tag and recreates it on each push | R6 | V |

**Reading of the HF tag discrepancy [I].** The `resolve/v1.15.0` URL, which is what Stanza's manifest uses, resolves to `caeb7742`. The refs API still lists the tag at `8fe630b3`. Both commits carry identical bytes for the files checked. The tag is recreated by design, so `v1.15.0` is not a stable identifier. The predecessor note's claim that the tag "resolves to `caeb7742`" is confirmed for the `resolve` URL and contradicted by the refs listing, which the note did not mention.

**Stanza default for French.** `specific_default_pretrains["fr"] = "conll17"` [V, R15, line 182]. `default_charlms["fr"] = "newswiki"` [V, R15, line 338].

### Manifest `fr` default processors (verified against R3 and R4)

| Role | Processor | Manifest md5 (R3) | HF LFS sha256 at `caeb7742…` (R4) | Size (bytes) | Declared dependencies (R3) |
|---|---|---|---|---|---|
| tokenize | `combined_nocharlm` | `34ce2e674142753bb26d2bd66adc7742` | `622a7613d78623cda63e53fc72f84c49b2f64ad959c0f6aeb51355b963eda78f` | 665712 | none |
| mwt | `combined` | `827fd37941d9bb51bdd7c473e0e80cf8` | `a46bc4149531762c2fd79d70eeaaff80479aa6852a08516ab0db567859c8cec0` | 544773 | none |
| lemma | `combined_nocharlm` | `07f18945c29490c60ff3a9dfa18fbdc5` | `31dd152c13d1d713085127e7b30d91e70e88652da4c2ea33c4b6f7d34d810177` | 2813969 | none |
| pos | `combined_charlm` | `505cd655d0474c51d1e35312d49bceae` | `e40eb06786fa691e84aa6ee1ff2321b37f9ff9cb9bf3fe4de94b63793b626f89` | 34563706 | conll17; newswiki fwd+bwd |
| depparse | `combined_charlm` | `c0c8d747238802ee03c40953a8e4d101` | `5ad85cd10e385894152154863c14f621b7d747c67c84eb8597039da12832315a` | 148809357 | conll17; newswiki fwd+bwd |
| pretrain | `conll17` | `f74659129e2008c6b7b6ca0a2db693d7` | `c8c456c1a48fba32d4ab827ae7d3fec40457f58ead7e1977b6267afc37474a3c` | 106908071 | none |
| charlm fwd | `newswiki` | `d98e6909619100c23b745cae4383eea2` | `a6874f7670f84c8bd9e57d5dcbafdfe7d7cbae2457c79975040709f1d69ca119` | 20094072 | none |
| charlm bwd | `newswiki` | `42eba035046673e195274f45e713653c` | `3ae24cade5cdca7d68193196a942a177af81fbc5d3b5ebb4550ff7b89023b09c` | 20094073 | none |

The eight rows are the runtime set. The md5 values are manifest values, not computed from bytes (G4). The sha256 values are HF LFS metadata, not computed from bytes (G4). The predecessor note's table matches these values.

Other facts from the same manifest [V, R3]:

- **`ner` `wikinergold_charlm`** is also declared to depend on conll17 + newswiki. It is out of scope, but it shares the same two inputs.
- **`fasttextwiki` pretrain** is listed in the manifest and present in the HF tree (`models/pretrain/fasttextwiki.pt`) [V, R3, R4]. No `fr` default processor declares it as a dependency [V, R3], so it is outside this note's 8-artifact set.

---

## 3. Reconciling the HF card, the performance page, and the maintainers

### 3.1 Sources and what each says

| Source | Date / currency | Statement about model license | Basis |
|---|---|---|---|
| HF `stanfordnlp/stanza-fr` card (`caeb7742`) | "Last updated 2026-09-29" | `license: apache-2.0` | Fixed template string [R5, R6] |
| Stanza software license (`stanfordnlp.github.io/stanza/`) | Current | "Stanza is licensed under the Apache License, Version 2.0 … you may not use the **software package** except in compliance…" [R9] | Scoped to the software package |
| Stanza performance page | Current (HTTP Last-Modified 2026-09-27); its *table* is "as of version 1.5.1 on UD 2.12" | "License information for models built from the UD data is unclear, but users are encouraged to click on the git links below … To the extent that The Trustees of Leland Stanford Junior University have ownership and rights over these language packs, all these Stanza language packs are made available under the Open Data Commons Attribution License v1.0." [R8] | Licensing paragraph is not tied to the table's version |
| Maintainer (Manning), issue #3, 2019-05-29 | Historical; no later statement found in that thread | "to the extent that we have ownership/rights over a language packs, it is made available under the Open Data Commons Attribution License v1.0." Also: "everything written above is my own best understanding, and is not an official legal position of Stanford University." [R10] | Personal view, labelled as such |
| Maintainer (AngledLuffa), issue #956, 2022-02-18 | Historical | "In terms of the models, they are licensed under ODC. … (We are not able to give legal advice regarding our license.)" [R11] | About i2b2 NER context, but stated about "the models" |
| Issue #468 (2020) | Closed 2020-10-26 with no maintainer reply | Asks about commercial use of models; the single comment points elsewhere [R13] | Unanswered |

### 3.2 Reading [I]

- The **HF Apache-2.0 value is metadata written by a generic template.** It is applied uniformly to every language and was not chosen per artifact [R6, R7]. It is weak evidence about the weights' license.
- The **project's documented and maintainer-stated position for the model files is ODC-By 1.0 "to the extent" of Stanford's rights** [R8, R10, R11]. That statement is current on the performance page, and the maintainer comments are consistent with it.
- **The HF card is a publisher's model-repository declaration.** Its automated origin explains why it lacks per-input detail; it does not establish that the declaration is invalid or applies only to software. No separate release-specific statement reconciling it with ODC-By was found.
- This narrows the predecessor's conflict to two evidenced publisher statements with different scope, not a choice between a grant and no grant. Maintainers can clarify their intended licensing and artifact provenance; they need not provide binding legal advice to supply useful authoritative evidence. Their historical comments are not release-specific guarantees.

### 3.3 What ODC-By 1.0 says, for the personal-use reading

Read from the ODC-By 1.0 text [V, R25]:

- **§3.1 grant.** "the Licensor grants to You a worldwide, royalty-free, non-exclusive, terminable (but only under Section 9) license to Use the Database … These rights explicitly include commercial use." "Use" includes "copying" and "preparing derivative works" [R25 definitions].
- **§4.2 notices** apply when "You Publicly Convey this Database" [R25].
- **§4.3** says "Creating and Using a Produced Work does not require the notice in Section 4.2. However, if you Publicly Use a Produced Work, You must include a notice…" [R25].
- **"Publicly"** is defined as "to Persons other than You or under Your control" [R25].

**[I]** For a single operator running the model for themselves, the §4.2 and §4.3 notice duties do not seem to be triggered, because no conveyance occurs and no Publicly-facing use occurs. **[L]** Whether a model file is a "Database" or contains a "Produced Work" under ODC-By is not analyzed by the sources and is for counsel. Under the "to the extent" qualifier, the grant reaches only Stanford's own rights; it does not reach third-party inputs.

---

## 4. Artifact-specific license status

Each row is the license the source states for that artifact, with its effect on the intended scope. Grant wording is quoted from the legal text.

### 4.1 Stanza code and its tooling
- **Apache-2.0** [V, R9; full text V-prior from predecessor S25]. Private use has no notice condition. Redistribution is not in scope.

### 4.2 Treebank-trained processors: tokenize, mwt, lemma, pos, depparse

**Inputs (combined models):** GSD, ParisStories, Rhapsodie, Sequoia [V, R14]. ParTUT not included [V, R14].

| Input | License (primary) | Grant for personal reproduction/execution | Residual |
|---|---|---|---|
| GSD | CC BY-SA 4.0 (`LICENSE.txt` at `94d5b68e`) [V, R20] | CC BY-SA §2(a)(1)(a): "reproduce and Share the Licensed Material, in whole or in part" [V, R24]; no NC limit | GSD README: underlying sentences "may be copyrighted"; Google "asserts no ownership … and no copyright"; the annotation license does not grant rights in those sentences [V, R20] |
| ParisStories | CC BY-SA 4.0 [V, R21] | same | third-party text not analyzed |
| Rhapsodie | CC BY-SA 4.0 [V, R21] | same | third-party text not analyzed |
| Sequoia | LGPL-LR [V, R22] | §0 "The act of running a program using the Linguistic Resource is not restricted"; §1 verbatim copies permitted with notices [V, R22] | Whether weights are a "work based on" Sequoia, and whether §2 conditions reach private copies [L] |

**Notes**

- **GSD internal inconsistency [V, R20].** The README's license section says the annotations are CC BY-SA 4.0, then says "See attached LICENSE file for the text of CC BY-NC-SA". The attached `LICENSE.txt` is CC BY-SA 4.0 [V]. The `LICENSE.txt` controls for this note [I].
- **Sequoia sources [V, R22].** The README lists French Europarl, Wikipédia Fr, the newspaper *Est Républicain*, and the European Medicines Agency. These source names alone do not establish all underlying-text rights or their legal effect on weights.
- **CC BY-SA reach to weights [L].** The sources do not settle whether weights are Adapted Material. Where the CC grant applies to both reproduction and adaptation, choosing between those two classifications does not create a missing private-copying permission; public Sharing introduces additional conditions [R24]. This does not answer separate rights outside that grant.

### 4.3 conll17 pretrain (`pretrain/conll17.pt`)

**Chain (verified) [V]:**

1. Stanza's `download_vectors.sh` fetches `word-embeddings-conll17.tar` from LINDAT handle `11234/1-1989` [V, R16].
2. The tar contains `fr.vectors.xz` (781 MB) in the French entry [V, R18]. The item lists its MD5 as `8cd278935969de9a9a4f7839b0d6a5e7` (tar) [V, R18].
3. Stanza's `convert_pretrain.py` converts a text vectors file to `.pt`, with an optional `max_vocab` [V, R17]. The script's own docs say "You probably want to keep fewer than that for most publicly released embeddings" [V, R17].
4. The hosted `pretrain/conll17.pt` is 106,908,071 bytes, sha256 `c8c456c1…474a3c` [M, R4], md5 `f7465912…d693d7` [manifest, R3].

**License statements [V]:**

- LINDAT item: "This item is Publicly Available and licensed under: Creative Commons - Attribution-NonCommercial-ShareAlike 4.0 International (CC BY-NC-SA 4.0)" [R18]. Metadata: `dataset_license` = `http://creativecommons.org/licenses/by-nc-sa/4.0/` [R18].
- Item description: "Note that the CC BY-SA-NC 4.0 license applies to the automatically generated annotations and word embeddings, not to the underlying data, which may have different license and impose additional restrictions." [R18]

**Underlying data [V].** The CoNLL 2017 task page says the raw data "were taken from CommonCrawl and Wikipedia and automatically sorted by a language recognizer" [R19]. The French deposit contains `fr-wikipedia-*.conllu.xz` and `fr-common_crawl-*.conllu.xz` file names [R18]. The task page says the task "will only utilize resources … under a license that is free at least for non-commercial usage (e.g., CC BY-SA or CC BY-NC-SA)" [R19].

**Scope effect [I, L].**

- **Direct source license.** The deposit expressly licenses its embeddings under CC BY-NC-SA 4.0. Stanza's download script and serialization tooling support the inference that its `conll17` pretrain repackages those vectors. The exact hosted conversion remains unverified. On that documented lineage reading, the source NC grant covers the matrix itself, independently of any argument about inherited terms on processor weights. This is not byte-verified conversion evidence.
- **Grant for personal use.** Under CC BY-NC-SA §2(a)(1)(a)–(b), the licensee may "reproduce and Share the Licensed Material … for NonCommercial purposes only" and "produce, reproduce, and Share Adapted Material for NonCommercial purposes only" [V, R23]. Personal, non-commercial reproduction and execution is within those rights. The NonCommercial definition is "not primarily intended for or directed towards commercial advantage or monetary compensation" [V, R23 §1(k)].
- **Limit.** The grant holds only for NonCommercial purposes. The intended scope excludes commercial hosting. Whether a particular operator's use is NonCommercial is a fact question for that operator [L].
- **Underlying text.** Wikipedia and Common Crawl text is third-party material. The deposit disclaims rights in it [R18]. This is an unresolved third-party question, not evidence that private use is prohibited.

**Conversion gap [G, G1].** The LINDAT deposit and the conversion tool are documented, but the exact conversion of the hosted `pretrain/conll17.pt` (source file checksum, `max_vocab`, command) is not published by Stanza [V, R16, R17]. This affects reproducibility and any future audit of truncation; it does not, on its own, change the license of the source embeddings.

### 4.4 `newswiki` forward and backward character models

**Mapping [V].** `default_charlms["fr"] = "newswiki"` [R15, line 338]. The same corpus-name key appears for `de`, `es`, `ru` [R15]. The charLM training code takes a corpus name from a directory layout (`{lang}/{corpus}/`) and does not define `newswiki` [V, R15, `make_lm_data.py`, `run_charlm.py`].

**Searches [V].** Code search for "newswiki" in `stanfordnlp/stanza` at `c9a2158` returns only `default_packages.py` [R15]. Issue search returns issue #658 (2021) [R12].

**Maintainer statement [V, R12].** "We trained our charlm on 1b or more tokens. You can reuse the existing charlm by downloading our FR models and looking in stanza_resources/fr/forward_charlm/newswiki.pt" (AngledLuffa, 2021-03-30).

**Status [G].** No examined source names the corpus or its terms. The maintainer gives scale and expressly suggests reuse, but not training provenance. The two files sit in the model repository carrying the Apache declaration, with the broader qualified ODC language-pack statement also relevant. Thus grant evidence exists; a reconciled artifact-specific basis and the effect of any third-party corpus rights remain unresolved. Public redistribution is not needed for this personal-use scope and is not a prerequisite to clear here.

### 4.5 Sequoia (LGPL-LR) — precise reading [L]

- §0: copying, distribution, and modification are covered; running a program is not restricted; output is covered only if it "constitutes a work based on the Linguistic Resource" [V, R22].
- §0 defines a "work based on" as "a work containing the Linguistic Resource or a portion of it, either verbatim or with modifications" [V, R22].
- §1: verbatim copies may be made and distributed with notices [V, R22].
- §2: modified works may be copied and distributed under §1 if conditions are met (the modified work "must itself be a linguistic resource", notices, and licensing "at no charge … under the terms of this License") [V, R22].
- §5: "You may not copy, modify, sublicense, link with, or distribute the Linguistic Resource except as expressly provided under this License" [V, R22].

**Open question [L]:** Does a shipped weight file contain Sequoia material, verbatim or modified, such that it is a "work based on" that resource? If so, map the relevant §§1–2 conditions to that artifact rather than asserting that only public distribution matters. §0's running-program exception does not itself settle copying the file. No binding learned-weight interpretation was obtained.

---

## 5. Use-by-use reading for the intended scope

This section is a reading of the grants and their conditions, not a clearance.

| Use | Applicable grant(s) | Conditions triggered by this use | Residual |
|---|---|---|---|
| Personal self-hosted **execution** of the 8 runtime artifacts | ODC-By §3.1 (for Stanford's rights, if the model files are covered); Apache-2.0 (code); CC BY-SA §2 (treebank inputs); CC BY-NC-SA §2 (conll17, NC purposes); LGPL-LR §0 (running not restricted) | None of the notice duties: ODC-By §4.2 needs Conveying and §4.3 needs Publicly Use; CC BY-SA attribution/ShareAlike need Sharing; CC BY-NC-SA attribution/ShareAlike need Sharing [I] | `newswiki` corpus rights unknown; third-party underlying text; LGPL-LR reach over weights [L] |
| Personal self-hosted **copying** to the operator's own host (incl. automated provisioning) | ODC-By, Apache and applicable CC reproduction/adaptation grants provide conditional routes. LGPL-LR-covered material has separate §§1–2 copying conditions (§4.5). | CC §3 public-Sharing duties do not arise solely from private copying; NC purpose still limits NC material. Preserve applicable LGPL-LR notices if it governs copied material. Pinning/checksums are engineering controls, not by themselves license conditions. | Artifact coverage, third-party rights and any LGPL-LR weight obligations remain the questions, not a categorical lack of private reproduction grants. |
| Automated operator provisioning from HF/stanza-resources | Same | Stanza's `request_file` verifies md5 per file [V, predecessor S2]; no notice is written [V, predecessor S32] | HF tag is mutable (§2); stanza-resources default branch is `main` [V, predecessor §1.1] |

**Reading [I].** Existing publisher and input-license texts provide conditional private-use routes. The predecessor already recognized the CC NonCommercial reproduction grant; its lack of overall acceptance was about coverage and third-party rights, not an express exclusion of private copying. This pass adds publisher history and the template explanation, but does not produce an authoritative reconciliation for all eight files. Remaining material uncertainties are:

1. **`newswiki` provenance and artifact-specific coverage** (publisher declarations and reuse encouragement exist, corpus terms remain unknown).
2. **Third-party underlying text** in GSD, Sequoia, the conll17 corpora, and the unknown `newswiki` corpus.
3. **LGPL-LR reach** over weights trained on Sequoia.
4. **Which Stanza license governs the weights** (Apache-2.0 metadata vs ODC-By "to the extent").

None of these is a public source saying private use is prohibited. The predecessor's "absence of notice ≠ permission" framing is correct as a general point, but for the CC licenses above the grant is in the text.

---

## 6. Remaining precise questions

**For the Stanza maintainers** (evidence requests; not legal opinions):

1. Which license governs `models/{pos,depparse,tokenize,mwt,lemma}/*.pt` for 1.15.0? Is the HF `license: apache-2.0` field intended to override the ODC-By statement on the performance page?
2. What corpus was used for `forward_charlm/newswiki.pt` and `backward_charlm/newswiki.pt`? Which publisher grant and retained third-party notices apply to personal copying/execution of these files? Public redistributability is not required for this scope.
3. For `pretrain/conll17.pt`: which deposit file (`fr.vectors.xz` from `word-embeddings-conll17.tar`) was converted, with which `convert_pretrain.py` options (in particular `max_vocab`), and what is the source checksum?
4. Per-processor training revisions and treebank snapshots for 1.15.0 (the combined-models page does not provide them) [G].
5. Which HF commit is canonical for `v1.15.0`? The refs API lists `8fe630b3`, `resolve/v1.15.0` returns `caeb7742`, and the tag is deleted and recreated on each push [R4, R6].

**For counsel or human review** (interpretation):

6. Is a Sequoia-trained weight file a "work based on" the LGPL-LR resource, and do §2 conditions apply to private copies? [L]
7. Under CC BY-NC-SA, is the operator's self-hosted, free use "NonCommercial" in each deployment? [L]
8. For the conll17 matrix, both reproduction and adaptation are within the source grant for NonCommercial purposes. The missing work is establishing artifact lineage/coverage, not obtaining a separate permission merely because conversion may be an adaptation.
9. Do the grants (ODC-By "to the extent"; CC BY-SA/NC-SA for their inputs) cover third-party text inside the inputs? If not, what is the reasonable basis for private use? [L]

**For the project's engineering pins** (not a license condition):

10. Pin the stanza-resources commit `569db9d983a47931673d7d265361ca93af85d176` and manifest sha256 `ae9741f2…4463`, rather than `main`. Pin the HF commit explicitly (`caeb7742…` as resolved), not the tag. Record both alongside the artifact md5 from the manifest.

---

## 7. Gaps

| # | Gap | Blocks |
|---|---|---|
| G1 | conll17 conversion lineage (file, `max_vocab`, command, checksum) | Reproducibility; not the source license |
| G2 | `newswiki` corpus identity/terms and reconciled coverage of the two hosted files | Complete artifact-specific basis; not evidence of an express private-use prohibition |
| G3 | Which license governs model files (HF Apache-2.0 vs ODC-By "to the extent") | Documented basis; does not change the personal-use reading above |
| G4 | No artifact bytes downloaded; md5 values are manifest values, sha256 values are HF LFS metadata | Byte-level verification |
| G5 | HF `v1.15.0` tag points to `8fe630b3` in the refs API but to `caeb7742` on resolve | Pin choice |
| G6 | Per-processor training revisions and notices | Per-artifact attribution |
| G7 | LGPL-LR reach over weights | Sequoia-derived weights; redistribution |
| G8 | Third-party underlying text (GSD, Sequoia, conll17 corpora, newswiki corpus) | Any broad claim about input rights |
| G9 | Kaikki/Wiktionary items | Addressed by the companion note, not this Stanza investigation |

---

## 8. Sources

Accessed 2026-10-17 unless noted. Where a source was not re-fetched in this pass, it is marked *predecessor*.

| ID | Source | Pinned revision / identifier | Used for |
|---|---|---|---|
| R1 | `api.github.com/repos/stanfordnlp/stanza/commits/v1.15.0`; `git/tags/c9a2158…` (404) | commit `c9a2158a87502dd63f757ab1aa933230b0acbce7` | Tag → commit |
| R2 | `api.github.com/repos/stanfordnlp/stanza-resources/tags` (`[]`); `git/ref/tags/v1.15.0` (404) | — | No stanza-resources tag |
| R3 | `raw.githubusercontent.com/stanfordnlp/stanza-resources/569db9d983a47931673d7d265361ca93af85d176/resources_1.15.0.json`; commit history for `resources_1.15.0.json` | `569db9d…` | Manifest; `fr` block; history |
| R4 | `huggingface.co/stanfordnlp/stanza-fr/resolve/{v1.15.0,8fe630b3…,main}/…` (`X-Repo-Commit`, `X-Linked-ETag`); `/api/models/stanfordnlp/stanza-fr/refs`; `/api/models/…/tree/caeb7742…?recursive=true` | `caeb7742…`, `8fe630b3…` | HF revision, LFS oids, sizes |
| R5 | [Pinned French model card](https://huggingface.co/stanfordnlp/stanza-fr/blob/caeb7742148cb0368e4a0f2c68a0f7b09bc22880/README.md) | `caeb7742…` | Card: `license: apache-2.0`, "Last updated 2026-09-29" |
| R6 | [Pinned model-card/upload generator](https://github.com/stanfordnlp/huggingface-models/blob/f50a9e0441b7cd6a708b651e2bf274175f147df5/hugging_stanza.py) (sha256 `4a61cf9b37bc28f2c6dcabc509cb07a99c3a73d16412ee685cd24e6cbc133216`) | commit `f50a9e0441` | Template license; tag delete/recreate; coordinating review also read this file |
| R7 | `huggingface.co/api/models/stanfordnlp/stanza-{it,de,el,en}` | `cardData.license` | Template applied to all languages |
| R8 | [Stanza model performance and licensing statement](https://stanfordnlp.github.io/stanza/performance.html) (reported Last-Modified 2026-09-27) | Currently served page, not release-specific | ODC-By paragraph; table "as of v1.5.1 on UD 2.12"; coordinating review also read the statement |
| R9 | `stanfordnlp.github.io/stanza/index.html` ("License" section) | — | Apache-2.0 scoped to "software package" |
| R10 | [Project director's model-license explanation](https://github.com/stanfordnlp/stanza/issues/3#issuecomment-497058182), 2019-05-29 (manning) | Historical; comment API independently checked | Qualified ODC position; non-expressive-model reasoning; "not an official legal position" |
| R11 | [Maintainer's model/data-license distinction](https://github.com/stanfordnlp/stanza/issues/956#issuecomment-1044169896), 2022-02-18 (AngledLuffa) | Historical; comment API independently checked | "the models are licensed under ODC"; clinical/NER context |
| R12 | [Questions about character LM — French reuse explanation](https://github.com/stanfordnlp/stanza/issues/658), comment 2021-03-30 (AngledLuffa) | Historical | "1b or more tokens"; French charlm reuse |
| R13 | `github.com/stanfordnlp/stanza/issues/468` (closed 2020-10-26; one comment) | — | Unanswered commercial-use question |
| R14 | [Stanza combined-model documentation](https://stanfordnlp.github.io/stanza/combined_models.html) | Not a release-specific training manifest | French combined treebanks |
| R15 | `stanfordnlp/stanza` at `c9a2158`: `stanza/resources/default_packages.py` (lines 43, 182, 338); `stanza/utils/charlm/make_lm_data.py`; `stanza/utils/training/run_charlm.py`; `stanza/models/charlm.py` | `c9a2158` | Default package mapping; charLM corpus layout |
| R16 | `stanfordnlp/stanza` at `c9a2158`: `scripts/download_vectors.sh` (lines 12–13, 78–82) | `c9a2158` | conll17 source URL and extraction |
| R17 | `stanfordnlp/stanza` at `c9a2158`: `stanza/models/common/convert_pretrain.py`; `stanza/models/common/pretrain.py` | `c9a2158` | `.pt` conversion; `max_vocab` |
| R18 | [LINDAT CoNLL17 deposit](https://lindat.mff.cuni.cz/repository/items/053071c7-84f7-4d0b-bdd7-f87cbafcd3a9) (handle `11234/1-1989`); `dataset_license` meta | item page | CC BY-NC-SA 4.0; embeddings-not-data note; tar MD5; French file list |
| R19 | [CoNLL17 data documentation](https://universaldependencies.org/conll17/data.html) | — | Raw-data sources (CommonCrawl, Wikipedia); license-type statement |
| R20 | `UniversalDependencies/UD_French-GSD` at `94d5b68e185fc22a9ef292040e84f476d36d9b0e`: `LICENSE.txt`, `README.md` | `94d5b68e…` | CC BY-SA 4.0; sentence-text disclaimer; README inconsistency |
| R21 | `UD_French-ParisStories` at `dec76f7a1731318b033c578d534410b0a3d4ea5c`; `UD_French-Rhapsodie` at `6979888f619d58edbb8a2de5352c5297b0eff902`: `LICENSE.txt`, `README.md` | pinned | CC BY-SA 4.0 |
| R22 | `UD_French-Sequoia` at `21e377f98e07109500618ea83f1deccd77670d8f`: `LICENSE.txt` (LGPL-LR), `README.md` (sources) | `21e377f9…` | LGPL-LR §§0–7; source list |
| R23 | [CC BY-NC-SA 4.0 legal text](https://creativecommons.org/licenses/by-nc-sa/4.0/legalcode.txt) | — | §1(k) NonCommercial, §1(l) Share, §2(a)(1) |
| R24 | [CC BY-SA 4.0 legal text](https://creativecommons.org/licenses/by-sa/4.0/legalcode.txt) | — | §1(k) Share, §2(a)(1), §3(b) ShareAlike |
| R25 | [ODC-By 1.0 legal text](https://opendatacommons.org/licenses/by/1-0/) | — | §3.1 grant; §4.2; §4.3; definitions "Publicly", "Use", "Convey" |

*Predecessor-only (not re-fetched in this pass):* Apache-2.0 full text (predecessor S25); `stanza/resources/common.py` download behavior (predecessor S2); `provision.py` (predecessor S32).

---

## 9. Verification notes for the predecessor claims

| Predecessor claim | This pass |
|---|---|
| Stanza tag `v1.15.0` → `c9a2158…` | Confirmed (R1) |
| stanza-resources has no `v1.15.0` tag | Confirmed (R2) |
| Manifest at `569db9d…` has sha256 `ae9741f2…` and md5 `462ea52f…` | Confirmed (R3) |
| HF tag `v1.15.0` → `caeb7742…` | Confirmed for `resolve`; **contradicted** by the refs API (`8fe630b3…`) (R4) |
| HF card `license: apache-2.0` | Confirmed (R5); found to be a fixed template string (R6) |
| Performance page: "unclear", ODC-By "to the extent" | Confirmed current (R8) |
| conll17 is the LINDAT deposit; CC BY-NC-SA 4.0 | Confirmed (R18); deposit's own statement covers embeddings |
| `newswiki` corpus not identified | Still not identified; maintainer gives "1b or more tokens" (R12) |
| GSD, ParisStories, Rhapsodie: CC BY-SA 4.0; Sequoia: LGPL-LR | Confirmed (R20–R22) |
| "CC BY-NC-SA §2 … a private copy is not automatically outside that grant" | **Refined**: the reproduce right is limited to NonCommercial purposes, and private non-commercial copying is within it (R23). Remaining question is NonCommercial application, not whether private copying is granted |
| Kaikki / Wiktionary items | Not in scope; not re-checked |
