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
GO_BIN_DIR="$(go env GOBIN 2>/dev/null)"
[ -n "$GO_BIN_DIR" ] || GO_BIN_DIR="$(go env GOPATH)/bin"
export PATH="$PATH:$GO_BIN_DIR:/usr/local/go/bin"
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- \
  -b "$GO_BIN_DIR" v2.13.2
make setup
make gen
make build
make test
make lint
```

Database-backed integration tests always execute rather than using Go's test
result cache. Use either `make test-integration` for the full internal package
scope or `make test-integration-shared` for the shared-database runner. Ordinary
unit-test commands such as `go test ./...` retain Go's normal caching behavior.

`make dev` starts the Go web server, which connects to PostgreSQL (required,
`MOUSEION_DATABASE_URL`) and the Python NLP gRPC service (required for analysis,
`MOUSEION_NLP_ADDR`, default `localhost:50051`). PostgreSQL schema changes live
in `migrations/` as paired golang-migrate SQL files.

## Running the complete stack

The full app is **three processes**: PostgreSQL, the Python NLP gRPC service
(Stanza), and the Go web server. Two ways to run it.

### Option A — Docker Compose (recommended for the home lab)

The one-time database recreation required after the migration baseline is
recorded in the [2026-09-15 cutover note](doc/cutover-2026-09-15.md).

```sh
cp .env.example .env   # set a strong MOUSEION_DB_PASSWORD and MOUSEION_SECRET
docker compose up -d --build
```

- `db` — PostgreSQL 17 with a named volume
- `nlp-init` — a one-shot provisioner that downloads the configured Stanza
  models into the named `stanza-data` volume
- `nlp` — the Stanza gRPC service on `:50051`, started only after provisioning
  succeeds and its configured pipelines are warmed
- `web` — the Go server on `http://localhost:8080`

Compose configures `MOUSEION_NLP_WARM_LANGUAGES=de,it` by default. The init
container provisions the full runtime processor set (`tokenize,pos,lemma,
depparse`) into `stanza-data`, mounted at
`STANZA_RESOURCES_DIR=/opt/stanza_resources`. A marker in that volume makes
unchanged restarts a no-op, downloads only a newly added language, and
re-provisions everything automatically when the Stanza version changes.

To add a language, set the comma-separated `MOUSEION_NLP_WARM_LANGUAGES` value
in `.env` and run `docker compose up -d`; no image rebuild is needed. The init
container must be able to reach Stanza's model source. If provisioning fails,
Compose does not start the NLP service and a language whose resources are not
available is not offered to learners.

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
# Provision the configured languages and full processor set into a persistent
# local directory. The marker makes reruns idempotent and refreshes on upgrades.
export STANZA_RESOURCES_DIR="$PWD/.stanza_resources"
export MOUSEION_NLP_WARM_LANGUAGES=de,it
.venv/bin/python -m mouseion_nlp.provision
# Start only after provisioning succeeds.
.venv/bin/python -m mouseion_nlp.server

# 3. Go web server (separate terminal) — runs migrations, starts River
GO_BIN_DIR="$(go env GOBIN 2>/dev/null)"
[ -n "$GO_BIN_DIR" ] || GO_BIN_DIR="$(go env GOPATH)/bin"
export PATH="$PATH:$GO_BIN_DIR:/usr/local/go/bin"
make dev
```

Then open `http://localhost:8080`.

## Refreshing the dictionary index

The optional dictionary index is a build artifact, not Postgres state and not a
service. After running `make setup`, derive the combined German and Italian
index from the current Kaikki raw Wiktextract dump with:

```sh
make dictionary-index \
  DICTIONARY_DUMP_DATE=YYYY-MM-DD \
  DICTIONARY_WIKTEXTRACT_COMMIT=<wiktextract-commit> \
  DICTIONARY_REFRESH=1
```

`DICTIONARY_REFRESH=1` forces the downloader to fetch the weekly dump again;
omit it when rebuilding from the cached dump. `KAIKKI_INPUT` can instead point
at an already downloaded raw JSONL or JSONL.GZ dump for an offline rebuild. The
input is the raw Wiktextract dump; the script filters it to the German and
Italian entries needed by Mouseion. Do not combine `KAIKKI_INPUT` with
`DICTIONARY_REFRESH=1`.

The command writes `dictionary/dictionary-index.sqlite` atomically, mode `0644`.
The web container runs as an unprivileged user, so the artifact must be
world-readable; an index built before this was fixed needs a one-time
`chmod 644 dictionary/dictionary-index.sqlite`. The `dictionary/` directory is
tracked (via `dictionary/.gitkeep`) so a fresh clone has it operator-owned; if
Compose ever created it as root, run `sudo chown "$(id -u):$(id -g)" dictionary`
once before rebuilding. Set `DICTIONARY_OUTPUT` (or `DICTIONARY_HOST_DIR`) to
choose another output path.

For Compose deployment, `MOUSEION_DICTIONARY_HOST_DIR` is the host directory
bind-mounted read-only at `/opt/mouseion/dictionary`; it is a directory rather
than a single-file mount so a missing index remains missing. Set
`MOUSEION_DICTIONARY_INDEX` to the corresponding path inside that mount, then
restart the web process after replacing the artifact. With no index, the server
falls back to the morphology heuristic with a warning. A configured index that
exists but is unreadable is a fatal startup error. Refreshing the index changes
only this build artifact: it requires no database migration and no additional
service. Its SQLite metadata records the dump date, UTC extraction date,
Wiktextract commit, source, license, and attribution.

The index contains Wiktionary-derived data from [Kaikki.org](https://kaikki.org/)
and is licensed under the source's dual CC BY-SA 3.0 / GFDL terms. Preserve the
generated metadata and attribution when shipping or sharing the index; derived
dictionary data remains subject to the applicable share-alike requirements.

## Re-normalizing German vocabulary

Stop the web server and other vocabulary writers, then run the owner-
transactional backfill with the same database environment:

```sh
go run ./cmd/vocabularybackfill
```

The command is idempotent and exits non-zero for curated-sentence conflicts.
Resolve reported conflicts before starting the v6 server; immutable
normalized-corpus runs and prepared-deck manifests are not rewritten.

## Language validation

German and Italian are the deployment-supported analysis languages. The
Italian vertical is covered deterministically from capability discovery and
learner selection through Stanza fixture consumption, content-word filtering,
coverage thresholds, Goal-owned reservation eligibility, and the generated
`Mouseion::it::<book title>` APKG. These tests also assert account isolation and
keep the German regression suite intact. See the [language-support feature
contract](doc/features/language-support.md) for the supported path and model
cache requirements.
