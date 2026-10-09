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

The Playwright projects own the desktop/compact × light/dark matrix. An
ordinary test reads its project's viewport (`page.viewportSize()`) and colour
scheme instead of looping over them, so each combination runs once per project.
A test keeps its own width only when that width is the contract: a named
breakpoint or special width (for example 320px, 768px, or 900px), a 200% text
check, or a transition between widths within one test. Tests that open their own
WebKit browser context read the project's viewport and scheme from
`test.info().project.use`. Tests that switch the OS colour scheme or the app's
`data-theme` attribute within one page keep those in-test switches, because the
projects do not drive those mechanisms.

Playwright runs four workers locally and one per CPU core in CI. Each worker starts and owns an independent
in-memory fixture server on a loopback port for its current spec file; it stops
that server and starts a fresh one at the next file boundary. Tests in a file
remain ordered and may share state; different files and projects start from a
fresh fixture baseline and cannot depend on execution order. The test fixture
provides each test's worker URL as Playwright `baseURL`, including
manually-created browser contexts and pages. The runner never attaches to an
existing server.

For manual visual or screenshot review, the recommended path is to let
Playwright manage the per-worker fixture servers, and capture all desired states
in one invocation and browser session. If an external fixture server is needed,
start it yourself and run one worker, for example:

```sh
cd e2e
MOUSEION_FIXTURE_ADDR=127.0.0.1:8099 go run ../cmd/fixtureserver
# In another terminal, from e2e:
MOUSEION_REUSE_FIXTURE=1 MOUSEION_FIXTURE_URL=http://127.0.0.1:8099 \
  npx playwright test tests/typography.spec.ts --workers=1 --project=desktop-light
```

External-fixture mode rejects parallel workers and multiple spec files because
that server has one mutable store. Use one spec file per invocation and restart
the external server between invocations. If source or embedded assets change,
rebuild and restart the external server before capturing the updated UI.

WebKit requires its browser binary and Linux runtime libraries. Locally,
`make browser-smoke` downloads the browsers with `npx playwright install chromium
webkit`; the system libraries for WebKit on a supported Linux host come from
`cd e2e && npx playwright install-deps webkit`, which needs administrator
privileges.

CI does not install browsers or system packages per shard. A `browser-fixture`
job builds the commit-matched fixture server once per run, and each of the four
`--shard=n/4` jobs downloads it and installs the locked npm dependencies on the
runner. The shard then runs `npx playwright test` inside the official Playwright
image pinned by digest in `.github/workflows/ci.yml` (`PLAYWRIGHT_IMAGE`). That image
provides Chromium, WebKit, and their Linux runtime libraries. The run passes
`--network=host` for the loopback fixture server, `--ipc=host` for Chromium
shared memory, `--init` for process cleanup, and the runner's own UID so
reports, traces, and screenshots stay writable for the failure-artifact upload.
To reproduce a shard locally, run `make templ`, build `.tmp/fixtureserver` with
`go build -o .tmp/fixtureserver ./cmd/fixtureserver`, and run `npm ci
--ignore-scripts` in `e2e/`. Then run the same command from the repository root,
with `PLAYWRIGHT_IMAGE` set to the workflow's value:

```sh
docker run --rm --init --ipc=host --network=host --user "$(id -u):$(id -g)" \
  -e CI=true -e HOME=/tmp -e MOUSEION_FIXTURE_BIN=/work/.tmp/fixtureserver \
  -v "$PWD:/work" -w /work/e2e "$PLAYWRIGHT_IMAGE" \
  npx playwright test --shard=1/4
```

A missing browser or library makes the job fail, because Playwright stops at
launch rather than skipping the project. The `Browser smoke` aggregate check
still requires every shard to pass.

Dependency and update policy: the image tag must equal the locked
`@playwright/test` version. The `Verify Playwright image matches locked version`
step fails CI if they differ. Bump `e2e/package.json`, `e2e/package-lock.json`,
and the image tag and digest in the same change, and confirm the new image
contains the browser builds that version expects. Image provisioning and pull
time are part of the CI cost; compare end-to-end shard timings against the
previous run before claiming a speedup.

The WebKit journey covers server-rendered sign-in and invalid-credential
recovery, the authenticated shell and native language/navigation forms,
keyboard-operable confirmation disclosure, compact/desktop overflow and target
geometry, focus, and a JavaScript-disabled sign-in/navigation/form/disclosure
path. The journey restores the active study language before finishing. Each
spec file has a fresh fixture store, while the ordered scenario sequence within
the file stays deterministic.

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
across desktop/compact and light/dark projects, as do the stateful
finish/history/reread flows, which each receive a fresh file-scoped fixture.
Migration tests separately assert retained active snapshot contents,
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

The Working desk's degraded Browse states (counts updating, counts unavailable,
no current analysis, no eligible vocabulary, and everything already accounted
for with annotated Known · Reserved and In a Book deck rows) are reached through
`POST /fixture/vocabulary-browse-scenario` with a `scenario` form value, which
exists only in the fixture server. The empty value restores the default fixture.
`working-desk-states.spec.ts` sets a scenario per test and resets it afterwards.

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

## Documentation screenshots of My Books, Reading, Concordance, and Study

`make screenshot-my-books` builds the documentation screenshots
`doc/images/my-books.png`, `doc/images/my-books-dark.png`,
`doc/images/reading.png`, `doc/images/concordance.png`, and
`doc/images/sentence-study.png` on an isolated Compose stack. It is separate from
`make browser-smoke` and CI. Nothing is written unless every assertion passes.
The procedure, durations, and clean-up are in the
[development guide](../doc/development.md#regenerating-screenshots).

- The learner workflow moves every scenario Book except the one marked `inbox`
  to To Read, waits for each analysis through Reading's chooser, starts and
  finishes the `read` Book, starts the `current` Book, and asserts its title and
  a non-empty Book vocabulary table before capturing Reading. My Books is then
  captured twice, in the light and dark color schemes, and each capture asserts
  the Inbox, To Read, Currently reading, and Read buckets first.
- The Concordance lookup types the lemma configured in the `concordance` block of
  `screenshot/manifest.json` into the Concordance form. The capture fails unless
  the analyzed corpus yields at least `minimumOccurrences` lines from at least
  `minimumBooks` Books. The sentence Study opens the first result, within the
  first page, whose identified token has the configured part of speech, and
  checks its lemma and dependency evidence.
- Before the `current` Book starts, the workflow derives a Known-vocabulary
  baseline from the analyzed Books other than it: their lemmas ranked by
  frequency, read-only from the stack's database, cut at the rank where the
  `current` Book's Known coverage reaches about 96%. If no rank reaches 96%, all
  of them are used. The list is imported through Vocabulary, and the run fails
  unless the measured coverage is within 94–98%. The cut-off and measured
  coverage are printed.
- Each wait is bounded and names the Book or step that stalled. Analysis of the
  full set is bounded at 30 minutes; the Playwright test budget is 75 minutes.
- Source books are pinned Project Gutenberg editions in
  `screenshot/manifest.json`, cached under the ignored `.tmp/screenshot/`.
  A checksum mismatch stops the run; review the manifest before changing a pin.
- The stack uses its own Compose project (`mouseion-screenshot`), binds the web
  port to `127.0.0.1`, generates secrets per run, provisions German only, and
  never reads the maintainer's `.env`.
- The Stanza and Hugging Face model volumes are reused across runs. Run
  `make screenshot-my-books-clean` to remove the project's containers and
  volumes, including those model caches.
- Requires Docker with Compose v2.24.4 or newer, Node.js 20 or newer, and
  network access to www.gutenberg.org on the first run.
