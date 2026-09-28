# ADR 0079: Contextual glosses require LLM-assisted deck preparation

Status: **Accepted (core implemented)** · Date: 2026-09-27 · Scope revised: 2026-09-28 · Author: Justin + OpenCode

The original multi-source expansion was dropped after the contextual-gloss core
shipped. FreeDict and PanLex imports are not required by this decision; see
[#1256](https://github.com/justin-hayes/mouseion/issues/1256) and its closed
source-expansion tickets #1263–#1265. Ready-deck re-preparation remains a
separate follow-up (#1266).

Amends the opt-in and offline-deck clauses of [ADR 0007](0007-enrichment-providers-caching-privacy.md),
the optional sentence-translation and request-field clauses of
[ADR 0021](0021-contextual-translation-cache.md),
the dictionary-authored default of [ADR 0064](0064-dictionary-enrichment-provider.md),
the sense-selection and fallback policy of [ADR 0069](0069-llm-sense-selection-and-fallback-gloss.md),
and the separately consented deck preparation described by
[ADR 0078](0078-book-dispositions-and-current-reading.md). Their other
decisions, including the restricted external-data boundary and the independence
of Reading from deck readiness, remain in force.

## Context

A compact list of dictionary senses is often a poor cue for the one meaning a
word has in its representative sentence. Previously, the optional LLM chose
among frozen Wiktionary senses or wrote a fallback only when none fit; without
consent or a configured provider, a local deterministic gloss still produced a
deck. That trade-off favored offline availability and verbatim source text over
contextual clarity. For recognition cards, contextual meaning is the essential
output, not an optional upgrade.

## Decision

Every newly prepared recognition card requires the configured LLM. The existing
per-item external translation call also produces **one short English gloss for
the target's meaning in its representative sentence**, preferably one to three
words or close comma-separated synonyms when that is faithful; it continues to
translate the whole sentence. A source sense list, a dictionary-only gloss, or
an ungrounded guess is not an alternative card. There is no per-user or
per-submission LLM consent/decline path: preparing a deck necessarily uses the
configured provider, including when preparation follows starting Reading.
The operator still configures the provider. Only the canonical lemma, tested
target form, one representative sentence, and public lexical evidence may be
sent; no book title, full text, learner identity, or reading history leaves the
installation.

Collect relevant English meaning evidence for German, Italian, and Modern Greek
from the existing versioned Wiktionary/Kaikki index. Freeze a bounded candidate
set with stable IDs, provenance, and the index version on the deck specification
before the LLM call. An exact lemma/POS match is preferred; an entry without POS
is labeled as a weaker match. The LLM cites the evidence IDs supporting its
gloss or explicitly identifies it as inferred from the sentence when no sense
fits; validate cited IDs without pretending that this proves semantic
correctness. A missing dictionary entry does not block preparation. Evidence
coverage, context-only inferences, and omissions are reported outside cards;
Wiktionary attribution stays in deck/export metadata, not on each card.

If the provider is missing, unavailable, or returns an unusable response, deck
preparation fails closed and can be retried; it never falls back to a local-only
deck. If a valid per-item response cannot support a defensible gloss, omit that
card and report it rather than guess. A preparation with no usable cards fails,
not publishes an empty deck. Omission does not change the frozen current-reading
vocabulary snapshot or silently mark an identity Known; completion omissions
remain explicit. Reading may start or continue despite deck failure.

Keep prior prepared decks and frozen evidence unchanged. Presentation-only
rerenders use the existing specification without new lexical lookups or LLM
calls. Explicit re-preparation creates a new generation under these rules and
may use newer source data. Preserve source, prompt, and provider identity in
the frozen/cached result so a refresh never silently changes an existing deck.

Do not import FreeDict or PanLex merely to add overlapping meanings. Their
license, attribution, source-ancestry, and ingestion costs are not justified by
demonstrated card-quality gains over Kaikki plus sentence context. The existing
Wiktionary-derived index retains its attribution and redistribution terms;
future source additions require a separate quality case and rights review.

## Why this trade-off

This deliberately gives up offline card creation and per-user privacy opt-in
to make the card's meaning block useful in context, while retaining narrow data
egress, an explicit operator-configured provider, durable retry, versioned
evidence, and independently functioning Reading. The local dictionary remains
valuable as grounded evidence and for form/pronunciation fields; it does not
dictate the final gloss text. Quality must be evaluated on representative
German, Italian, and Greek cards, not inferred from data volume or prompt
validity alone.

## Related

- [Contextual gloss preparation specification](../features/contextual-gloss-preparation.md)
- [ADR 0021: Contextual translation cache and privacy](0021-contextual-translation-cache.md)
- [ADR 0071: Deck specifications and presentation versions](0071-decouple-deck-data-from-presentation.md)
- [ADR 0076: Re-prepare a ready deck](0076-reprepare-ready-deck.md)
