# ADR 0009: Home-lab authentication and corpus-artifact isolation

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0002 established a multi-user account model with an admin role and per-user learning state, but left two details open:

1. **Authentication mechanism** and where credentials live in a self-hosted home-lab deployment (local accounts vs. an external identity provider).
2. **Whether normalized corpus artifacts are shareable across users**, accounting for the licensing and privacy of a user's private EPUBs.

These are Open Questions 8 and 10 in `product.md` and consolidated as issue #29. The deployment context is a **self-hosted home lab** used by two people, and — critically for the security posture — the application is reachable **only over a private Tailscale tailnet for the first pass**, with no public-internet exposure planned unless that proves insufficient.

## Decision

### 1. Authentication — local accounts, admin-provisioned; no external identity provider in v1

- **Local accounts**, no external IdP (no Keycloak/OAuth/SSO) in v1. An external identity provider adds a heavy operational dependency and external trust for a private two-person tool with no benefit at this scale.
- **Registration is OFF by default.** Accounts are **admin-provisioned** (no open self-signup) — the secure default for a private tool.
- **First-admin bootstrap:** the first user becomes the admin (or via an explicit bootstrap flag/env). Documented and deliberate, not accidental.
- **Credential storage:** **argon2id** (OWASP-recommended, memory-hard). Never plaintext or reversible hashes.
- **Sessions:** **HTTP-only** session cookies; server-side session store in Postgres (ADR 0003); **short lifetime** (~24h) with sliding expiry; per-user session invalidation (logout-everywhere).
- **Recovery:** no self-serve email password reset in v1 (no email infra). **Admin-reset** is the recovery path (an admin sets a new password).
- **No CLI authentication** — ADR 0004 removed the standalone CLI; the web session is the only client.

### 2. Corpus-artifact isolation — lemma-level data shared; source text and example sentences stay per-user

- **Source EPUBs and their full text are NOT shared.** Each user's uploaded source material is **owned by that user**, private, and never exposed to another user — the licensing/privacy red line.
- **Example sentences remain per-user.** They are drawn from the user's own source (which they own) and are not exposed to other users. This avoids any cross-user reproduction of copyrighted text.
- **Lemma-level normalized data is shared.** The vocabulary identity, canonical lemma, POS, morphology, and frequency signal (the ADR 0005 identity) are shared for cross-user dedup and ranking. This is not copyrighted content and is required for the shared vocabulary cache (ADR 0007) and global ranking.
- **Concretely:** the normalized-corpus artifact is keyed by a **content-derived hash** of the source (not the owner), but the *shareable* form carries only the lemma/statistical level. Full verbatim sentence text is stored per-user, not shared.
- This means **sentence selection (issue #20) and review (issue #21) stay per-user**, while **identity, ranking, and cross-user caches operate on lemma-level data** — consistent with ADR 0005 and ADR 0007.

### 3. Ownership, authorization, and lifecycle

- **Ownership:** source materials carry an `owner_user_id`; user-owned learning-state rows carry `user_id`.
- **Authorization:** row-level ownership enforced in the core (issue #8 requires one user cannot read/mutate another's learning state). Source materials: owner-only. Lemma-level data: shared, read-only.
- **Deletion/retention:** a user's source materials are deleted on account deletion or explicit delete; derived lemma-level data may persist (it is not the user's private text). No indefinite retention of private EPUB content.
- **At-rest:** encrypt the Postgres volume at rest (e.g. LUKS on the host volume or DB-level encryption). This is largely a deployment concern (see §4) but is the practical control for a self-hosted DB.

### 4. Network exposure and threat model

- **Network exposure: Tailscale-only for v1.** The app is reachable only over the private tailnet; **not exposed to the public internet** unless that proves insufficient.
- **TLS:** because Tailscale provides WireGuard peer-to-peer encryption in transit, the app may serve **plain HTTP over the tailnet** as the initial default — transport is already encrypted. Session cookies are HTTP-only. *(If HTTPS is later enabled — e.g. `tailscale cert` or a TLS reverse proxy — flip cookies to `Secure`.)*
- **Threat model (bounded, home-lab):**
  - **Another home-lab user** accessing another's data (the primary threat; two users) → per-user ownership + authorization.
  - **Compromised web session** → short sessions, HTTP-only cookies, CSRF protection (issue #24).
  - **Stolen DB volume / backups** → at-rest encryption of the DB volume.
  - **Compromised admin account** → admin is trusted by design; documented.
  - **Public-internet exposure** → explicitly out of scope for v1; if it ever happens, add a TLS reverse proxy, Secure cookies, and reassess MFA.

## Alternatives considered

- **External identity provider / SSO in v1.** Rejected: heavy operational dependency and external trust for a two-person private tool; local accounts suffice.
- **Full normalized-corpus sharing across users (including verbatim sentence text).** Rejected: could reproduce a user's private copyrighted source to another user; the lemma-level sharing avoids the leak entirely.
- **Self-registration / open signup.** Rejected: insecure default for a private tool; accounts are admin-provisioned.
- **bcrypt/scrypt for password hashing.** Rejected in favor of argon2id (memory-hard, OWASP-recommended).
- **HTTPS required from the start.** Rejected for v1: Tailscale already encrypts in transit; plain HTTP over the tailnet is sufficient and simpler. HTTPS becomes required only if exposed publicly.

## Consequences

- Auth (issue #11) and the web shell (issue #24) implement local accounts, argon2id, admin-provisioned users, and short HTTP-only sessions.
- The persistence layer (issue #8) enforces row-level ownership: per-user source/learning data, shared read-only lemma-level data.
- Sentence selection (#20) and review (#21) are per-user; identity/ranking/caches are shared.
- Source materials are deleted with the user; at-rest DB encryption is a deployment concern recorded for the home-lab setup.
- Open Questions 8 and 10 in `product.md` are resolved; #29 can be closed. Issues #11, #24, #8 are updated per the decision.

## Open questions

- Whether to enable HTTPS within the tailnet (e.g. `tailscale cert`) for defense-in-depth and `Secure` cookies, even though it is not strictly required.
- Whether MFA should be added if the tool is ever exposed beyond the home LAN.

---

## Related

- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — the account model and open questions this resolves.
- [ADR 0004: Web application as the sole v1 client](0004-web-only-v1-client.md) — no CLI, so no CLI authentication.
- [ADR 0003: PostgreSQL as the initial persistence backend](0003-postgresql-persistence.md) — Postgres session store and at-rest volume.
- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md) — the lemma-level identity shared across users.
- [Product specification](../product.md) — resolves Open Questions 8 and 10; updates the Decision Register.
