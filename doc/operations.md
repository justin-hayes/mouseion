# Operations Guide

How to run, configure, and maintain a Mouseion installation. For building and
testing the code, see the [development guide](development.md).

Mouseion is **three processes**: PostgreSQL, the Python NLP gRPC service
(Stanza), and the Go web server, which also runs the River background workers.
The application is intended to be reachable only over a private network such as
a Tailscale tailnet (plain HTTP over WireGuard,
[ADR 0009](adr/0009-home-lab-auth-corpus-isolation.md)); do not expose it to the
public internet.

## Option A — Docker Compose

```sh
cp .env.example .env   # set a strong MOUSEION_DB_PASSWORD and MOUSEION_SECRET
docker compose up -d --build
```

Compose starts:

- `db` — PostgreSQL 17 with a named volume
- `nlp-init` — a one-shot provisioner that downloads the configured Stanza
  models into the named `stanza-data` volume
- `nlp` — the Stanza gRPC service on `:50051`, started only after provisioning
  succeeds and its configured pipelines are warmed
- `web` — the Go server on `http://localhost:8080`

Open `http://<host>:8080`. A fresh installation presents first-account
onboarding; afterwards, learners sign in with existing accounts. There is no
administrator role and no public-registration setting
([ADR 0024](adr/0024-learner-owned-catalogs-no-admin.md)).

## Option B — run the processes manually

```sh
# 1. PostgreSQL (any running instance; create a database)
createdb mouseion
export MOUSEION_DATABASE_URL="postgres://postgres@localhost:5432/mouseion?sslmode=disable"

# 2. Python NLP gRPC service (separate terminal)
export PYTHONPATH=nlp/src:gen/python
# Provision the configured languages and full processor set into a persistent
# local directory. The marker makes reruns idempotent and refreshes on upgrades.
export STANZA_RESOURCES_DIR="$PWD/.stanza_resources"
export HF_HOME="$PWD/.huggingface"
export MOUSEION_NLP_WARM_LANGUAGES=de,it,el
.venv/bin/python -m mouseion_nlp.provision
# Start only after provisioning succeeds.
.venv/bin/python -m mouseion_nlp.server

# 3. Go web server (separate terminal) — runs migrations, starts River
GO_BIN_DIR="$(go env GOBIN 2>/dev/null)"
[ -n "$GO_BIN_DIR" ] || GO_BIN_DIR="$(go env GOPATH)/bin"
export PATH="$PATH:$GO_BIN_DIR:/usr/local/go/bin"
make dev
```

Then open `http://localhost:8080`. The Python environment is created by
`make setup`; see the [development guide](development.md).

## Configuration

Compose reads these from `.env`; see [`.env.example`](../.env.example).

| Variable | Purpose |
| --- | --- |
| `MOUSEION_DATABASE_URL` | PostgreSQL connection string (required). |
| `MOUSEION_SECRET` | Application secret; validated at startup and used to encrypt OPDS catalog credentials at rest (required). |
| `MOUSEION_DB_PASSWORD` | Compose only: the PostgreSQL password shared by `db` and `web`. |
| `MOUSEION_HTTP_ADDR` | Web listen address. |
| `MOUSEION_COOKIE_SECURE` | `true` only when serving HTTPS (for example through a TLS reverse proxy); defaults to `false`. |
| `MOUSEION_NLP_ADDR` | Address of the NLP gRPC service; defaults to `localhost:50051`. |
| `MOUSEION_NLP_WARM_LANGUAGES` | Comma-separated languages to provision, preload, and advertise (see below). The singular `MOUSEION_NLP_WARM_LANGUAGE` remains supported. |
| `MOUSEION_ANALYSIS_JOB_TIMEOUT` | Maximum duration of one analysis job. |
| `MOUSEION_DICTIONARY_INDEX` | Path to the optional read-only dictionary index (see below). |
| `MOUSEION_DICTIONARY_HOST_DIR` | Compose only: host directory bind-mounted read-only at `/opt/mouseion/dictionary`. |
| `MOUSEION_LLM_ENABLED` | `true` enables the OpenAI-compatible provider; also requires `MOUSEION_LLM_API_KEY` and `MOUSEION_LLM_MODEL`. |
| `MOUSEION_LLM_BASE_URL` | OpenAI-compatible API root; defaults to `https://api.openai.com/v1`. Batch mode requires the official OpenAI endpoint. |
| `MOUSEION_LLM_TIMEOUT` | Positive Go duration; defaults to `30s`. |
| `MOUSEION_LLM_REASONING_EFFORT` | `low`, `medium`, or `high`; defaults to `low`. |
| `MOUSEION_LLM_SUPPORTS_REASONING_EFFORT` | `true` only when a custom endpoint and model accept `reasoning_effort`. |
| `MOUSEION_PREPARED_DECK_TRANSLATION_MODE` | `standard` (default, interactive) or explicit `batch`. |
| `MOUSEION_PREPARED_DECK_STANDARD_*` | Concurrency, attempt, and retry-delay bounds for standard translation. |
| `MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS` | Requests per Batch input file; defaults to `5000`, capped at `50000`. |
| `MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL` | Batch polling interval; defaults to `30s`, capped at `24h`. |

Reading and analysis work without an LLM. Preparing a new Anki deck requires the
configured provider: each card receives a contextual English gloss grounded in
Wiktionary evidence and the source sentence
([ADR 0079](adr/0079-contextual-glosses-require-llm.md)). Without a provider,
deck preparation is reported as unavailable rather than silently degraded.

### Prepared-deck translation

Standard execution is the default. Batch remains available for explicit
offline or economy work; those preparations can remain in `preparing` for up to
the provider's 24-hour completion window. The status endpoint reports the
current phase, durable counts, retrying items, and a cancellation control.
Cancellation stops local publication first; provider file cleanup is best
effort. Batch files are temporary: Mouseion requests seven-day provider
expiration and deletes input, output, and error files after reconciliation or
cancellation, retrying failed deletion a bounded number of times.

## NLP language models

The running NLP service is authoritative for analysis-language availability
([ADR 0023](adr/0023-nlp-capabilities.md)). German (`de`), Italian (`it`), and
Modern Greek (`el`) are the supported languages; Compose configures
`MOUSEION_NLP_WARM_LANGUAGES=de,it,el` by default, while a manually launched
service defaults to `de`.

The init container provisions each language's configured Stanza package and
processor set into the `stanza-data` volume, mounted at
`STANZA_RESOURCES_DIR=/opt/stanza_resources`
([ADR 0063](adr/0063-stanza-models-on-volume.md)). GreekBERT, used by the Modern
Greek dependency parser, is provisioned into the `huggingface-data` volume at
`HF_HOME=/opt/huggingface` ([ADR 0073](adr/0073-modern-greek-language-support.md)).
The Stanza marker makes unchanged restarts a no-op, downloads only a newly
added language, and re-provisions everything when the Stanza version or an
effective language model configuration changes. The Hugging Face cache is
checked on each run and repaired if a model is missing. Serving loads only
local artifacts and never downloads models.

To add a language, set `MOUSEION_NLP_WARM_LANGUAGES` in `.env` and run
`docker compose up -d`; no image rebuild is needed. The init container must be
able to reach Stanza's model source. If provisioning fails, Compose does not
start the NLP service, and a language whose resources are unavailable is not
offered to learners. If capability discovery is temporarily unavailable,
previously discovered language names remain visible while operations that need
a newly available language are blocked.

## Dictionary index

The optional dictionary index supplies dictionary evidence and morphology (article, gender, plural, IPA,
and German-first verb principal parts) for German, Italian, and Modern Greek
([ADR 0064](adr/0064-dictionary-enrichment-provider.md)). It is a build
artifact, not PostgreSQL state and not a service. After `make setup`, derive it
from the current Kaikki raw Wiktextract dump:

```sh
make dictionary-index \
  DICTIONARY_DUMP_DATE=YYYY-MM-DD \
  DICTIONARY_WIKTEXTRACT_COMMIT=<wiktextract-commit> \
  DICTIONARY_REFRESH=1
```

`DICTIONARY_REFRESH=1` forces the downloader to fetch the weekly dump again;
omit it when rebuilding from the cached dump. `KAIKKI_INPUT` can instead point
at an already downloaded raw JSONL or JSONL.GZ dump for an offline rebuild; do
not combine it with `DICTIONARY_REFRESH=1`. The script filters the raw dump to
the German, Italian, and Modern Greek entries Mouseion needs.

The command writes `dictionary/dictionary-index.sqlite` atomically with mode
`0644`. The web container runs as an unprivileged user, so the artifact must be
world-readable; an index built before this was fixed needs a one-time
`chmod 644 dictionary/dictionary-index.sqlite`. The `dictionary/` directory is
tracked (via `dictionary/.gitkeep`) so a fresh clone has it operator-owned; if
Compose ever created it as root, run `sudo chown "$(id -u):$(id -g)" dictionary`
once before rebuilding. Set `DICTIONARY_OUTPUT` (or `DICTIONARY_HOST_DIR`) to
choose another output path.

For Compose, `MOUSEION_DICTIONARY_HOST_DIR` is the host directory mounted
read-only at `/opt/mouseion/dictionary`. It is a directory rather than a
single-file mount so a missing index remains missing. Set
`MOUSEION_DICTIONARY_INDEX` to the corresponding path inside that mount, then
restart the web process after replacing the artifact. With no index, the server
falls back to the morphology heuristic with a warning; a configured index that
exists but is unreadable is a fatal startup error. Refreshing the index needs no
database migration. Its SQLite metadata records the dump date, UTC extraction
date, Wiktextract commit, source, license, and attribution.

The index contains Wiktionary-derived data from [Kaikki.org](https://kaikki.org/)
under the source's dual CC BY-SA 3.0 / GFDL terms. Preserve the generated
metadata and attribution when shipping or sharing it; derived dictionary data
remains subject to the share-alike requirements.

## Schema migrations

The web server applies the embedded golang-migrate migrations at startup.
`migrations/000001_initialize.up.sql` is the current-state baseline
([ADR 0070](adr/0070-migration-and-documentation-reboot.md)); the pre-baseline
history is preserved at the `migrations/pre-baseline` Git tag. Schema changes
follow [ADR 0038](adr/0038-schema-change-governance.md).

## Maintenance commands

These commands run against `MOUSEION_DATABASE_URL`.

- `go run ./cmd/vocabularybackfill` — re-applies the active German
  canonicalization profile (for example pre-1996 ß spellings,
  [ADR 0065](adr/0065-german-pre-1996-sharp-s-canonicalization.md)) to
  mutable learner vocabulary. Stop the web server and other vocabulary writers
  first. The command exits non-zero for curated-sentence conflicts; resolve
  them before restarting. Immutable normalized-corpus runs and prepared-deck
  manifests are not rewritten.
- `go run ./cmd/cataloguebackfill` — assigns legacy catalogue-entry aliases to
  their owning OPDS connection
  ([ADR 0044](adr/0044-catalogue-entry-connection-scoped-identity.md)).
- `go run ./cmd/vocabularyselectioncutover --apply` — removes retired saved
  Browse selections after a verified backup; follow the
  [archived runbook](archive/operations/2026-10-06-vocabulary-browse-selections.md).

Dated one-time cutover records are kept in
[`archive/operations/`](archive/operations/).
