PYTHON ?= python3
UV ?= uv
VENV := .venv
VENV_BIN := $(VENV)/bin
PROTO_FILE := proto/mouseion/v1/normalized_corpus.proto
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc
SQLC := $(shell go env GOPATH)/bin/sqlc
SQLC_VERSION := v1.31.1
MOUSEION_TEST_PG_PORT ?= 55432
MOUSEION_TEST_PACKAGES ?= ./internal/...
DICTIONARY_HOST_DIR ?= dictionary
DICTIONARY_OUTPUT ?= $(DICTIONARY_HOST_DIR)/dictionary-index.sqlite
DICTIONARY_DUMP_DATE ?=
DICTIONARY_EXTRACTION_DATE ?= $(shell date -u +%Y-%m-%d)
DICTIONARY_WIKTEXTRACT_COMMIT ?= unknown
DICTIONARY_VERSION ?= dump=$(DICTIONARY_DUMP_DATE);extraction=$(DICTIONARY_EXTRACTION_DATE);wiktextract=$(DICTIONARY_WIKTEXTRACT_COMMIT)
DICTIONARY_REFRESH ?= false
DICTIONARY_SOURCE_ARGS := $(if $(strip $(KAIKKI_INPUT)),--input "$(KAIKKI_INPUT)",--download $(if $(filter 1 true yes,$(DICTIONARY_REFRESH)),--force-download,))
export GOTMPDIR := $(CURDIR)/.tmp/go

.PHONY: setup build test test-integration test-integration-shared lint lint-go gen templ dev clean go-tmp browser-smoke sqlc dictionary-index

GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.13.2

go-tmp:
	mkdir -p $(GOTMPDIR)

setup:
	$(UV) venv --clear --python 3.11 $(VENV)
	$(UV) pip install --python $(VENV_BIN)/python -r nlp/requirements-dev.txt
	$(UV) pip install --python $(VENV_BIN)/python torch --index-url https://download.pytorch.org/whl/cpu
	$(UV) pip install --python $(VENV_BIN)/python -e nlp

build: go-tmp
	go build ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/python -m compileall -q nlp/src gen/python

test: go-tmp
	go test ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/pytest -q nlp/tests

test-integration: go-tmp
	go test -count=1 -tags=integration ./internal/...

test-integration-shared: go-tmp
	@set -eu; \
	if test -n "$${MOUSEION_TEST_DATABASE_URL:-}"; then \
		go test -count=1 -tags=integration -p 1 $(MOUSEION_TEST_PACKAGES); \
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
	MOUSEION_TEST_DATABASE_URL="$$database_url" go test -count=1 -tags=integration -p 1 $(MOUSEION_TEST_PACKAGES)

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

templ:
	templ generate

# Regenerate the committed sqlc query layer (gen/sqlc) from sqlc/queries and
# the current-state baseline/successor migrations. sqlc is pinned; CI installs
# the same version and asserts `git diff --exit-code` after regeneration.
sqlc: go-tmp
	test "$$($(SQLC) version 2>/dev/null)" = "$(SQLC_VERSION)" || go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	$(SQLC) generate

dictionary-index:
	test -n "$(DICTIONARY_DUMP_DATE)" || (printf '%s\n' 'DICTIONARY_DUMP_DATE is required (for example, 2026-09-14)' >&2; exit 1)
	test -z "$(strip $(KAIKKI_INPUT))" || test -z "$(filter 1 true yes,$(DICTIONARY_REFRESH))" || (printf '%s\n' 'DICTIONARY_REFRESH cannot be used with KAIKKI_INPUT' >&2; exit 1)
	test -x $(VENV_BIN)/python
	mkdir -p "$(dir $(DICTIONARY_OUTPUT))"
	test -w "$(dir $(DICTIONARY_OUTPUT))" && test -x "$(dir $(DICTIONARY_OUTPUT))" || (printf '%s\n' "dictionary output directory '$(dir $(DICTIONARY_OUTPUT))' is not writable." "If Docker created it as root, run: sudo chown \"$$(id -u):$$(id -g)\" '$(dir $(DICTIONARY_OUTPUT))'" >&2; exit 1)
	$(VENV_BIN)/python nlp/scripts/derive_dictionary_index.py $(DICTIONARY_SOURCE_ARGS) --output "$(DICTIONARY_OUTPUT)" --provider-version "$(DICTIONARY_VERSION)" --dump-date "$(DICTIONARY_DUMP_DATE)" --extraction-date "$(DICTIONARY_EXTRACTION_DATE)" --wiktextract-commit "$(DICTIONARY_WIKTEXTRACT_COMMIT)"

gen:
	mkdir -p gen/go gen/python $(GOTMPDIR)
	test "$$($(PROTOC_GEN_GO_GRPC) --version 2>/dev/null)" = "protoc-gen-go-grpc 1.5.1" || go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	test -x $(VENV_BIN)/python || $(PYTHON) -m venv $(VENV)
	$(VENV_BIN)/python -c 'import importlib.metadata; assert importlib.metadata.version("grpcio-tools") == "1.71.2"' || $(VENV_BIN)/python -m pip install --disable-pip-version-check grpcio-tools==1.71.2
	protoc -I proto --go_out=gen/go --go_opt=paths=source_relative --plugin=protoc-gen-go-grpc=$(PROTOC_GEN_GO_GRPC) --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative --python_out=gen/python $(PROTO_FILE)
	$(VENV_BIN)/python -m grpc_tools.protoc -I proto --grpc_python_out=gen/python $(PROTO_FILE)
	touch gen/python/mouseion/__init__.py gen/python/mouseion/v1/__init__.py

browser-smoke:
	cd e2e && npm ci --ignore-scripts && npx playwright install chromium && npx playwright test

dev: go-tmp
	go run ./cmd/server

clean:
	rm -rf bin .pytest_cache .ruff_cache nlp/src/mouseion_nlp.egg-info
