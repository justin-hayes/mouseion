# French resource rights basis: reviewed evidence handoff

**Date:** 2026-10-17. **Repository base:** `6e02d180b1015536127839e0a45953684c0e5a83`.

**Task:** [Obtain a documented French resource rights basis](https://github.com/justin-hayes/mouseion/issues/2155).
**Decision:** [Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141).
**Map:** [Chart French study-language support and resource selection](https://github.com/justin-hayes/mouseion/issues/2138).

## Outcome and scope

Three independent background investigations were reviewed against the cited primary
sources, with overbroad draft conclusions corrected. They establish affirmative
publisher/source grant evidence and intended-use readings, **not a complete,
authoritative reconciliation for every selected artifact and field**. The task
remains open. This is neither adoption nor a finding that the candidates are
unlawful or lack any usable grant.

The human's instruction is the scope: **French should function exactly as Italian
does today**. That means operator-side Stanza provisioning/local analysis, local
Kaikki index derivation/refresh, the existing contextual-gloss/translation provider
path, and personal APKG export. It does not mean importing an Italian legal review
that never occurred, accepting unknowns as limitations, auditing earlier languages,
or expanding into public bundles, public deck sharing or commercial hosting.

The evidence assumes the issue's personal, NonCommercial deployment purpose.
"Personal" is not a substitute for the NC definition if actual use differs.

## Evidence assets

- [French Stanza artifact rights](french-stanza-rights-basis.md): verified manifest,
  eight runtime files, model-card generator, current project statements and
  historical maintainer explanations, vector-source grant and residual lineage.
- [Raw English-Wiktionary Kaikki rights](french-kaikki-rights-basis.md): release
  metadata, collaborator reuse statement, contributor grants, bounded quotation
  evidence and shared-pipeline behavior.
- [Existing grants on the French path](french-equivalent-use-rights.md): express
  license grants and their actual triggers, Italian-equivalent use matrix,
  default OpenAI service/data terms, and feasible source/history/notice path.

These supplement the [prior provenance investigation](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md),
not its historical record. They distinguish a missing documented acceptance from
an absence of all permission evidence.

## What the research answers

### 1. Private copying and derivation have express grant routes

[CC BY-SA 4.0 §2](https://creativecommons.org/licenses/by-sa/4.0/legalcode.txt)
grants reproduction and adaptation of covered material. Its §3 attribution and
ShareAlike duties have public-Sharing triggers. Wholly private derivation does
not, by itself, require publishing the index or licensing unrelated book text.
[CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/legalcode.txt)
also grants private reproduction/adaptation, **for NonCommercial purposes only**.
The purpose limit exists even without public Share. LGPL-LR copying notices and
ODC-By's distinct Publicly Use/Convey definitions must not be collapsed into the
CC rules. See the use matrix for qualifications, including broader ToU wording.

This answers a license-text question; it does not establish every artifact's
coverage. Bespoke permissions or separate legal opinions are not inherently
required merely because a copy is made on a self-hosted server.

### 2. Stanza has two evidenced publisher statements, not no model license

The [pinned French model card](https://huggingface.co/stanfordnlp/stanza-fr/blob/caeb7742148cb0368e4a0f2c68a0f7b09bc22880/README.md)
says Apache-2.0. The [official generator](https://github.com/stanfordnlp/huggingface-models/blob/f50a9e0441b7cd6a708b651e2bf274175f147df5/hugging_stanza.py)
hard-codes that value for every language. Automation explains the lack of
artifact-specific detail; it does **not** invalidate the publisher declaration.

The [currently served Stanza model documentation](https://stanfordnlp.github.io/stanza/performance.html)
publishes a qualified ODC-By grant for Stanford's rights and describes the
underlying-model licensing as unclear. Historical [project-director](https://github.com/stanfordnlp/stanza/issues/3#issuecomment-497058182)
and [maintainer](https://github.com/stanfordnlp/stanza/issues/956#issuecomment-1044169896)
explanations support that reading but do not reconcile these exact 1.15.0 files.
Both texts have conditional private-use routes; their coexistence is not itself
an express private-use prohibition.

The [CoNLL17 deposit](https://lindat.mff.cuni.cz/repository/items/053071c7-84f7-4d0b-bdd7-f87cbafcd3a9)
expressly licenses the embeddings under CC BY-NC-SA 4.0. Stanza's source scripts
support their lineage to its runtime `conll17` matrix, but the hosted conversion
was not independently reconstructed. This is direct input-license evidence,
not an argument that fine-tuned weights necessarily inherit source terms.

### 3. Kaikki has positive publisher-side reuse evidence

In [Torrent for kaikki.org Wiktextract raw data?](https://github.com/tatuylonen/wiktextract/issues/1721#issuecomment-6208854795),
a repository collaborator says the JSON is "open source and libre (thanks to the
licenses used by Wiktionary)" and torrenting it is "completely ok". This is
materially better evidence than the earlier search of three web pages. It is not
a release-specific warranty or a separately enumerated database-right waiver.

[Wikimedia ToU §7](https://foundation.wikimedia.org/wiki/Policy:Terms_of_Use#7._Licensing_of_Content)
and the [dumps notice](https://dumps.wikimedia.org/legal.html) provide the grant
chain for covered contributor text. A separate Kaikki notice is not inherently
needed just to reuse that already-licensed text. Any independently protectable
additions/database layer must be analyzed rather than presumed to exist.

### 4. The quotation concern is concrete, shared and field-specific

A capped 3,000,000-byte raw-file probe found French and Italian quotation-type
examples with bibliographic references. The derivation retains their text but
drops `ref`; dictionary examples can enter provider candidate-sense payloads.
They are not directly exported as dictionary example fields in the current
card-export code. The probe's hash, command and parser are in the Kaikki note;
no full dictionary was downloaded and no quotation-rate estimate is claimed.

Some quotations are public-domain or compatibly licensed; others rely on
exceptions or have an unestablished basis. A `quotation` label proves neither
permission nor infringement. Existing grants for contributor prose are not a
warranty for every quoted work. Preserving citations and establishing a basis,
or conservatively excluding unresolved examples, are possible paths requiring
an explicit choice, **not changes made here**.

### 5. The default provider is researchable without reading secrets

The source defaults to OpenAI's API. Its [Services Agreement](https://openai.com/policies/services-agreement/)
limits service-related content use and requires adequate input rights; the
[API data documentation](https://developers.openai.com/api/docs/guides/your-data)
states no default training opt-in but documents retention, including endpoint
differences for Chat Completions, Files and Batch. This narrows the previously
unreviewed-provider gap. It does not reveal this operator's overrides/settings,
license unknown quotations, or supply rights in learner book sentences.

## What remains, precisely

| Item | Finding and remaining route |
| --- | --- |
| Reconciled coverage of the eight Stanza files | Publisher grants and reuse encouragement exist. Exact training snapshots, `newswiki` corpus/terms, retained notices and reconciliation remain undocumented in the examined sources. An authoritative artifact-specific notice/clarification or supported human/legal interpretation may settle material residual issues; no universal prohibition is inferred. |
| Effect of source terms on trained weights | Do not presume either automatic inheritance or automatic independence. In particular, any LGPL-LR weight obligations and underlying-text rights need a supported applicability reading. Running-program permission does not alone settle copying weights. |
| Kaikki output layer, if independently protectable | The collaborator statement supports reuse through Wiktionary licenses. A separate right/grant must not be invented from silence. Whether that evidence plus existing grants resolves the selected raw release's independent rights remains an interpretation question, not an automatic demand to obtain bespoke permission. |
| Quotation examples and feasible notices | Current citation loss is established. Document a basis for retained expression or choose an exclusion boundary; review the proposed source-title/history, modification and license-notice inventory. A generic provider-name notice alone is not a complete source-history path. |
| Deployment facts if they differ from scope | Commercial purpose, another provider, data-training opt-ins or materially different account terms change the conditional use analysis. No further workflow choice or re-approval of Italian-equivalent sentence transmission is requested. |

Checksums, exact upstream Kaikki input filename, private regeneration configuration
and moving Stanza tags are separately recorded **reproducibility gaps**, not proof
that grants are absent. Full-byte verification remains for future authorized
provisioning. The Kaikki date may describe completion of Wikimedia's September 1
dump run on September 2; this is a candidate explanation, not a verified input pin.

## Handoff boundary

The requested research was performed. The original rights-basis task is **not
closed merely because the source passes finished**: no new maintainer response,
complete reconciled artifact-specific notice, binding legal opinion, final
human interpretation or adoption was obtained. No application behavior, existing
language support, prototype-fetch gate or resource-selection decision is changed.

The remaining work is narrower than a blanket request for permission: assess the
evidenced grants and material exceptions above. If the human requires an explicit
publisher reconciliation rather than a supported interpretation, the Stanza
note contains the targeted factual inquiry. New outreach is not sent on the
human's behalf by this research handoff.
