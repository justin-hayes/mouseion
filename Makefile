PYTHON ?= python3
UV ?= uv
VENV := .venv
VENV_BIN := $(VENV)/bin
PROTO_FILE := proto/mouseion/v1/normalized_corpus.proto
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc
MOUSEION_TEST_PG_PORT ?= 55432
MOUSEION_TEST_PACKAGES ?= ./internal/...
export GOTMPDIR := $(CURDIR)/.tmp/go

.PHONY: setup build test test-integration test-integration-shared lint gen templ dev clean go-tmp browser-smoke

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
	go test -tags=integration ./internal/...

test-integration-shared: go-tmp
	@set -eu; \
	if test -n "$${MOUSEION_TEST_DATABASE_URL:-}"; then \
		go test -tags=integration -p 1 $(MOUSEION_TEST_PACKAGES); \
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
	MOUSEION_TEST_DATABASE_URL="$$database_url" go test -tags=integration -p 1 $(MOUSEION_TEST_PACKAGES)

lint: go-tmp
	go vet ./...
	$(VENV_BIN)/ruff check nlp/src nlp/tests

templ:
	templ generate

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
