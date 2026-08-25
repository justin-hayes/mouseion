# Phase 5 EPUB recommendation corrections implementation plan

> **For Hermes:** Use the comprehensive issue template from prior phases. Implement in dependency order and preserve immutable historical classifications, reviewed scopes, and corpus records.

**Goal:** Correct unsafe EPUB scope recommendations exposed by the German book review while retaining deterministic, explainable classification.

**Architecture:** Separate structural classification from inclusion policy. Give explicit EPUB/title/path signals precedence over generic text shape, model degraded recommendation state explicitly, and validate the complete book shape with fixtures and end-to-end tests.

## Dependency order

1. #283 — recommendation policy and safe fallback;
2. #284 — structural precedence and Roman-numeral heading recognition;
3. #285 — German/reference marker coverage;
4. #286 — recommendation explanations and scope-summary UX;
5. #287 — complete regression fixtures, migration/version review, and documentation.

## Verification gate

Run `templ generate`, `go build ./...`, `go vet ./...`, `go test ./...`, `make lint`, generated-code checks, focused EPUB classifier/scope tests, and CI Go/Python/Protobuf checks. PostgreSQL integration is authoritative in CI when unavailable locally.

## Risks

- Changing policy must not rewrite immutable prior scopes.
- Existing users may have saved scopes under older classifier versions; retain them and disclose version differences.
- Prefaces and appendices are context-dependent; use reviewable recommendations rather than universal exclusion.
- Threshold changes require full-book regression fixtures to prevent another global fallback failure.
