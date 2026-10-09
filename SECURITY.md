# Security Policy

## Deployment model

Mouseion is designed for a small, trusted, self-hosted deployment reachable
only over a private network such as a Tailscale tailnet
([ADR 0009](doc/adr/0009-home-lab-auth-corpus-isolation.md)). In that model:

- The web server speaks plain HTTP; transport encryption is provided by the
  private network. Set `MOUSEION_COOKIE_SECURE=true` when serving HTTPS through
  a reverse proxy.
- The first sign-in on a fresh installation creates the first account. There is
  no public registration setting and no administrator role.
- Passwords are hashed with Argon2id. Sessions use `HttpOnly`, `SameSite=Lax`
  cookies, and state-changing forms carry CSRF tokens.
- OPDS catalog credentials are encrypted at rest with AES-256-GCM using a key
  derived from `MOUSEION_SECRET`.
- Learner data (books, analyses, vocabulary, and decks) belongs to its owning
  account, and each learner sees only their own
  ([ADR 0002](doc/adr/0002-multi-user-accounts.md)).
- Optional external translation sends sentences from a learner's books to the
  configured OpenAI-compatible provider. It is disabled unless
  `MOUSEION_LLM_ENABLED=true`
  ([ADR 0007](doc/adr/0007-enrichment-providers-caching-privacy.md)).

Exposing Mouseion directly to the public internet is outside this model and is
not supported.

## Supported versions

Only the latest commit on `main` is supported. There are no maintained
release branches.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://github.com/justin-hayes/mouseion/security/advisories/new)
rather than in a public issue. Include the affected component, steps to
reproduce, and the impact you expect. You can expect an acknowledgement within
a week. This is a personal project maintained in spare time, so fixes are made
on a best-effort basis.
