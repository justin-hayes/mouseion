# Concordance workbench review package

**Review date:** 2026-10-03<br>

**Scope:** visual and automated review of the experimental workbench from [#1376](https://github.com/justin-hayes/mouseion/issues/1376), after the scanning journey in [#1379](https://github.com/justin-hayes/mouseion/issues/1379) landed. This is evidence for a human decision, not an adoption decision.

## Visual evidence

Captured from the deterministic fixture server with the applied query `mode=surface&term=Haus`. Desktop is 1280×800; compact is 375×812; the no-JavaScript case is 320×812. Screenshots include the full page so the result rows and expansion are reviewable.

| State | Evidence |
| --- | --- |
| Desktop, light, collapsed Book / KWIC / study columns | [Full-page screenshot](concordance-review/desktop-light-collapsed.png) |
| Desktop, light, flowing expanded sentence | [Full-page screenshot](concordance-review/desktop-light-expanded.png) |
| Desktop, dark | [Full-page screenshot](concordance-review/desktop-dark-collapsed.png) |
| Compact, light, reflowed rows | [Full-page screenshot](concordance-review/compact-light-collapsed.png) |
| Compact, light, expanded sentence | [Full-page screenshot](concordance-review/compact-light-expanded.png) |
| Compact, JavaScript disabled | [Full-page screenshot](concordance-review/compact-no-javascript.png) |
| Desktop, island bundle returns 404 | [Full-page screenshot](concordance-review/desktop-failed-bundle.png) |

Regenerate with `cd e2e && npm ci --ignore-scripts && npm run capture:concordance`. The script starts and stops the in-memory fixture server if one is not already available; override `MOUSEION_FIXTURE_URL` to use another local fixture address. These captures use fixture content, not a learner corpus.

## Comparison and findings

- **Original AntConc-inspired exploration:** [prototype at `b022559`](https://github.com/justin-hayes/mouseion/blob/b022559/internal/webapp/prototype-concordance.html), `?variant=A&entry=browse&detail=inline`. The original is an explicitly throwaway, invented-data comparison with several placement alternatives. The current workbench narrows that exploration to server-rendered real routes, Book-attributed results, an always-visible study action, and a complete sentence in normal prose flow. It avoids the original's prototype switcher and floating/remote detail placements. This is a behavioral/layout comparison, not a claim that the screenshots use identical data or chrome.
- **Sketch Engine references:** [KWIC source column](https://www.sketchengine.eu/wp-content/uploads/conc_2-1536x858.png) and [expanded context](https://www.sketchengine.eu/wp-content/uploads/2020-03-18_14-28-38.png). The persistent Book column and aligned observed form make provenance and lexical scanning easy to compare; opening a row reads as a complete flowing sentence. This borrows the source/context presentation idea without cloning Sketch Engine's wider product chrome.
- **Scan quality — revise:** the row hierarchy is legible and provenance remains present in compact and expanded states. However, in the captured ordinary 1280×800 desktop viewport, the first result begins at about 740px, leaving almost no usable first-row scan before scrolling. This falls short of #1376's first-viewport goal and should be addressed or explicitly accepted by the product reviewer before adoption.
- **Fallback/island duplication:** native server-rendered results and Lit-enhanced results are two presentation paths. The screenshots and browser tests show that native disclosures, links, lookup, and paging remain available without JavaScript and when the bundle fails. That resilience is valuable, but the duplicated rendering contract creates ongoing parity and regression work.
- **Shadow DOM focus and anchors:** the island owns an internal copy of each row while the fallback is removed after enhancement. Return-to-row focus and missing-row summary fallback are covered by browser tests, including paging; nevertheless, fragment navigation and focus crossing the custom-element/Shadow DOM boundary remain behaviors to watch. The browser tests do not prove assistive-technology interoperability.
- **Build and CI upkeep:** Lit is pinned with the browser-test dependencies, and the committed bundle is reproducible with the local esbuild command. This adds a JS dependency, build output, and regeneration check to an otherwise Go/Templ-rendered surface. No workflow or CI files were changed in this review package.
- **Accepted-contract differences:** the separate always-visible study link differs from the accepted disclosure interaction. The experimental Up/Down row-focus behavior also conflicts with the accepted normal-scrolling contract. The tests verify that experiment; they do not authorize changing the contract or adopting Lit.

## Verification results

Run from the repository root on 2026-10-03:

- `make frontend` — passed; regenerated Concordance bundle without a generated-file diff.
- `make templ` — passed; generated output unchanged.
- `go test ./...` — passed.
- `make lint` — passed (Go and Ruff).
- `make browser-smoke` — passed: 172 passed, 24 intentionally skipped in the four-project fixture matrix; the separately gated corrected-reading-start test passed (1/1). Concordance native/no-JavaScript, enhanced, failed-bundle, keyboard trial, study return, and page-boundary tests passed.
- `cd e2e && npm run capture:concordance` — passed; produced the seven screenshots linked above.

These checks establish reproducibility and tested browser behavior, not screen-reader usability.

## Recommendation: revise before adoption

Retain the native fallback and the compact Book/KWIC/study presentation as a promising prototype, but **do not adopt yet**. First resolve the desktop first-viewport scan-height gap, then reconcile the always-visible study link and Up/Down trial with the accepted feature contract. Treat the bundle/fallback duplication and Shadow DOM focus/anchor boundary as explicit maintenance costs rather than free enhancement. This recommendation is provisional until human accessibility review.

## Manual screen-reader verification — pending human gate

No screen-reader session was performed for this package. A human reviewer should test at least one supported screen reader/browser pairing (for example, NVDA with Firefox or VoiceOver with Safari) and record the actual pairing and outcome for each item:

- [ ] Each result row has a useful accessible name that identifies its occurrence/context without an unwieldy full-sentence announcement.
- [ ] The disclosure announces its collapsed/expanded state accurately; Enter/Space opens and closes it, and Escape closes it and returns focus as intended.
- [ ] The Book title/provenance is announced with the corresponding result in both collapsed and expanded states.
- [ ] The always-visible arrow/link is announced with the full “Study this sentence and its syntax” name, not just its glyph or “Study”.
- [ ] Tab and Shift+Tab order moves predictably from a row disclosure to its separate study link and onward; focus remains visible and does not get trapped or lost at the Shadow DOM boundary.
- [ ] Pending lookup, successful apply, stale-revision recovery, and errors are announced at the right time without announcing old results as new.
- [ ] Activating study and returning restores the applied query/page and announces or focuses the originating result; when it is absent, the result summary receives focus.
- [ ] Repeat the key checks with JavaScript disabled or the bundle unavailable to confirm the native fallback remains understandable.

**Human sign-off is still required.** Do not mark these checks complete based on Playwright or automated accessibility-tree assertions alone.
