# mouseion

A self-hosted reading environment for learning foreign languages at an advanced reading level. It turns books from your own Calibre-Web/OPDS library into vocabulary you can actually study — analyzing texts, surfacing the words worth learning with example sentences, and exporting an Anki deck.

Built around a Go core with a Python NLP service for analysis.

See [the product specification](doc/product.md) and [documentation governance](doc/documentation-governance.md).

## Development

The workspace supports Go 1.24 and Python 3.11. It also requires Protobuf
29.x for regenerating the shared contract. Go dependencies are pinned in
`go.mod`/`go.sum`; Python development dependencies are pinned in
`nlp/requirements-dev.txt`.

```sh
export PATH="$PATH:$(go env GOBIN 2>/dev/null || echo "$HOME/go/bin"):/usr/local/go/bin"
make setup
make gen
make build
make test
make lint
```

`make dev` starts the Go web server, which connects to PostgreSQL (required,
`MOUSEION_DATABASE_URL`) and the Python NLP gRPC service (required for analysis,
`MOUSEION_NLP_ADDR`, default `localhost:50051`). PostgreSQL schema changes live
in `migrations/` as paired golang-migrate SQL files.

## Running the complete stack

The full app is **three processes**: PostgreSQL, the Python NLP gRPC service
(Stanza), and the Go web server. Two ways to run it.

### Option A — Docker Compose (recommended for the home lab)

```sh
cp .env.example .env   # set a strong MOUSEION_DB_PASSWORD and MOUSEION_SECRET
docker compose up -d --build
```

- `db` — PostgreSQL 17 with a named volume
- `nlp` — the Stanza gRPC service on `:50051`
- `web` — the Go server on `http://localhost:8080`

Open `http://<host>:8080` and bootstrap the admin account. Per ADR 0009 the app
is meant to be reachable only over your tailnet (plain HTTP over WireGuard); do
not expose `:8080` publicly.

### Option B — run the three processes manually

```sh
# 1. PostgreSQL (any running instance; create a db)
createdb mouseion
export MOUSEION_DATABASE_URL="postgres://postgres@localhost:5432/mouseion?sslmode=disable"

# 2. Python NLP gRPC service (separate terminal)
export PYTHONPATH=nlp/src:gen/python
.venv/bin/python -m mouseion_nlp.server

# 3. Go web server (separate terminal) — runs migrations, starts River
export PATH="$PATH:$(go env GOBIN 2>/dev/null || echo "$HOME/go/bin"):/usr/local/go/bin"
make dev
```

Then open `http://localhost:8080`.

