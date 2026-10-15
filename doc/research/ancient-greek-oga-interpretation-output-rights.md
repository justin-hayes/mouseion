# Operation-specific rights evidence for the retained OGA Book 1 interpretation output

Bounded evidence pass for
[Clear operation-specific rights for the retained OGA interpretation output](https://github.com/justin-hayes/mouseion/issues/2054),
as [authorized](https://github.com/justin-hayes/mouseion/issues/2054#issuecomment-6190129651)
after [triage](https://github.com/justin-hayes/mouseion/issues/2054#issuecomment-6189985399).
Date: 2026-10-15. Researcher: Claude Code Opus 5.5.

**Status: analytical input to the human disposition. This is not a rights
clearance, a legal opinion or a release approval.** It grants no permission, and
agreement with it would not grant one either. The issue stays `enhancement` +
`ready-for-human`. The downstream gates,
[catalog publication](https://github.com/justin-hayes/mouseion/issues/2056) and
[provider payloads/deck exports](https://github.com/justin-hayes/mouseion/issues/2055),
are not cleared by anything here.

Controlling decisions, preserved unchanged:

- [planning-source acceptance](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6188837579);
- [per-work packaging resolution](https://github.com/justin-hayes/mouseion/issues/2053#issuecomment-6188570169)
  and its [immutable findings](https://github.com/justin-hayes/mouseion/blob/50a93846fd8ddab68751c577d2d8bccd23fa3c5a/doc/research/ancient-greek-oga-per-work-packaging.md);
- [licensing policy](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026);
- [architecture resolution](https://github.com/justin-hayes/mouseion/issues/2031#issuecomment-6189488434),
  i.e. [ADR 0098](https://github.com/justin-hayes/mouseion/blob/8341cef8121e47b9915e365059c040c4cfecc4aa/doc/adr/0098-shared-core-classical-acquisition.md)
  and the [staged handoff](https://github.com/justin-hayes/mouseion/blob/8341cef8121e47b9915e365059c040c4cfecc4aa/doc/features/annotation-backed-ancient-greek.md).

## Summary

1. **Third-party dependence is narrower than "the interpretation", but it is
   real.** The retained repaired ledger was verified read-only against its
   recorded hash. Of 10,462 printed nodes, almost every canonical value either
   carries an OGA (CC BY-SA 4.0) value through unchanged or comes from a
   Mouseion rule over OGA syntax. The output-specific third-party dependence
   falls in three places:
   - **Morpheus (CC BY-SA 3.0 US).** 149 lemma values depend on it:
     - 141 numbered-label nodes, where Morpheus *confirms* an OGA-supplied
       headword and number;
     - one compound record, plus six of the seven citation repairs, whose
       headword *is* a decoded Morpheus key.
   - **UD Perseus (CC BY-NC-SA 2.5): 756 conjunction UPOS values.** These are
     28 distinct lemma-level CCONJ/SCONJ decisions, drawn from 29 raw labels.
     All 756 nodes are non-vocabulary either way. In the proof's code they
     affect neither the 438 selected identities nor the 434 sentence inputs.
   - **UD Perseus as lineage/validator** for the fixed ten-letter tag→UPOS
     table. The table is a Mouseion code constant. The proof only checks it
     against UD Perseus and fails closed if they disagree.
2. **The licence texts settle what each licence governs, and when.** They do not
   settle whether these particular values are protected expression.
   - The 2.5 and 3.0 US licences reach only "the copyrightable work of
     authorship".
   - BY-SA 4.0 reaches Copyright and Similar Rights, including sui generis
     database rights, but only rights that apply to Your use.
   - All three disclaim limiting uses that need no permission.

   So the decisive question for each value class is factual and legal: does
   storing or Sharing that value exercise an exclusive right in the
   witness? The named evidence cannot answer that. Neither "causally derived,
   so licensed" nor "factual-looking, so exempt" is established.
3. **Private local interpretation/retention:**
   - **BY-SA 4.0 members:** supported. §3 conditions attach only to Sharing.
   - **Morpheus-dependent values:** no distribution condition is triggered.
     *If* the frozen values are a Derivative Work, BY-SA 3.0 US §3(b)
     conditions even *creating* one on labelling the changes.
   - **UD-dependent values:** *if* the decisions exercise any licensed right,
     BY-NC-SA 2.5 §4(c) limits every exercise, private ones included, to
     non-commercial purposes.
4. **Redistribution and catalog hosting of a frozen witness-free
   interpretation:**
   - **OGA-derived content:** supported with conditions, namely BY-SA 4.0
     attribution, ShareAlike for Adapted Material and no additional
     restrictions.
   - **Morpheus-dependent values: unresolved.** If they attach, the 3.0 US
     §4(b) text allows a later version with the same License Elements, which
     reads on BY-SA 4.0. Its Perseus "offer modifications" sentence is
     unresolved.
   - **UD-dependent conjunction decisions: unresolved, and restricted if they
     attach.** If they are a Derivative Work, BY-NC-SA 2.5 §4(b)–(c) and BY-SA
     4.0 §3(b) cannot both be met for one Adapted Material. That would be a
     real incompatibility for that artifact form, not something a notice can
     fix.
5. **Provider transmission and exports:** not approved here. The proof's
   card-relevant fields involve OGA content and, for 14 of the 438 selected
   identities, Morpheus-dependent headwords. They involve no UD-dependent
   conjunction value. Whether any operation is Sharing, plus each provider's
   terms, remain with
   [the provider/export gate](https://github.com/justin-hayes/mouseion/issues/2055).

**Stop.** Applicability for the Morpheus- and UD-dependent value classes is a
legal/policy judgment that the permitted primary evidence does not settle. No
further research, replacement rule or outreach is started.

## 1. Exact inventory and pins

### Retained pins (verified, not re-acquired)

| Item | Pin | Verification in this pass |
| --- | --- | --- |
| OGA release | v0.2.0, Zenodo 14206061; archive SHA-256 `fffc246e…ecf6c3` | Taken from the retained [repair records](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus-repair-records.json) `component` block; archive not reopened |
| Edition/text | Marchant 1904; canonical-greekLit `0.0.11376425141` / `476fb613…` | As recorded in [packaging findings](https://github.com/justin-hayes/mouseion/blob/50a93846fd8ddab68751c577d2d8bccd23fa3c5a/doc/research/ancient-greek-oga-per-work-packaging.md#source-revision-notices) |
| Reading unit | Book 1, 1.1.1–1.10.19 | Unchanged |
| Source core package | 31 members, 42,300,425 bytes; ZIP 4,234,495 bytes, SHA-256 `19bc870c…6d76`; private, unpublished | Not rebuilt (no artifact assembly) |
| Interpretation proof | `e610a5aa6c77c25864114ef9604ac9d50515039b` (`opus_completion.py`, `opus_repair.py`, `opus-repair-records.json`) | Code and records read with `git show`; the retained proof worktree is at the same `HEAD` |
| Rulesets | `oga-greta-lemma/2`, `oga-agdt-upos/3` (εἰμί v3) + `oga-anabasis1-research-repairs/1`, `oga-reading-terminal/2`, `ancient-greek/1`, `mouseion-3-15-single-book-empty-known/1`, `grc-earliest-capped/1` | [`opus-repair-summary.json`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus-repair-summary.json) `rulesets` |
| Repaired private outputs | `node-ledger.json` `f5a61b43…f19fc`; `selection-model.tsv` `855ea59e…6076`; `card-inputs.json` `f0686b17…2789`; `analysis.json` `a8b652ef…91d2` | Rehashed read-only; all match the committed summary |
| Morpheus witness | PerseusDL/morpheus `b1b33c56…`; index `57f81c86…`, 188,174 rows over 84 stem-source files, 8 stem files retained in full | Index manifest read; witness files grepped for licence/copyright statements only |
| UD Perseus witness | `331ddef9…`, train/dev/test (202,989 tokens) | Retained `LICENSE.txt` hash `1e610c18…af` equals the freshly fetched pinned file |

### Emitted interpretation components (what a frozen artifact would have to carry)

The counts come from the hash-verified repaired node ledger and the committed
summary. The code citations show where each value comes from.

| # | Component (rule) | Nodes / items | Where the emitted value comes from | Third-party dependence |
| --- | --- | ---: | --- | --- |
| A | Native evidence carried through: surface, span, raw lemma, native POS letter and features, relation/head, CTS, node IDs | 10,572 nodes (10,462 printed) | OGA PAULA/CoNLL-U members | None beyond OGA/Perseus BY-SA 4.0 |
| B | Plain lemma (`I-PLAIN`) | 9,156 | The headword *is* the OGA label ([`interpret_lemma`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L280-L323)). Morpheus is consulted only to set `attested_key` (8,926 true, 230 false). It changes an outcome only for orthographically flagged labels, and no `I-PLAIN` node is flagged | Morpheus lookup has no outcome effect on Book 1; the boolean is a Morpheus-derived flag |
| C | Numbered lemma (`I-NUM-MORPHEUS-KEY`) | 141 nodes, 20 labels, 110 eligible | Headword and digits are split from the **OGA label**. The discriminator is `{scheme: "morpheus", value: <OGA digits>}`. Morpheus supplies the existence check that makes the rule apply, plus proof-ledger fields `morpheus_key` (Beta code), `declared.stem_classes`/`compound_bases` and witness `file:line` | Outcome-determining: without the exact key the rule falls to `I-UNRESOLVED-NUMBERED`. Values are mostly OGA strings; the optional fields are Morpheus strings |
| D | Compound record (`I-COMPOUND-RECORD`) | 1 node | The headword is **decoded from the Morpheus key** of a unique compound record | Morpheus-sourced value |
| E | Citation repairs (`R-CITATION-REPAIR`) | 7 nodes (5 labels) | Headword decoded from `after.morpheus_key` (6 nodes) or from a Mouseion-formed `formed_key` (1 node), checked against the Morpheus index ([`repaired_lemma`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_repair.py#L203-L220)). Three records also cite other Book 1 OGA nodes carrying the same headword. The supporting evidence includes **UD Perseus lemma/form counts** ([`verify_evidence`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_repair.py#L183-L200)) | Morpheus-sourced value; UD Perseus witness for the choice, but not the value source |
| F | Conjunction UPOS (`P-C-PAIR`) | 756 nodes: 541 CCONJ, 215 SCONJ; 29 raw labels → 28 (lemma, UPOS) decisions; all `non-vocabulary` | A (letter `c`, lemma) pair must be single-valued in UD Perseus counts ([`interpret_pos`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L426-L451)). 31 of the 756 also depend on C, because the pair key uses the Morpheus-confirmed headword | UD Perseus-sourced decision. It does not affect vocabulary status ([`vocabulary_status`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L454-L463)), selection or card inputs ([`card_inputs`/`card_rejections`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L638-L678)) |
| G | Letter table (`P-LETTER-*`) | 10-letter table, 8 letters occurring; 7,740 printed nodes in the repaired ledger (incl. 1,157 punctuation) | A fixed Mouseion constant ([L98–L100](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L98-L100)), described as the producer-lineage UD Perseus conversion. The proof aborts unless UD Perseus confirms each letter is single-valued ([`ud_perseus_evidence`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_completion.py#L351-L381)) | UD Perseus used as lineage and validator; it is not consulted per node for the value. The proof ledger's `pos.pair_attested` adds a UD Perseus count to every POS record |
| H | Verb/εἰμί UPOS (`P-V-VERB`, `P-V-EIMI-*` v3) | 1,788 + 172 | Mouseion rule over OGA native syntax ([`eimi_rule_v3`](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/opus_repair.py#L81-L96)), justified from UD documentation definitions | No witness-data dependence found; UD documentation is cited, not embedded |
| I | POS repairs (`R-POS-REPAIR`) | 6 | `after_upos` (AUX/VERB) checked against cited native OGA edges; reasons cite UD documentation categories | OGA plus Mouseion judgment |
| J | Mouseion-authored material | — | Rule code, ruleset/version identifiers, repair reasons and uncertainty text, manifests, hashes and counts | Mouseion; the repository code licence is AGPL-3.0 (GitHub `licenseInfo`) |
| K | Learner-facing derivatives | 1,426 identities, 438 selected, 434 sentence inputs | Computed from A–I. **14 of the 438 selected identities** have at least one Morpheus-dependent occurrence (C/D/E); 11 depend wholly on it and carry a `#morpheus:` discriminator. 4 include a repaired occurrence. **None** depends on F | Inherits C/D/E for those 14 identities; sentence text is OGA/Perseus BY-SA 4.0 |

**Excluded from the runtime (proof-only witnesses):** the Morpheus index and
stem sources, the UD Perseus treebank, producer and UD documentation, and Gorman
(not used). ADR 0098 requires any frozen runtime to work without them
([§Catalog packages](https://github.com/justin-hayes/mouseion/blob/8341cef8121e47b9915e365059c040c4cfecc4aa/doc/adr/0098-shared-core-classical-acquisition.md);
[Stage 1](https://github.com/justin-hayes/mouseion/blob/8341cef8121e47b9915e365059c040c4cfecc4aa/doc/features/annotation-backed-ancient-greek.md)).

**Not built yet.** No witness-free interpretation artifact exists, and none was
assembled here. Its field selection is therefore still open, and that choice
matters for rights. A runtime artifact *could* carry only the values: headword,
discriminator and UPOS per node, plus repair values and provenance. It *could
also* carry the proof ledger's Morpheus key strings, stem classes, witness
locations and per-node UD counts. The second set adds third-party material the
runtime does not need, so it is a distinct, larger operation for review.

A further architectural fact: under ADR 0098, decoder and mapping *code* ships in
reviewed Mouseion releases, while catalog packages are *data*. A frozen table of
C/D/E/F values could therefore end up embedded in a public AGPL-3.0 code release,
or in a separately noticed data artifact. These are different distribution
operations (see Q6).

**Already public, not re-assessed:** the research branches in this public
repository already contain short Morpheus and UD Perseus witnesses and the repair
values. The [import-proof notice](https://github.com/justin-hayes/mouseion/blob/e610a5aa6c77c25864114ef9604ac9d50515039b/doc/research/ancient-greek-oga-anabasis1-import-proof/NOTICE.md)
assessed that as a non-commercial research report with separately attributed
excerpts. It says it "does not clear app-data reuse, full-source redistribution,
commercial reuse of NC witnesses, decks/exports, bundling or provider use".
This pass leaves that prior assessment as it stands.

## 2. What the primary texts actually say

Only the operative language is quoted. Section letters follow each legal code.

### Witness declarations

- **Morpheus README at `b1b33c56`**
  ([source](https://github.com/PerseusDL/morpheus/blob/b1b33c56ef2338fe0dcd1893628ed638f00c0986/README.md);
  the fetched bytes equal the retained copy, SHA-256 `5c5342e0…5fc0`):
  - "Unless otherwise indicated, all contents of this repository are licensed
    under a Creative Commons Attribution-ShareAlike 3.0 United States License.
    You must offer Perseus any modifications you make."
  - It also says the Perseus DL materials "are provided for the personal use of
    students, scholars, and the public" and "have varying copyright status".
  - No "otherwise indicated" statement was found in the eight retained stem
    files or the three retained Morpheus documentation files. The other 76
    indexed stem sources are not retained in full, so they were not checked.
- **UD_Ancient_Greek-Perseus `LICENSE.txt` at `331ddef9`**
  ([source](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/LICENSE.txt)):
  - "This work is licensed under the Creative Commons Attribution-NonCommercial-
    ShareAlike 2.5 Generic License."
  - Its [README](https://github.com/UniversalDependencies/UD_Ancient_Greek-Perseus/blob/331ddef91411d0e6549744ee889e05549e6da77d/README.md)
    calls the treebank "an automatic conversion" of AGDT 2.1. It names the
    annotators and editors, and it says morphology and lemmatization "have been
    performed with the help of the Morpheus morphological analyzer".
  - Its work table does not list Xenophon. **No Anabasis text passes through UD
    Perseus**; the decisions in F are lemma-level aggregates over other works.

### CC BY-NC-SA 2.5 Generic ([legal code](https://creativecommons.org/licenses/by-nc-sa/2.5/legalcode))

- **§1.** "'Work' means the copyrightable work of authorship offered under the
  terms of this License." A "Derivative Work" is "a work based upon the Work …
  or any other form in which the Work may be recast, transformed, or adapted".
- **§2.** Nothing in the licence restricts "rights arising from fair use … or
  other limitations on the exclusive rights … under copyright law".
- **§3.** It grants rights to reproduce, "to create and reproduce Derivative
  Works", and to distribute both. "All rights not expressly granted by Licensor
  are hereby reserved."
- **§4(b).** "You may distribute … a Derivative Work only under the terms of
  this License, a later version of this License with the same License
  Elements…", and with no terms that "alter or restrict" recipients' rights.
- **§4(c).** "You may not exercise any of the rights granted to You in Section 3
  above in any manner that is primarily intended for or directed toward
  commercial advantage or private monetary compensation." This reaches
  reproduction and the creation of Derivative Works, not only distribution.
- **§4(d).** On distribution: keep copyright notices intact; name the Original
  Author and Attribution Parties, the title and the URI; and "in the case of a
  Derivative Work, a credit identifying the use of the Work".
- There is no database-rights clause.

### CC BY-SA 3.0 United States ([legal code](https://creativecommons.org/licenses/by-sa/3.0/us/legalcode))

- **§1.** It defines "Work" and "Derivative Work" the same way as 2.5.
- **§3(b).** "to create and reproduce Derivative Works **provided that** any such
  Derivative Work … takes reasonable steps to clearly label, demarcate or
  otherwise identify that changes were made to the original Work." The
  condition attaches at creation.
- **§3, last paragraph.** "All rights not expressly granted by Licensor are
  hereby reserved."
- **§4(b).** A Derivative Work may be distributed under "(ii) a later version of
  this License with the same License Elements…", "(iii) … Unported … or a
  Creative Commons jurisdiction license … with the same License Elements", or
  "(iv) a Creative Commons Compatible License". The distributor must include
  the licence or its URI, impose no restricting terms and keep notices intact.
- **§4(c).** Attribution as in 2.5, including the Derivative-Work credit. It
  adds no implied endorsement.
- **§2** (fair use) as in 2.5. There is no database-rights clause.

### CC BY-SA 4.0 International ([legal code](https://creativecommons.org/licenses/by-sa/4.0/legalcode)), for the OGA and canonical-greekLit members

- **§1(a).** Adapted Material is material "derived from or based upon the
  Licensed Material and in which the Licensed Material is translated, altered,
  arranged, transformed, or otherwise modified **in a manner requiring
  permission** under the Copyright and Similar Rights held by the Licensor."
- **§1(d).** Copyright and Similar Rights include "Sui Generis Database Rights".
- **§1(i).** Licensed Rights are "limited to all Copyright and Similar Rights
  that apply to Your use of the Licensed Material".
- **§1(k).** To "Share" means "to provide material to the public by any means or
  process that requires permission under the Licensed Rights".
- **§2(a)(1).** The licence grants the rights to "reproduce and Share" and to
  "produce, reproduce, and Share Adapted Material".
- **§2(a)(2).** Where Exceptions and Limitations apply, "this Public License
  does not apply".
- **§2(a)(4).** Technical modifications "never produce" Adapted Material.
- **§2(a)(5)(C).** No downstream restrictions.
- **§3(a).** These conditions apply "If You Share": attribution, notices, the
  URI, modification indication and licence indication.
- **§3(b).** These apply "if You Share Adapted Material": the Adapter's License
  must be "a Creative Commons license with the same License Elements, this
  version or later, or a BY-SA Compatible License", and you may not impose
  restricting terms.
- **§4(a)–(c).** Extracting a substantial portion is permitted. A database that
  includes a substantial portion and in which You have sui generis rights is
  Adapted Material, but "not its individual contents". §3(a) applies when
  Sharing a substantial portion.
- **§8(a).** The licence does not "impose conditions on any use … that could
  lawfully be made without permission".

## 3. Source-member rights versus output rights

**Source members.** The packaging findings showed explicit CC BY-SA 4.0
declarations with no file-level exception, which supports conditional
core-package redistribution. That covers the members. It does not cover outputs
that depend on Morpheus or UD Perseus. The policy says so directly:

> rights in text, annotations, conversions and derived artifacts remain separate
> ([ADR 0097](https://github.com/justin-hayes/mouseion/blob/8341cef8121e47b9915e365059c040c4cfecc4aa/doc/adr/0097-classical-evidence-refresh-adoption-and-retention.md))

and

> Do not assume that ShareAlike necessarily applies to every count, or that
> statistical outputs are automatically unrestricted
> ([policy §4](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026))

**The trigger in every licence is the exercise of a right the licence
governs.** The 2.5 and 3.0 US licences operate on "the copyrightable work of
authorship". BY-SA 4.0 operates on rights "that apply to Your use". All three
leave alone uses that need no permission (2.5/3.0 US §2; 4.0 §2(a)(2) and
§8(a)). The licences therefore attach to C/D/E/F only if storing or Sharing
those values reproduces or adapts protected expression in the witness, or uses
another right the licence covers. That depends on the facts of each class:

- **C (141 nodes).** The emitted headword and digits are OGA's own BY-SA 4.0
  strings. Morpheus contributes a yes/no existence check and the *meaning* of
  the namespace. Whether a confirmation reproduces Morpheus expression is
  unresolved. The analytical expectation is that it is weaker than D/E. It
  changes if the artifact carries Morpheus key strings, stem classes or witness
  locations.
- **D/E (7 values; 6 decoded Morpheus keys, 1 Mouseion-formed).** These are
  individual dictionary citation forms taken from a Morpheus record, chosen
  under Mouseion's reasons. For three of the five labels, other Book 1 OGA
  nodes independently carry the same headword. Whether isolated headwords, or
  this small selection of them, are protected expression or adaptation of the
  stemlib is unresolved.
- **F (28 decisions).** Each is a binary class per lemma, derived from whether
  aggregate counts over a 202,989-token treebank are single-valued. No UD text,
  tree or annotation sequence is reproduced. Whether such an aggregate
  classification is "recast, transformed, or adapted" from a copyrightable work
  (2.5 §1) is unresolved. So is whether any copyright subsists in the parts
  consulted. The analytical expectation is that this is closer to a statistical
  fact than D/E, but **it is not established**, and the policy forbids assuming
  it.
- **G (10-letter table).** This is Mouseion code describing a tag-set
  correspondence. UD Perseus checks it but does not supply it per node.
  Applicability is unresolved; the dependence is weaker than F.

**Database rights.** BY-SA 4.0 §4 addresses OGA's sui generis rights expressly.
The 2.5 Generic and 3.0 US codes license only copyright in "the Work" and
reserve "all rights not expressly granted". *If* a sui generis database right
existed in UD Perseus or Morpheus and were engaged by extracting the counts or
key lookups, neither licence would grant it. Whether such a right exists (it
turns on maker nationality/establishment and substantial investment) and whether
Mouseion's extraction is "substantial" is **not assessed**. The one attempt to
read Directive 96/9/EC on EUR-Lex returned HTTP 202 with an empty body (an
access challenge). It was not retried.

### Version and NC interactions, only where the terms attach

- **Morpheus → BY-SA 4.0 artifact.** *If* C/D/E were Derivative Work of a
  BY-SA 3.0 US Work, §4(b)(ii) allows "a later version of this License with the
  same License Elements". Read literally, that covers BY-SA 4.0 (Attribution,
  ShareAlike). No CC interpretive or compatibility page was consulted, so this
  is a textual reading only. On that reading a combined BY-SA 4.0 interpretation
  artifact is not excluded. Two conditions would apply on top:
  - the §3(b) change-labelling and §4(c) credit;
  - the "offer Perseus any modifications" sentence. Whether that sentence binds
    is unresolved, and complying with it would mean contacting Perseus, which is
    unauthorized.
- **UD Perseus → BY-SA 4.0 artifact.** *If* F were a Derivative Work under
  BY-NC-SA 2.5, then 2.5 §4(b) allows distribution "only under" the same licence
  or a later one with the same elements (NC included). Meanwhile BY-SA 4.0
  §3(b)(1)/(3) requires any Adapted Material that includes OGA content to carry a
  BY-SA licence and no additional restrictions. **No licence satisfies both for a
  single Adapted Material.** That would be a real incompatibility, not a notice
  problem.

  It is unresolved whether keeping F in a separate, separately licensed
  component avoids this:
  - The 2.5 "Collective Work" carve-out requires the Work "in its entirety in
    unmodified form", which a 28-value derivative is not.
  - BY-SA 4.0 ShareAlike reaches only Adapted Material "derived from or based
    upon" OGA content.
- **NC for private use.** *If* F, or the witness use behind it, exercises a
  licensed right, then 2.5 §4(c) limits even private reproduction and derivation
  to non-commercial purposes. The [policy](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026)
  accepts NC data "for permitted personal, non-commercial use through explicit
  opt-in" and does not bundle or mirror it.
- **No automatic contamination.** None of these effects follows from derivation
  alone. Each depends on the applicability question above.

## 4. Component × operation matrix (analytical input, not clearance)

Legend: **Supported** = a primary grant reaches the operation, under the
conditions listed. **Restricted** = a primary term limits or forbids the
operation *if it attaches*. **Unresolved** = applicability is not settled by the
named evidence. Under the policy, that operation stays blocked.

| Component | O1 Private local interpretation/retention | O2 Redistribute frozen witness-free interpretation | O3 Catalog hosting (Mouseion-hosted package/index) | O4 Provider transmission | O5 Exports (decks/APKG/analysis) |
| --- | --- | --- | --- | --- | --- |
| Source members (OGA/canonical, BY-SA 4.0) | **Supported.** §2(a)(1); §3 applies only on Sharing | **Supported with conditions.** §3(a); §3(b) for Adapted Material; §2(a)(5)(C) | **Supported with conditions** as the packaging findings record; publication approval and owner sit with [#2056](https://github.com/justin-hayes/mouseion/issues/2056) | Not approved. Whether it is Sharing, and the provider's terms, are unknown ([#2055](https://github.com/justin-hayes/mouseion/issues/2055)) | Not approved. When Shared: §3(a), plus §3(b) for Adapted Material |
| A, B, H, I (OGA-carried values; Mouseion rules over OGA syntax) | **Supported** under BY-SA 4.0. The Mouseion rules and reasons are Mouseion's own | **Supported with BY-SA 4.0 conditions**, treating the artifact conservatively as possible Adapted Material (§1(a)) | Same as O2 | As above | As above |
| C (141 numbered nodes; OGA strings + Morpheus confirmation) | No 3.0 US distribution term is triggered. §3(b) labelling only if the values are a Derivative Work. **Unresolved, low exposure in this analysis** | **Unresolved.** If the terms attach: 3.0 US §4(b)–(c) credit/licence; BY-SA 4.0 is readable as a permitted later version; the Perseus modifications sentence is open. Optional Morpheus key/stem/witness fields widen this | Same as O2 | Concerns the headword/discriminator of the 14 selected identities touching C/D/E (11 wholly); not approved | Same as O4 |
| D, E (7 Morpheus-sourced headwords; 1 formed) | As C, with a stronger value dependence. **Unresolved** | **Unresolved**; same conditional obligations as C | Same as O2 | As C | As C |
| F (756 conjunction UPOS / 28 decisions; UD-sourced) | **Unresolved.** If the terms attach: **restricted** to non-commercial purposes (2.5 §4(c)); policy opt-in mode | **Unresolved; restricted if the terms attach.** NC-SA only (2.5 §4(b)–(c)), which conflicts with BY-SA 4.0 §3(b) inside one Adapted Material. Blocked under the policy, which bars bundling NC data | **Unresolved; restricted if the terms attach.** The policy says NC data is "not bundled or mirrored by Mouseion" | Not in any card-relevant field in the proof. Not approved | Present only if an export includes non-vocabulary UPOS/analysis; then same as O2 |
| G (10-letter table; UD as lineage/validator) | **Unresolved**; weaker dependence | **Unresolved** | **Unresolved** | Not in payload | Only in analysis exports |
| J (Mouseion code, reasons, manifests) | Mouseion's own; AGPL-3.0 code licence | Mouseion's own. Dataset terms do not relicense code ([policy §4](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026)) | Same | — | — |
| K (identities/selection/434 inputs) | Inherits A–E. **Supported** for the OGA-only identities; the 14 Morpheus-touching identities inherit C/D/E | Inherits; unresolved for the 14 | Inherits | Downstream gate | Downstream gate |
| Proof witnesses (Morpheus, UD Perseus) | Retained privately as research evidence ([ADR 0098](https://github.com/justin-hayes/mouseion/blob/8341cef8121e47b9915e365059c040c4cfecc4aa/doc/adr/0098-shared-core-classical-acquisition.md) keeps them) | **Excluded**; not part of any runtime | **Excluded** (UD Perseus also blocked by the policy) | Excluded | Excluded |

The one firm restriction the texts establish without resolving applicability is
this: under the existing policy, the UD Perseus witnesses themselves are not
redistributable by Mouseion, and NC terms, *where they attach*, cannot coexist
with BY-SA 4.0 ShareAlike in one Adapted Material.

## 5. Conditional notice obligations (apply only if the terms attach)

- **OGA/Perseus BY-SA 4.0, when Shared** (applies to O2/O3 for all OGA-derived
  content):
  - creators and citation;
  - copyright and licence notice with the URI;
  - warranty-disclaimer reference;
  - a link to the material;
  - a statement that Mouseion modified it ("Mouseion research interpretation
    `oga-anabasis1-research-repairs/1` …"), keeping prior OGA modification
    notices;
  - for Adapted Material: a BY-SA 4.0 (or later/compatible) Adapter's License
    and no additional restrictions or effective technological measures.
- **Morpheus, if C/D/E are Derivative Work:**
  - a change label at creation (3.0 US §3(b));
  - on distribution:
    - licence/URI;
    - intact notices;
    - credit to Tufts University / Perseus Digital Library;
    - title and URI;
    - a credit "identifying the use of the Work" (§4(c));
    - no implied endorsement.
  - Display Tufts' general notices. The "offer Perseus any modifications"
    sentence is an open human question.
- **UD Perseus, if F is a Derivative Work:**
  - on distribution, BY-NC-SA 2.5 or a later NC-SA licence and URI;
  - intact notices;
  - the README-named annotators/editors and the converter (Celano), and the
    title;
  - a use credit (§4(d));
  - a non-commercial restriction. Because of §3, that cannot sit inside a BY-SA
    4.0 Adapted Material.
- **If applicability is decided against attachment:** the policy still requires
  retained provenance and a transformation notice.

## 6. Human decision frontier

### Questions for law/policy judgment (cannot be settled from the named evidence)

1. **Q1.** Does a frozen artifact carrying class C (OGA headword/digits plus a
   Morpheus existence confirmation, with or without Morpheus key strings)
   exercise any right in Morpheus "copyrightable work of authorship"?
2. **Q2.** Do the seven D/E citation values, taken individually from Morpheus
   records, constitute reproduction or Derivative Work of the stemlib?
3. **Q3.** Do the 28 F decisions, or the G table check, reproduce or adapt
   protected expression in UD Perseus? Or do they fall outside the licence as
   unprotected aggregate facts?
4. **Q4.** If Q3 is "attaches": is any separation of F into a separately
   licensed component acceptable? Or does that operation stay blocked because NC
   and BY-SA 4.0 ShareAlike cannot both apply to one Adapted Material?
5. **Q5.** Does the Morpheus "You must offer Perseus any modifications you make"
   sentence bind Mouseion's distribution of C/D/E? Compliance would need contact
   with Perseus, which is not authorized.
6. **Q6.** If any C–G values ship, should they ship in AGPL-3.0 release code or
   in a separately noticed data artifact? The policy forbids automatic
   relicensing in either direction.
7. **Q7.** Are self-hosted Mouseion installations and Mouseion's own hosting
   "non-commercial" in the 2.5 §4(c) sense? This matters if F attaches, even
   privately.

### Missing inventory or text evidence (not judgments)

- The actual witness-free artifact's field list does not exist yet. Q1–Q3
  exposure depends on whether it carries only values or also the Morpheus keys,
  stem classes, witness locations or UD counts.
- 76 of 84 Morpheus stem sources are not retained in full, so the "unless
  otherwise indicated" scan covers only the eight retained files.
- Sui generis database-right applicability was not read: the EUR-Lex fetch was
  challenged and not retried. CC interpretive and compatibility pages were not
  consulted.

### Precise stop

The named primary evidence establishes:

- the inventory;
- the dependence classes;
- the licences' operative triggers;
- the conditional conflict for F.

It does **not** establish whether C/D/E/F/G are protected adaptations. Those
operations therefore remain **blocked as unresolved** under the
[policy](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026)
pending the human disposition of Q1–Q7.

Options exist, for example withholding F's values, or a replacement rule with
re-validation. Each would need separate authorization. None is proposed as
authorized, started or evaluated here. No new ticket or research chain is
created.

## 7. What was read, fetched, verified and not performed

**Read (repository/tracker):**

- `AGENTS.md`, `CONTEXT.md`, `doc/agents/{domain,issue-tracker}.md` and
  `doc/documentation-governance.md`;
- ADRs 0094 and 0096–0098 and the staged handoff, at `8341cef8`;
- issue #2054 and its triage/authorization comments;
- the named resolutions on #2030, #2031, #2035 and #2053;
- the packaging findings at `50a93846`;
- at `e610a5aa`: the import-proof `NOTICE.md`, `opus_completion.py`,
  `opus_repair.py`, `opus-repair-records.json` (structure and field types only)
  and the repaired/v2 summaries.

**Read-only private provenance (not republished):**

- the retained repaired `node-ledger.json` (hash-verified), used for the rule,
  status and dependency counts;
- `selection-model.tsv` (hash-verified);
- the Morpheus index manifest;
- a licence/copyright grep of the retained Morpheus and UD files.

A small scratch script was used only to count identity dependencies from those
two files.

**Fetched (ordinary HTTP GET, once each), 2026-10-15:**

| Source | Result |
| --- | --- |
| Morpheus `README.md` at `b1b33c56` | 200; equals the retained copy (`5c5342e0…`) |
| UD Perseus `LICENSE.txt` and `README.md` at `331ddef9` | 200; equal the retained copies (`1e610c18…`, `dfa8f938…`) |
| CC BY-SA 4.0 `legalcode.txt` | 200 (`28a9529c…`) |
| CC BY-SA 3.0 US and BY-NC-SA 2.5 `legalcode.txt` | 404 (no plain-text variant at that path); the HTML `legalcode` pages were fetched instead, 200 |
| EUR-Lex Directive 96/9/EC | HTTP 202 with an empty body (challenge); **not retried** |

**Not performed:**

- corpus or witness download or reacquisition;
- rights-holder outreach;
- a retry of the stopped access;
- new repairs or mappings;
- artifact assembly;
- execution of any proof, model, analyzer or build;
- schema, code or dependency changes;
- provider calls;
- hosting or publication;
- changes to labels or accepted documents;
- a PR.

The primary session worktree and the retained proof worktree were not modified.
