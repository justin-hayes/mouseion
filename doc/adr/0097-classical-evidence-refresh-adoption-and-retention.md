# ADR 0097: Classical evidence refresh, adoption and retention

Status: **Accepted; implementation pending** · Date: 2026-10-14 · Author: Justin + OpenCode

Recorded through three live decision rounds and explicit confirmation in
[Choose classical evidence refresh, adoption and retention](https://github.com/justin-hayes/mouseion/issues/2036).
Planning only: no implementation, schema changes, production purge, deployment,
or new legal-erasure workflow is authorized. Repository promotion requires human
review. Existing OPDS/NLP behavior is unchanged.

## Context

ADRs 0093, 0094 and 0096 separate stable owner-scoped edition/unit Books from
immutable sources, canonical interpretation and metadata-only discovery.
Discovery must not globally change Books or rewrite exact Reading evidence.
The licensing decision distinguishes reusable licensed evidence from restricted
direct acquisition; another owner's acquisition is not permission.

Today's EPUB analysis jobs already persist intent and work transactionally and
guard current-analysis promotion against superseded runs in
`internal/analysis/jobs.go`. OPDS sync preserves Books on failure or missing
entries in `internal/cataloguesync/sync.go`. These are useful precedents, not a
classical shared-evidence implementation. No classical adoption, shared retention,
or reclamation mechanism has been delivered.

## Decision

### Discovery detects candidates; owners adopt exact evidence

Sync detects advertised changes using supported release identifiers/manifests
and conditional requests where available. Adapters without reliable change
signals disclose that limitation, rather than claiming freshness. Show last
check, update availability and upstream availability separately from local
evidence validity. A newer release or upstream disappearance does not itself
make retained, valid adopted evidence stale.

Acquire changed text/annotation only for an owner's explicit preparation or
refresh request, not by downloading every discovery candidate. Verify acquired
artifacts by hash. An upstream revision label or URL is not permission to
overwrite bytes. Identical bytes can reuse retained evidence while preserving
distinct origin/release provenance; hashes alone do not prove Edition/unit
identity or rights under ADR 0093. Changed interpretation still requires a new
exact analysis under ADR 0094.

Each owner explicitly approves replacement text, annotation or interpretation
configuration, including a new release of an existing producer. Discovery only
offers the update. A refresh request pins the candidate and authorizes its
validated replacement for that owner's Book; initial To Read preparation may
publish the already-selected candidate automatically. Repeated To Read actions
do not adopt newly discovered evidence. Producer alternatives are informed
choices, never silent fallback. Edition or Reading-unit changes remain different
Books, not recovery strategies.

Explain that replacement can change personal counts and does not transfer
corrections/exclusions. Keep one current published analysis per Book, with
owner-specific adoption. Frozen Reading, deck preparation, reservations, Known
facts, corrections and history keep their exact original evidence. Updating
active-reading vocabulary still requires End, review and Start.

### Publication, supersession and cancellation

Retain and validate the complete candidate before adoption. Required vocabulary
count projections must be ready. Switch the owner's current text, binding and
analysis consistently; never expose mixed revisions or partially written
evidence. Protect newly referenced dependencies before committing the adoption.
Failed acquisition, validation or publication leaves valid old evidence current.

At commit, recheck applicable authorization, the expected current evidence, and
the still-current request. Superseded or cancelled requests cannot adopt a late
result. Choosing C while B is running prevents B from winning by finishing last.
Sharing reusable evidence does not share adoption authority or cancellation.

Leaving To Read cancels outstanding preparation/adoption intent, not completed
evidence or a Current reading. Re-entering does not silently revive a cancelled
replacement. A cancelled owner's request cannot cancel another owner's
independent work; retained reusable results need not be destroyed.

### Durable bounded work and recovery

Persist intent and pinned choices with durable work transactionally. Requests
and resumable stages are idempotent and restart-safe. Every request has enforced,
finite per-stage attempt limits, a total automatic-retry deadline, request
timeouts, capped backoff with jitter and upstream rate-limit handling. Restarts
do not reset the budget. Exhaustion exposes explicit Retry, starting a new bounded
attempt against the same pinned choice, not whatever upstream currently offers.

Transient network/service failures may retry. Unresolved rights, missing
annotation, unsupported mappings, invalid alignment and incomplete required
coverage need explained blocked/failed states and intervention, not endless
retry. Cancellation prevents adoption even when a worker cannot immediately
stop its external request. Exact restoration from a permitted mirror is transport
recovery only when the artifact is verified; another revision/Edition is not.

If exact local evidence is lost, restore from backup or a permitted source with
exact verification. Otherwise report unavailable evidence and block operations
requiring it. Preserve intact frozen records, reservations, Known facts and
history; do not automatically End or Finish a Reading. Operations wholly
supported by intact frozen records may continue, including explicit End/Finish
when their existing requirements are satisfied.

Upstream disappearance differs from confirmed invalid evidence. An integrity
failure or confirmed binding/interpretation defect records an explained usability
restriction without rewriting or deleting immutable evidence. Block affected
consumers and new adoption, notify affected owners, and leave unaffected
capabilities usable where validated. There is no automatic substitute, correction
transfer or frozen-state rewrite. Recovery requires exact restoration or an
explicitly chosen, validated replacement.

### Retention follows every retained reference

Retain exact evidence without expiry while any retained record needs it:
adopted Book evidence; active and ended Reading snapshots; corrections/exclusions;
completed history; deck specifications/artifacts; and in-flight work. Protection
includes the required original artifacts, bindings, interpretations,
transformation dependencies and provenance/rights records, not merely pointers.

Upstream removal, hiding a Book, leaving To Read or removing a connection does
not release those references. Removing one owner's interest cannot reclaim
evidence another retained reference needs. Referenced history can grow
indefinitely; no bounded lifetime-storage promise is made.

Only genuinely unreferenced evidence can become eligible for separately
implemented reclamation. Retain it by default in the MVP. No TTL, purge job,
storage layout or production deletion is selected. Ordinary retention does not
define a new account-deletion or legal-erasure workflow; ADR 0035's separate
decision requirement remains.

### Permission is independent of physical reuse

Follow the component- and operation-specific
[licensing decision](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026).
Reuse openly licensed evidence only within reviewed permissions. Restricted
acquisitions and credentials remain owner/access-scoped. Bob does not inherit
permission from Alice's acquisition, even if identical bytes exist locally.
Acquisition, inspection and exports enforce their respective rights; rights in
text, annotations, conversions and derived artifacts remain separate.

Upstream removal alone does not revoke established permission to retain a
licensed copy. Applicable permissions still govern local use. A rights concern
blocks affected operations for review, not automatic deletion or substitution.
This does not authorize retention or distribution contrary to applicable terms.
Credentials are separately protected and never shared through evidence reuse.

### Capacity and availability guarantees

Adoption requires durable local retention of required evidence, so valid acquired
evidence remains usable during upstream outages subject to applicable permissions.
The operator owns capacity and backups covering both records and artifacts,
with integrity and restore checks required before rollout. Database pointers
alone are not a recoverable publication. No high-availability, zero-data-loss or
uninterrupted-new-acquisition guarantee is implied.

Configure bounds on discovery batches, artifact/decompressed sizes, concurrent
jobs and queued work. Oversized candidates fail clearly rather than being
truncated into apparent completeness. Monitor storage and queue pressure;
pause/reject new acquisition before exhausting capacity instead of evicting
protected evidence. Do not ingest a whole repository as a convenience.

The architectural handoff must supply initial numerical retry/time/resource
limits and their enforced scope. This policy does not invent a universal timeout
or corpus-size estimate before selecting the exact MVP candidate. Persistent
history growth, upstream rate limits and unreliable change signals, artifact
sizes, shared-job contention and backup recovery remain bounded rollout risks.
Use candidate-specific measurements to set limits; if a concrete experiment
becomes a prerequisite, ticket it then rather than claiming measured capacity.

## Stress cases

| Case | Accepted result |
| --- | --- |
| Alice and Bob adopt different producers | Independent current evidence; Alice's refresh cannot switch Bob or reveal his restricted acquisition. |
| Replacement fails validation or publication | Old valid evidence and To Read remain; no mixed revisions or correction transfer. |
| Upstream withdraws exact text | Retained permitted copy remains usable; discovery reports upstream unavailability separately. |
| Exact local copy is lost too | Restore verified exact evidence or show unavailable; never silently choose another Edition/revision. |
| B finishes after owner chooses C or cancels | B cannot adopt; no cancellation of another owner's independent request. |
| Published evidence has a confirmed defect | Restrict affected usability and explain it; retain immutable provenance and frozen learner state. |
| Storage pressure or connection removal | Stop new work as needed; protected references and evidence are not evicted. |

## Trade-offs, compatibility and handoff

Global latest-wins updates and silent fallback are convenient but make scholarly
and learner evidence unstable. Expiring referenced revisions makes history
unreproducible. Blanket reuse conflates storage with permission. The accepted
costs are explicit owner choice, retained storage growth, failure states and
operator backup responsibilities.

For future classical support, this qualifies ADR 0040's automatic completed-run
replacement with explicit owner-authorized adoption, while preserving its
single-current-analysis rule. Preserve ADR 0052's one domain classification:
upstream freshness is not a second or competing Analysis evidence classifier.
Extend that future classification to adopted evidence/capability restrictions
without changing today's EPUB behavior. Preserve ADRs 0093/0094/0096,
ADR 0081's exact-analysis corrections, and ADRs 0078/0085/0071's frozen
Reading/history/deck contracts. No current schema or modern-language lifecycle
is changed.

[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030)
owns exact candidate acceptance.
[Validate the shared-core architecture and staged implementation handoff](https://github.com/justin-hayes/mouseion/issues/2031)
owns concrete interfaces, storage/reference protection, migration/backfill,
numerical limits, recovery checks and staged acceptance. Its test plan must
cover the stress cases above, duplicate/restarted jobs without budget reset,
access isolation, retention/adoption races, and exact restore. No empirical
capacity result or new research prerequisite is asserted by this decision.
