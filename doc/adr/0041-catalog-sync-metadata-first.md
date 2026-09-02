# ADR 0041: Catalogue sync is metadata-first and non-destructive

Status: **Proposed** · Date: 2026-09-02 · Author: Justin + Codex

## Context

Mouseion's manual acquisition path is intentionally selective: a learner-owned
catalogue connection (ADR 0024) exposes entries, and **Add to My Books** acquires
one validated EPUB at a time under ADR 0035's membership and source-provenance
contract. That path is appropriate for deliberate per-entry acquisition, but it
does not scale to a learner whose collection is already curated in a separate
Calibre installation and exposed through Calibre-Web OPDS. The learner wants the
whole studyable portion of that collection represented in My Books without
clicking every entry or downloading every EPUB immediately.

Mouseion already has the foundations for a bounded sync: owner-scoped encrypted
catalogue credentials, an origin-scoped OPDS client, Calibre-Web language feeds,
bounded pagination, metadata-only My Books membership, and River background
work. The remaining decision is which books enter sync, what state sync may
change, and how content acquisition relates to reconciliation.

## Decision

Each learner-owned catalogue connection receives one River periodic sync job.
The connection surface also provides **Sync now**, which enqueues the same job,
and each catalogue-backed book provides a metadata-refresh action for that book.
Sync work remains operational work visible through the existing `/jobs` surface;
it does not create a learner destination.

A sync walks the Calibre-Web language feeds for the learner's **ready study
languages, excluding English**. A ready study language is both saved by the
learner and advertised as ready by the NLP service. English is excluded under
the product assumption that it is every learner's native language. Languages
that are merely present upstream, not saved, or not currently ready are outside
the run.

Sync is **metadata-first**. It upserts bibliographic metadata only: title,
language, and catalogue identity. New matches become metadata-only Books with
active My Books membership through ADR 0035's existing owner-scoped identity,
alias, and duplicate rules. Sync does not download EPUB content. Content is
acquired lazily after learner intent is expressed by opening or choosing a
metadata-only book for analysis; the exact trigger and execution experience are
left open below. Acquired state still requires a complete, persisted, validated
source snapshot.

Reconciliation is upsert-only and never destructive:

- metadata changes update the matching owner-scoped Book;
- newly eligible entries create metadata-only Books and membership;
- entries removed upstream remain in My Books;
- deleting a catalogue connection never removes Books, membership, acquired
  content, scopes, analyses, decks, or vocabulary; and
- a sync never re-downloads content.

The product owner accepts the underlying EPUB content as immutable for sync.
Consequently, sync never creates a source revision and never invalidates a
reviewed scope or analysis. ADR 0035's rule that manual acquisition of different
bytes for the same source identifier creates a new immutable revision remains
unchanged and applies only to that manual acquisition path.

Each connection exposes a last-synced status and distinguishes never synced,
syncing, last synced, and failed. A worker loads the connection in the requesting
owner's scope and decrypts its existing AES-256-GCM credential server-side at job
time. It reuses the origin-scoped OPDS client, `ListLanguages` / `ListLanguage`
Calibre-Web feeds, `rel=next` traversal, and the existing 100-page cap. Runs are
owner-isolated, idempotent, retryable, and safe at any supported interval.

This ADR uses ADR 0035's metadata-only membership. It does not amend ADR 0035 or
the status line of any prior ADR.

## Alternatives considered

- **Acquire all EPUB content during the first sync.** Rejected because a
  library-scale download consumes storage and network resources before learner
  intent, crosses the language-capability gate, and turns calm background work
  into a large mandatory operation.
- **Re-download content during every sync and detect changes.** Rejected by the
  product owner. Sync trusts content immutability and avoids source-revision,
  scope-invalidation, and analysis-invalidation machinery. The manual
  acquisition path retains ADR 0035's changed-byte behavior.
- **Sync every non-English language.** Rejected because books in languages the
  learner has not selected or the NLP service does not advertise as ready
  cannot enter the analysis lifecycle and would be dead weight.
- **Use administrator-managed catalogues.** Already rejected by ADR 0024;
  catalogue connections and credentials remain learner-owned, and this decision
  does not reopen that boundary.

## Consequences

- My Books may contain many metadata-only entries whose content is acquired only
  when the learner expresses intent.
- The manual selective acquisition workflow remains available and retains its
  stronger validated-content behavior.
- Repeated and periodic runs are idempotent and safe on any supported cadence;
  upstream removal cannot remove learner-owned state.
- No sync event can enter the scope or analysis lifecycle as an invalidation.
- Connection status and operational job history must explain progress and
  recovery without adding navigation.

## Open questions

- What is the default sync cadence, and may each connection configure it?
- Does opening a metadata-only book trigger content acquisition, or must the
  learner choose an explicit acquire action?
- Does lazy acquisition run inline or as queued River work?
- Does per-connection last-synced status require a schema change? If so, its
  migration crosses the repository's human-review boundary.
- When a learner removes a study-language preference, existing synced
  metadata-only Books remain, just as Settings already preserves books,
  analyses, decks, and vocabulary; what explanatory copy should accompany that
  state?

## Related

- [ADR 0010: Adopt River as the background-job queue](0010-river-job-queue.md)
- [ADR 0012: Enrichment execution via River](0012-enrichment-execution-via-river.md)
- [ADR 0024: Learner-owned catalogs and removal of the admin role](0024-learner-owned-catalogs-no-admin.md)
- [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](0028-explicit-scoped-analysis-lifecycle.md)
- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [Catalogue sync feature](../features/catalog-sync.md)
- [Catalogue sync workflow](../design/workflows/catalog-sync.md)
