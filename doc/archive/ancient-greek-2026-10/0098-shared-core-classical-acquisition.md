# ADR 0098: Shared-core classical acquisition and automatic-evidence admission

Status: **Withdrawn, never implemented. Superseded by spec [#2098](https://github.com/justin-hayes/mouseion/issues/2098); background in [#2097](https://github.com/justin-hayes/mouseion/issues/2097).** · Date: 2026-10-15 · Author: Justin + OpenCode

Recorded through the live discussion in
[Validate the shared-core architecture and staged implementation handoff](https://github.com/justin-hayes/mouseion/issues/2031).
Justin accepted the architectural recommendations and authorized a documentation-only
PR. Final handoff remains open until that documentation is human-reviewed. This
decision authorizes no implementation, SQL, backfill, acquisition, provider call,
hosting, publication or deployment. The
[staged handoff](annotation-backed-ancient-greek.md) owns rollout gates
and acceptance tests; the linked prior decisions retain their detailed contracts.

## Context and alternatives

The [accepted planning source](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6188837579)
is the exact retained OGA v0.2.0/Marchant *Anabasis* Book 1 interpretation, with
automatic provenance and the documented seven citation/six POS research repairs.
Only Book 1 is validated. The
[per-work investigation](https://github.com/justin-hayes/mouseion/issues/2053#issuecomment-6188570169)
demonstrates bounded packaging, not a cleared runtime interpretation or publication.

Challenge the same-application hypothesis against three alternatives:

- **Separate Greek application:** isolates classical acquisition and presentation,
  but duplicates Book ownership, Reading, corrections, Known/Reserved vocabulary,
  frozen history and decks, or introduces cross-application learner-state coupling.
  The evidence does not demonstrate a distinct learner lifecycle requiring that cost.
- **Greek inside the NLP deployment:** makes importing existing annotations depend
  on model infrastructure that does not produce them. No Greek analyzer execution,
  retraining or NLP-service implementation belongs to this route.
- **Adapter-only integration into today's token tables:** understates the work.
  `corpus_tokens` requires dependencies/heads; occurrence corrections use offsets;
  vocabulary keys lack lexical discriminators. Generic `grc` normalization falls
  back to language-neutral folding. Classical acquisition also needs durable work
  rather than the current inline EPUB acquisition path. See
  `internal/analysis/jobs.go`, `internal/selection/selection.go`,
  `internal/canonicalization/canonicalization.go`,
  `internal/webapp/reading_page_handlers.go`, and
  `migrations/000001_initialize.up.sql` at code baseline
  `bb438d2e80410554ac7ce00122b1aa8a261013b8`.

Keep one application with an additive canonical-analysis model and source adapters.
This is conditional reuse, not zero integration work. Reconsider if implementation
requires producer-specific semantics throughout the learner core, cannot preserve
old facts, or fails the rights/usefulness gates. Do not silently pick another source.

## Ownership and interfaces

An independently operated **opt-in curated catalog** advertises selected offerings,
not an entire corpus to import into every installation. Initially it needs only
versioned discovery metadata and immutable downloadable data packages. Separate
ownership does not imply another running server, database or dynamic protocol.
The catalog never owns personal learner state or generates annotations on demand.

The interfaces below specify responsibilities, invariants and failures, not Go
signatures, a wire format or an infrastructure implementation:

| Module | Interface and responsibility |
| --- | --- |
| Catalog discovery | Read bounded versioned metadata with continuation: entry/offering identity, Work/Edition/unit, available annotation layers/provenance, component rights/availability, package and supported interpretation versions. No content acquisition or personal counts. |
| Source acquisition adapter | Retrieve the explicitly pinned choice under applicable authorization; return retained immutable bytes and verified inventory. A permitted exact mirror is transport recovery, never a changed edition/unit/interpretation. |
| Source interpretation adapter | Decode/map and bind exact evidence using reviewed application code; return a canonical-analysis candidate and native lineage/diagnostics. It does not approve itself or change learner state. |
| Shared canonical validation | Check full declared scope, exact correspondence, occurrences/identities, sentence boundaries and capability-specific evidence; return validated analysis or explained diagnostics. Invalid optional syntax is withheld, never fabricated. |
| Shared preparation/adoption | Authorize and durably pin intent, coordinate bounded work, retain evidence, prepare exact count projections, and atomically adopt against expected owner/Book/request state. Failure preserves valid current evidence. |
| Shared learner core | Consume validated capabilities and effective identities for corrections, personal counts, Reading, freezes, Known/Reserved and Reading-owned deck preparation. Consumers do not select behavior by producer name. |

Imported and model-produced candidates target the same validation interface.
Implement the Stanza compatibility adapter and parity tests without rewriting
stored analyses or switching modern-language behavior wholesale. A producer
declaring a capability is not proof that its particular output validates.
Discovery/import/Reading for existing annotations must work with NLP stopped.

Catalog packages are **data, never executable plugins**. Decoder/mapping code
ships through reviewed Mouseion releases. Unknown contract, mapping or profile
versions stop import. A later publisher-provided frozen interpretation is a
separate reviewed possibility, not assumed present or cleared. Retain native
evidence and transformation provenance in either case. The witness-free gate
concerns runtime dependencies; it does not authorize deleting retained research
evidence or bundling uncleared witnesses.

Use independently reviewed, pinned manifest and package hashes under an explicitly
trusted release process. A payload and a checksum from the same untrusted download
do not authenticate a publisher. Connection opt-in is neither permission for every
operation nor permission to execute catalog-supplied code. Concrete release trust,
publication and maintenance ownership remain a
[deferred human gate](https://github.com/justin-hayes/mouseion/issues/2056).

## Identity, persistence and preservation

Preserve [ADR 0093](0093-classical-identity-and-reading-units.md),
[ADR 0094](0094-source-independent-annotation-and-provenance.md), the
[Greek identity decision](https://github.com/justin-hayes/mouseion/issues/2033#issuecomment-6163487386),
and [ADR 0097](0097-classical-evidence-refresh-adoption-and-retention.md).
The concrete additive storage direction is:

- Separate stable Work/Edition/Reading-unit records from source revisions and
  owner-scoped Books. Relate connection-scoped Catalog entries to explicit unit
  offerings; relate verified edition/unit offerings to each owner's Book. Do not
  stretch the existing unique entry alias into a one-entry/many-Book relationship.
- Retain immutable packages initially in PostgreSQL, using its existing binary
  content storage mechanism, with hashes, component inventories, rights and lineage.
  Store canonical interpretations separately from acquired bytes. Do not store
  the 8.6 GB research archive in the application or introduce object storage now.
- Associate immutable text/annotation revisions, exact bindings, interpretation
  versions and capability diagnostics. Verified references, not titles, CTS aliases,
  identical bytes or source availability, establish applicable identity/binding.
- Add a shared rich vocabulary-identity representation with canonical language,
  lemma, UPOS and optional scheme-qualified lexical discriminator. Preserve the
  discriminator separately, never encode it into lemma spelling. Keep profile and
  mapping versions as provenance; unchanged stable identities may still match.
- Add exact-analysis occurrence identities and explicit shared/discontinuous span
  references; use them for new occurrence decisions. Represent missing, unsupported,
  ambiguous, invalid and usable optional information explicitly. Do not relax old
  token constraints by manufacturing complete syntax or matching on offsets alone.
- Use additive identity membership/acceptance records attached to the existing
  Reading/snapshot/completion lifecycle for new rich identities. Shared persistence
  interfaces read legacy and new representations without creating a second Greek
  learner-state machine. Existing tuple keys, constraints and immutable records
  remain intact; do not force a whole-library identity backfill.
- Protect transitive dependencies from adopted Books, in-flight requests, exact
  occurrence decisions, active and retained snapshots, completions, specifications
  and artifacts. Physical deduplication is separate from owner/access authorization.
  Retain even unreferenced evidence by default; no TTL or purge is introduced.

Schema review must check uniqueness, ownership, reference protection and atomic
publication against these invariants before SQL is written. No table names or
migration numbers in this decision are executable prescriptions. Existing EPUB
Books need no Work/Edition backfill. Audit for legacy `grc` records before rollout;
keep their original interpretation and frozen facts, and stop any reconciliation
until a separate human-approved plan exists. Legacy unqualified identities must
not become wildcards for new lexical distinctions. Never transfer Known status
automatically across a merge/split or reuse a key for a different lexical meaning.

## Shared experience and independently releasable Reading

Use My Books, Reading, Vocabulary and Catalogs with existing server-rendered rows,
disclosures, confirmations and job/recovery states. Add edition/unit, source,
automatic/repair provenance and rights/availability details where needed; no Greek
workspace or philological editor. Unsupported syntax-dependent features explain
their unavailability instead of reporting missing evidence as zero results.
Greek headword, target, morphology and card presentation remain deliberate work.

Keep [ADR 0095](0095-greek-personal-counts-and-external-frequency.md)'s personal
3/15 rule and per-Book overlap semantics; no statistical-corpus ingestion,
provenance weighting or external-frequency selection. Use the existing disposition,
current-reading, effective-count/review and frozen-vocabulary contracts. A catalog
connection, sync or acquired package contributes no personal counts by itself.

Validated vocabulary-ready Reading can release independently with Greek card
preparation explicitly unavailable. It is not the complete Greek MVP. Rights and
artifact integrity apply to that release too. Cards require the separate
[sentence/card contract](https://github.com/justin-hayes/mouseion/issues/2034#issuecomment-6164245458),
as amended below, concrete operation permission, semantic review and Anki-client
tests. Card failure/omissions never shrink snapshots or gate Reading completion.

## Amendment: automatic evidence is eligible, not certified

The [human-authorized scope reset](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6177588105)
and the subsequent planning-source acceptance supersede **only** the
candidate-specific human-reviewed MVP requirement in:

- ADR 0094, “Provenance, versions, and competing interpretations”;
- ADR 0095, “Count occurrences equally, without conflating evidence”;
- ADR 0096, “Annotation selection and fallback”;
- the corresponding historical resolutions of
  [Choose a source-independent annotation and provenance contract](https://github.com/justin-hayes/mouseion/issues/2027#issuecomment-6177593598),
  [Choose how Greek corpus evidence relates to personal vocabulary counts](https://github.com/justin-hayes/mouseion/issues/2029#issuecomment-6177594309),
  [Choose the Greek repository sync, browse and acquisition flow](https://github.com/justin-hayes/mouseion/issues/2028#issuecomment-6177593952), and
  [Choose Ancient Greek representative-sentence and card acceptance rules](https://github.com/justin-hayes/mouseion/issues/2034#issuecomment-6177594631).

Human, human-reviewed automatic, automatic and unknown-provenance evidence are
eligible with explicit informed choice and accurate layer/scope provenance.
Human review remains optional quality evidence, not an architectural admission
requirement. Preserve ADR 0096's explained preference where a suitable reviewed
option exists; it is not an exclusion or certification rule. The OGA repair layer
is not scholarly gold, and model Glosses are not human-reviewed by association.

Full declared scope, exact binding, usable canonical identities, lexical
distinctions, rights/notices, personal-count isolation, owner adoption and frozen
learner-state protections are unchanged. Human review of practical examples and
semantic/presentation acceptance remain necessary: removing a human-only source
gate does not remove those tests. Historical wording is retained behind explicit
amendment pointers rather than rewritten to conceal the earlier decision.

Latin is an extension consideration for these interfaces, not a dataset, adapter,
schema backfill or delivery requirement of this handoff.
