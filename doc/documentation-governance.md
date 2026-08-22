# Documentation Governance

How project documentation is split between the Obsidian vault and this repository, and which direction truth flows. Read this before adding, editing, or moving any project documentation.

## The one-way promotion boundary

The vault and the repo serve different roles and are **not** kept in sync. Divergence between them is expected and correct; the rule below makes divergence safe by making the direction of truth unambiguous.

- **Vault = origin / working.** Raw ideas, session write-ups, brainstorming transcripts, exploration, and the extended product description. It grows freely and is never canonical. Nothing in the vault should be treated as an authoritative statement of the product.
- **Repo = canonical / stable.** Distilled, implementable requirements, the ADRs, and the Decision Register. Content only enters the repo through **promotion** from the vault, in simplified form, once requirements are stable enough to implement against.

The flow is **vault → repo, at promotion events only.** Content is never copied repo → vault. The vault accumulates and is messy; the repo reflects current truth.

## ADRs live only in the GitHub repo

Architecture Decision Records are the canonical record of "we decided X." They live with the code they describe, so they belong **only** in this repository (`doc/adr/`). They are **never duplicated in the vault** — and the vault does **not** keep ADR pointers either. All ADR references (pointers, links, index notes) live in the GitHub repos; the Obsidian vault is a place for raw thinking and session write-ups, not for ADRs or pointers to them.

If a decision's existence needs to be referenced outside the repo, point at the GitHub repo/ADR URL directly rather than maintaining a mirror in the vault.

## The Decision Register

The repo's Decision Register is authoritative. The vault may carry a working/planning register as a roadmap of what is still undecided, but once a decision is accepted and promoted, it is referenced **only** by the repo ADR — the vault does not restate it or hold a pointer to it.

## Practical rules

1. New sessions and brainstorming go in the vault as-is. Do not also write them to the repo.
2. When a requirement or decision stabilizes, **promote** a simplified version to the repo (product.md, an ADR, or both). The vault note that originated it needs no ADR pointer — leave it as raw thinking, or link to the repo ADR URL if a link is helpful.
3. Never edit an ADR in the vault. ADRs are edited only in the repo; a superseded decision gets a new ADR that amends it.
4. If you find the same decision documented in both places, the repo is authoritative — remove or ignore the vault copy, do not try to keep them in sync.
5. When unsure whether something belongs in the vault or the repo, ask: *is it stable and implementable, or still being worked out?* Stable → repo. Working → vault.

## Rationale

This avoids the cost and corruption of bidirectional sync: no copy of a decision is ever maintained in two places, so they cannot drift into disagreement. A single canonical location per artifact makes "what did we decide?" always answerable from the repo, while the vault remains free to capture raw thinking.

---

## Related

- [Product specification](product.md)
- [ADR 0001](adr/0001-go-core-python-nlp-service.md), [ADR 0002](adr/0002-multi-user-accounts.md)
- Obsidian vault: `1 Projects/Vocabulary Acquisition Tool/` (origin; session write-ups and the extended product description)
