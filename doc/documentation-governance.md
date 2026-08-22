# Documentation Governance

How project documentation is split between the Obsidian vault and this repository, and which direction truth flows. Read this before adding, editing, or moving any project documentation.

## The lean-summary split

The vault and the repo serve different roles and are **not** kept in sync. Divergence between them is expected and correct.

- **Repo = present state.** The lean [`product.md`](product.md) summarizes what Mouseion is, its current pipeline and stack, operations, and the ADR index. Accepted decisions live in `doc/adr/`.
- **Vault = planning and working material.** MVP planning, milestones, sessions, raw ideas, and exploration stay in the vault rather than expanding the repo's product summary.
- **ADRs = repo-only.** Accepted architecture and product-boundary decisions live only in `doc/adr/`; they are not duplicated in the vault.

The flow is **vault → repo, at promotion events only**: stable decisions are distilled into ADRs and the lean present-state summary is updated when necessary. Content is not copied back into the vault.

## ADRs live only in the GitHub repo

Architecture Decision Records are the canonical record of "we decided X." They live with the code they describe, so they belong **only** in this repository (`doc/adr/`). They are **never duplicated in the vault** — and the vault does **not** keep ADR pointers either. All ADR references (pointers, links, index notes) live in the GitHub repos; the Obsidian vault is a place for raw thinking and session write-ups, not for ADRs or pointers to them.

If a decision's existence needs to be referenced outside the repo, point at the GitHub repo/ADR URL directly rather than maintaining a mirror in the vault.

## Practical rules

1. New sessions and brainstorming go in the vault as-is. Do not also write them to the repo.
2. When a requirement or decision stabilizes, **promote** it to a repo ADR and update the lean `product.md` only if the present-state summary changed. The vault note that originated it needs no ADR pointer.
3. Never edit an ADR in the vault. ADRs are edited only in the repo; a superseded decision gets a new ADR that amends it.
4. If you find the same decision documented in both places, the repo is authoritative — remove or ignore the vault copy, do not try to keep them in sync.
5. When unsure whether something belongs in the vault or the repo, ask: *is it stable and implementable, or still being worked out?* Stable → repo. Working → vault.

## Rationale

This avoids the cost and corruption of bidirectional sync: no copy of a decision is ever maintained in two places, so they cannot drift into disagreement. A single canonical location per artifact makes "what did we decide?" always answerable from the repo, while the vault remains free to capture raw thinking.

---

## Related

- [Product summary](product.md)
- [ADR 0001](adr/0001-go-core-python-nlp-service.md), [ADR 0002](adr/0002-multi-user-accounts.md)
- Obsidian vault: `1 Projects/Vocabulary Acquisition Tool/` (origin; session write-ups and the extended product description)
