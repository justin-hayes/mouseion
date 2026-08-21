PYTHON ?= python3
VENV := .venv
VENV_BIN := $(VENV)/bin
PROTO_FILE := proto/mouseion/v1/normalized_corpus.proto
export GOTMPDIR := $(CURDIR)/.tmp/go

.PHONY: setup build test lint gen dev clean go-tmp

go-tmp:
	mkdir -p $(GOTMPDIR)

setup:
	$(PYTHON) -m venv $(VENV)
	$(VENV_BIN)/python -m pip install --disable-pip-version-check -r nlp/requirements-dev.txt
	$(VENV_BIN)/python -m pip install --disable-pip-version-check -e nlp

build: go-tmp
	go build ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/python -m compileall -q nlp/src gen/python

test: go-tmp
	go test ./...
	PYTHONPATH=nlp/src:gen/python $(VENV_BIN)/pytest -q nlp/tests

lint: go-tmp
	go vet ./...
	$(VENV_BIN)/ruff check nlp/src nlp/tests

gen:
	mkdir -p gen/go gen/python
	protoc -I proto --go_out=gen/go --go_opt=paths=source_relative --python_out=gen/python $(PROTO_FILE)
	touch gen/python/mouseion/__init__.py gen/python/mouseion/v1/__init__.py

dev: go-tmp
	go run ./cmd/server

clean:
	rm -rf bin .pytest_cache .ruff_cache nlp/src/mouseion_nlp.egg-info
