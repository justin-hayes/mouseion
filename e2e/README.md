# Browser acceptance smoke

The harness uses a Go in-memory fixture server and Playwright, as decided in
ADR 0033. It binds no PostgreSQL, River, NLP, OPDS, or other network service.
The fixture credentials are `fixture-learner` / `fixture-password` and are
testing data only.

From the repository root, run `make browser-smoke`. The command installs the
pinned Node dependency from `package-lock.json` when needed and starts the
fixture server through Playwright. Playwright runs the small smoke suite on
desktop and compact viewports in light and dark colour schemes; traces,
screenshots, video, and the HTML report are retained for failures only.

`@playwright/test` is updated through ordinary dependency changes with the
lockfile committed. This suite complements, and does not replace, Go tests.

## Responsive/theme snapshots and failure artifacts

The responsive/theme suite primarily uses geometry, ARIA, and computed-style
assertions. Add a visual snapshot only for a stable, high-value region whose
structure is not adequately covered by those checks; keep dynamic identifiers,
dates, and progress values masked. Snapshot changes require explicit human
review for hierarchy and semantic regressions. Never update snapshots
automatically in CI.

From `e2e/`, update reviewed baselines explicitly with
`npx playwright test --update-snapshots` (or the equivalent project wrapper).
Committed baselines belong under `e2e/tests/**/__snapshots__/`. Failed runs
retain reviewable output in `playwright-report/` and `test-results/`; traces,
screenshots, and videos are retained there according to the Playwright config.
