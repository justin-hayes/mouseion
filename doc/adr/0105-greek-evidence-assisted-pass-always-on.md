# ADR 0105: The Ancient Greek evidence-assisted pass runs whenever its assessment is recorded

Status: **Accepted** · Date: 2026-10-18 · Author: Justin + opencode

Tracked by [#2171](https://github.com/justin-hayes/mouseion/issues/2171).
Amends the "Evidence-assisted provider pass (opt-in)" section of
[ADR 0101](0101-ancient-greek-cards-from-source-evidence.md) for Ancient Greek
(`grc`) only.

## Context

ADR 0101 made the evidence-assisted pass opt-in per deck: the deck
specification records the choice, off by default, offered only while a
matching provider payload assessment is recorded. The pass shipped (#2202) with
that flag stored and gated on the server, but no control sets it, so even a
recorded assessment would leave the pass unreachable.

Modern languages take the opposite position. Under
[ADR 0079](0079-contextual-glosses-require-llm.md) every newly prepared card
requires the configured LLM, with no per-user or per-deck consent or decline
path. The reason for a Greek-specific gate is the personal-use review of the
exact payload, and that gate is the recorded assessment, not a per-deck
choice. A per-deck switch adds a control and a decision to every preparation
without adding a safeguard.

## Decision

**The pass runs for every Ancient Greek deck when a provider is configured and
a matching payload assessment is recorded.** "Matching" is unchanged from
ADR 0101: the configured provider, the prompt contract and the transmitted
field set equal the recorded ones.

**Otherwise the deck is the default card, and nothing is sent.** This covers
no configured provider, an unrecorded assessment, and a recorded assessment
that no longer matches (a changed provider, prompt or payload). A drifted
assessment therefore never fails preparation; it withholds the pass until it
is re-recorded.

**Everything else in ADR 0101 stands.** The pass only adds to the default
card, every field it contributes is labelled provider-proposed, a failed or
abstaining answer leaves the default card, and Greek decks have
sentence-quality omissions only. The default card remains the fallback and
needs no provider.

**There is no per-deck choice.** The planner no longer reads the deck
specification's `evidence_assisted_provider_pass` column. The column added by
migration `000043` stays in place, unread, because shipped migrations are
immutable; dropping it would be a separate, reviewed migration.

## Consequences

- No web control is needed for the pass.
- Recording the #2171 assessment, with a provider configured, changes every
  Greek deck prepared afterwards. A prepared deck keeps what it froze;
  re-preparation (ADR 0076) is how an existing deck gains the pass.
- An operator who wants providerless Greek decks while a provider is
  configured for modern languages has no switch. Leaving the assessment
  unrecorded is the only way to keep the pass off.
- Modern languages are unchanged.

## Related

- [ADR 0101](0101-ancient-greek-cards-from-source-evidence.md): amended as
  described above.
- [ADR 0079](0079-contextual-glosses-require-llm.md): the modern-language rule
  this aligns with.
- [ADR 0104](0104-compact-lsj-definition-outline.md): the LSJ definitions the
  pass sends as meaning evidence.
