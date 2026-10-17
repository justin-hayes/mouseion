# French Resource Provenance and Permitted Uses

**Research date:** 2026-10-17 (subagent investigation, reviewed and corrected the same day)

**Repository base:** `6e02d180b1015536127839e0a45953684c0e5a83` (`origin/main`,
the commit this branch was cut from).

**Tickets:**
[#2153 Confirm French resource provenance and permitted uses](https://github.com/justin-hayes/mouseion/issues/2153) (this research),
[#2138 Chart French study-language support and resource selection](https://github.com/justin-hayes/mouseion/issues/2138) (map),
[#2141 Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141) (adoption decision gate),
[#2139 Assess French NLP resources against the modern-language pipeline](https://github.com/justin-hayes/mouseion/issues/2139),
[#2140 Assess French dictionary and learner-facing form resources](https://github.com/justin-hayes/mouseion/issues/2140),
[#2152 Validate pinned French resources and pronominal identity](https://github.com/justin-hayes/mouseion/issues/2152).

**Working decision checkpoint:** [Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141#issuecomment-6234969922) (justin-hayes, 2026-10-17).

**Baseline research notes:**
[`doc/research/french-nlp-resources.md`](https://github.com/justin-hayes/mouseion/blob/c44a35c014ba6d8f3c090f1abc38b63299101d1e/doc/research/french-nlp-resources.md)
at `c44a35c0`;
[`doc/research/french-lexical-resources.md`](https://github.com/justin-hayes/mouseion/blob/332251d44a4ad8f91fa45d39f1a1b3e5a5961d45/doc/research/french-lexical-resources.md)
at `332251d4`.

**Scope:** two inputs from that provisional checkpoint:

1. The Stanza 1.15.0 French default package (`fr`, `combined` processors).
2. Raw English Wiktionary as extracted by Wiktextract and published by Kaikki.org.

Named-entity recognition (`ner`) and coreference (`coref`) are out of scope.

## Status

**BLOCKED. No intended use is accepted.** This note records evidence. It does not
grant permission to use, download, run, bundle, derive, export, or distribute either
resource. Final adoption needs a human decision in
[Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141)
that accepts a documented basis. “Not cleared” describes this investigation's
handoff status, not a finding that the resources are unlawful.

- **Private self-hosted execution is not cleared.** Self-hosting is not a license
  grant. Where a source says nothing, this note treats the question as unresolved,
  not as permitted. Ordinary private-use conditions are separated from the open
  grant question in §4.
- The Stanza code is Apache-2.0. The model artifacts' license is **not settled** by
  the sources, which conflict (§2.2).
- Wiktionary text is offered under CC BY-SA 4.0 or GFDL by the Wikimedia Terms of
  Use. Kaikki's own terms for the raw dump were not found (§3.2).

**Not done:** no model bundle or Kaikki dump was downloaded (the raw dump is about
2.98 GB). No checksum of an artifact's bytes was computed. Legal interpretations
are marked and need counsel or human review. Nothing here is legal advice.

## Evidence labels

| Label | Meaning |
|---|---|
| **[V]** | Read or fetched from the primary source in this pass (2026-10-17). |
| **[V-prior]** | Read in an earlier pass on this ticket; not re-fetched in this revision. |
| **[M]** | Registry or API metadata (GitHub or Hugging Face API, HTTP headers). Not verified against file bytes. |
| **[I]** | Inference: a reading the sources do not state directly. |
| **[L]** | Legal interpretation. Needs human or counsel review. |
| **[G]** | Gap: not found, not checked, or not resolvable from public sources. |

## Sources

Each `[S#]` tag in the text refers to this list. Pinned revisions are used
wherever a source has one.

| ID | Source | Pinned revision / access | Used for |
|---|---|---|---|
| S1 | Stanza tag `v1.15.0` → commit | [`git/ref/tags/v1.15.0`](https://api.github.com/repos/stanfordnlp/stanza/git/ref/tags/v1.15.0) → `c9a2158a87502dd63f757ab1aa933230b0acbce7` [V] | Source revision for code and files below |
| S2 | Stanza `stanza/resources/common.py` | [blob at `c9a2158`](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/resources/common.py) [V] | Download and checksum behavior |
| S3 | Stanza `stanza/resources/default_packages.py` | [blob at `c9a2158`](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/resources/default_packages.py) [V] | `newswiki` mapping for `fr` |
| S4 | Stanza `scripts/download_vectors.sh` | [blob at `c9a2158`](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/scripts/download_vectors.sh) [V] | conll17 vector source URL |
| S5 | Stanza `stanza/models/common/convert_pretrain.py` | [blob at `c9a2158`](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/models/common/convert_pretrain.py) [V] | What a `.pt` pretrain file is |
| S6 | Stanza `LICENSE` | [blob at `c9a2158`](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/LICENSE) (603 bytes: copyright header and Apache URL, not the full text) [V] | Code license |
| S7 | Stanza `stanza/_version.py` | [blob at `c9a2158`](https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/_version.py) (`__resources_version__ = '1.15.0'`) [V] | Resources version |
| S8 | stanza-resources manifest `resources_1.15.0.json` | [blob at `569db9d983a47931673d7d265361ca93af85d176`](https://github.com/stanfordnlp/stanza-resources/blob/569db9d983a47931673d7d265361ca93af85d176/resources_1.15.0.json) (committed 2026-10-01T02:51:50Z [M]) [V] | Artifact mapping, md5 values |
| S9 | stanza-resources tags | [`/tags`](https://api.github.com/repos/stanfordnlp/stanza-resources/tags) returned `[]` (HTTP 200); [`git/ref/tags/v1.15.0`](https://api.github.com/repos/stanfordnlp/stanza-resources/git/ref/tags/v1.15.0) returned 404 [V] | No `v1.15.0` tag in stanza-resources |
| S10 | Hugging Face `stanfordnlp/stanza-fr` tag `v1.15.0` | Resolves to commit `caeb7742148cb0368e4a0f2c68a0f7b09bc22880` [M] | Pinned model revision |
| S11 | Hugging Face tree at `caeb7742…` | [`api/models/stanfordnlp/stanza-fr/tree/caeb7742…`](https://huggingface.co/api/models/stanfordnlp/stanza-fr/tree/caeb7742148cb0368e4a0f2c68a0f7b09bc22880?recursive=true) [M] | LFS sha256 and sizes |
| S12 | Hugging Face model card at `caeb7742…` | [`README.md`](https://huggingface.co/stanfordnlp/stanza-fr/blob/caeb7742148cb0368e4a0f2c68a0f7b09bc22880/README.md) (679 bytes; `license: apache-2.0`; no dataset or training section) [V] | Model-card license |
| S13 | Stanza combined models page | [`combined_models.html`](https://stanfordnlp.github.io/stanza/combined_models.html) (HTTP 200) [V] | French training treebanks |
| S14 | Stanza performance page | [`performance.html`](https://stanfordnlp.github.io/stanza/performance.html) [V-prior] | "unclear" and ODC-By statements |
| S15 | UD_French-GSD | [`LICENSE.txt` and `README.md` at `94d5b68e185fc22a9ef292040e84f476d36d9b0e`](https://github.com/UniversalDependencies/UD_French-GSD/tree/94d5b68e185fc22a9ef292040e84f476d36d9b0e) [V] | License, source note |
| S16 | UD_French-ParisStories | [tree at `dec76f7a1731318b033c578d534410b0a3d4ea5c`](https://github.com/UniversalDependencies/UD_French-ParisStories/tree/dec76f7a1731318b033c578d534410b0a3d4ea5c) [V] | License |
| S17 | UD_French-Rhapsodie | [tree at `6979888f619d58edbb8a2de5352c5297b0eff902`](https://github.com/UniversalDependencies/UD_French-Rhapsodie/tree/6979888f619d58edbb8a2de5352c5297b0eff902) [V] | License |
| S18 | UD_French-Sequoia | [tree at `21e377f98e07109500618ea83f1deccd77670d8f`](https://github.com/UniversalDependencies/UD_French-Sequoia/tree/21e377f98e07109500618ea83f1deccd77670d8f) [V] | README metadata: `License: LGPL-LR`; `LICENSE.txt` is the LGPL-LR text |
| S19 | LINDAT item "CoNLL 2017 word embeddings automatic annotation" | [`items/053071c7-84f7-4d0b-bdd7-f87cbafcd3a9`](https://lindat.mff.cuni.cz/repository/items/053071c7-84f7-4d0b-bdd7-f87cbafcd3a9) (HTTP 200) [V] | CC BY-NC-SA 4.0 statement |
| S20 | LINDAT handle used by Stanza's script | [`11234/1-1989`](https://lindat.mff.cuni.cz/repository/xmlui/handle/11234/1-1989) [V] (URL in S4). Redirects to S19; the destination also gives this handle as `dataset_identifier`. | Source link |
| S21 | CC BY-NC-SA 4.0 legal code | [`legalcode.txt`](https://creativecommons.org/licenses/by-nc-sa/4.0/legalcode.txt) [V] | §1(k) NonCommercial, §1(l) Share, §4 database rights |
| S22 | CC BY-SA 4.0 legal code | [`legalcode`](https://creativecommons.org/licenses/by-sa/4.0/legalcode) [V-prior] | §1(k) Share, §3 conditions |
| S23 | ODC-By 1.0 | [`opendatacommons.org/licenses/by/1-0/`](https://opendatacommons.org/licenses/by/1-0/) [V] | §§4.2–4.3 notices; "Produced Work" and "Publicly" definitions |
| S24 | LGPL-LR text | [`UD_French-Sequoia/LICENSE.txt` at `21e377f9`](https://github.com/UniversalDependencies/UD_French-Sequoia/blob/21e377f98e07109500618ea83f1deccd77670d8f/LICENSE.txt) [V] | §§0–7 |
| S25 | Apache-2.0 full text | [`apache.org/licenses/LICENSE-2.0.txt`](https://www.apache.org/licenses/LICENSE-2.0.txt) (fetched; sha256 prefix `cfc7749b96f63bd3`) [V] | §4(a)–(d) |
| S26 | Wikimedia Terms of Use, §7 "Licensing of Content" and attribution | [`Policy:Terms_of_Use`](https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use) [V] (page says effective June 7, 2023) | Wiktionary grant chain |
| S27 | Wiktionary:Copyrights | [`en.wiktionary.org/wiki/Wiktionary:Copyrights`](https://en.wiktionary.org/wiki/Wiktionary:Copyrights) [V] for the "outdated" notice; other text [V-prior] | Secondary; outdated |
| S28 | Wiktionary:Quotations, §Copyright | [`en.wiktionary.org/wiki/Wiktionary:Quotations`](https://en.wiktionary.org/wiki/Wiktionary:Quotations) [V] | Quotation policy |
| S29 | Wiktextract `LICENSE` at `05f0d933cbadb94a20d63a7b2c5f865e45ed381b` | [blob](https://github.com/tatuylonen/wiktextract/blob/05f0d933cbadb94a20d63a7b2c5f865e45ed381b/LICENSE) [V] (MIT statement for code [V-prior]) | Extractor license |
| S30 | Kaikki pages | [French index](https://kaikki.org/dictionary/French/index.html), [raw data](https://kaikki.org/dictionary/rawdata.html), raw dump [`raw-wiktextract-data.jsonl.gz`](https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz) (HEAD [M]) | Kaikki terms, dump headers |
| S31 | Repo `nlp/scripts/derive_dictionary_index.py` at base `6e02d180` | [blob](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/nlp/scripts/derive_dictionary_index.py) [V] | Derivation, metadata |
| S32 | Repo `nlp/src/mouseion_nlp/provision.py` at base | [blob](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/nlp/src/mouseion_nlp/provision.py) [V] | Provisioning path |
| S33 | Repo `internal/dictionary/attribution.go`, `internal/cardexport/cardexport.go`, `internal/cardexport/presentation.go`, `cmd/server/main.go`, `internal/enrichment/llm_codec.go` at base | e.g. [`cardexport.go`](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/internal/cardexport/cardexport.go) [V] | Export and gloss paths |
| S34 | ADR 0079 (contextual glosses require LLM) | [`doc/adr/0079-contextual-glosses-require-llm.md`](https://github.com/justin-hayes/mouseion/blob/6e02d180b1015536127839e0a45953684c0e5a83/doc/adr/0079-contextual-glosses-require-llm.md) [V-prior] | Attribution placement |
| S35 | Raw release extractor revisions | [Wiktextract `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a`](https://github.com/tatuylonen/wiktextract/commit/1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a), [Wikitextprocessor `e3d6d4edb77618f4d6680edc66e3f774bea59820`](https://github.com/tatuylonen/wikitextprocessor/commit/e3d6d4edb77618f4d6680edc66e3f774bea59820) [V] | Resolve the two short identifiers reported by S30; the second is not a Wiktextract commit |

## Bottom line

- **Stanza `fr` weights: license not settled.** The code is Apache-2.0 [S6]. The
  model card says `apache-2.0` with no training-data notice [S12]. Stanza's
  performance page says the UD-derived model license is "unclear" and that, "to the
  extent" Stanford holds rights, the packs are ODC-By 1.0 [S14, V-prior]. These
  conflict. This note does not decide which governs the `.pt` files.
- **Copyleft and non-commercial source notices need tracing to the French defaults.** The `combined`
  `pos` and `depparse` models depend on the conll17 pretrained vectors [S3, S8] and
  on `newswiki` character models whose training corpus is not identified [S3, G].
  The conll17 vectors trace to a CC BY-NC-SA 4.0 LINDAT item [S19], which says that
  license covers the embeddings, not the underlying data. The French treebanks are
  CC BY-SA 4.0 (GSD, ParisStories, Rhapsodie) and LGPL-LR (Sequoia) [S15–S18].
- **Wiktionary text: grant chain identified, Kaikki terms not found.** The Wikimedia
  Terms of Use §7 requires contributors to license text under CC BY-SA 4.0 and GFDL,
  and reusers "may comply with either license or both" [S26]. Kaikki's raw dump has
  no resource-specific license statement in the pages searched [S30, G].
- **Private use is not cleared.** Under CC BY-SA 4.0 the attribution and ShareAlike
  conditions attach to "Share," defined as making material available "to the
  public" [S22, §1(k)]. Under CC BY-NC-SA 4.0 the NonCommercial definition is
  "not primarily intended for or directed towards commercial advantage or monetary
  compensation" [S21, §1(k)]. These licenses contain grants, but the definitions
  alone do not establish applicability or complete provenance for these artifacts.
  See §4.

## 1. Exact dependency artifacts

### 1.1 Pinned manifest and revision

| Field | Value | Source |
|---|---|---|
| Stanza code and files | tag `v1.15.0` → `c9a2158a87502dd63f757ab1aa933230b0acbce7` | [S1] |
| Resources version in code | `__resources_version__ = '1.15.0'` | [S7] |
| Default resources URL | `STANZA_RESOURCES_GITHUB + 'main'`, overridable by env `STANZA_RESOURCES_URL` (`common.py:36`) | [S2] |
| Manifest (examined) | `stanfordnlp/stanza-resources` `resources_1.15.0.json` at `569db9d983a47931673d7d265361ca93af85d176` | [S8] |
| Manifest sha256 (of fetched copy) | `ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463` | [V] |
| Manifest md5 (of fetched copy) | `462ea52f652d309152a102ca15ffd942` | [V] |
| `v1.15.0` tag in stanza-resources | **No.** The ref endpoint returned 404; the tags list returned an empty array (HTTP 200), so there are no tags. | [S9] |
| HF model revision | tag `v1.15.0` → `caeb7742148cb0368e4a0f2c68a0f7b09bc22880` | [S10] |
| Model URL template | `https://huggingface.co/stanfordnlp/stanza-{lang}/resolve/v{resources_version}/models/{filename}` (manifest `url` field) | [S8] |

**Consequences [I].** The default resources URL is a branch (`main`), so the
manifest that a provisioning run uses is whatever `main` holds at download time,
unless `STANZA_RESOURCES_URL` is set. The model URL uses the mutable tag `v1.15.0`.
A reproducible pin must record a stanza-resources commit and the manifest sha256,
and the HF commit `caeb7742…` in place of the tag.

### 1.2 Default `fr` package → artifacts

Read from the manifest [S8]; the HF sha256 and size columns come from the HF tree
at `caeb7742…` [S11]. **Neither the md5 nor the sha256 was computed from downloaded
bytes in this pass.** Every md5 is 32 hex characters and every sha256 is 64, as
checked by script.

Manifest `fr.default_processors`: `tokenize: combined_nocharlm`, `mwt: combined`,
`lemma: combined_nocharlm`, `pos: combined_charlm`, `depparse: combined_charlm`,
`ner: wikinergold_charlm` (out of scope), `optional.coref: udcoref_xlm-roberta-lora`
(out of scope) [S8].

| Role | Processor | Artifact (`models/…` at `caeb7742…`) | md5 [S8] | sha256 (HF LFS oid) [S11] | Size [S11] | Declared dependencies [S8] |
|---|---|---|---|---|---|---|
| tokenize | `combined_nocharlm` | `tokenize/combined_nocharlm.pt` | `34ce2e674142753bb26d2bd66adc7742` | `622a7613d78623cda63e53fc72f84c49b2f64ad959c0f6aeb51355b963eda78f` | 665712 | none |
| mwt | `combined` | `mwt/combined.pt` | `827fd37941d9bb51bdd7c473e0e80cf8` | `a46bc4149531762c2fd79d70eeaaff80479aa6852a08516ab0db567859c8cec0` | 544773 | none |
| lemma | `combined_nocharlm` | `lemma/combined_nocharlm.pt` | `07f18945c29490c60ff3a9dfa18fbdc5` | `31dd152c13d1d713085127e7b30d91e70e88652da4c2ea33c4b6f7d34d810177` | 2813969 | none |
| pos | `combined_charlm` | `pos/combined_charlm.pt` | `505cd655d0474c51d1e35312d49bceae` | `e40eb06786fa691e84aa6ee1ff2321b37f9ff9cb9bf3fe4de94b63793b626f89` | 34563706 | `pretrain/conll17`, `forward_charlm/newswiki`, `backward_charlm/newswiki` |
| depparse | `combined_charlm` | `depparse/combined_charlm.pt` | `c0c8d747238802ee03c40953a8e4d101` | `5ad85cd10e385894152154863c14f621b7d747c67c84eb8597039da12832315a` | 148809357 | same as pos |
| pretrain | `conll17` | `pretrain/conll17.pt` | `f74659129e2008c6b7b6ca0a2db693d7` | `c8c456c1a48fba32d4ab827ae7d3fec40457f58ead7e1977b6267afc37474a3c` | 106908071 | none |
| charlm fwd | `newswiki` | `forward_charlm/newswiki.pt` | `d98e6909619100c23b745cae4383eea2` | `a6874f7670f84c8bd9e57d5dcbafdfe7d7cbae2457c79975040709f1d69ca119` | 20094072 | none |
| charlm bwd | `newswiki` | `backward_charlm/newswiki.pt` | `42eba035046673e195274f45e713653c` | `3ae24cade5cdca7d68193196a942a177af81fbc5d3b5ebb4550ff7b89023b09c` | 20094073 | none |

Other mapping facts:

- **[V]** The French default package's `default_md5` is `1d68653a7e1bbe220b3f1e1d0a2a07c6` [S8]. Stanza uses it when a `default.zip` is downloaded (`common.py:688–703`, md5 at 700) [S2].
- **[V]** The `combined` French treebanks are GSD, ParisStories, Rhapsodie, and
  Sequoia [S13]. ParTUT is not in the default [S13].
- **[V]** `default_packages.py` maps `"fr": "newswiki"` in `default_charlms` [S3].
- **[V]** Stanza's `download_vectors.sh` fetches the conll17 French vectors from the
  LINDAT handle `11234/1-1989` [S4, S20].
- **[I]** `conll17` is a dependency of `pos` and `depparse` in every variant the
  manifest lists, including `combined_nocharlm`-style variants. Dropping the charlm
  does not drop conll17.
- **[G]** The `pretrain/conll17.pt` file is Stanza's `Pretrain` serialization (a
  vocabulary and an embedding matrix) [S5]. Whether the hosted file was built from
  the LINDAT tarball, and with what options, is not documented in Stanza's sources.
  **Gap G1.**
- **[V]** S20 redirects to S19, whose HTML metadata identifies the same handle.
  The deposit identity is confirmed; the hosted Stanza artifact's conversion
  lineage remains unverified (G1).
- **[G]** The treebank revisions cited below establish notices at those revisions,
  not the exact training snapshots of each shipped processor. The combined-models
  documentation is not a release-specific training manifest. Exact training
  revisions, processor-specific mixes, and their original notices still need
  confirmation from Stanza (G13).

## 2. License evidence

### 2.1 Notices found

| Artifact or input | Notice in source | Source | Label |
|---|---|---|---|
| Stanza code | Apache-2.0. The repo `LICENSE` is only the copyright header and a URL. Full text: Apache-2.0 | [S6, S25] | [V] |
| Stanza tree | No file whose path contains "notice" (case-insensitive) at `c9a2158` | [S1] | [V] |
| HF `stanfordnlp/stanza-fr` card | `license: apache-2.0`; no dataset or training section | [S12] | [V] |
| Stanza performance page | UD-derived model license "unclear." "To the extent" Stanford has rights, ODC-By 1.0 | [S14] | [V-prior] |
| GSD | CC BY-SA 4.0. Google dropped the NC restriction for UD annotations; the README says the underlying text is not Google's and may be copyrighted | [S15] | [V] |
| ParisStories, Rhapsodie | CC BY-SA 4.0 | [S16, S17] | [V] |
| Sequoia | LGPL-LR. README lists "French Europarl" among sources | [S18] | [V] |
| conll17 (LINDAT item S19) | "This item is … licensed under CC BY-NC-SA 4.0." The item says the license applies to the generated annotations and word embeddings, "not to the underlying data, which may have different license and impose additional restrictions" | [S19] | [V] |
| `newswiki` character models | No license notice found. Corpus not identified. Searched the source (see §2.4) | [S1, S3] | [G] |

### 2.2 Conflicts and unresolved points

1. **Apache-2.0 vs. "unclear" / ODC-By "to the extent."** The model card and code
   say Apache-2.0. The performance page says the UD-derived license is unclear and
   points to ODC-By to the extent Stanford holds rights [S12, S14]. Neither source
   says which governs the packaged `.pt` files. **[G][L]**
2. **CC BY-SA 4.0 on the treebanks.** Whether trained weights are "Adapted Material"
   (CC BY-SA §1(a), [S22]) is not settled. ShareAlike attaches to sharing adapted
   material. **[L]**
3. **CC BY-NC-SA on the conll17 vectors.** If the vectors' terms reach the fine-tuned
   `pos` and `depparse` weights, the NonCommercial term applies to those weights.
   Whether they do is not settled. The LINDAT item's carve-out for underlying data
   does not answer it. Stanza's link from the `.pt` file to the LINDAT item is not
   byte-verified [S19, G1]. **[L][G]** Independently of trained-weight inheritance,
   `pretrain/conll17.pt` contains an embedding matrix [S5]. Repackaging source
   vectors may directly reproduce licensed material [I][L]. Clarify rights for
   that runtime dependency itself, not only the downstream processor weights.
4. **Sequoia LGPL-LR.** Its effect on weights trained on Sequoia text is not analyzed
   here. LGPL-LR §2 requires modified works to be under LGPL-LR "as a whole," and §5–§6
   prohibit copying or distributing outside its terms [S24]. **[L]**
5. **Underlying text rights.** The GSD README says the annotated sentences may be
   copyrighted and were collected "as is" [S15]. This is separate from the annotation
   license. **[V][L]**

### 2.3 Use-level status of the model artifacts

Not cleared by this investigation for any intended use. The model card and the
qualified ODC-By statement supply licensing evidence, but their relationship and
coverage of third-party inputs are unresolved. The code license alone cannot
establish rights in separately supplied resource artifacts [I]. S14 describes
an older performance table (Stanza 1.5.1 / UD 2.12), so it is not a release-specific
answer for the 1.15.0 artifacts. Do not treat either statement as nonexistent;
request a reconciled, artifact-specific basis.

### 2.4 Searched for the `newswiki` source

- Full-text search of the stanza tag at `c9a2158` for `newswiki` (blobs under 1 MB,
  shallow clone): the only hit is `default_packages.py` [S3]. The mapping appears for
  `de`, `es`, `fr`, and `ru` (lines 333, 335, 338, 351). **[V]**
- Searched `stanza/utils/charlm/` (8 files, including `conll17_to_text.py`,
  `make_lm_data.py`, `dump_oscar.py`, `oscar_to_text.py`) [S1]. No `newswiki` name
  or corpus description. **[V]**
- GitHub code search needs authentication (HTTP 401 unauthenticated). **[G]**
- Files over 1 MB were excluded from the sweep. **[G]**

### 2.5 Exact maintainer inquiry (for the Stanza team)

> Which corpus was used to train the `newswiki` forward and backward character language
> models for French (`forward_charlm/newswiki.pt`, `backward_charlm/newswiki.pt`, Stanza
> 1.15.0 default `fr`, HF `stanfordnlp/stanza-fr` at `caeb7742…`)? Please give the source
> dataset, its license or terms, and whether it is publicly redistributable.
>
> Was `models/pretrain/conll17.pt` built from the LINDAT deposit `11234/1-1989`
> (`word-embeddings-conll17.tar`), and if so by which `convert_pretrain.py` command and
> options? Please give the source file's checksum.
>
> Which license governs the `pos/combined_charlm.pt` and `depparse/combined_charlm.pt`
> weights: Apache-2.0 (model card), ODC-By 1.0 "to the extent" (performance page), or
> another? Please give the documented basis.
>
> Please also cover tokenize, mwt, lemma, pretrain and both character models,
> identify exact treebank/corpus revisions used per processor, and supply
> original notices applicable to those versions. The generic combined-models
> page does not identify those training snapshots.

## 3. Wiktionary and Kaikki evidence

### 3.1 Wiktionary

- **[V]** Wikimedia Terms of Use §7 requires contributors to license their text
  under CC BY-SA 4.0 and GFDL (unversioned, no invariant sections), and says
  "Reusers may comply with either license or both" [S26]. The same section sets
  attribution by hyperlink or URL "to the article to which you contributed (since
  each article has a history page that lists all contributors)," or to a stable
  alternative copy, and requires an indication of modification and a license notice
  for each distributed copy [S26]. **This is evidence of the grant chain for
  contributor text**, not a warranty covering every imported passage or quotation.
  Section 7's imported-text and non-text provisions must also be respected; do not
  assume every entry component is available under both licenses. A CC BY-SA reuse
  route is feasible where that grant applies; the slash in existing metadata is
  not a requirement to comply cumulatively with both CC and GFDL.
  Section 7 also says contributors waive their covered Sui Generis Database
  Rights, explicitly giving freely reusable contributed facts as an example
  [S26]. Do not presume all factual entries require attribution, or that this
  waiver establishes rights in a separate extractor/publisher's database.
- **[V]** The Wiktionary:Copyrights page carries an "outdated" notice. It dates to
  2002, was copied from Wikipedia, and may not apply [S27]. This note does not treat
  that page as the grant. Its other statements are [V-prior] only.
- **[V]** Wiktionary:Quotations, §Copyright: citation sentences "should not be added
  … in a way that violates the copyright of the work." It says a citation is
  "likely … a de minimis portion" or "a clear fair use," but that long citations from
  short works, or multiple citations from one work, can infringe. Works published in
  the US before 1931 and US government documents are listed as public domain [S28].
  **Quotations are third-party material with their own status** [L].
- **[V-prior]** The en.wiktionary footer says text is under CC BY-SA and that
  "additional terms may apply." This was not re-fetched in this revision [G].
- **[V-prior]** Wiktextract's code is MIT. Files under `tests/` are under the
  Wiktionary license (CC BY-SA or GFDL) [S29].
- **[G]** Per-entry quotation handling, imported external text, and per-entry
  history attribution were not sampled on individual entries.

### 3.2 Kaikki.org

- **[V]** Searched the French index, the raw-data page, and the home page for
  license, copyright, and citation terms [S30]. The only use term found is an
  academic citation request (Ylonen, LREC 2022). A citation request is not a license
  condition [L].
- **[G]** No resource-specific license declaration for the raw dump was found on the
  pages searched. This is not proof that none exists; the rest of kaikki.org was
  not read exhaustively.
- **[V-prior]** The index page says the **displayed** (post-processed) data includes
  "additional data merged from other sources" [S30]. The raw dump is unprocessed
  Wiktextract output [S30]. The post-processing merge therefore does not establish
  that the raw dump contains merged or contaminated text. It is a separate question.
- **[V]** The derivation script downloads
  `https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz` [S31, line 29].
- **[M]** HEAD on the raw dump: HTTP 200, `content-length: 2981058381`,
  `last-modified: Sat, 03 Oct 2026 08:24:38 GMT`, `etag: "6ac0bbc6-b1af574d"`
  [S30]. The etag is not a content hash. No sha256 was computed.
- **[V-prior]** The raw-data page lists the dump dated 2026-09-02 and extracted
  2026-10-03 [S30].
- **[V]** Its footer reports `wiktextract (1a05e46 and e3d6d4e)` [S30]. GitHub
  resolves these to Wiktextract `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a`
  and Wikitextprocessor `e3d6d4edb77618f4d6680edc66e3f774bea59820` [S35].
  They are published release metadata, not proof that every byte was generated
  with those revisions. Preserve both and obtain the extraction configuration
  and dependency versions needed for reproduction; the moving URL cannot recover
  an old dump after replacement.

### 3.3 Repository facts at base `6e02d180`

- `nlp/scripts/derive_dictionary_index.py:21` `LANGUAGES = {"de", "it", "el"}`. French
  is not a derivation target [S31] [V].
- `nlp/scripts/derive_dictionary_index.py:521` and `:524` hard-code the license
  string `"Wiktionary-derived data: CC BY-SA 4.0 / GFDL"` and the attribution string
  [S31] [V].
- `nlp/scripts/derive_dictionary_index.py:601–603` take `--dump-date` (default `""`),
  `--extraction-date` (default today's date), and `--wiktextract-commit` (default
  `""`) [S31] [V].
- `nlp/scripts/derive_dictionary_index.py:615` writes the version string
  `dump=…;extraction=…;wiktextract=…`, with `unknown` for an empty dump date or commit
  [S31] [V].
- **The script has no sha256, md5, or hashlib code** [S31] [V]. The dump's checksum is
  not recorded anywhere.
- `internal/dictionary/attribution.go:4` `AttributionNotice` names Wiktionary,
  Wiktextract, and Kaikki.org, and states CC BY-SA 4.0 / GFDL with share-alike. It
  records **no** dump, extraction date, or commit [S33] [V].
- `internal/cardexport/cardexport.go:1609–1631` (`deckDescription`,
  `attributionNotices`) adds `AttributionNotice` to the APKG deck description and
  to the TSV header when `dictionaryAttribution` finds dictionary evidence, meaning a
  non-empty `DictionaryProviderVersion` and at least one candidate sense [S33] [V].

## 4. Use-by-use status

Each use is **not accepted**. The "conditions" lists what the sources say applies if
the use occurs. "Grant gap" is what keeps the use from being cleared.

### Use 1: Personal self-hosted execution

**Status: not cleared.**

- **Ordinary private-use conditions.** Apache-2.0 has no condition on private use
  beyond its terms (§4 applies to redistribution) [S25]. ODC-By's notice duties apply
  when one "Publicly Convey[s]" the database [S23 §4.2], and "Conveying" is defined
  as enabling another person to make or receive copies [S23]. ODC-By §4.3 also
  requires an associated source/license notice on **Public Use of a Produced
  Work**, even without conveying the database. Its definition of Publicly differs
  from CC's Share; do not interchange them [S23]. CC BY-SA attribution/ShareAlike
  generally attach to Sharing; CC BY-NC-SA also limits reproduction/adaptation
  grants to NonCommercial purposes, including non-public activity [S21, S22].
- **Grant gap (the main issue).** Running a model on one's own hardware involves
  copying the weights onto that hardware. An applicable license can supply the
  necessary reproduction rights. CC BY-NC-SA §2 permits NonCommercial
  reproduction; a private copy is **not automatically outside that grant**.
  Personal use is not itself proof of NonCommercial purpose, however, and
  applicability, provenance and upstream rights remain unresolved [L][G].
  **Missing notice is not private permission.**
- **Dictionary.** The Wikimedia grant supports reproduction and adaptation of
  covered contributor text, including private local derivation [S22 §2, S26].
  Unknown Kaikki-specific rights and excluded third-party text still require a
  documented basis [G]. No public ShareAlike publication duty is inferred merely
  from making a private index.
- **Stanza and LGPL-LR.** Whether LGPL-LR conditions attach to a model trained on
  Sequoia is not settled [L]. LGPL-LR §0 expressly does not restrict running a
  program using the resource; output is covered only if it constitutes a work
  based on the resource. That is different from copying/modifying the resource,
  and does not identify which provisions govern these particular weights [S24].

### Use 2: Automated provisioning and download

**Status: not cleared. The current code does not pin or record a basis.**

Verified behavior, separated from proposals:

- **[V] Stanza verifies a checksum per file.** `request_file` calls
  `assert_file_exists(path, md5, alternate_md5)` after download [S2, `common.py:195–221`].
  `assert_file_exists` raises `ValueError` on mismatch [S2, `common.py:126–135`].
  Each model download passes the manifest md5 [S2, `download_models`, `common.py:609–640`, md5 at 637].
  The `default.zip` path passes `default_md5` [S2, `common.py:688–703`, md5 at 700].
- **[V] The manifest is not checked.** `download_resources_json` fetches the manifest
  with `request_file(..., raise_for_status=True)` at `common.py:533–557`, and the call
  passes no md5 argument [S2]. The per-file md5 values therefore show consistency with
  the manifest that was fetched. They do not show that manifest is the one a version
  pins [I].
- **[V] The project's provisioning passes through Stanza.** `provision.py` assigns
  `download = download or stanza.download` (line 216) [S32]. It has no md5, sha256, or
  checksum string [S32]. Its Hugging Face dependencies use `snapshot_download` and
  `external.download` (`ensure_external_dependency`) [S32].
- **[V] No notice is written.** `provision.py` writes no license notice file [S32].

**Proposed safeguards** (engineering recommendations, not license requirements):

1. Pin the stanza-resources commit (or the `STANZA_RESOURCES_URL` value) and the
   manifest sha256 in provisioning.
2. Pin the HF model revision to the commit `caeb7742…`, not the tag `v1.15.0`.
3. Verify each artifact's md5 against the manifest (Stanza already does this),
   record the published sha256, and compute/compare sha256 on the actual file at
   future authorized provisioning. Metadata alone is not byte verification.
   These proposals are not a license condition.
4. Write a per-artifact notice file (§5).

**Gaps:** [G] conll17 → LINDAT byte link (G1); the provenance of the Stanza tag's
wheel manifest was not checked beyond the commit in §1.1.

### Use 3: Local dictionary derivation and refresh

**Status: not cleared.**

- **Source license conditions.** The Wiktionary grant chain in §3.1 [S26] applies to
  the text the index contains. Its attribution, modification, and license-notice
  conditions attach to distribution of the derived index [S26].
- **Proposed safeguards:** record the dump filename, dump date, extraction date,
  both extractor commits, download time/URL, and dump sha256, plus derivation-code
  revision, options, resulting index sha256 and captured notices. The script
  records dump date, extraction date and a Wiktextract commit when supplied,
  **not** the input filename, Wikitextprocessor commit or sha256 [S31]. Capture
  provenance anew at each refresh; do not relabel an old index with a new date.
- **Grant gap:** Kaikki's own terms for the raw dump were not found [G]. The
  per-entry quotation status (§3.1) is unsampled [G].

### Use 4: Distribution of models, data, or derived dictionaries

**Not a launch destination. Records what would apply, not a clearance.**

- **Model weights:** no distribution clearance. Redistributing Apache-licensed
  code requires the license copy, prominent change notices for modified files,
  retained applicable notices, and any supplied NOTICE attribution [S25 §4(a)–(d)].
  If applicable, ODC-By §4.2 requires license/URI and intact notices in the database
  and relevant documentation; §4.3 requires an associated source/license notice
  for Public Use of a Produced Work [S23]. CC BY-SA ShareAlike for adapted
  material, if the weights are adapted from
  GSD, ParisStories, or Rhapsodie [L]. CC BY-NC-SA NonCommercial, if the conll17 terms
  reach the weights [L]. LGPL-LR §5–§7 restrictions on copying and redistribution,
  and the automatic license in §7, if Sequoia applies [S24] [L].
  For LGPL-LR-covered resources, §1 requires copyright/warranty notices and a
  license copy; §2 adds dated modification notices and same-license terms for
  covered modifications. Its §4 package exception has notice and
  machine-readable legible-form/replacement/offer alternatives; applicability to
  these artifacts needs review, not an assumption that the entire application
  becomes copyleft [S24].
- **Dictionary-derived data:** CC BY-SA 4.0 §3 attribution, indication of
  modification, license notice or URI, and ShareAlike for adapted material [S22] [L].
  Wikimedia ToU §7 attribution and notice duties also apply [S26]. Preserve
  supplied creator/copyright/disclaimer/source notices where required; do not add
  terms or technical measures restricting licensed downstream rights [S22 §§2–3].
- **Gaps:** whether the derived index is adapted material and/or exercises database
  rights; whether third-party quotations/imports have a separate usable basis.
  Post-processed merged data is a separate unsupported artifact, not the selected
  raw input.

### Use 5: Dictionary-derived material in APKG exports

**Not cleared.** Split into copied and generated fields.

**5a. Copied fields** (sense gloss, verbatim examples, IPA, gender, plural, article).

- **Copyrightable expression vs. facts.** Isolated facts such as an IPA transcription,
  a gender, or a short form are not automatically copyrightable expression [L].
  Longer glosses and example sentences can be [L]. The question is per-field, not per-deck [L].
- **Database rights and repeated extraction.** CC BY-SA §4 covers sui generis
  database rights, including substantial extraction and reuse of a database [S22 §4].
  A new database containing a substantial portion can be Adapted Material for
  database-rights purposes even if isolated facts are not copyrightable. Repeated
  extraction must not be assumed insubstantial [L].
  Whether the Wiktionary extract is a protected database in the relevant jurisdiction
  is [L][G]; account for the Wikimedia contributor waiver noted in §3.1 rather
  than asserting that database rights necessarily survive.
- **Attribution and ShareAlike.** Merely creating or transferring a file is not
  automatically Sharing. CC BY-SA §1(k) defines Share as making material available "to the
  public by any means or process that requires permission" [S22] [L]. Giving the file
  to a single known recipient is not clearly "to the public" [L]. That is a limited
  recipient question, not a categorical answer: a deck handed to one person may still
  be Shared under some readings [L].
- **Whole-archive status.** The APKG does not become ShareAlike as a whole merely
  because a copied field is in it [L]. Coexisting book text in the same deck does
  not become CC-licensed merely because Wiktionary fields coexist with it [L].
  Conversely, a notice cannot authorize sharing the learner's copyrighted book
  excerpts. Attribution and any applicable ShareAlike attach to covered material
  when the license's Sharing conditions are triggered [S22 §3].
- **Generated does not clear rights.** Generated fields do not automatically clear
  the rights in the copied source material [L].
- **What the code does [V].** `applySenseSelectionGloss` sets `input.Gloss`
  (`cardexport.go:1480–1483`). `chooseSenseSelectionGloss` calls
  `selectedSenseGloss` (`cardexport.go:1512`). `applyExactEnrichment` calls
  `applySenseSelectionGloss` on the non-contextual path (`cardexport.go:1402`), and
  `applyExactEnrichment` is called at `cardexport.go:1375`. The function that contains
  line 1375 is `renderManifest`, which applies exact enrichment before rendering.
  `MeaningEvidenceHash != ""` selects `applyContextualGloss` instead of copied
  sense selection (`cardexport.go:1399–1402`). Thus copied sense glosses are a
  supported non-contextual branch, not evidence that every current prepared deck
  copies a gloss. ADR 0079 requires the contextual path for the normal current
  workflow [S34]. The
  dictionary index's gloss also flows through `resolveLexicalEntry` (`cardexport.go:1175–1189`)
  and into the render field `Gloss` (`cardexport.go:1063`), and that wiring is
  live from `cmd/server/main.go:196` [S33].
- **French scope.** `LANGUAGES` does not include `fr` [S31]. The current index
  therefore does not contain French target-language senses derived from the English
  dump. This investigation assesses the **proposed** French pipeline; it does not
  claim French dictionary-backed decks already exist.

**5b. Generated or contextual fields** (LLM output grounded in candidate senses,
`applyContextualGloss`, ADR 0079) [S34].

- **Input transmission [V].** Candidate senses (glosses and examples) are sent in the
  LLM request payload (`internal/enrichment/llm_codec.go:180–196`) [S33].
- **Whether provider processing is Sharing is [L].** Provider processing for one user is
  not obviously "to the public." Provider terms (retention, logging, training) were
  **not reviewed** [G].
- **Attribution placement.** ADR 0079 places Wiktionary attribution in deck or export
  metadata, not per card [S34 §55]. Whether that meets CC BY-SA §3(a)(2) for generated
  fields is [L].

## 5. Attribution path (proposed; not implemented)

Each item is a proposal, not a license condition, except where marked.

1. **[License, sourced] Wikimedia attribution.** Keep author/source links: a hyperlink
   or URL to the page, or to a stable alternative copy with history [S26]. The current
   `AttributionNotice` (`attribution.go:4`) names the sources but has no link to the
   entry or the history page [S33].
2. **[License, sourced] Modification notice and license notice.** Mark modified
   material and include the license name and URI for each distributed copy [S26][S22 §3].
3. **[Proposed] Versioned dictionary notice.** Extend `AttributionNotice` with the dump
   filename, dump date, extraction date, both extractor commits, derivation revision
   and sha256s. The script records only dump date, extraction date and the
   Wiktextract commit of those fields [S31]. Retain a source-entry/revision URL
   inventory alongside each frozen deck, so a deck-description link or bundled
   notice can actually lead to the authors/history and applicable terms. A generic
   provider name alone does not establish this path. Check medium-appropriate
   attribution with the human before adoption; no schema/UI/export change is made.
4. **[Proposed] Per-artifact Stanza notice.** Provisioning writes the processor, path,
   published and locally verified hashes, the stanza-resources and HF commits,
   and the §2 evidence.
   It is not authoritative until §2 is resolved.
5. **[Proposed] LLM path.** Record that contextual glosses came from a named provider
   from candidate senses, with a link to the notice [S34].
6. **[Proposed] Exclusion list.** Drop or flag quotation examples and imported external
   text [S28]. This needs a per-entry classifier that does not exist.

## 6. Human, maintainer, and legal questions

Each item is a precise request for evidence or an interpretation, not a request to
"accept risk." Adoption stays blocked until a human accepts a documented basis in
[Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141).

**6a. Maintainer inquiries (evidence requests)**

1. **Stanza team:** the `newswiki` corpus and its license (§2.5).
2. **Stanza team:** the conversion of `pretrain/conll17.pt` from the LINDAT tarball,
   with the command and source checksum (§2.5).
3. **Stanza team:** reconciled licensing evidence for all eight runtime artifacts,
   exact training-source revisions and retained third-party notices (§2.5).
4. **Kaikki:** for the raw English release extracted 2026-10-03 from dump 2026-09-02,
   confirm applicable content/database rights and notices, whether any independently
   licensed material beyond that Wiktionary extraction is included, quotation/import
   treatment, a stable archive/checksum, and extraction configuration corresponding
   to S35. The post-processing notice alone does not answer rights in this raw file.

**6b. Legal interpretation (counsel or human review)**

1. Does CC BY-NC-SA §1(k) NonCommercial plus §2 grants reach weights trained on
   conll17 vectors? (§2.2 item 3.)
2. Does CC BY-SA 4.0 reach weights trained on the GSD, ParisStories, or Rhapsodie
   text? (§2.2 item 2.)
3. Does LGPL-LR reach weights trained on Sequoia? (§2.2 item 4.)
4. Do the reconciled grants cover local copying/execution and dictionary derivation
   for the actual personal, NonCommercial deployment, including the runtime
   embedding matrix and excluded source material? (Uses 1–3.)
5. Is a single-recipient APKG transfer "to the public"? (Use 5a.)
6. Is LLM provider processing of candidate senses a Share? (Use 5b.)
7. Which attribution/source-history path is reasonable for a private APKG export
   and, separately, any subsequently shared dictionary-derived fields? Does the
   proposed deck notice/inventory preserve required notices and distinguish
   adapted expression, factual fields and unrelated book text? (§5.)

**6c. Optional distribution questions (not needed for private launch)**

These apply only if the project distributes artifacts. They do not need answers for
a private, self-hosted launch, and they do not mean every hypothetical distribution
must be cleared now.

1. ODC-By Produced Work notices, if the ODC-By term applies.
2. Apache-2.0 §4 change notices and NOTICE handling, if redistributing Stanza code.
3. Public distribution of a full derived index and any commercial-hosting basis.
   The intended local derivation and personal APKG path still need review above;
   these optional expansions are not prerequisites simply because they can be imagined.

## 7. Gaps summary

| # | Gap | Blocks |
|---|---|---|
| G1 | Runtime conll17 matrix conversion lineage not verified; LINDAT deposit identity is confirmed | Use 1, 2, 4 |
| G2 | `newswiki` training corpus not identified (§2.4) | Use 1, 2, 4 |
| G3 | Stanza's weight license conflict (Apache-2.0 vs. "unclear" / ODC-By) unresolved (§2.2) | Use 1, 4 |
| G4 | No artifact bytes downloaded; sha256 values are HF API metadata, md5 values are manifest values | Use 2 |
| G5 | Kaikki raw dump sha256 not computed; etag is not a hash | Use 3, 4 |
| G6 | Kaikki: no resource-specific license found on three pages; not exhaustive | Use 3, 4, 5 |
| G7 | Wiktionary:Quotations read (§Copyright only); quotation handling in entries not sampled | Use 3, 4, 5 |
| G8 | Per-entry Wiktionary history and external-source marking not sampled | Use 3, 4, 5 |
| G9 | LLM provider terms not reviewed | Use 5b |
| G10 | Whether any particular historical/non-contextual deck uses copied sense selection requires its frozen cache identity; current contextual and non-contextual branches distinguished above | Per-deck Use 5a |
| G13 | Exact per-processor training-source revisions/notices not established by generic combined-model documentation | Use 1, 2, 4 |

These are investigation limits, not thirteen independent mandatory legal
opinions. G4/G5 are deliberately deferred byte checks for future authorized
provisioning, not a reason to download bundles during planning. Exact source
rights, intended-use interpretation and feasible notices need a documented
basis; optional public distribution and commercial hosting need not be cleared
to choose a personal self-hosted launch.

## 8. Status

- **Stanza 1.15.0 French default:** provenance mapped from the pinned manifest and HF
  tree (§1). License terms conflicting or unresolved (§2). **Not cleared.**
- **Raw Wiktionary via Kaikki:** Wikimedia ToU §7 grant chain identified (§3.1). Kaikki
  terms not found (§3.2). **Not cleared.**
- **Private self-hosted execution:** **not cleared.** Self-hosting is not a grant (§4,
  Use 1).
- **Adoption:** **BLOCKED.** Requires a human decision in
  [Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141) that accepts a
  documented basis for each use the project intends.
