# Annotation-backed Ancient Greek: staged implementation handoff

Status: **Direction accepted; human documentation review pending; not implemented** · Date: 2026-10-15

Owning decision:
[Validate the shared-core architecture and staged implementation handoff](https://github.com/justin-hayes/mouseion/issues/2031).
[ADR 0098](../adr/0098-shared-core-classical-acquisition.md) owns architectural
trade-offs, interfaces, additive storage and the automatic-evidence amendment.
This document owns staged acceptance, not authorization to build, change schema,
investigate rights, acquire data, call providers or publish anything. Human review
of the documentation is required before final handoff. Stages require separately
authorized work; recording them starts no agent or prerequisite chain.

## Accepted evidence and bounded scope

Use the canonical resolutions rather than restating their investigations:

- [Planning-source acceptance](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6188837579):
  retained OGA v0.2.0, Marchant 1904 *Anabasis* Book 1, **1.1.1–1.10.19**;
  canonical-greekLit release `0.0.11376425141`, commit
  `476fb6133498e5710de7eec0b492791da86be4b4`. Exact PAULA authority,
  original-TEI correspondence and declared projection are retained unchanged.
- [Repaired whole-unit proof](https://github.com/justin-hayes/mouseion/issues/2052#issuecomment-6187073387):
  explicit seven citation/six POS research repairs, raw and uncorrected comparison,
  automatic provenance and known quality limitations. **438 selected identities /
  434 deterministic sentence inputs** describe the demonstrated baseline under the
  superseded `oga-agdt-upos/3` interpretation, not ready cards, accuracy, a
  promised yield for every learner, or a target for the replacement interpretation
  (see [No treebank data](#no-treebank-data)).
- [Per-work packaging resolution](https://github.com/justin-hayes/mouseion/issues/2053#issuecomment-6188570169)
  and [immutable inventory, hashes and notices](https://github.com/justin-hayes/mouseion/blob/50a93846fd8ddab68751c577d2d8bccd23fa3c5a/doc/research/ancient-greek-oga-per-work-packaging.md):
  private core ZIP **4,234,495 bytes**, with **31 source members** totaling
  **42,300,425 uncompressed source-member bytes** (plus wrapper manifest/notice),
  versus the **8,629,032,478-byte** official archive. Trust the reviewed exact hashes,
  not a newly chosen archive or checksum. Whole-work bytes are carried; only Book 1
  is validated. The initial curated offering is Book 1 only: carrying the other
  bytes does not advertise a whole-work Book or validate/count Books 2–7.

Keep source-content identity (reviewed manifest and member hashes) separate from
the ZIP transport hash of one published build; different compression software can
preserve all members without reproducing that ZIP. Installations verify the exact
reviewed release's transport, manifest and member pins, not the entire upstream
archive. Pin interpretation separately, including `oga-greta-lemma/2`, the
POS mapping `oga-agdt-upos/4` with its εἰμί rule, the morphology mapping
`oga-agdt-feats/1`, the
repair layer `oga-anabasis1-research-repairs/2` (or a later version from the
Stage 1 lineage audit), `oga-reading-terminal/2`, `ancient-greek/1` and
`grc-earliest-capped/1`. Under [No treebank data](#no-treebank-data),
`oga-agdt-upos/4` replaces the linked handoff's `oga-agdt-upos/3`, and
`/2` drops the six POS repairs that corrected `/3`'s εἰμί rule.
`oga-agdt-feats/1` is new and maps the source's AGDT morphology tags to UD
FEATS. The other identifiers are as recorded in the linked handoff.
An analysis-affecting change requires a new interpretation and explicit adoption,
not an in-place change or a new implicit identity equivalence. A card-selection
policy change applies to a newly prepared specification, not an analysis rewrite
or mutation of frozen specifications.

Prefer one-time verified archive extraction followed by reviewed immutable per-work
hosting, conditional on the gates below. The retained research archive is not
reacquired or required on every installation. Direct full-archive extraction/cache
is an explicitly chosen heavy alternative requiring its own resource/operation
review, not an implementation requirement or automatic fallback.

Known prediction/homograph/name errors, duplicated inputs, split spacing and repair
maintenance costs remain disclosed. No automatic fidelity/stopword/repair loop,
source survey, Greek NLP execution or Latin support follows. GLAUx and Lysias
remain retained alternatives, not automatic replacements.

## No treebank data

Decided by Justin on 2026-10-15 in the
[design review](https://github.com/justin-hayes/mouseion/issues/2076)
([#2089](https://github.com/justin-hayes/mouseion/issues/2089)). It binds every
stage below.

- **The rule.** No treebank is a runtime input, mapping basis, repair witness or
  test oracle. This covers AGDT, PROIEL, Gorman and every UD conversion or UD
  treebank. No source with a non-commercial licence, such as the NC UD
  witnesses, is used either. OGA's own annotation is the retained source, read
  as evidence; it is not a treebank witness.
- **What mappings are written from.** Each interpretation table is written only
  from the source's own tagset documentation (for OGA, the AGDT tagset and its
  annotation guidelines) and from the UD UPOS and FEATS tag definitions. In
  `oga-agdt-upos/4` and `oga-agdt-feats/1`, "agdt" names the tagset that the
  OGA source uses. It does not name treebank data.
- **Lineage.** Every interpretation slot and every exception-table value records
  its lineage: the documentation it is written from, or the retained source
  evidence it reads. A value whose only basis is a treebank witness is replaced
  or dropped through a new interpretation version (Stage 1).
- **Deliberate override.** This rule overrides two earlier lines:
  Stage 1's "do not … change mappings merely to evade the rights gate", and
  [#2058](https://github.com/justin-hayes/mouseion/issues/2058#issuecomment-6191383128)'s
  "do not claim independence" for the UPOS table, whose fixed letter table had
  UD-validator lineage. A mapping rewritten under this rule is the decided
  remedy, not evasion. Do not treat it as one, and do not restore the earlier
  mapping to reproduce an earlier result.
- **What stays.** The source decision is unchanged:
  [OGA v0.2.0 and canonical-greekLit](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6188837579)
  under CC BY-SA 4.0, with Morpheus under MPL-2.0 or CC BY-SA. Morpheus-derived
  values stay subject to the rights gate. The rule is not a new source, survey
  or research chain, and it clears no rights by itself.
- **Examples are invented.** No example, fixture or test input uses treebank
  annotation values.

## Initial operational envelope

These are human-agreed **configurable engineering defaults**, not measured latency,
memory, throughput or lifetime-storage guarantees. Enforce them before new work;
test the actual authorized runtime package, including required interpretation and
notices, before enabling acquisition. The private fixture's size does not establish
the eventual witness-free artifact's size or suitability.

| Enforced scope | Initial bound |
| --- | --- |
| Each transiently failing preparation stage | 3 total attempts, including the first; request deadline 24 hours from creation, including queue/backoff time |
| Each metadata request | 30-second timeout |
| Each package transfer / each local import stage | 5-minute timeout each |
| Automatic retries | Jittered exponential backoff starting at 10 seconds, capped at 5 minutes; honor longer upstream retry instructions only within the remaining deadline, otherwise stop |
| Each package | At most 16 MiB compressed, 64 MiB expanded and 128 members; exact reviewed manifest inventory, hashes and sizes are additionally required |
| Each Mouseion installation | 1 active classical acquisition/import pipeline and at most 32 pending requests across owners; still-needed work is explicit, not silently dropped |
| Metadata discovery | At most 500 offerings per batch with explicit continuation; incomplete discovery is not represented as a complete catalog |

Persist stage attempts, deadlines, pinned choices and intent; duplicate delivery or
restart cannot reset budgets. Explicit Retry creates a new bounded request for the
same choice, not a new source/edition/unit. Honor rate limits without hot loops.
Rights, hash/inventory, binding, required-scope and unsupported-mapping failures
stop immediately for intervention rather than consuming transient retries.

Validate untrusted packages before interpretation: no unlisted/duplicate members,
path traversal, executable payloads, unsafe links or unbounded expansion; hard
expanded-size/member limits apply even to dishonest metadata. Safe bounded parsing
must not resolve external XML entities or fetch undeclared resources. A hash proves
bytes, not rights, bibliographic identity or linguistic validity. Oversize or
incompatible candidates fail explicitly; no dropped layers or shortened units.

Configure operator capacity checks/alerts and pause new work before exhaustion.
Protected evidence has no expiry or eviction budget. Shared bytes do not authorize
another owner's access. Credentials remain separate; per-owner cancellation cannot
cancel another owner's preparation. Admission must enforce bounds transactionally.

## Stages and stop/go gates

Ordering constrains release, not permission to execute. Separately authorized
review/testing may proceed independently where prerequisites permit; no stage
implicitly provisions hosting or runs another investigation.

### 0. Documentation and deferred decision gates

**Go:** human-reviewed ADR 0098 and this handoff reconcile the old human-only source
requirement while retaining all general integrity and ownership contracts.
The architecture issue remains open until that review. These three deliberately
deferred, non-agent-ready tickets hold remaining operation decisions:

- [Clear operation-specific rights for the retained OGA interpretation output](https://github.com/justin-hayes/mouseion/issues/2054).
- [Authorize curated catalog publication and assign release ownership](https://github.com/justin-hayes/mouseion/issues/2056).
- [Approve concrete Ancient Greek provider payloads and deck exports](https://github.com/justin-hayes/mouseion/issues/2055).

They are shipping gates outside the planning map, not new blockers on recording a
conditional architecture. They resolve only through separately authorized work;
this handoff does not certify a legally distributable Greek MVP.
The publication and provider/export approvals have native dependencies on the
interpretation-output rights decision; none blocks the documentation issue.

### 1. Clear the exact interpretation and validate a witness-free runtime artifact

**Inputs:** retained source/repair evidence and the operation-specific rights
decision. No new acquisition or script execution is authorized by this document.

**Go tests:**

- Inventory every runtime dependency, emitted interpretation field and notice.
  Resolve Morpheus-derived decodings/repair values and NC UD-derived conjunction
  decisions for the intended local/distribution operations separately from the
  source members' existing BY-SA declarations. Private reproduction is not clearance.
- Build and independently verify a frozen runtime fixture without undeclared
  Morpheus/NC witness installations, runtime downloads or analyzer execution.
  Every interpreted value must have supported lineage; do not bundle witnesses.
  Writing mappings under [No treebank data](#no-treebank-data) is required, not
  evasion of the rights gate.
- Write `oga-agdt-upos/4` and `oga-agdt-feats/1` independently, from the AGDT
  tagset documentation and the UD UPOS/FEATS definitions only, and record that
  lineage in each reviewed definition. Neither reuses `oga-agdt-upos/3`'s
  letter table. A native value that the documentation does not map stays
  unmapped and is reported, never guessed.
- Confirm that the core package's annotation members carry the AGDT morphology
  tags. If they do not, the retained candidate declares no morphology mapping,
  and morphology is reported unavailable rather than filled in.
- Decide and record the εἰμί rule (AUX or VERB) again for `oga-agdt-upos/4`, as
  an explicit human decision from the UPOS definitions and the source's own
  native annotation (design OD-77). Its errors are disclosed, not repaired.
- Drop the six POS repairs, which corrected the superseded rule, and audit the
  lineage of every remaining exception-table value: the 20 confirmed labels,
  the compound record and the seven citation repairs. Record, per value, the
  documentation or retained source evidence it rests on. Any value that depends
  on a treebank witness is replaced by a value with permitted lineage or
  dropped, through a new repair-layer version and table digest. Nothing adds a
  repair.
- Verify exact source/package/member hashes, retained original projections,
  declared Book 1 extent, binding, versioned interpretation and repair provenance.
  Re-run complete-unit accounting and selection under the new interpretation and
  the same declared selection context, compare it explicitly with the 438/434
  baseline, and accept the resulting numbers. Do not tune any mapping or table
  value to reproduce 438/434. Rights clearance is not semantic certification.
- Exercise actual size, parser/time and storage limits, including the final
  interpretation artifact and notices, rather than assuming the research scripts
  are a deployable importer. Deliberately review any required limit change.

**Stop:** unresolved required rights, hidden witnesses, treebank lineage in any
runtime value, missing required mappings (including occurrences that the new
interpretation leaves unresolved), incomplete binding/scope or practical
infeasibility. A lower selected-identity or sentence-input count is not in
itself a stop. Return the exact obstacle for
an explicit human decision; do not launch another repair/source loop or approve a
smaller scope. Failed publication permission can block distribution even where a
separately reviewed local operation is permissible.

### 2. Review additive schema, canonical interfaces and compatibility

Design under review: [Ancient Greek additive analysis design](ancient-greek-additive-analysis-design.md) (proposed; not approved).

**Go:** separately reviewed design satisfies ADR 0098 before any SQL is written.
Only then may authorized implementation add the needed structures. Keep the
immutable baseline and every shipped migration untouched; allocate successor
numbers from the then-current tree, not from this document.

**Acceptance:**

- Model entry-to-unit offerings, edition/unit Books, immutable source revisions,
  bindings/interpretations, rich vocabulary/occurrence identities, capability
  diagnostics and transitive retention references. Use existing Book, exact-analysis
  and Reading/snapshot/completion anchors; no parallel Greek learner lifecycle.
- Cover shared/discontinuous spans, same-span distinct occurrences, unprinted
  nodes, multiword containers and incomplete sentence boundaries without invented
  offsets or roots. Optional invalid syntax remains inspectable but unavailable.
- Implement explicit `ancient-greek` version 1 normalization and source decoding
  as separate versioned steps under the
  [identity resolution](https://github.com/justin-hayes/mouseion/issues/2033#issuecomment-6163487386).
  Cross-implementation fixtures preserve diacritics, iota subscripts, elision,
  sigma and scheme-qualified homographs; reject generic casefold fallback.
- Exercise Stanza and imported candidates through the shared validation interface.
  Preserve existing modern-language results, not merely serialization. Legacy
  analyses/decisions/snapshots remain readable without mass conversion.
- Preflight for legacy `grc`, owner isolation, uniqueness and compatibility. Any
  identity reconciliation or populated-data transformation requires a separately
  approved plan; never mark both descendants of a split Known.

**Migration/backfill separation:** structural DDL and data-only work are separate.
No existing EPUB Work/Edition, correction, identity, snapshot or Known backfill is
required or authorized. If a later approved additive projection initialization is
needed, specify owner scope, exact input versions, resumability/checkpoints,
idempotency and expected counts independently. It must not alter immutable facts.
Recovery is tested backup/restore or reviewed roll-forward; destructive `down`
migrations are not an assumed production rollback. SQL/sqlc regeneration and
human review follow repository rules when implementation is separately authorized.

**Stop:** silent homograph collapse, offset-only decision transfer, changed legacy
counts/identities, destructive baseline edits, or a design requiring producer
branches throughout Reading/Known/snapshots.

### 3. Authorize catalog release and implement durable acquisition/adoption

**Go:** exact distributed components are cleared; publication is explicitly
authorized, ownership assigned and release trust reviewed. The runtime fixture
passed Stage 1. A manifest/package format can be static; no dynamic server is
assumed. Metadata-only discovery can be tested without publishing a catalog.

**Acceptance:**

- Explicit connection opt-in; bounded metadata sync and edition/unit browsing
  work without NLP. Show automatic/repair provenance, versions, component rights,
  availability and verified capabilities separately. Partial/unresolved offerings
  can retain To Read intent without promising permitted acquisition or Reading.
- To Read persists pinned intent and durable work together. Download/import runs
  outside the HTTP request and long-held database transactions. Validate all
  required evidence and prepare complete count projections in staging.
- Adopt current text/binding/analysis, projections and protected references
  atomically after checking owner authorization, expected current evidence and
  still-current request. Initial selected preparation can adopt its already chosen
  result; replacement always needs explicit owner approval. Sync/repeated To Read
  never select a new revision implicitly.
- Crash/duplicate/restart tests cover each stage, publication boundaries, persisted
  retry budgets, timeout, queue capacity, storage pressure and safe package rejection.
  A cancelled/superseded late finisher cannot adopt; leaving To Read cancels intent,
  not retained evidence or another owner's request. A failed candidate preserves
  valid current evidence and does not transfer corrections.
- Backup/restore all exact bytes, records, references and notices. Upstream outage,
  withdrawal, disconnect or hiding must not evict permitted retained evidence.
  Test exact verified recovery; if impossible, report unavailability without
  replacement or automatic End/Finish. A confirmed defect restricts affected
  capabilities while preserving immutable facts and unaffected capabilities.

**Stop:** incomplete publication, unprotected references, lost exact restore,
unauthorized reuse, exhausted capacity/budgets or unknown package/interpretation
versions. Do not use the full archive or another source as automatic recovery.

### 4. Release annotation-backed Reading independently

**Go:** authorization, full-scope validation, publication, counts, required learner
review and recovery tests pass. Greek cards may be explicitly unavailable.

**Acceptance:**

- End-to-end connect → metadata sync → My Books/To Read → durable preparation →
  explicit Current reading → Vocabulary/coverage → End/Switch/Finish, with NLP
  stopped throughout. Use existing four destinations and accessible native UI,
  including keyboard, narrow-screen, no-JavaScript and failed-enhancement paths.
- Discovery, acquisition, other owners and nonselected whole-work bytes contribute
  no personal counts. Test exact owner/`grc`/`el` isolation, effective occurrence
  decisions and [ADR 0095](../adr/0095-greek-personal-counts-and-external-frequency.md)'s
  3/15 rules, pending projections and intentionally separate overlapping Books.
  Retain current eligible POS rules; do not expand the vocabulary pool by admitting
  every annotation node. No external statistical-corpus ingestion or weighting.
- Same-lemma/POS but distinct lexical discriminators remain distinct in Browse,
  Known, reservations, new freezes and completion. Two occurrences sharing a span
  can have different exact decisions. Missing syntax disables dependent queries
  with an explanation, never invented evidence or a false zero-results claim.
- Two owners adopt different revisions independently. Refresh, new counts,
  corrections and new interpretations cannot mutate a running/historical freeze,
  reservations, old decks or Known facts. Changing active vocabulary still requires
  End → review → Start. Finish uses its frozen identities regardless of deck state.
- Practical examples and omission/quality disclosures remain usable after actual
  integration. Label this as the Reading increment, **not the complete Greek MVP**.

**Stop:** misleading coverage/provenance, practical unusability, mutable frozen
facts, personal-count leakage or a dependency on NLP/provider availability for
Reading. Do not retune thresholds or source interpretation silently.

### 5. Complete Greek card, provider/export and client acceptance

**Go:** the concrete provider/export decision permits the actual payload/output.
Use the [sentence/card resolution](https://github.com/justin-hayes/mouseion/issues/2034#issuecomment-6164245458)
with ADR 0098's source-provenance amendment, not a human-only annotation gate.

**Acceptance:**

- Deterministic source-based sentence selection and verified target/headword
  evidence pass the existing complete-sentence, length and alignment rules; no
  German syntax gates, guessed lexical repairs or truncated sentences. Freeze
  inputs before provider work. Record per-identity omissions and reasons; the
  Stage 1 count of deterministic inputs is not a count of semantically accepted
  cards.
- Review a small concrete set spanning short complete sentences, implicit subjects,
  repeated targets, NFC/NFD polytonic text, lexical homographs, missing optional
  forms, Gloss abstention and source-supported elision/crasis/shared spans where
  applicable. Missing source cases use separately identified contract fixtures,
  not invented occurrences claimed to come from Book 1.
- Assess dictionary availability and contextual Gloss/translation usefulness
  separately. Context-only inference is explicit. Preserve operator-configured
  provider behavior and permitted minimal payloads; no new per-user consent control
  or dictionary-only substitute. Provider unavailability fails preparation closed;
  zero usable cards is failure, not an empty ready deck. Abstention never silently
  reselects a sentence in the same frozen generation.
- Exercise generic specification/note/APKG packaging with Greek-specific target,
  citation/discriminator, optional forms, provenance, attribution/licence/modification
  notices and export restrictions. Test actual supported Anki clients for polytonic
  rendering, exact emphasis and notices; identify client/version results explicitly.
- Card omissions/failure do not mutate Reading snapshots, reservations, Known or
  history. Presentation-only rerenders preserve frozen inputs and perform no new
  lexical/provider calls. Modern-language cards/notices retain their behavior.

**Stop:** unpermitted provider/export operations, poor semantic/presentation
usefulness or client failures. Keep the independently accepted Reading increment
available where its own gates still pass. A new policy question exposed by examples
needs a narrow human decision, not an implicit implementation choice.

## Risks, reconsideration and extension limits

Remaining empirical questions are the exact witness-free artifact, final resource
costs and restores, integrated Reading usefulness, semantic Gloss yield, and client
presentation. They are bounded Stage 1–5 tests, not claims already demonstrated by
research reproduction. No universal accuracy or card-coverage threshold is invented.
Public-evidence rights questions and accountable publication ownership have their
own deferred tickets; no rights-holder outreach is implied.

PostgreSQL retention grows with referenced revisions and may need capacity work;
there is no bounded-lifetime-storage, high-availability or zero-data-loss promise.
NLP gate decoupling, richer identity/membership storage and exact occurrence review
are real integration costs. Review whether reuse still earns its keep at Stages 2
and 4. A demonstrated blocking gate returns for explicit scope/source/architecture
reconsideration, never automatic fallback, scope shortening or a new research chain.

Latin can reuse these interfaces later; no Latin source, adapter, trial, migration
or UI is part of this handoff. Books 2–7, philological tooling, arbitrary raw-text
Greek NLP and external frequency are likewise not delivered by this plan.
