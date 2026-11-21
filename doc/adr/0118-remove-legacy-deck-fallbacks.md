# ADR 0118: Remove legacy deck fallbacks and fail loudly

Status: **Accepted** · Date: 2026-11-21 · Author: Justin + OpenCode

Amends [ADR 0071](0071-decouple-deck-data-from-presentation.md)'s
automatic-regeneration and immutable-corpus recovery clauses. Decided while
implementing [issue #2516](https://github.com/justin-hayes/mouseion/issues/2516),
whose "Legacy and impossible paths" section records the production check; the
narrower implementation is
[issue #2522](https://github.com/justin-hayes/mouseion/issues/2522).

## Context

Three compatibility paths existed for data the only deployment does not have.
A read-only production check on 2026-11-21 found:

- 0 live ready decks with render inputs below version 6;
- 0 current readings without a reading snapshot;
- one passage pin in use, and it is listed in the translation register.

The paths still had to be reasoned about on every deck change, and two of them
could hide bad state:

- corpus recovery rebuilt missing render inputs for old manifests;
- an empty-snapshot finish silently treated a corrupt current reading as a
  legacy reading;
- an unlisted passage pin, or an unparseable register, silently credited
  Brownson (contrary to ADR 0111's no-misattribution posture).

## Decision

- A re-render whose run froze render inputs below `RenderInputVersion` 6 is
  refused with `prepared deck requires re-preparation`. It reads no corpus and
  publishes no artifact. ADR 0076's explicit re-preparation path handles it.
- A current reading always names a snapshot. Finishing one without a snapshot is
  stale, not a legacy empty-snapshot completion.
- Passage attribution is looked up only in the release's translation register.
  A pin the register does not list is an error. A register that fails to parse
  is an error. There is no fallback credit.

## Consequences

- Older deployments that still hold these rows would need re-preparation or
  repair. The only deployment was checked and holds none.
- Corrupt current readings and unknown passage pins fail visibly instead of
  producing a completion or a card with the wrong credit.
- The render-input recovery API and its tests are deleted. Card export now
  propagates attribution errors through the deck and TSV/APKG build.
- Learners see no change, because no current data reaches these paths.

## Alternatives considered

- **Keep recovery and the Brownson fallback.** Rejected: both were defensive
  paths for data that does not exist, and the fallback could ship a silent
  misattribution.
- **Treat an unlisted pin as "no passage".** Rejected: it would silently drop
  translation context the card was frozen with, hiding a broken register.
