# ADR 0033: Deterministic in-memory fixture server driven by Playwright for browser acceptance

Amended by [ADR 0088](0088-fixture-adapter-transition-store-contracts.md): fixture
adapter transitions require shared store contracts run against fixtures and
PostgreSQL; other canned browser states are explicitly illustrative.

## Context

Mouseion ships a server-rendered Go web application (Templ views, HTMX
enhancement, Pico CSS) on top of PostgreSQL, a Python/Stanza NLP gRPC service,
and external OPDS catalogs. The Go unit and integration suites exercise handlers
and rendering, but "Design Phase 6 — Quality gates" (issue #378) needs durable
browser-level evidence for the authenticated learner journey, responsive
layout, focus behavior, and realistic edge content, independent of live
external catalogs or NAT/downloads of the NLP service.

The web layer is deliberately infrastructure-independent: `webapp.New(Services{...})`
takes a small set of Go interfaces (`Store`, `OPDS`, `Analysis`,
`AnalysisInsights`, `KnownVocab`, `Enrichment`, `PreparedDeck`,
`Capabilities`, plus `auth.Store`/`webauth`). This is the seam that makes a
deterministic browser harness possible without running Postgres, River, the NLP
service, or a live catalog.

## Decision

Add a **fixture-driven browser acceptance harness** made of two parts:

1. A **Go "fixture server"** (`cmd/fixtureserver`) that wires `webapp.NewWithError`
   with deterministic, in-memory implementations of every `webapp.Services`
   dependency, plus an in-memory `auth.Store`. It binds no external service:
   no Postgres, no River, no gRPC NLP client, no network. All fixture data is
   seeded in-process and served over plain HTTP on a configurable address
   (`MOUSEION_FIXTURE_ADDR`, default a loopback port).
2. **Playwright** (`@playwright/test`, Node) as the browser runner. Playwright
   natively provides everything the acceptance criteria require: desktop and
   compact viewport projects, light/dark colour-scheme runs via `colorScheme`,
   trace/video/screenshot capture on failure, the HTML report, retries, and a
   bounded, CI-runnable smoke command.

### Why Playwright specifically

- Aligns with the existing worker runtime and CI runner (Node ≥ 20 present);
  no new language is introduced.
- First-class support for the required configuration axes: two viewports
  (desktop + compact) and two colour schemes (light + dark) are declared as
  matrixed projects, not hand-rolled.
- Built-in failure evidence (trace + screenshot) with the `--trace on-first-retry`
  mode, satisfying "retain useful failure artifacts" without hand-writing
  capture plumbing.
- Real-browser behaviour for HTMX: Playwright can assert the initial
  server-rendered HTML before the HTMX enhancement, and then observe the
  enhanced DOM after an HX-Request.

### Determinism and test-data policy

- The fixture server gives every workflow state a stable, cycle-independent
  identity: no random IDs, no wall-clock timing, no live network. States that
  in production are reached via River jobs (analysis status, deck preparation,
  enrichment, known-vocab import, campaign progress) are represented directly
  by fixtures already in a given state, so tests are repeatable without a job
  queue.
- Fixture content includes the required breadth: short, long, empty, and failed
  states, plus multilingual (German and Italian, the two first-class NLP
  languages).
- The harness asserts initial server-rendered HTML (`HX-Request != "true"`) for
  at least one representative async workflow, then exercises the HTMX-enhanced
  path. Ordinary server-rendered navigation and forms remain the primary target;
  HTMX is covered where it is an enhancement.
- No secrets, learner data, generated `.apkg` binaries, or brittle production
  snapshots are committed. The fixture seed lives in committed Go source; any
  test artifact (Playwright report, traces) is ignored by `.gitignore`.

### Dependency and update policy

- The Node toolchain is pinned via `package-lock.json` (committed).
- `@playwright/test` is added as an explicit dev dependency; its version is
  pinned and updated through ordinary dependency PRs like any other manifest.
- The fixture server uses only the module's existing dependencies; it adds no
  external Go module.

## Alternatives considered

- **Chrome driven from Go (chromedp/rod).** Keeps a single language but does
  not provide viewport/theme matrices, trace/report tooling, retries, or
  `on-first-retry` failure evidence without substantial hand-rolled plumbing.
  Adds a second competing Go browser library.
- **Puppeteer (Node).** Similar to Playwright but less test-oriented; lacks the
  project-matrix and trace/report facilities out of the box.
- **Testcontainers + real Postgres, seeded via the production server.** Closer
  to production but pulls Postgres, River, and NLP wiring into browser tests,
  weakening the "no external dependencies" requirement and slowing each run.
  The existing Postgres-backed integration suite already covers production
  wiring; the browser harness exists to add deterministic UI evidence, not to
  re-run the integration stack.
- **Server-rendered-only assertions via `httptest`.** Already covered by the Go
  unit tests; provides no browser-level viewport, theme, focus, or HTMX
  evidence.

## Consequences

- A clean checkout can run a single documented command (e.g. `make browser-smoke`)
  that starts the fixture server and runs the Playwright smoke set, with no
  Postgres, NLP, catalog, or network.
- CI gains a bounded browser-smoke job that uploads trace/screenshot evidence
  only on failure.
- The fixture server is a testing seam only; it does not ship or run in
  production and is not a user-facing preview mode.
- Subsequent Phase 6 issues (#379 keyboard/focus/async-state, #380
  responsive/theme/realistic-content) build on this harness rather than
  re-inventing browser test infrastructure.
- `product.md` gains a note about the harness; the ADR index gains this entry.

## Related

- Issue #378 (this harness); design docs under `doc/design/`.
- `doc/design/roadmap.md` Phase 6, plus the annotated workflow documents.
- `docker/hermes-worker/Dockerfile` (Node present in the worker runtime).
