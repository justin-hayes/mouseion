# ADR 0088: Fixture adapter transitions are held to store contracts

Status: **Accepted** · Date: 2026-10-11 · Author: Justin + OpenCode

Tracked by [issue #1720](https://github.com/justin-hayes/mouseion/issues/1720). Amends [ADR 0033](0033-browser-acceptance-harness.md).

## Context

ADR 0033 describes the fixture server as fixtures "already in a given state".
The in-memory adapter has since grown transitions that browser tests click
through: Start, Switch, End, Finish, deck admission, lemma decisions, visibility,
disposition, and previously-read import. Only Finish currently shares cases with
the PostgreSQL adapter, and the duplicated deck admission tables have already
drifted. Browser evidence alone cannot establish that a fixture transition
behaves like production.

## Decision

1. **Keep the in-memory fixtures adapter.** The browser harness continues to
   need no Docker or external services and retains deterministic data and stable
   IDs, for the reasons in ADR 0033.
2. **Transitions are contractual.** A fixtures transition is allowed only when
   an `internal/storecontract` scenario holds it to the same behavior in both
   the fixtures and PostgreSQL harnesses. Fixtures-only tests or independently
   maintained case tables do not establish parity.
3. **Everything else is illustrative.** Canned states for browser scenarios
   must be marked as illustrative; they are not held to production parity.
   Illustrative fixtures do not exempt a transition from the shared-contract
   requirement.

## Alternatives considered

- **Run the fixtures server on PostgreSQL.** Requires Docker, loses stable IDs,
  and would move 204 browser-spec references. It sacrifices ADR 0033's
  deterministic, infrastructure-independent harness.
- **Allow canned states only.** Avoids adapter transition drift but loses browser
  coverage of transitions through real navigation and forms.

## Consequences

- Browser tests retain both deterministic state presentation and transition
  coverage, while shared store contracts establish transition parity.
- Adding or changing a fixtures transition requires a shared scenario exercised
  against both adapters; presentation-only canned states need no parity test.
- This decision does not claim that the existing adapter already satisfies the
  boundary. [Issue #1721](https://github.com/justin-hayes/mouseion/issues/1721)
  splits fixtures by feature and marks contractual versus illustrative code;
  [issue #1722](https://github.com/justin-hayes/mouseion/issues/1722) introduces
  `internal/storecontract` for Current reading lifecycle and deck admission,
  including migration of the existing shared Finish cases. Other transitions
  must also be covered before they can be treated as contractual.
