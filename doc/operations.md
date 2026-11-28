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

`MOUSEION_DB_PASSWORD` has no default: Compose stops with a message naming the
variable if it is unset or empty, and nothing in the repository supplies one.
Use letters and digits only, or URL-encode the value, because it is embedded in
the web service's database URL.

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
export MOUSEION_NLP_WARM_LANGUAGES=de,it,el,fr
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
| `MOUSEION_DB_PASSWORD` | Compose only, required: the PostgreSQL password shared by `db` and `web`. Compose refuses to start when it is unset or empty. |
| `MOUSEION_HTTP_ADDR` | Web listen address. |
| `MOUSEION_COOKIE_SECURE` | `true` only when serving HTTPS (for example through a TLS reverse proxy); defaults to `false`. |
| `MOUSEION_ACQUISITION_TEMP_DIR` | Directory where acquisition streams catalog EPUB files before ingest; must already exist. Empty uses the system temporary directory. Allow up to 512 MiB free per concurrent acquisition (see below). |
| `MOUSEION_NLP_ADDR` | Address of the NLP gRPC service; defaults to `localhost:50051`. |
| `MOUSEION_NLP_WARM_LANGUAGES` | Comma-separated languages to provision, preload, and advertise (see below). The singular `MOUSEION_NLP_WARM_LANGUAGE` remains supported. |
| `MOUSEION_ANALYSIS_JOB_TIMEOUT` | Maximum duration of one analysis job. |
| `MOUSEION_DICTIONARY_INDEX` | Path to the optional read-only dictionary index (see below). |
| `MOUSEION_DICTIONARY_HOST_DIR` | Compose only: host directory bind-mounted read-only at `/opt/mouseion/dictionary`. |
| `MOUSEION_LLM_ENABLED` | `true` enables the OpenAI-compatible provider; also requires `MOUSEION_LLM_API_KEY` and `MOUSEION_LLM_MODEL`. |
| `MOUSEION_LLM_BASE_URL` | OpenAI-compatible API root; defaults to `https://api.openai.com/v1`. Batch mode requires the official OpenAI endpoint. |
| `MOUSEION_LLM_TIMEOUT` | Positive Go duration; defaults to `30s`. |
| `MOUSEION_LLM_REASONING_EFFORT` | `low`, `medium`, or `high`; defaults to `low`. |
| `MOUSEION_LLM_IDIOM_REASONING_EFFORT` | Optional `low`, `medium`, or `high` for idiom-card requests only; unset uses `MOUSEION_LLM_REASONING_EFFORT`. |
| `MOUSEION_LLM_SUPPORTS_REASONING_EFFORT` | `true` only when a custom endpoint and model accept `reasoning_effort`. |
| `MOUSEION_IDIOM_CARD_LANGUAGES` | Comma-separated per-language Idiom switch for Study; empty by default. A language may appear only after its review report is committed and its provider payload assessment is recorded (ADR 0114, ADR 0137). |

My Books and analysis work without an LLM. Preparing Study material requires
the configured provider: each word receives a contextual English gloss grounded
in dictionary evidence and the source sentence
([ADR 0079](adr/0079-contextual-glosses-require-llm.md)). Without a provider,
Study material is unavailable rather than silently degraded.

### Acquisition temporary directory

Acquisition streams each Catalog file to a temporary file before deriving the
Retained EPUB ([ADR 0109](adr/0109-retained-epub-from-catalog-acquisition.md)).
A Catalog file may reach 512 MiB, so allow that much free space in
`MOUSEION_ACQUISITION_TEMP_DIR` for each acquisition that can run at once.

Point it at a disk-backed directory. The file exists to keep large downloads
out of memory, and a RAM-backed filesystem such as a `tmpfs` `/tmp` puts them
back there. Under Compose the default is the container's own `/tmp`, which is
on the container's writable layer unless you mount a `tmpfs` there. On a manual
install, check `findmnt /tmp`; if it reports `tmpfs`, create a directory on
disk and set the variable to it. A configured directory that is missing or not
a directory stops startup.

### Classical catalog preparation

The release already carries each Work's Opening, a short excerpt of the Greek
text shown on the catalog index. Adding an Ancient Greek Book from the Classical
catalog fetches its full pinned work file, and its pinned reference translation,
over HTTPS from the upstreams named in the embedded manifest
([`internal/classical/layers.json`](../internal/classical/layers.json)):
`raw.githubusercontent.com` for the `mgbilby/oga_src` text and
`PerseusDL/canonical-greekLit` translations. Each file is verified against its
reviewed size and SHA-256 and stored once per installation, so a later Book of
the same Work reuses it. The rest of the installation stays tailnet-only, but
the web server needs outbound HTTPS to those hosts for classical preparation; a
failed fetch is recorded as that Book's preparation failure and can be retried
from My Books ([ADR 0116](adr/0116-classical-catalog.md)).

### Idioms in Study

Idioms are off by default. Adding a language to `MOUSEION_IDIOM_CARD_LANGUAGES`
lets Study match the language's dictionary and reviewed idiom entries against
sentences of the learner's Corpus and introduce confirmed Idioms as Study items
([ADR 0137](adr/0137-idioms-are-study-items.md)). The switch fails closed for a
covered language until the provider payload assessment and the language review
report are recorded (ADR 0114). Italian (`it`) is recorded by the
[passing review](reviews/idiom-review-v3-gate2-2026-11-08.md); run it with
`MOUSEION_LLM_IDIOM_REASONING_EFFORT=medium`, the setting that review used.

## NLP language models

The running NLP service is authoritative for analysis-language availability
([ADR 0023](adr/0023-nlp-capabilities.md)). German (`de`), Italian (`it`),
French (`fr`), and Modern Greek (`el`) are the supported languages; Compose
configures `MOUSEION_NLP_WARM_LANGUAGES=de,it,el,fr` by default, while a
manually launched service defaults to `de`.

The init container provisions each language's configured Stanza package and
processor set into the `stanza-data` volume, mounted at
`STANZA_RESOURCES_DIR=/opt/stanza_resources`
([ADR 0063](adr/0063-stanza-models-on-volume.md)). GreekBERT, used by the Modern
Greek dependency parser, is provisioned into the `huggingface-data` volume at
`HF_HOME=/opt/huggingface` ([ADR 0073](adr/0073-modern-greek-language-support.md)).
The Stanza marker makes unchanged restarts a no-op, downloads only a newly
added language, and re-provisions everything when the Stanza version or an
effective language model configuration changes. The provisioner also verifies
each language's declared Stanza pretrain (the `conll17` word vectors behind the
Greek lemma classifier) is present, and the serving pipeline points each
processor at the provisioned copy so Greek loads offline. The Hugging Face cache
is checked on each run and repaired if a model is missing. Serving loads only
local artifacts and never downloads models.

The provisioned models carry their own third-party terms, and the default warm
set includes non-commercial ones. The
[third-party data register](third-party-data.md) lists each model, treebank,
dictionary and text with its terms. Provisioning logs a pointer to it. A
commercial deployment is the deployer's responsibility
([ADR 0111](adr/0111-third-party-licence-posture.md)).

To add a language, set `MOUSEION_NLP_WARM_LANGUAGES` in `.env` and run
`docker compose up -d`; no image rebuild is needed. The init container must be
able to reach Stanza's model source. If provisioning fails, Compose does not
start the NLP service, and a language whose resources are unavailable is not
offered to learners. If capability discovery is temporarily unavailable,
previously discovered language names remain visible while operations that need
a newly available language are blocked.

### French release gate

French support is guarded by an opt-in pinned-model golden check, outside
default CI. It replays the committed 47-string regression corpus
(`nlp/tests/testdata/french_golden_corpus.json`) through the exact pinned
Stanza 1.15.0 combined package and asserts the expected sentence boundaries,
tokens, lemmas, and target anchors. The gate passes only when it anchors 116/116
eligible corpus targets with byte-identical persisted sentences; 19 of those
targets have no word-bounded match and are reached by the persisted-token
anchor. Run it before releasing French support and after any Stanza, package,
processor, or analysis change.

```sh
export STANZA_RESOURCES_DIR="$PWD/.stanza_resources"
export HF_HOME="$PWD/.huggingface"
export MOUSEION_NLP_WARM_LANGUAGES=fr
PYTHONPATH=nlp/src:gen/python .venv/bin/python -m mouseion_nlp.provision
MOUSEION_TEST_STANZA_RESOURCES_DIR="$STANZA_RESOURCES_DIR" make french-golden
```

`make french-golden` fails fast when `MOUSEION_TEST_STANZA_RESOURCES_DIR` is
unset; `make test` collects the check but skips it, so default CI never needs
the models. `HF_HOME` must be exported before the test process starts. A
deterministic Go test (`internal/cardexport`) anchors the same committed corpus
in default CI, so only the pinned-model replay is opt-in.

## Dictionary index

The optional dictionary index supplies dictionary evidence and morphology (article, gender, plural, IPA,
and German-first verb principal parts) for German, Italian, Modern Greek, French,
and Ancient Greek
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
the German, Italian, Modern Greek, French, and Ancient Greek entries Mouseion
needs.
Ancient Greek entries are stored one row per source entry with no normalization
key; the server derives their keys when it opens the index, and an index older
than the Ancient Greek format reports `grc` as unavailable.

French lemma-risk review reads structured Kaikki `form_of` links and headword
membership from the index. Re-derive the index after upgrading so French Books
can be assessed; an index older than that format reports French lemma review
unavailable rather than passed. German review does not depend on the new data.

Idiom detection reads a word-to-multi-word-entry lookup from the index. The
derivation keeps two-to-four-word entries that have at least one ordinary sense,
excluding entries made up only of form-of or alt-of senses, proper-name entries,
and entries whose only sense is Wiktionary's literal-use marker. It also loads
Mouseion's reviewed idiom list from `nlp/reviewed_idioms.json`. The reviewed
list records its own source and version, and starts empty for Italian and
German. Re-derive the index after upgrading; an older index reports no idiom
candidates rather than failing. Study uses the lookup only for
languages whose Idiom switch is on (`MOUSEION_IDIOM_CARD_LANGUAGES`).

Rebuild the index whenever a deployed change touches
`nlp/scripts/derive_dictionary_index.py` or `nlp/reviewed_idioms.json`, even
when the index format is unchanged. The server and review tools only refuse an
index that lacks a table they need; an index from an older derivation loads
without warning and keeps the old filtering. Check with
`git log <deployed-before>..<deployed-after> -- nlp/scripts/derive_dictionary_index.py nlp/reviewed_idioms.json`.
To change only the derivation, rebuild from the same dump (`KAIKKI_INPUT`, the
same `DICTIONARY_DUMP_DATE`, and the same `DICTIONARY_LSJ_DIR`); the index
metadata records the previous dump date, source sha256 and LSJ pin.

Greek cards use the Perseus LSJ dictionary entry, not Wiktionary. To derive it,
point `DICTIONARY_LSJ_DIR` at the pinned `PerseusDL/lexica`
`CTS_XML_TEI/perseus/pdllex/grc/lsj` directory:

```sh
make dictionary-index \
  KAIKKI_INPUT=/path/to/raw-wiktextract-data.jsonl.gz \
  DICTIONARY_DUMP_DATE=YYYY-MM-DD \
  DICTIONARY_LSJ_DIR=/path/to/perseus/lexica/grc/lsj
```

The derivation preserves each LSJ entry's native id and sense hierarchy, keeps
only Xenophon citations, and records the revision pin (`DICTIONARY_LSJ_PIN`,
default commit `56061ca…`) in the index metadata. It also records whether a
numbered entry is a phrase sub-entry or a cross-reference stub filed under a
bare word's key, so such a sub-entry never counts as a homograph when the bare
lemma is looked up. An index built before this format must be rebuilt. The real
LSJ files never enter the repository or CI; both use synthetic fixtures.

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
date, Wiktextract commit, source-file sha256, source, license, and attribution.

The index contains Wiktionary-derived data from [Kaikki.org](https://kaikki.org/)
under the source's dual CC BY-SA 4.0 / GFDL terms. Preserve the generated
metadata and attribution when shipping or sharing it; derived dictionary data
remains subject to the share-alike requirements.

## Schema migrations

The web server applies the embedded golang-migrate migrations at startup.
`migrations/000001_initialize.up.sql` is the current-state baseline
([ADR 0070](adr/0070-migration-and-documentation-reboot.md)); the pre-baseline
history is preserved at the `migrations/pre-baseline` Git tag. Schema changes
follow [ADR 0038](adr/0038-schema-change-governance.md).

### Idiom candidate count rebuild

Migration `000077_idiom_candidate_counts` adds the per-Book Idiom candidate
count tables empty. Filling them is a data backfill in the sense of
[ADR 0038](adr/0038-schema-change-governance.md), and it is the server's own
River job, not a command. Its operational contract:

- **Owner.** The operator who deploys the release. The server schedules it at
  every start (`EnqueueMissingIdiomCountRebuilds`) and River runs it on the
  `idiom_counts` queue, so no one starts it by hand.
- **When it runs.** Once after the release for every Book with a current
  analysis, and again for a Book whenever its analysis publishes, a Lemma
  correction or Occurrence exclusion changes it, or the idiom index (a
  dictionary refresh, or a change to the reviewed idiom list) or the builder
  version changes. A Book is rebuilt when its readiness row is missing or names
  another analysis run, builder version or index version.
- **Idempotent and safe to retry.** A job replaces one Book's count rows and
  readiness row in one transaction, so a repeat gives the same rows and a
  failed attempt leaves the previous rows. A rebuild already waiting or running
  for the same Book, run and decision revision absorbs a repeated insert, so
  restarting the server does not duplicate work. A job retries up to 10 times.
- **Locking and impact.** Each job works in one transaction that touches only
  that Book's count and readiness rows. It runs on its own queue, so analysis
  and enrichment never wait for it. A rebuild of every Book takes minutes, not
  seconds.
- **While it runs.** A Book whose counts are not ready is not counted as having
  no Idioms: an Idiom already in study is not withdrawn for want of a
  candidate, and Idioms are offered as each Book's counts become ready.
- **Detecting failure.** A job that exhausts its attempts is `discarded` in
  River: look for jobs of kind `rebuild_idiom_candidate_counts` in state
  `discarded` (or `retryable` for long), and for Books with no row in
  `idiom_candidate_count_readiness` for the current analysis run.
- **Recovery.** Fix the cause in the job's error (a dictionary index that cannot
  be opened is the usual one), then restart the server: startup queues a
  rebuild for every Book still missing ready counts, discarded jobs included,
  because readiness rather than job history decides what is missing.
- **Rollback.** None is needed. The tables are derived data: dropping the
  feature leaves them unread, and the forward fix for wrong counts is a
  builder version bump, which rebuilds every Book on the next start.

### Single-version schema and evidence cutover

Some releases add a new source-extraction or analysis contract that must not
coexist with older writers. The source-structure release
(`000045_source_structure_v2`, [ADR 0103](adr/0103-source-structure-and-poetry-evidence.md))
is the current case. Deploy one version at a time; this procedure is not
deployment permission.

1. **Quiesce the old version.** Stop the web process, which owns the River
   workers: `docker compose stop web`. Confirm no old web process, worker,
   analysis run, catalogue sync or one-off maintenance command still writes.
   Mixed old/new writers are unsupported.
2. **Optionally back up.** A manual `pg_dump --format=custom` outside the
   repository is insurance, not an automated rollback step. ADR 0103 does not
   require it.
3. **Apply the additive DDL and start the new version.** Start the new image
   with `docker compose up -d --build web`. On startup the binary applies the
   embedded migrations before it opens the store, serves HTTP or starts River,
   so DDL completes before any v2 write. Verify the migration reached version
   45 with no dirty state, then run the container healthcheck.
4. **Keep one version running.** Do not start the old image again after the
   migration. There is no mixed-writer mode and no supported downgrade after a
   Book adopts v2.
5. **Verify the new state.** The health endpoint returns `ok`; River workers are
   registered; existing v1 snapshots, unit text and analyses are unchanged; a
   newly acquired Book records `schema_version = 2` with the versioned
   convention set; no existing Book changed snapshot without learner
   confirmation.
6. **Adopt deliberately.** Existing Books remain on v1 until a learner confirms
   **Source structure update** at Book level. Do not run a library-wide
   adoption, pre-create candidate snapshots or enqueue automatic reanalysis.

Failure during the migration prevents the server from starting. The migration
runs in a transaction, so the previous schema stays in place; resolve the cause
and retry the same version rather than editing migration history. Failure after
the switch uses the existing stale/failed analysis states and **Re-analyze**;
retries converge on the same extraction and run identity.
`000045_source_structure_v2.down.sql` drops the structure tables and is not a
supported production rollback after v2 adoption. Recovery is retry and
forward-fix only: there is no revert-to-v1 action and no correction or Reading
remapping. Restoring a pre-cutover backup is a break-glass operator
decision, not a downgrade procedure.

## Maintenance commands

These commands run against `MOUSEION_DATABASE_URL`.

- The retired `cmd/greekregister` command has no replacement to run. At startup
  the binary reconciles the embedded Classical catalog manifest and converts
  every pre-upgrade Ancient Greek Book its legacy-rows check counts. The same
  check and conversion share one definition, so a counted Book is never left
  outside the conversion:
  - A Book with a published analysis is reproduced under the current
    interpretation. Matching evidence also records the prepared definition;
    differing evidence leaves the published analysis and corpus untouched so
    the ordinary Analysis update stays outstanding.
  - A Book whose operator import was interrupted after the whole-file source row
    was written has no published analysis. The conversion rewrites the source to
    unit-only content under the Reading-unit key, and the ordinary classical
    preparation path publishes its analysis later.
  - A Book whose Reading unit the manifest later withdrew converts to unit-only
    content under its key and records no prepared definition, so it behaves
    like any other Book with a withdrawn definition instead of blocking the
    installation. A unit withdrawn before its first reconciliation has no
    recorded source format from the retired registration; the conversion reads
    that empty value as the OGA CoNLL-U dialect the old path required.

  In every case the Book, its Reading-unit association, disposition, learner
  decisions, vocabulary, completions, current-reading snapshots and prepared
  decks are kept. A source that cannot be reproduced (for example malformed
  bytes or a pin mismatch) stays whole-file, is reported as skipped in the
  startup log and keeps the catalog fail-closed for a later start; the learner
  copy names a restart as the retry and never a conversion command that does not
  exist.
  The server owns the conversion; it is idempotent and retry-safe, and a failed
  start leaves the previous schema and learner state in place to retry.
  Historical whole-file revisions remain immutable evidence for frozen Readings;
  the Book's current source is the Reading unit.
- `go run ./cmd/greekhitrate` — reports how many of the installation's Ancient
  Greek vocabulary identities resolve an unambiguous LSJ dictionary entry (the
  entry a Greek card shows) alongside the Wiktionary hit rate, so the operator
  can judge how much meaning evidence cards will carry. It needs both
  `MOUSEION_DATABASE_URL` and `MOUSEION_DICTIONARY_INDEX`, prints counts only
  (overall and per part of speech) and never any Greek text, and fails with a
  clear message when the index is older than the Ancient Greek or LSJ format and
  must be rebuilt with `make dictionary-index`.
- `MOUSEION_DATABASE_URL=... go run ./cmd/backfill` — the one-time, data-only
  My Books backfill for the release that retired the Inbox state. Run it once,
  after deploying that release and after every catalog connection has completed
  at least one sync. The release image builds only the server binary, so run it
  from a source checkout with the Go toolchain. It removes the My Books
  membership of every active Book that has no chosen language, and of every
  active Book that is not To Read, has no reading history and is not the
  current reading. Then it prints `removed N unchosen Books from My Books` and
  one `removed <id> ("<title>") from My Books: no chosen language; its reading
  history is kept, ...` line for each language-less Book with history. Its
  completions are kept, and adding the Book again with a language restores it. The refusal `refusing to run: every catalog connection must
  complete a sync first` means a connection has never completed a sync: run
  **Sync now** for that connection from the Catalog page or Manage sources, or
  delete a connection that no longer exists, then re-run. The command is
  idempotent and retry-safe: a repeated run removes nothing new, and re-running
  it restores nothing, because membership is the only thing it changes. Recover
  a removed Book through Catalog **Add**, which restores the same Book with its
  evidence while its entry is in the connection's Catalog index; a classical
  Book is restored by adding its Reading unit from the Classical catalog. See
  [My Books Collection Browsing](features/collection-browsing.md#upgrading-existing-accounts).

Dated one-time cutover records are kept in
[`archive/operations/`](archive/operations/).
