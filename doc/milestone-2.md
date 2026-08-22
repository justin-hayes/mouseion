# Milestone 2: Feedback-driven polish (v1 → v2)

Status: **Planned** · Date: 2026-08-22

## v1 retrospective (what the MVP established)

The v1 MVP is complete and working end-to-end:

- **Pipeline**: OPDS/EPUB ingest → River analysis (size-chunked) → gRPC → Stanza → selection → ranking → enrichment → sentence selection → review → Anki TSV export.
- **13 ADRs** (0001–0013) record the architecture.
- **28 issues** closed across the build.
- Deployed as 3 processes (Postgres + Stanza gRPC + Go web) on the home lab.

**Feedback channels** for v2:
- Local usage telemetry in Postgres (review decisions, export behavior, drop-off).
- In-app feedback channel.
- Structured manual walkthroughs + interview with the second user.

## Milestone 2 scope (agreed)

Three topics promoted from the v1 backlog to plan and develop now:

1. **Known-vocab import UX** — a web UI for importing per-user known-vocabulary lists (issue #16 is backend-only; add the UI). Lets selection correctly exclude what the learner already knows.
2. **LLM translation provider** — wire a real LLM translation provider into the enrichment pipeline (the provider interface exists; the LLM call is currently a stub). Per ADR 0007: translation is external/optional, lemma + one example sentence only, admin-config + per-user opt-in, cached + versioned.
3. **OPDS import UX polish** — improve the catalog browse/acquire flow (per-connection navigation, search, acquisition feedback, error surfacing).

**Deferred to the backlog:**
- Priority-list / word-list management (selection's `PriorityIdentities` mechanism exists; the management UX is deferred).
- `.apkg` deck export.
- Multi-language beyond German.
- Dictionary translation provider (alternative to LLM).

## Workflow

- Open one issue per topic with Overview, Acceptance criteria, Dependencies, and References (mirroring the v1 issue format).
- Implement via the established dev loop: Codex brief → implement → independent verify → PR → review.
- Record any new architectural decisions as ADRs.

## Open questions / future backlog

- Whether to add the local usage telemetry + feedback channel first (enables evidence-based prioritization).
- Exact LLM provider choice (OpenAI/Anthropic/self-hosted) and per-user opt-in UX.
- Whether OPDS polish should include per-connection credentials management.

---

## Related

- [Product specification](product.md)
- [ADR 0006](adr/0006-anki-export-import-contracts.md) (known-vocab import contract)
- [ADR 0007](adr/0007-enrichment-providers-caching-privacy.md) (enrichment providers, translation)
- [ADR 0009](adr/0009-home-lab-auth-corpus-artifact-isolation.md) (owner scoping, per-user state)
