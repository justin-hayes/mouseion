UV ?= uv
VENV := .venv
VENV_BIN := $(VENV)/bin
PROTO_FILE := proto/mouseion/v1/normalized_corpus.proto
# protoc-gen-go and protoc-gen-go-grpc are `tool` directives in go.mod, so their
# versions and checksums come from go.sum. `go tool -n` builds them (cached) and
# prints the binary path that protoc needs for --plugin.
PROTOC_GEN_GO = $(shell go tool -n protoc-gen-go)
PROTOC_GEN_GO_GRPC = $(shell go tool -n protoc-gen-go-grpc)
# sqlc is installed separately (the CI setup action provides the release
# binary); callers may override this with a locally installed executable.
SQLC ?= sqlc
SQLC_VERSION := v1.31.1
MOUSEION_TEST_PG_PORT ?= 55432
MOUSEION_TEST_PACKAGES ?= ./internal/...
# RACE=1 adds the race detector to the Go test targets (`test`, `test-go`,
# `test-go-integration`, `test-integration`, and `test-integration-shared`). CI
# sets it for the unit suite, and the integration-test ticket (#1883) reuses the
# same switch.
RACE ?= 0
RACE_FLAG := $(if $(filter 1 true yes,$(RACE)),-race,)
DICTIONARY_HOST_DIR ?= dictionary
DICTIONARY_OUTPUT ?= $(DICTIONARY_HOST_DIR)/dictionary-index.sqlite
DICTIONARY_DUMP_DATE ?=
DICTIONARY_EXTRACTION_DATE ?= $(shell date -u +%Y-%m-%d)
DICTIONARY_WIKTEXTRACT_COMMIT ?= unknown
DICTIONARY_VERSION ?= dump=$(DICTIONARY_DUMP_DATE);extraction=$(DICTIONARY_EXTRACTION_DATE);wiktextract=$(DICTIONARY_WIKTEXTRACT_COMMIT)
DICTIONARY_REFRESH ?= false
DICTIONARY_SOURCE_ARGS := $(if $(strip $(KAIKKI_INPUT)),--input "$(KAIKKI_INPUT)",--download $(if $(filter 1 true yes,$(DICTIONARY_REFRESH)),--force-download,))
export GOTMPDIR := $(CURDIR)/.tmp/go

.PHONY: setup build test test-go test-go-integration go-test-clean test-integration test-integration-shared coverage coverage-new lint lint-go gen templ templ-if-installed dev clean go-tmp browser-smoke browser-smoke-webkit screenshot-my-books screenshot-my-books-clean sqlc dictionary-index frontend-css check-frontend-css check-frontend-css-sources python-test-preflight

GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.14.0

# Every target that compiles Go refreshes the committed templ output first when
# the pinned templ is installed, and otherwise builds the committed files as-is.
go-tmp: templ-if-installed
	mkdir -p $(GOTMPDIR)

# uv is pinned because its lock output is part of the reproducible install. CI and
# the NLP image pin the same version (ADR 0089).
UV_VERSION := 0.13.0

# Installs the Python environment from nlp/uv.lock: binary-only, locked, and with
# the NLP project itself never built (nlp/pyproject.toml sets package = false).
setup:
	@test "$$($(UV) --version 2>/dev/null | cut -d' ' -f2)" = "$(UV_VERSION)" || { \
		printf 'uv $(UV_VERSION) is required; install it (for example, uv self update 0.13.0 or pip install uv==0.13.0).\n' >&2; \
		exit 1; \
	}
	UV_PROJECT_ENVIRONMENT=$(CURDIR)/$(VENV) $(UV) sync --project nlp --python 3.13 --locked --no-build --group dev

build: go-tmp
	go build ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/python -m compileall -q nlp/src gen/python

# Fails before any Go test runs when the Python test environment is missing, so
# a fresh checkout does not spend a full Go run discovering it. Never installs.
python-test-preflight:
	@$(VENV_BIN)/python -c 'import pytest' >/dev/null 2>&1 || { \
		printf '%s\n' "Python test environment is missing pytest ($(VENV_BIN)/python)." \
			"Run 'make setup' to create .venv from nlp/uv.lock, then retry." >&2; \
		exit 1; \
	}

test: python-test-preflight go-tmp
	go test $(RACE_FLAG) ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/pytest -q nlp/tests

# Targeted Go tests get a per-invocation build temp directory under .tmp, so
# interrupted runs cannot leave build files in a RAM-backed system temp dir.
test-go: templ-if-installed
	tools/go-test.sh unit $(RACE_FLAG) $(if $(strip $(PACKAGES)),$(PACKAGES),./...)

test-go-integration: templ-if-installed
	tools/go-test.sh integration $(RACE_FLAG) $(if $(strip $(PACKAGES)),$(PACKAGES),./internal/...)

go-test-clean:
	tools/go-test.sh clean

test-integration: go-tmp
	go test -count=1 -tags=integration $(RACE_FLAG) ./internal/...

test-integration-shared: go-tmp
	@set -eu; \
	if test -n "$${MOUSEION_TEST_DATABASE_URL:-}"; then \
		go test -count=1 -tags=integration $(RACE_FLAG) $(MOUSEION_TEST_PACKAGES); \
		exit; \
	fi; \
	container="mouseion-test-postgres-$$$$"; \
	port="$${MOUSEION_TEST_PG_PORT:-55432}"; \
	database_url="postgres://postgres:postgres@127.0.0.1:$${port}/mouseion_test?sslmode=disable"; \
	cleanup() { docker rm -f "$$container" >/dev/null 2>&1 || true; }; \
	trap cleanup EXIT INT TERM; \
	docker run --detach --rm --name "$$container" \
		--env POSTGRES_PASSWORD=postgres \
		--env POSTGRES_DB=mouseion_test \
		--publish "127.0.0.1:$${port}:5432" \
		postgres:16-alpine >/dev/null; \
	ready=0; \
	for attempt in $$(seq 1 60); do \
		if docker exec "$$container" pg_isready -U postgres -d mouseion_test >/dev/null 2>&1; then ready=1; break; fi; \
		sleep 1; \
	done; \
	if test "$$ready" -ne 1; then docker logs "$$container"; exit 1; fi; \
	MOUSEION_TEST_DATABASE_URL="$$database_url" go test -count=1 -tags=integration $(RACE_FLAG) $(MOUSEION_TEST_PACKAGES)

# Python and Go coverage reports in .tmp/coverage (gitignored). The Python
# report runs first so it is written before the Go integration run, which needs
# a database. Fails before any test runs when the Python environment is missing.
COVERAGE_DIR := .tmp/coverage

coverage: python-test-preflight go-tmp
	mkdir -p $(COVERAGE_DIR)
	PYTHONPATH=nlp/src:gen/python COVERAGE_FILE=$(COVERAGE_DIR)/python.coverage $(VENV_BIN)/pytest -q nlp/tests --cov --cov-config=nlp/pyproject.toml --cov-report=xml:$(COVERAGE_DIR)/python.xml
	tools/go-coverage.sh

# Previews SonarQube Cloud's 80% new-code coverage gate for the lines changed
# since the merge base with origin/main (COVERAGE_BASE overrides) and lists the
# uncovered ones. Run it before opening a pull request; Sonar's gate decides.
coverage-new: coverage
	tools/coverage-new.sh

lint-go: go-tmp
	@version="$$( "$(GOLANGCI_LINT)" version 2>&1 )" || { \
		printf '%s\n' "golangci-lint $(GOLANGCI_LINT_VERSION) is required; unable to execute $(GOLANGCI_LINT)." >&2; \
		printf 'detail: %s\n' "$$version" >&2; \
		exit 1; \
	}; \
	case "$$version" in \
		*"has version $(GOLANGCI_LINT_VERSION) "*) ;; \
		*) \
			printf '%s\n' "golangci-lint $(GOLANGCI_LINT_VERSION) is required; found:" >&2; \
			printf '%s\n' "$$version" >&2; \
			exit 1; \
		;; \
	esac
	"$(GOLANGCI_LINT)" run ./...

lint: lint-go
	$(VENV_BIN)/ruff check nlp/src nlp/tests
	$(VENV_BIN)/ruff format --check nlp/src nlp/tests

# templ is pinned in go.mod (tool directive) and must match TEMPL_VERSION: newer
# releases require Go >= 1.25.
TEMPL ?= go tool templ
TEMPL_VERSION := v0.3.1070

templ:
	mkdir -p $(GOTMPDIR)
	@test "$$($(TEMPL) version 2>/dev/null)" = "$(TEMPL_VERSION)" || { \
		printf 'templ $(TEMPL_VERSION) is required; check the templ tool directive in go.mod.\n' >&2; \
		exit 1; \
	}
	$(TEMPL) generate

# Fresh checkouts without a working Go module cache fall back to the committed
# views; CI's generated-code check runs the strict `make templ` and fails on
# stale output.
templ-if-installed:
	@if test "$$($(TEMPL) version 2>/dev/null)" = "$(TEMPL_VERSION)"; then \
		$(TEMPL) generate; \
	else \
		printf 'templ $(TEMPL_VERSION) not found; using the committed internal/webapp/*_templ.go (run make templ to regenerate).\n' >&2; \
	fi

# Build committed frontend stylesheets with checksummed standalone tools; Go
# serves the generated assets and never invokes the compiler at runtime.
frontend-css:
	tools/build-frontend-css.sh

check-frontend-css-sources:
	tools/verify-frontend-css-sources.py

check-frontend-css:
	$(MAKE) check-frontend-css-sources
	tools/build-frontend-css.sh
	git diff --exit-code -- internal/webapp/static/app.css

# Regenerate the committed sqlc query layer (gen/sqlc) from sqlc/queries and
# the current-state baseline/successor migrations. sqlc is pinned; CI installs
# the same version and asserts `git diff --exit-code` after regeneration.
sqlc: go-tmp
	test "$$($(SQLC) version 2>/dev/null)" = "$(SQLC_VERSION)"
	$(SQLC) generate

dictionary-index:
	test -n "$(DICTIONARY_DUMP_DATE)" || (printf '%s\n' 'DICTIONARY_DUMP_DATE is required (for example, 2026-09-14)' >&2; exit 1)
	test -z "$(strip $(KAIKKI_INPUT))" || test -z "$(filter 1 true yes,$(DICTIONARY_REFRESH))" || (printf '%s\n' 'DICTIONARY_REFRESH cannot be used with KAIKKI_INPUT' >&2; exit 1)
	test -x $(VENV_BIN)/python
	mkdir -p "$(dir $(DICTIONARY_OUTPUT))"
	test -w "$(dir $(DICTIONARY_OUTPUT))" && test -x "$(dir $(DICTIONARY_OUTPUT))" || (printf '%s\n' "dictionary output directory '$(dir $(DICTIONARY_OUTPUT))' is not writable." "If Docker created it as root, run: sudo chown \"$$(id -u):$$(id -g)\" '$(dir $(DICTIONARY_OUTPUT))'" >&2; exit 1)
	$(VENV_BIN)/python nlp/scripts/derive_dictionary_index.py $(DICTIONARY_SOURCE_ARGS) --output "$(DICTIONARY_OUTPUT)" --provider-version "$(DICTIONARY_VERSION)" --dump-date "$(DICTIONARY_DUMP_DATE)" --extraction-date "$(DICTIONARY_EXTRACTION_DATE)" --wiktextract-commit "$(DICTIONARY_WIKTEXTRACT_COMMIT)"

# go-tmp runs first: make expands every recipe line before running any of them,
# and the go tool plugin paths below need $(GOTMPDIR) to exist on a fresh checkout.
gen: go-tmp
	mkdir -p gen/go gen/python $(GOTMPDIR)
	test -x $(VENV_BIN)/python || $(MAKE) setup
	$(VENV_BIN)/python -c 'import importlib.metadata; assert importlib.metadata.version("grpcio-tools") == "1.71.2"' || { printf '%s\n' "grpcio-tools 1.71.2 is missing from $(VENV); run 'make setup'." >&2; exit 1; }
	protoc -I proto --plugin=protoc-gen-go=$(PROTOC_GEN_GO) --go_out=gen/go --go_opt=paths=source_relative --plugin=protoc-gen-go-grpc=$(PROTOC_GEN_GO_GRPC) --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative --python_out=gen/python $(PROTO_FILE)
	$(VENV_BIN)/python -m grpc_tools.protoc -I proto --grpc_python_out=gen/python $(PROTO_FILE)
	touch gen/python/mouseion/__init__.py gen/python/mouseion/v1/__init__.py

browser-smoke:
	cd e2e && npm ci --ignore-scripts && npx playwright install chromium webkit && npx playwright test

# Documentation screenshots of My Books and Reading on an isolated stack (not
# part of browser-smoke or CI). See e2e/screenshot/run.mjs. `screenshot-my-books-clean`
# removes the screenshot project's containers and volumes.
# Checked before any install or download so a missing prerequisite fails fast.
SCREENSHOT_PREFLIGHT = command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 && node -e 'process.exit(Number(process.versions.node.split(".")[0]) >= 20 ? 0 : 1)' || { printf '%s\n' 'Node.js 20 or newer with npm is required for the screenshot; install it and rerun.' >&2; exit 1; }; \
	docker info >/dev/null 2>&1 || { printf '%s\n' 'Docker is unavailable. Start the Docker daemon and make sure this user can run docker, then rerun.' >&2; exit 1; }; \
	docker compose version >/dev/null 2>&1 || { printf '%s\n' 'Docker Compose v2 is unavailable. Install the Docker Compose plugin (v2.24.4 or newer) and rerun.' >&2; exit 1; }

screenshot-my-books:
	$(SCREENSHOT_PREFLIGHT)
	cd e2e && npm ci --ignore-scripts && npx playwright install chromium && node screenshot/run.mjs

screenshot-my-books-clean:
	$(SCREENSHOT_PREFLIGHT)
	cd e2e && npm ci --ignore-scripts && node screenshot/run.mjs --clean

browser-smoke-webkit:
	cd e2e && npm ci --ignore-scripts && npx playwright install webkit && npx playwright test --project='webkit-*'

dev: go-tmp
	go run ./cmd/server

clean:
	rm -rf bin .pytest_cache .ruff_cache
