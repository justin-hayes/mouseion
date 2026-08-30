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
