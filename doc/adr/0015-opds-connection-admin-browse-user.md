# ADR 0015: OPDS connections are admin-configured; browsing and import are user-level

Status: **Accepted** · Date: 2026-08-22 · Author: Justin + Hermes

## Context

Mouseion uses OPDS catalogs as sources for books. A catalog connection combines server-level configuration—the catalog URL and credentials—with learner-facing actions such as browsing a catalog and choosing a book to import. Earlier product documentation did not assign those responsibilities consistently, and the current implementation from issues #15 and #24 stores connections per user. The 2026-08-22 session raised this ambiguity for resolution.

## Decision

**OPDS catalog connections are admin-configured, while browsing and importing from a configured catalog are user-level actions.**

An admin creates and maintains each connection, including its server URL and credentials. Mouseion holds those credentials as server configuration; learners can use the configured connection but cannot view or change its credentials.

Any user can browse the catalogs made available by an admin and import a selected book into that user's own corpus for analysis. Imported source material, analysis, and subsequent vocabulary state remain scoped to the importing user.

The current implementation from issues #15 and #24 models OPDS connections as per-user records. **That implementation must change** to admin-configured connections shared for user-level browsing and import.

## Alternatives considered

- **Keep connections per user.** Rejected: it duplicates common home-lab configuration and credentials, and makes each learner responsible for server setup.
- **Restrict browsing and import to admins.** Rejected: selecting source material is part of the learner workflow, not a server-administration task.
- **Let users configure shared connections.** Rejected: catalog endpoints and credentials are privileged server configuration and require an explicit administrative boundary.

## Consequences

- OPDS connection storage and authorization must move from per-user ownership to admin-managed server configuration.
- Credentials remain hidden from learner-facing views and are used server-side when a learner browses or imports.
- Catalog availability is shared, while imported books and derived learning state remain owned by the learner who imports them.
- Existing per-user connection data will need a migration or other transition when this decision is implemented.

---

## Related

- [ADR 0014: Admin and learner roles are orthogonal](0014-admin-learner-roles.md) — defines the capability model used here.
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — establishes per-user learning-state ownership.
- [Product summary](../product.md)
