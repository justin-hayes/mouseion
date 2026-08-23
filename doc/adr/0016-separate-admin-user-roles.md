# ADR 0016: Separate admin and user account roles

Status: **Accepted** · Date: 2026-08-23 · Author: Justin + Hermes

## Context

ADR 0014 made administration and learning orthogonal capabilities so that a server operator could use one account for both. Justin prefers the clearer boundary and is comfortable managing two accounts: one for operating the server and one for studying. Allowing an administrator into the learner workflow also gives a privileged account unnecessary access to book imports and per-user learning state.

## Decision

**Admin and user are mutually exclusive account types. An account is exactly one of them, never both.** This decision amends and supersedes ADR 0014's orthogonal-role model, including its statement that an admin can also be a learner.

An admin account is limited to server configuration and account management. It can create accounts, configure supported languages, upload shared frequency data, and manage OPDS connections. It cannot select a study language, browse or import catalog books, analyze books, manage known vocabulary, review vocabulary, or export decks.

A user account is limited to the learner workflow. It can use administrator-configured languages, frequency resources, and OPDS connections to import and analyze books, manage known vocabulary, review vocabulary, and export decks. It cannot manage accounts, supported languages, frequency data, or OPDS connections.

Authentication records use one role discriminator, and account creation accepts one validated role. Web routes and navigation are split by that role: authenticated admins are denied learner routes, and authenticated users are denied admin routes. The application root sends each account type to its own surface.

ADR 0015's division between admin-managed OPDS connections and user-level browsing and import remains unchanged.

## Alternatives considered

- **Keep roles orthogonal.** Rejected: Justin prefers managing separate accounts, and the separation makes the privilege boundary explicit.
- **Hide the other surface only in navigation.** Rejected: navigation is not an authorization boundary; direct requests must also be denied.
- **Add independent admin and learner flags.** Rejected: two flags can represent an invalid both-roles state and weaken the mutually exclusive model.

## Consequences

- The server operator uses a separate user account for learning.
- Admin sessions expose only server configuration and account-management navigation and routes.
- User sessions expose only learner navigation and routes.
- Existing admin-owned learner data remains stored but is inaccessible while the account is an admin.
- Authorization tests cover both directions of the route boundary.

---

## Related

- [ADR 0014: Admin and learner roles are orthogonal](0014-admin-learner-roles.md) — superseded by this mutually exclusive account model.
- [ADR 0015: OPDS connections are admin-configured; browsing and import are user-level](0015-opds-connection-admin-browse-user.md) — retains the configuration-versus-use boundary.
- [Product summary](../product.md)
