# ADR 0014: Admin and learner roles are orthogonal

Status: **Accepted** · Date: 2026-08-22 · Author: Justin + Hermes

## Context

ADR 0002 introduced multi-user accounts and an admin role for global resources, but its "admin vs. user" wording can imply that administration and learning are mutually exclusive account types. In practice, the person operating a self-hosted Mouseion server may also use it to study a language. The product needs a clear capability boundary without forcing that person to maintain separate accounts or exposing server configuration throughout the learning experience.

## Decision

**An account can be both an admin and a learner; the two roles are orthogonal.** Administration is a set of server/configuration capabilities, while learning is the study workflow available to an account.

### Admin capabilities

An admin can:

- create other non-admin users;
- configure which languages the server supports;
- upload frequency data for a language, such as DWDS data for German; and
- configure OPDS catalog connections.

### Learner capabilities

A learner can:

- select a language of study from the admin-configured set;
- add known-vocabulary lists;
- browse and import from configured OPDS catalogs;
- import and analyze books;
- view analysis results;
- review vocabulary; and
- export Anki decks.

Admin tools must be coherent and organized as a distinct configuration surface so that they do not clutter the learner UI. An admin who also learns uses the same account and moves between these capability areas as needed.

This decision supersedes or refines earlier language that treats "admin" and "user" as a separation, including ADR 0002 §3. ADR 0002's ownership model remains: learner state is per-user, while server reference resources are global and language-scoped. This ADR clarifies that an admin may also own learner state.

The OPDS capability boundary is defined more precisely by [ADR 0015](0015-opds-connection-admin-browse-user.md).

## Alternatives considered

- **Make admin and learner mutually exclusive account types.** Rejected: a self-hosted operator commonly studies with the same application, and separate accounts would add friction without improving the capability boundary.
- **Expose admin controls alongside learner actions.** Rejected: server configuration is infrequent and privileged; mixing it into the study workflow would clutter the primary UI and make authorization boundaries less legible.
- **Treat every learner as an admin.** Rejected: non-admin users must not be able to create accounts, change supported languages, upload shared reference data, or manage server-held catalog credentials.

## Consequences

- Authorization checks model admin capability independently of access to learner features.
- Admin accounts can hold ordinary per-user learning state and complete the full study workflow.
- Navigation and screens group server configuration into a coherent admin area separate from learner-facing work.
- Non-admin users consume the supported languages, frequency resources, and OPDS connections configured for the server without being able to alter them.

---

## Related

- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — establishes the account and ownership model refined here.
- [ADR 0015: OPDS connections are admin-configured; browsing and import are user-level](0015-opds-connection-admin-browse-user.md) — defines the OPDS boundary.
- [Product summary](../product.md)
