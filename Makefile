PYTHON ?= python3
VENV := .venv
VENV_BIN := $(VENV)/bin
PROTO_FILE := proto/mouseion/v1/normalized_corpus.proto
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc
HERMES_WORKER_IMAGE ?= mouseion-hermes-worker:local
export GOTMPDIR := $(CURDIR)/.tmp/go

.PHONY: setup build test test-integration lint gen templ dev clean go-tmp hermes-worker-smoke

go-tmp:
	mkdir -p $(GOTMPDIR)

setup:
	$(PYTHON) -m venv $(VENV)
	$(VENV_BIN)/python -m pip install --disable-pip-version-check -r nlp/requirements-dev.txt
	$(VENV_BIN)/python -m pip install --disable-pip-version-check torch --index-url https://download.pytorch.org/whl/cpu
	$(VENV_BIN)/python -m pip install --disable-pip-version-check -e nlp

build: go-tmp
	go build ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/python -m compileall -q nlp/src gen/python

test: go-tmp
	go test ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/pytest -q nlp/tests

test-integration: go-tmp
	go test -tags=integration ./internal/...

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

hermes-worker-smoke:
	docker run --rm --entrypoint /bin/sh $(HERMES_WORKER_IMAGE) -ceu '\
		command -v go >/dev/null; \
		command -v protoc >/dev/null; \
		command -v protoc-gen-go >/dev/null; \
		command -v protoc-gen-go-grpc >/dev/null; \
		command -v codex >/dev/null; \
		go version; \
		protoc --version; \
		protoc-gen-go --version; \
		protoc-gen-go-grpc --version; \
		codex --version'

dev: go-tmp
	go run ./cmd/server

clean:
	rm -rf bin .pytest_cache .ruff_cache nlp/src/mouseion_nlp.egg-info
