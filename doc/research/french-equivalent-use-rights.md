# French equivalent-use rights: existing grants on the French path

**Research date:** 2026-10-17 (access date for every primary source; §8)
**Ticket:** [#2155 Obtain a documented French resource rights basis](https://github.com/justin-hayes/mouseion/issues/2155)
**Decision gate:** [#2141 Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141)
**Repository base:** `6e02d180b1015536127839e0a45953684c0e5a83`
**Builds on:** [`doc/research/french-resource-provenance.md`](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md) at `c90b50de` (the "prior note")

**Status: legal interpretation of published license texts. Not binding legal advice, and not an adoption decision.** The note answers one question: which existing grants apply to the uses Mouseion makes of the French Stanza package and the Kaikki-derived dictionary index, if French works the way Italian does. It does not audit German, Italian, or Greek, and it does not ask for new permissions. Where a conclusion depends on an artifact's coverage, the matrix states the outcome under each reading.

## 0. Conclusions

1. **Existing grants provide private-use routes.** Where the governing license reaches an artifact, its grant covers the operator's own copying, running, local derivation, and personal export, subject to its actual terms. CC §3 attribution/ShareAlike duties have public-Sharing triggers (BY-SA §1(k); BY-NC-SA §1(l)). Purely private use does not trigger those CC §3 duties; that is not a waiver of every license term or of separate rights.
2. **Grant scope and conditions must be separated.** The CC grants are conditioned on attribution and ShareAlike only when Sharing occurs. Other conditions are not limited to Sharing: LGPL-LR §1 conditions copying of verbatim copies on keeping notices; the NonCommercial limit in BY-NC-SA §2(a)(1) limits the grant itself; Apache-2.0 §4 conditions redistribution only.
3. **NonCommercial is a scope limit, not only a sharing trigger.** BY-NC-SA §2(a)(1) grants reproduction "for NonCommercial purposes only." Commercial private reproduction would fall outside the grant. Whether the deployment is non-commercial is an assumed scope fact, not one the docs confirm (§6).
4. **Coverage is the open variable.** The outcome depends on the publisher/source grants' actual scope. The completed [Stanza evidence note](french-stanza-rights-basis.md) documents two publisher declarations and the runtime-embedding source grant; the [Kaikki evidence note](french-kaikki-rights-basis.md) adds a collaborator's explicit reuse statement and quotation examples. This matrix is conditional, not a final acceptance.
5. **Provider processing now has a concrete default-provider reading.** The project defaults to OpenAI's API, not an unknown vendor. Its published service and data-use terms were checked in the coordinating review (§4.1). Those terms do not themselves license third-party inputs. Actual endpoint overrides, account terms/settings and source exceptions remain conditional, rather than a blanket missing provider permission (U4).

## 1. Corrections to the prior note

| Prior note | Status | Basis |
|---|---|---|
| Use 1: "Self-hosting is not a license grant." | Accurate as stated, but incomplete. Where a license covers the artifact, its grant covers private reproduction and use (see §3). The open item is coverage. | [S1] §2(a)(1); [S2] §2(a)(1); [S3] §2 |
| Use 1: under BY-NC-SA "a private copy is not automatically outside that grant." | Accurate; make the affirmative route explicit. Private reproduction is inside the grant when it covers the material and the purpose is NonCommercial. Neither private use alone nor missing documentation proves permission or prohibition. | [S2] §1(k), §2(a)(1) |
| §4 Use 1: "LGPL-LR §0 expressly does not restrict running a program using the resource." | **Accurate. Keep it.** §0 says: "The act of running a program using the Linguistic Resource is not restricted, and output from such a program is covered only if its contents constitute a work based on the Linguistic Resource." | [S5] §0 (full text checked 2026-10-17) |
| Use 3 and §3.2 withhold overall acceptance while recognizing a contributor-text grant and an unresolved Kaikki layer. | Distinguish those layers rather than treating the whole file as unlicensed. The contributor grant is evidence for covered text regardless of a separate Kaikki notice. Any independently protectable additions or database rights require their own analysis; silence does not establish that such rights exist. | [S6] §7 |
| Use 5a: a single known recipient is "not clearly 'to the public'." | Keep as a question, not as a conclusion. This note does not presume a single-recipient transfer (§4, U5). | [S1] §1(k) |

## 2. Italian: operational precedent and shared architecture

This section records what the Italian path already does and what French would share. It does not analyze Italian's component licenses, and it does not treat Italian's status as a documented basis.

- **Shared provisioning.** Both languages use the same `provision()` path: `stanza.download` with the language's processors, then `snapshot_download` for external dependencies, with no checksum or notice written (`nlp/src/mouseion_nlp/provision.py:216`).
- **Shared Stanza defaults.** Stanza's default mappings name the same component families for both languages. French: pretrain `conll17` (`stanza/resources/default_packages.py:182`), charlm `newswiki` (line 338). Italian: pretrain `conll17` (line 201), charlm `conll17` (line 342). These are mapping facts from the pinned Stanza commit `c9a2158a87502dd63f757ab1aa933230b0acbce7` [S7]. Whether the mapped files carry the same licenses for each language is not examined here.
- **Shared dictionary index.** `nlp/scripts/derive_dictionary_index.py:21` lists `{"de", "it", "el"}`. Adding `fr` is a configuration change; the metadata license string (line 521) and the attribution notice (`internal/dictionary/attribution.go:4`) apply to all languages.
- **Operator posture.** The repository's stated posture for Stanza artifacts is that they are "downloaded at deployment time and never redistributed," with operators to "review those licenses for their deployment" (ADR 0073, around lines 45–47; `doc/features/language-support.md`, Greek paragraph, around line 116). That posture is operational. It is not a license analysis, and this note does not treat it as one.

**Parity caveat.** Matching Italian's operational path reproduces the same architecture and the same posture. It does not transfer a documented grant from Italian to French, and it is not a substitute for the French grant analysis below. The French rows in §3 and §4 are the analysis.

## 3. Artifact-level grants

"Grant" quotes the operative text. "Conditions" lists what the text makes conditional, and when. Coverage is the open question for each artifact.

| Artifact | Grant (quoted) | Conditions and their triggers | Coverage (open item and owner) |
|---|---|---|---|
| **Stanza code** (Apache-2.0) | §2: "a perpetual, worldwide, non-exclusive, no-charge, royalty-free, irrevocable copyright license to reproduce, prepare Derivative Works of, publicly display, publicly perform, sublicense, and distribute the Work" [S3] | The grant is subject to the license's terms. §4's redistribution duties include the license, modification notices, retained notices and any applicable NOTICE attribution. They do not create a public-release duty for private operation. | Covered. |
| **Stanza French weights** (`stanfordnlp/stanza-fr` at `caeb7742…`) | The model card says `license: apache-2.0` [prior note S12]. Apache-2.0 §2 is the grant if the publisher's notice governs the file. | Private use: none under Apache-2.0. Redistribution: §4 notices. | The Stanza performance page says the UD-derived license is "unclear," and that ODC-By 1.0 applies "to the extent" Stanford holds rights [prior note S14, not re-fetched]. The two statements conflict; which governs the packaged files is open. **Owner: Stanza-artifact agent.** |
| **Stanza French `newswiki` charlm** (`forward_charlm/newswiki.pt`, `backward_charlm/newswiki.pt`) | Repository-level Apache declaration, qualified project ODC statement, and a maintainer's explicit suggestion to reuse the French charlm are evidenced in the [Stanza note](french-stanza-rights-basis.md). No corpus-specific notice was found. | These supply candidate private-use routes within their scope. An automatically generated declaration is still publisher evidence, not automatically invalid. | Reconciled coverage and any third-party corpus terms remain undocumented. Do not describe this as an absence of all grant evidence. |
| **conll17 vectors and `pretrain/conll17.pt`** (LINDAT item, CC BY-NC-SA 4.0) | Per the prior note's reading of the LINDAT item [prior note S19, not re-fetched]: the NC-SA license covers the annotations and embeddings, not the underlying data. BY-NC-SA §2(a)(1): "reproduce and Share the Licensed Material, in whole or in part, for NonCommercial purposes only" [S2]. | Private, non-commercial reproduction is within the grant. §3(a)(1) attribution and the ShareAlike duty apply "If You Share." Commercial use is outside the grant. | Whether the LINDAT notice reaches the Stanza `pretrain` conversion and the fine-tuned `pos` and `depparse` weights. **Owner: Stanza-artifact agent.** |
| **Treebank-derived weights** (GSD, ParisStories, Rhapsodie; CC BY-SA 4.0 per the prior note) | BY-SA §2(a)(1): "a. reproduce and Share the Licensed Material, in whole or in part; and b. produce, reproduce, and Share Adapted Material." No purpose limit [S1]. | Attribution and ShareAlike under §3(a) and §3(b) apply "If You Share" (§3(a)(1)) or "if You Share Adapted Material" (§3(b)). | Whether trained weights are "Adapted Material": "derived from or based upon the Licensed Material and in which the Licensed Material is translated, altered, arranged, transformed, or otherwise modified in a manner requiring permission" (§1(a)). **[L].** |
| **Sequoia-derived material** (LGPL-LR; `UD_French-Sequoia` `LICENSE.txt` at `21e377f9`) | §1: "You may copy and distribute verbatim copies of the Linguistic Resource as you receive it, … provided that you conspicuously and appropriately publish on each copy an appropriate copyright notice and disclaimer of warranty; keep intact all the notices …" [S5]. §2 allows modification provided "The modified work must itself be a linguistic resource," the work is "licensed at no charge to all third parties under the terms of this License," and "These requirements apply to the modified work as a whole." | §1 notice-keeping conditions attach to each verbatim copy and to distribution. §5 says copying outside §§1–2 is not permitted. | Whether trained weights are a "linguistic resource," defined in §0 as "a collection of data about language prepared so as to be used with application programs," and so are a §2 modification **[L]**. |
| **Wiktionary text in the Kaikki-derived index** | Wikimedia ToU §7: "Text to which you hold the copyright … license it under: Creative Commons Attribution-ShareAlike 4.0 International License … and GNU Free Documentation License … Reusers may comply with either license or both" [S6]. BY-SA §2(a)(1) grants reproduction and adaptation. | Private derivation and copies: no notice duty. Sharing: BY-SA §3(a), §3(b), and SA §4(c). | Contributor-licensed text is covered on its face. Imported third-party text and quotations in the Kaikki `examples` field keep their own status (ToU §7 "Importing text"; prior note §3.1). **Owner: Kaikki and per-entry review (not in this note's scope).** |
| **Kaikki compilation layer, if independently protectable** | The [Kaikki companion note](french-kaikki-rights-basis.md) records a repository collaborator explicitly endorsing raw-data distribution through Wiktionary's licenses. | Contributor-text grants do not require a fresh Kaikki notice merely because text is mechanically extracted. The statement is affirmative reuse evidence, not a complete release-specific rights inventory. | Whether any independent right exists and whether existing evidence covers it needs a supported interpretation; do not invent a separate right or mandatory permission request from silence. |
| **Wiktextract code** (MIT) | MIT, per the prior note's reading of the repository LICENSE [prior note S29]. | Redistribution notices. | Not an index input; only the dump's text is. |

## 4. Use matrix

"Covered" means the grant reaches the use if the governing license reaches the artifact. "Triggered" means a condition applies. Each row states the outcome under the coverage readings in §3.

| Use | Grant that reaches it | Private-use conditions | What changes the outcome | Outcome under the coverage readings |
|---|---|---|---|---|
| **U1. Provisioning and copying** (`provision.py`: `stanza.download`, `snapshot_download`) | Apache-2.0 §2 for code and covered weights; qualified ODC §3.1 for the rights it licenses; applicable CC reproduction/adaptation grants; LGPL-LR §§1–2 for material it governs. | No public-release duty arises solely from private copying. CC NC purpose and LGPL-LR notice/copying conditions remain material. | Commercial purpose falls outside NC-SA grants. Distribution requires its own applicable-license analysis, not an assumption every license defines Share identically. | Affirmative conditional routes exist. Artifact-specific scope, third-party rights and any applicable LGPL-LR weight conditions remain as documented in the [Stanza note](french-stanza-rights-basis.md). |
| **U2. Self-hosted execution** | CC grants reproduction "in all media and formats whether now known or hereafter created" (BY-SA §2(a)(4)). LGPL-LR §0 says running a program "is not restricted." | None for private running. | Serving weights to others: a Share question. Serving outputs (lemmas, glosses) to others is not Sharing the weights [L]. | Covered where the governing license reaches the weights. For the LGPL-LR-covered Sequoia material, running is outside its scope (§0). |
| **U3. Local dictionary derivation and refresh** | BY-SA §2(a)(1)(b) adaptation; BY-SA §4(a): "Section 2(a)(1) grants You the right to extract, reuse, reproduce, and Share all or a substantial portion of the contents of the database." | None for private derivation. §4(c) applies "if You Share all or a substantial portion" of the database. | Sharing the derived index or any substantial part: BY-SA §3(a) and §4(c) apply. | Covered for the Wiktionary text. Kaikki-layer and quotation status open (§3). |
| **U4. Provider processing** (lemma, UPOS, target word, example sentence, candidate senses including `Gloss` and `Examples`) | BY-SA §2(a)(1) grants reproduction with no purpose limit, including commercial reproduction of covered contributor text. BY-SA §2(a)(5)(a) gives each recipient an offer of the same Licensed Rights. | §3 attribution and ShareAlike duties have Sharing triggers. §2(a)(5)(c) separately bars imposing terms or measures that restrict a recipient's Licensed Rights. | Actual public Share and provider input-rights requirements are distinct questions. Retention, logging and training are not presumed prohibited or automatically authorized; recipients may exercise applicable CC rights themselves. Book text and quotation examples have separate status. **Default OpenAI public terms are reviewed conditionally in §4.1.** | Covered contributor text has reproduction/adaptation grants, not blanket approval of every provider arrangement. The payload does not contain the conll17 matrix or charlm files, so their NC terms are not automatically terms of the dictionary payload. Source exceptions still need their own basis. |
| **U4a. Generated glosses**, split by case | (i) **Copied expression**: a verbatim gloss or example carried into the card. BY-SA §2(a)(1)(a) grants reproduction. (ii) **Adaptation**: an output "derived from or based upon" the Wiktionary text and "translated, altered, arranged, transformed, or otherwise modified" in a way that requires permission (BY-SA §1(a)). BY-SA §2(a)(1)(b) grants adaptation. (iii) **Independent output**: an output not based on the source's expression. No BY-SA grant is needed for it [L]. | Private: none for (i) and (ii). Sharing: §3(a) and §3(b); the adapter's license must be a BY-SA-compatible license (§3(b)(1)). | Factual content is not itself protected expression; whether a given gloss is facts, expression, adaptation, or independent output is case-by-case [L]. | Covered on the BY-SA grant for (i) and (ii). For (iii), no CC grant is needed. Generated output is not automatically rights-free; each output is classified on its content. |
| **U5. Personal APKG export** (one learner) | BY-SA §2(a)(1) for covered Wiktionary-derived fields in the deck. | No CC §3 notice duty for the learner's purely private copies; retain the existing deck notice. | Publicly posting covered material triggers the Sharing analysis. A transfer to another person is not categorized solely by recipient count and is not needed for this personal-export scope. | Covered for contributor-licensed fields, subject to source exceptions. Book text in the deck has its own status and is not covered by these grants (§5). |
| **U6. Shared deck or public hosting** | Not analyzed. | No private-use conclusion is asserted for these other uses. | Public Share triggers BY-SA §3(a), §3(b) and §4(c). NC purpose remains a separate limit for NC-SA material even without public Share. | Outside this note's scope; no new launch prerequisite. |

### 4.1 Default OpenAI provider: coordinating source review

The code sets `defaultLLMBaseURL = "https://api.openai.com/v1"` and uses it when
the operator does not supply a base URL (`internal/enrichment/llm.go:18,62–64`).
The ordinary request uses Chat Completions; an official-OpenAI Batch path also
exists. These are source-code facts, **not** an inspection of this operator's
endpoint, account or secrets [R1].

The [OpenAI Services Agreement][S8] (page states updated December 1, 2025,
effective January 1, 2026; read October 17, 2026) states:

- §4.2: content is used as necessary to provide services, comply with law,
  enforce policies and prevent abuse; it is not used to develop/improve services
  unless the customer explicitly agrees.
- §4.3: the customer must have the rights, licenses and permissions required to
  submit input. §9.2 grants only the limited rights reasonably necessary to
  deliver services, including subcontractors/sub-processors.
- §4.1's input/output ownership language is **as between customer and OpenAI**;
  it does not extinguish a dictionary contributor's or book author's rights.

The [API data-controls documentation][S9] states that API data is not used for
training unless explicitly opted in. Default abuse-monitoring retention is up
to 30 days, with stated exceptions. Chat Completions has endpoint/model-specific
storage and prompt-cache qualifications. Batch and Files can retain application
state until deletion; **do not equate no default training with zero retention**.
No actual account opt-in, special agreement or retention configuration was read.

**Conditional reading [L]:** CC BY-SA's reproduction/adaptation grant and its
direct offer to recipients provide a route for processing covered contributor
text; commercial operation of the provider alone does not add an NC restriction
to BY-SA text. Provider terms requiring sufficient input rights are not a new
grant in unknown quotations. Whether a particular transmission is public Share,
and the broader ToU reuse wording, remain distinct notice questions. These
sources substantially narrow the previous unreviewed-provider gap without
claiming blanket clearance of every payload, every endpoint or the book text.

[S8]: https://openai.com/policies/services-agreement/
[S9]: https://developers.openai.com/api/docs/guides/your-data

## 5. Book text and generated output

- **Book text is outside these grants.** The `example_sentence` sent in U4 and included in U5 is an excerpt from a learner's imported book. The grants analyzed here cover the lexical and model artifacts. They do not license that excerpt, and no CC or LGPL-LR text in this note covers it. The learner's private reproduction of a book they own is outside this note.
- **Generated output is not automatically rights-free.** U4a shows that output is classified by content: it may be copied expression, adaptation, or independent output, and each case has its own status.

## 6. Notice and author/source-history path

**What the texts require for each use.**

- **Private uses (U1–U5).** CC §3 attribution/ShareAlike duties have Sharing triggers. LGPL-LR §1 has notice requirements on verbatim copies it governs. ODC-By §§4.2–4.3 have different Publicly Convey/Use triggers and a definition of Publicly distinct from CC Share. Do not infer that all transmission to a provider is purely private under every text. The broader Wikimedia ToU wording is discussed separately below.
- **Sharing (U6).** BY-SA §3(a)(1)(a) requires retaining the specified creator, copyright, license, disclaimer and source-link information **if supplied by the Licensor**. §3(a)(1)(b) requires indicating modifications and retaining indications of previous modifications; (c) requires identifying the license and including its text or link. §3(a)(2) permits a reasonable manner based on medium, means and context, including a link to the required information [S1].
- **Wikimedia ToU §7 attribution routes** are separate from the CC license conditions. The ToU reuse paragraph lists three options: "Through hyperlink (where possible) or URL to the page or pages that you are reusing (since each page has a history page that lists all contributors, authors and editors)"; "through hyperlink (where possible) or URL to an alternative, stable online copy"; or "through a list of all authors" [S6]. The ToU is the contributor-licensing and attribution text Wikimedia uses. Whether it binds a dump reuser as a contract is not analyzed here. The CC text is the license that applies to the reused text.

**Design proposal (no change made now).** A future author/source history path could store, per index row, the source record's headword, language, part of speech, dump date, and extractor commit (the script already records dump date and Wiktextract commit at `nlp/scripts/derive_dictionary_index.py:521–527`), and then link to the Wiktionary history page for each contributing title. The current index stores the `word` headword as the lemma (`derive_dictionary_index.py:432` region) but not a title-level URL. A page title is not necessarily identical to the stored lemma after normalization, so the mapping needs checking before it is built. This would be a metadata change and an export change, and it is not implemented here.

**What this does not establish.** Current `AttributionNotice` names Wiktionary and CC BY-SA 4.0 / GFDL. It does not itself provide the ToU §7 page/history route for shared material. The CC §3 trigger is public Share; the ToU's own reuse/attribution wording is broader. This note does not settle whether additional contractual ToU duties bind this dump reuser. Keeping a source/history inventory and captured notices is a feasible conservative path, not evidence that current generic attribution already satisfies every applicable duty.

## 7. Inputs that change the matrix

Only two facts change the analysis, and the docs do not settle either:

1. **Purpose.** NC-SA coverage (conll17 artifacts, U1 and U2 for those files) depends on the purpose being "not primarily intended for or directed towards commercial advantage or monetary compensation" [S2 §1(k)]. "Personal" or "unpaid" does not by itself establish that definition. The repository calls itself free software (`README.md`, License), but how the operator actually uses or charges for a deployment is not in the docs. **This note assumes a non-commercial, operator-only purpose.** If that is false, the NC-SA rows drop out for the commercial use.
2. **Provider overrides/settings, if applicable.** The source defaults to OpenAI's API; §4.1 analyzes its public terms conditionally. A different configured endpoint, data-training opt-in, or materially different account agreement changes that part of the reading. No secret values are needed. These facts do not change the underlying CC grant for covered contributor text.

These are the deployment-specific facts relevant here, not requests to reopen the
Italian-equivalent workflow. Artifact and field-coverage questions are documented
in the companion Stanza and Kaikki notes; no human acceptance is implied.

## 8. Sources

All fetches on **2026-10-17** unless marked otherwise. sha256 prefixes are of the bytes fetched.

| ID | Source | Revision / access | Used for |
|---|---|---|---|
| S1 | CC BY-SA 4.0 legal code, `https://creativecommons.org/licenses/by-sa/4.0/legalcode.txt` | sha256 prefix `28a9529c7d0bb4dc` | §§1(a), 1(k), 2(a)(1), 2(a)(4), 2(a)(5), 3(a), 3(b), 4, 6 |
| S2 | CC BY-NC-SA 4.0 legal code, `https://creativecommons.org/licenses/by-nc-sa/4.0/legalcode.txt` | sha256 prefix `e66c269d4819aaab` | §§1(k), 1(l), 2(a)(1), 3(a)(1) |
| S3 | Apache License 2.0, `https://www.apache.org/licenses/LICENSE-2.0.txt` | sha256 prefix `cfc7749b96f63bd3` | §§2, 4 |
| S4 | ODC-By 1.0, `https://opendatacommons.org/licenses/by/1-0/` (HTML) | sha256 prefix `7d7e622bbb4b5f21` | §§2.4, 3.1, 4.2, 4.3; definitions of "Publicly" and "Use" |
| S5 | LGPL-LR, `UD_French-Sequoia` `LICENSE.txt` at `21e377f98e07109500618ea83f1deccd77670d8f`, `https://raw.githubusercontent.com/UniversalDependencies/UD_French-Sequoia/21e377f98e07109500618ea83f1deccd77670d8f/LICENSE.txt` | sha256 prefix `d15c5249ad87636d` | §§0, 1, 2, 3, 4, 5 |
| S6 | Wikimedia Terms of Use §7, `https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use` (page states "went into effect on June 7, 2023") | Text fetched; not hashed | §7 licensing, attribution, importing, re-use |
| S7 | Stanza `stanza/resources/default_packages.py` at `c9a2158a87502dd63f757ab1aa933230b0acbce7`, `https://raw.githubusercontent.com/stanfordnlp/stanza/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/resources/default_packages.py` | Commit from the prior note | Default mappings at lines 182, 201, 338, 342 |
| S8 | [OpenAI Services Agreement](https://openai.com/policies/services-agreement/) | Coordinating review 2026-10-17; page states effective January 1, 2026 | §§4.1–4.3, 9.2; conditional default-provider reading |
| S9 | [API data controls](https://developers.openai.com/api/docs/guides/your-data) | Coordinating review 2026-10-17; reached via platform.openai.com guide redirect | Default no-training, abuse monitoring, endpoint-specific retention |
| R1 | Repository at `6e02d180`: `nlp/src/mouseion_nlp/provision.py:216`; `nlp/scripts/derive_dictionary_index.py:21, 432, 521–527`; `internal/enrichment/llm_codec.go:54, 180–196`; `internal/enrichment/lexical.go:22–34`; `internal/dictionary/attribution.go:4`; `internal/cardexport/cardexport.go:1609–1631`; `doc/adr/0073-modern-greek-language-support.md` (~45–47); `doc/features/language-support.md` (~116); `README.md` (License) | Base commit | §§2, 6, 7 |
| P1 | Prior note: HF stanza-fr README (S12), HF tree (S11), performance page (S14), LINDAT item (S19), Wiktextract LICENSE (S29), §2.4 newswiki search, §3.2 Kaikki search | Prior note at `c90b50de`; **not re-fetched in this revision** unless marked | Coverage and component facts |

## 9. Not done

- No artifact or dump was downloaded. No checksum was computed.
- The LINDAT item, the Stanza performance page, and the HF README were taken from the prior note, not re-fetched.
- Default OpenAI public terms were reviewed in §4.1; the actual operator account,
  endpoint overrides and other providers' terms were not inspected.
- No jurisdiction's private-copying or database rules were applied.
- No code, schema, UI, or export change was made.
