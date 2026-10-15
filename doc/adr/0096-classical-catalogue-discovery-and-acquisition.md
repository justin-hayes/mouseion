# ADR 0096: Classical catalogue discovery and intent-driven acquisition

Status: **Accepted; implementation pending** · Date: 2026-10-14 · Author: Justin + OpenCode

**Candidate-scope amendment:** [ADR 0098](0098-shared-core-classical-acquisition.md)
reconciles the historical human-reviewed Greek MVP requirement below with the
human-authorized automatic-evidence scope. Its documentation review is pending;
informed choice, rights, full-unit validation and owner adoption remain required.

Recorded through the human-confirmed decision in
[Choose the Greek repository sync, browse and acquisition flow](https://github.com/justin-hayes/mouseion/issues/2028).
Planning only: no production implementation, schema changes, backfills, source
approval, or deployment changes are authorized. Repository promotion requires
human review. Existing OPDS behavior is unchanged.

## Context

ADRs 0093 and 0094 distinguish stable edition/unit Books from reusable exact
text and annotation evidence. Classical repositories offer editions and partial
annotation coverage, not necessarily EPUBs or a one-entry-to-one-Book catalogue.
CTS identity and advertised coverage do not establish exact token correspondence.

Today's OPDS sync is metadata-only, gated by NLP readiness. Moving to To Read
persists disposition, downloads/imports the EPUB inline, and then queues analysis;
acquisition has no automatic retry. These are implemented behaviors, not the
required execution model for classical acquisition. See
`internal/cataloguesync/sync.go`, `internal/webapp/library_handlers.go`,
`internal/webapp/reading_page_handlers.go`, and `internal/analysis/jobs.go`.

## Decision

### Supported connections and metadata-only discovery

Connections remain learner-owned. Classical discovery uses explicitly supported,
reviewed source-specific adapters and pinned release manifests, not assumed
compatibility with arbitrary CTS endpoints or Git repositories. Text repositories
such as Perseus/OGL and annotation repositories are separate inputs whose offerings
may be matched. Supporting a repository does not approve its files, component
rights, bindings, or annotation quality. Exact initial candidates belong to
[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030).

Sync performs bounded metadata discovery: bibliographic descriptions, source and
release identifiers, declared Reading-unit bounds, annotation availability,
claimed coverage, scoped provenance, component rights and supported features.
It does not download or interpret every text/annotation. Advertised availability
is distinct from verified acquired evidence; a manifest is not proof of alignment
or vocabulary readiness.

Advertised edition/unit choices become metadata-only Inbox Books. One Catalog
entry can offer several explicitly declared Reading units and therefore several
Books. Keep its owner/connection/stable-entry identity separate from each Book's
stable edition/unit identity; do not invent an entry ID from title, annotation
fragments, or mutable coverage. Reconcile Books only under ADR 0093's verified
same-edition/same-unit rule. This requires a classical entry-to-unit relationship,
not reuse of today's unique one-entry-to-Book alias as if it represented all units.
No SQL/table layout or alias backfill is selected here.

Sync does not manufacture every possible passage. Offer declared whole-work,
constituent-book and bounded-passage units; no arbitrary passage editor in the
MVP. Group Books by Work without merging edition/unit dispositions or history.
When only Book I is annotated, offer Book I separately; do not silently shrink a
whole-work selection or its analysis denominator.

### Browse, intent and acquisition

Unannotated, partially annotated and rights-unresolved offerings remain browsable
and selectable as To Read. Show missing supported annotation, partial coverage,
unverified availability and rights blocks honestly. To Read records intent, not
readiness or permission. Follow the component-specific policy in
[Choose the licensing and distribution boundary for classical data](https://github.com/justin-hayes/mouseion/issues/2035):
reviewed restricted direct acquisition requires explicit opt-in; unresolved rights
block supported acquisition, not saving intent. No corpus has blanket approval.

Browsing, opening an item, or syncing triggers no text/annotation acquisition.
Entering To Read durably records preparation intent and queues acquisition and
validation, with intent and durable work persisted together. Pin selected text
and annotation source/releases and interpretation configuration before acquisition.
Reuse permitted exact evidence where available. A rights-approved unannotated
Book may acquire text alone, with vocabulary preparation visibly unavailable;
do not silently invoke raw-text Stanza. Supported annotation may be prepared
without NLP execution. Current reading remains a separate explicit action.

Ensure-once operations are idempotent and reuse selected completed/in-flight
work. They do not adopt newly discovered annotations on repeated To Read actions.
Source changes are explicit preparation/refresh choices, not browsing or sync
side effects. Leaving To Read does not delete evidence or history; exact
cancellation and later adoption rules belong to the operational decision below.

### Annotation selection and fallback

Expose source/release, documented review provenance by layer and scope, coverage,
and usable features. A reviewed adapter may nominate an explained default, with
a suitable documented human/human-reviewed option preferred where available.
This is not a universal quality hierarchy: native, UD and human labels alone
do not prove superiority, complete coverage, or reviewed lemma/morphology layers.

Automatic or unknown-provenance annotation requires an explicit informed choice.
It may satisfy ADR 0094 technically, but cannot satisfy the human-reviewed MVP
requirement by assumption. Do not silently mix interpretations.

Retries retain the exact selection. Failure or unavailability stops the selected
preparation and offers alternatives; a different annotation source/release requires
an informed choice and fresh validation even within the same provenance category.
Never silently downgrade provenance, substitute an Edition, or change unit bounds.
A permitted mirror supplying the identical verified artifact is transport recovery,
not a replacement interpretation. Annotation-carried text authority remains
subject to ADR 0093, not an excuse to switch text underneath the chosen Edition.

### Capabilities, publication and recovery

Distinguish producer feature support, present execution availability, and validated
acquired-evidence capabilities. Classical metadata discovery needs a supported
catalogue adapter, not a ready Stanza model. Acquisition needs permitted accessible
sources and supported retrieval/interpretation. A supported producer temporarily
unavailable remains supported and retryable; an unsupported producer needs an
adapter/configuration change, not repeated execution attempts.

Annotation-backed discovery, acquisition and Reading must work without Stanza
installed or reachable. Current reading requires valid published full-scope
vocabulary-ready evidence under ADR 0094 and the retained Reading gates, including
required count projections and learner review. Vocabulary readiness does not imply
syntax-, morphology- or card-readiness; each consumer requires its own validated
inputs. No unrelated global NLP readiness fix is authorized.

Use durable, idempotent, resumable background stages, with bounded backoff/retries
for transient network/service failures. Missing annotation, unresolved rights,
unsupported mappings, invalid alignment and incomplete required coverage need
explained blocked/failed states, not endless retry. Preserve To Read and valid
current evidence on failure; publication is validated and atomic under ADR 0094.
Expose progress and explicit Retry through existing operational patterns.

Classical adapters own source-specific discovery, retrieval, citation/binding
rules and native decoding/mappings. Shared modules own authorization, durable
intent/jobs, validation/publication and learner-state transitions. Keep consumers
on validated canonical evidence rather than branching on producer names. Sharing
these semantics with OPDS does not mean forcing classical data through EPUB-only
or Calibre language-feed interfaces, or retrofitting new policy gates onto OPDS.

### Shared experience and preservation

Catalogs remains connection maintenance; My Books remains bibliographic browsing
and To Read; Reading owns commitment. Reuse Book rows, native disclosures,
filters, confirmations and job status. Add Work grouping, Edition/unit choices,
annotation details and source/license/provenance filters within those surfaces,
not a separate Greek application or browse destination.

Show author/title, Edition, exact unit bounds and source/release. Present concise
attribution/restriction summaries alongside choices, with component-level rights,
review outcomes and evidence in disclosures. Explain restrictions before acquisition
and retain applicable notices in exports. Annotation coverage, scoped review
provenance, technical capabilities and personal vocabulary coverage stay distinct.

Later sync updates descriptions and advertised availability only. Disappearance,
connection removal or changed coverage never rewrites Book identity, adopted
sources/analysis, dispositions or history. New edition/unit choices create separate
Books; unresolved identity conflicts are flagged rather than silently merged.
Frozen readings keep their exact evidence; changing their vocabulary still requires
End, review and Start. Preserve Known/Reserved separation and Reading-owned decks.

Metadata-only sync, text-only acquisition and incomplete annotation contribute
no personal vocabulary counts. Participation requires validated, full-scope
published vocabulary evidence and ready projections. External frequency and
duplication between overlapping Books remain with
[Choose how Greek corpus evidence relates to personal vocabulary counts](https://github.com/justin-hayes/mouseion/issues/2029);
the current selection rule is not changed here.

## Trade-offs and compatibility

Acquiring/analyzing an entire repository at sync wastes resources and confuses
discovery with personal evidence. Requiring annotations before saving intent
hides useful texts. Silent fallback makes preparation convenient at the cost of
unreviewed evidence changes. The accepted cost is explicit edition/unit/source
choice, honest unavailable states and durable acquisition work.

For future classical support, this amends ADR 0023's universal NLP authority,
ADR 0041's ready-NLP discovery gate, ADR 0044's one-entry-to-one-Book representation,
and ADR 0047's EPUB-only/inline-acquisition and acquisition-always-implies-analysis
clauses. Preserve metadata-first non-destructive discovery, connection-scoped
identity, catalogue-derived Books, ADRs 0093/0094, and frozen Reading/history.
Existing OPDS/NLP behavior, schema and production ownership constraints remain
unchanged until separately reviewed implementation.

Exact MVP files and rights approval, sentence/card acceptance, and corpus-count
policy remain with their existing tickets.
[Choose classical evidence refresh, adoption and retention](https://github.com/justin-hayes/mouseion/issues/2036)
owns retry budgets, cancellation, revision detection/adoption and retention.
[Validate the shared-core architecture and staged implementation handoff](https://github.com/justin-hayes/mouseion/issues/2031)
owns concrete interfaces, storage/migrations, deployment decoupling and the staged
plan. No new research, access, rights-holder contact or runtime test is claimed.
