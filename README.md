# mouseion

A self-hosted reading environment for learning foreign languages at an advanced reading level. It turns books from your own Calibre-Web/OPDS library into vocabulary you can actually study — analyzing texts, surfacing the words worth learning with example sentences, and exporting an Anki deck.

Built around a Go core with a Python NLP service for analysis.

Mouseion uses local learner accounts. On a fresh installation, the sign-in page
creates the first account; after that, existing accounts use the normal login
form. There is no in-application administrator role or public registration
setting. Each learner owns their books, vocabulary state, generated decks, and
OPDS catalog connections, including catalog credentials encrypted at rest.

The running NLP service is authoritative for analysis-language availability.
Mouseion discovers the languages and features it currently advertises instead
of maintaining a separate web-app language allowlist. If discovery is
temporarily unavailable, previously discovered language display names remain
available while operations requiring a newly available analysis language are blocked.

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
- `nlp` — the Stanza gRPC service on `:50051`, with German and Italian models
  provisioned in the image and warmed before they are advertised as ready
- `web` — the Go server on `http://localhost:8080`

Compose configures `MOUSEION_NLP_WARM_LANGUAGES=de,it` by default. Override the
comma-separated value only with languages whose Stanza resources are already
installed. The image sets `STANZA_RESOURCES_DIR=/opt/stanza_resources` and
pre-populates that immutable model cache with `de` and `it`; adding another
language requires adding its `stanza.download(...)` entry to `nlp/Dockerfile`
and rebuilding the image. A configured language whose model is absent remains
not ready and is not offered to learners.

Open `http://<host>:8080`. A fresh installation presents first-account
onboarding; otherwise, sign in with an existing account. The app is meant to be
reachable only over your tailnet (plain HTTP over WireGuard); do not expose
`:8080` publicly.

### Option B — run the three processes manually

```sh
# 1. PostgreSQL (any running instance; create a db)
createdb mouseion
export MOUSEION_DATABASE_URL="postgres://postgres@localhost:5432/mouseion?sslmode=disable"

# 2. Python NLP gRPC service (separate terminal)
export PYTHONPATH=nlp/src:gen/python
# Provision de and it in the local Stanza cache once if they are not installed.
.venv/bin/python -c "import stanza; [stanza.download(code, processors='tokenize,pos,lemma') for code in ('de', 'it')]"
# Warm both deployment languages (the manual-launch default is de only).
export MOUSEION_NLP_WARM_LANGUAGES=de,it
.venv/bin/python -m mouseion_nlp.server

# 3. Go web server (separate terminal) — runs migrations, starts River
export PATH="$PATH:$(go env GOBIN 2>/dev/null || echo "$HOME/go/bin"):/usr/local/go/bin"
make dev
```

Then open `http://localhost:8080`.

## Language validation

German and Italian are the deployment-supported analysis languages. The
Italian vertical is covered deterministically from capability discovery and
learner selection through Stanza fixture consumption, content-word filtering,
coverage thresholds, prepared-deck/campaign eligibility, and the generated
`Mouseion::it::<book title>` APKG. These tests also assert account isolation and
keep the German regression suite intact. See the [language-support feature
contract](doc/features/language-support.md) for the supported path and model
cache requirements.
