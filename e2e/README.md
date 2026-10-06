# Browser acceptance smoke

The harness uses a Go in-memory fixture server and Playwright, as decided in
ADR 0033. It binds no PostgreSQL, River, NLP, OPDS, or other network service.
The fixture credentials are `fixture-learner` / `fixture-password` and are
testing data only.

From the repository root, run `make browser-smoke`. The command installs the
pinned Node dependency from `package-lock.json` when needed, installs Chromium
and WebKit, and starts the fixture server through Playwright. Chromium continues
to run the complete existing suite on desktop and compact viewports in light
and dark colour schemes. WebKit runs only `tests/webkit-native.spec.ts` in the
same four viewport/appearance combinations, so the complete workflow suite is
not multiplied by a second engine. Run only that focused matrix with
`make browser-smoke-webkit` (or `cd e2e && npm run smoke:webkit`).

WebKit requires its browser binary and Linux runtime libraries. On a supported
Linux runner image, provision the system libraries as an image/setup step with
`cd e2e && npx playwright install-deps webkit` (this system-package command
requires administrator privileges); then `npx playwright install webkit` can
download the browser as the unprivileged test user. The repository CI runner is
intentionally not granted `sudo`: its host image must have the WebKit
dependencies installed in advance. CI installs the pinned browser binaries and
executes the same bounded smoke command; missing libraries fail the job rather
than silently skipping WebKit.

The WebKit journey covers server-rendered sign-in and invalid-credential
recovery, the authenticated shell and native language/navigation forms,
keyboard-operable confirmation disclosure, compact/desktop overflow and target
geometry, focus, and a JavaScript-disabled sign-in/navigation/form/disclosure
path. Fixture state is shared, so browser projects and workers remain serialized
and the journey restores the active study language before finishing.

The focused Concordance journey also runs in all four WebKit projects. It checks
the applied lookup and Book scope, occurrence-list semantics in Playwright's
browser accessibility snapshot, native context disclosures, sentence-study
navigation/return, and the JavaScript-disabled server-rendered path. An
accessibility snapshot is browser automation evidence, not a real VoiceOver
validation; assistive-technology verification must be reported separately.

Screenshots, video, traces, and the HTML report are retained for failures only.

`@playwright/test` is updated through ordinary dependency changes with the
lockfile committed. This suite complements, and does not replace, Go tests.

Reading acceptance includes My Books catalog arrivals and disposition,
current-reading confirmation and stale-write fields, coverage-band selection,
focus restoration, reduced-motion preference, completion receipt, Read history,
and Read again. Read-only assertions run
across desktop/compact and light/dark projects; the stateful finish/history/
reread loop runs in `desktop-light` only because all projects share one fixture
server. Migration tests separately assert retained active snapshot contents,
completion provenance, and prepared-deck provenance.

## Browser rendering and contrast checks

`tests/migration-coverage.spec.ts` verifies the one embedded application
stylesheet, absence of retired Pico/split stylesheets, and removal of Pico CSS
tokens. It intentionally does not ban generic utility class names: those are
implementation details, while the responsive/theme suite checks representative
pages for their semantic content, usable controls, and layout behavior.

Rendered text contrast assertions use `support/contrast.ts`. It measures the
browser-computed foreground and composites translucent foreground/background
colors through transparent element ancestors before calculating WCAG sRGB
contrast; a fully transparent document canvas uses the browser's white default.
CSS colors accepted by the browser (including RGB and modern color syntax such
as OKLCH) are converted through a 1×1 canvas to sRGB. A non-solid background
image or unresolvable color fails with a diagnostic instead of producing a
ratio. This deliberately does not attempt to sample images/gradients, account
for ancestor `opacity`, blend modes, pseudo-elements, or prove text is actually
visible; those require different rendering evidence. Automated DOM and contrast
checks do not establish real VoiceOver behavior.

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
