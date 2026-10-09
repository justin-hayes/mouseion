# Documentation Governance

Where each kind of project documentation lives, which source is authoritative,
and how a decision moves from exploration into the repository. Read this before
adding, editing, or moving project documentation.

## The repository records present state

The repository documents what Mouseion is and what has been decided. Each kind
of document has one home:

| Document | Location | Role |
| --- | --- | --- |
| Product summary | [`product.md`](product.md) | Lean present-state summary: pipeline, stack, and the ADR index. |
| Domain glossary | [`CONTEXT.md`](../CONTEXT.md) | The ubiquitous language used in code, UI, and documentation. |
| Architecture | [`ARCHITECTURE.md`](../ARCHITECTURE.md) | How the system is built: processes, pipelines, jobs, data model, and code map. |
| Linguistic design | [`linguistics.md`](linguistics.md) | The linguistic decisions behind analysis, selection, and evidence, with evaluation and limitations. |
| Decisions | [`adr/`](adr/) | Accepted architecture and product-boundary decisions. |
| Feature specifications | [`features/`](features/) | Behavior, motivation, and scope of individual features. |
| Design | [`design/`](design/) | Principles, design system, and information architecture. |
| Research and evidence | [`research/`](research/), [`evidence/`](evidence/), [`reviews/`](reviews/) | Source surveys and reproducible measurements that informed decisions. |
| Archive | [`archive/`](archive/) | Retired features and dated operational records, kept for provenance. |

Exploration, brainstorming, and session notes are kept outside the
repository. They are working material, not a second source of truth, and they
are not synchronized with the repository.

## How decisions are promoted

The flow is one-way: exploration → repository, at promotion events only.

1. Work that is still being shaped stays outside the repository.
2. When a requirement or decision stabilizes, record it as an ADR and update
   `product.md` only if the present-state summary changed.
3. ADRs are edited only in the repository. A changed decision gets a new ADR
   that amends or supersedes the earlier one; the earlier record is kept.
4. If the same decision appears in two places, the repository is
   authoritative.
5. When unsure whether something belongs in the repository, ask: *is it stable
   and implementable, or still being worked out?* Stable → repository.

When documents disagree with the code, executable truth wins; fix the document.

## Rationale

Keeping one canonical location per artifact avoids the cost and drift of
bidirectional synchronization. "What did we decide?" is always answerable from
the repository, while exploration stays free to be incomplete.

## Related

- [Product summary](product.md)
- [Documentation index](README.md)
- [ADR 0070: Migration and documentation reboot](adr/0070-migration-and-documentation-reboot.md)
