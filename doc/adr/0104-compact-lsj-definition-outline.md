# ADR 0104: Compact LSJ definition outline on Ancient Greek cards

Status: **Accepted** · Date: 2026-10-18 · Author: Justin + opencode

Implemented by [#2206](https://github.com/justin-hayes/mouseion/pull/2206).
Amends the "Dictionary entry display" bullet of
[ADR 0101](0101-ancient-greek-cards-from-source-evidence.md) for Ancient Greek
(`grc`) only.

## Context

ADR 0101 shows the matched LSJ entry with its full sense hierarchy, never
reranked or truncated, and accepts that entries may be long. The local Anki
check of the real Book 1 deck (#2110) showed how long: γίγνομαι filled about
two screens. Most of that length is not definition. In the Perseus LSJ, `<tr>`
marks any English rendering, so a sense's glosses include translations of
quoted passages ("God forbid", "suppose it done") and fragments of them
("turning" from "turning every way"). Etymology cognates and stray punctuation
were also taken as glosses; that derivation bug is fixed separately (#2204).

LSJ's own layout separates the two. A sense states its definition first and
then cites usages, and `:—` ends a forms paragraph and starts the meaning.

## Decision

**The derivation records each sense's definitions**: the English glosses LSJ
gives before the sense's first citation, with the window restarting after
`:—`. All glosses are still kept, in source order.

**The card shows a compact outline first.** It lists, in source order with
LSJ's numbering, each sense's definitions only, with no usage glosses and no
citations. A sense without definitions appears, as its number only, when a
deeper sense in its span has some; otherwise it is left out of the outline.
Perseus LSJ senses are mostly flat siblings that carry a level, so a span is
the run of deeper-level senses after a sense.

**The full entry is unchanged and follows behind a collapsed toggle**,
labelled "Full LSJ entry": the full hierarchy, every gloss and every kept
Xenophon citation. Senses are still never reranked, and the full entry is
never truncated. The outline selects by a position LSJ itself marks, and it
never chooses a sense for the sentence.

An entry derived before definitions existed keeps the full layout, open, with
no outline. A prepared deck keeps its frozen entry and never changes; new
preparations show the outline once the operator rebuilds the dictionary index.

The provider-written LSJ sense summary
([#2172](https://github.com/justin-hayes/mouseion/issues/2172)) and the
evidence-assisted pass ([#2171](https://github.com/justin-hayes/mouseion/issues/2171))
remain future, opt-in work under ADR 0101.

## Consequences

- Across the pinned LSJ, 137,235 of the 140,333 senses with glosses have
  definitions. Outlines measured 11 lines for γίγνομαι, 9 for ἀνήρ, 11 for
  ὅπλον and 61 for λόγος.
- The rule is positional, not semantic. A usage gloss that comes before the
  first citation ("born" after "to be born") can appear in the outline, and a
  definition placed after a citation is shown only in the full entry.
- The toggle relies on the HTML `<details>` element, so the full entry is one
  tap away rather than visible at once.
- This ADR changes no other language and approves no provider payload.

## Related

- [ADR 0101](0101-ancient-greek-cards-from-source-evidence.md): amended as
  described above; its other decisions stand.
