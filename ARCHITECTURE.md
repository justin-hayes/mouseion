# Architecture

This document describes how Mouseion is built: its processes, how a book
becomes an analyzed corpus and then Study material, how data is owned and stored,
and where each responsibility lives in the code. It describes the system as it
is. The reasons behind each decision are in the
[architecture decision records](doc/adr/), linked throughout, and the
linguistic design is covered in the [linguistic design notes](doc/linguistics.md).

## Principles

A few rules shape most of the design:

- **Product logic lives in Go; Python only does NLP.** The Python service
  turns text into annotated sentences and nothing else. Vocabulary identity,
  selection, scoring, and Study scheduling are Go code that can be tested without models
  ([ADR 0001](doc/adr/0001-go-core-python-nlp-service.md)).
- **Analysis output is immutable.** A completed analysis is never edited.
  Learner corrections, newer analyses, and changed presentation are layered on
  top or recorded as new runs, so every Study card can be traced to the exact text
  and analyzer version it came from
  ([ADR 0028](doc/adr/0028-explicit-scoped-analysis-lifecycle.md),
  [ADR 0081](doc/adr/0081-learner-owned-occurrence-lemma-corrections.md)).
- **Ownership is enforced by the database.** Learner-owned tables key on
  `owner_id`, and references between them are composite `(owner_id, …)`
  foreign keys, so a row cannot point at another learner's data even if
  application code is wrong
  ([ADR 0002](doc/adr/0002-multi-user-accounts.md)).
- **Long work is durable.** Analysis, catalog sync, and Study material preparation run as
  [River](https://riverqueue.com/) jobs in PostgreSQL. Jobs are inserted in the
  same transaction as the state they act on, survive restarts, and record
  progress in ordinary tables
  ([ADR 0010](doc/adr/0010-river-job-queue.md)).
- **The NLP service decides what is possible.** Languages and features are
  discovered from the running service, not configured in the web application
  ([ADR 0023](doc/adr/0023-nlp-capabilities.md)).
- **The interface is server-rendered.** Pages are HTML from Go templates,
  enhanced with htmx; core workflows also work without JavaScript
  ([ADR 0083](doc/adr/0083-concordance-server-rendering-and-htmx-4.md)).

## System overview

```mermaid
flowchart LR
    learner([Learner]) -->|browser, private network| web

    subgraph host[Mouseion deployment]
        subgraph server[Go server process]
            web[HTTP handlers<br/>templ + htmx]
            workers[River workers]
            core[Domain core<br/>selection, scoring, export]
            web --> core
            workers --> core
        end
        nlp[NLP service<br/>Python + Stanza]
        pg[(PostgreSQL<br/>learner data, corpus,<br/>River jobs)]
        dict[(Dictionary index<br/>SQLite, read-only)]
    end

    opds[(OPDS catalog<br/>e.g. Calibre-Web)]
    llm[OpenAI-compatible<br/>LLM provider]

    core <--> pg
    workers -->|gRPC: Analyze,<br/>GetCapabilities| nlp
    web -->|download EPUB| opds
    workers -->|catalog sync| opds
    core --> dict
    workers -->|glosses and<br/>sentence translation| llm
```

| Process | Responsibility |
| --- | --- |
| Go server ([`cmd/server`](cmd/server)) | HTTP interface, authentication, domain logic, and River workers in one binary. Applies database migrations at startup. |
| NLP service ([`nlp/`](nlp/src/mouseion_nlp)) | Long-lived gRPC server that keeps Stanza pipelines warm and returns tokenized, tagged, lemmatized, and parsed sentences. A one-shot provisioner downloads models into a volume ([ADR 0063](doc/adr/0063-stanza-models-on-volume.md)). |
| PostgreSQL | All learner state, the analyzed corpus, Study progress, and the River job tables. |
| Dictionary index | Optional SQLite file derived at build time from Wiktionary data and mounted read-only ([ADR 0064](doc/adr/0064-dictionary-enrichment-provider.md)). |

The Go↔Python boundary is a single versioned Protobuf contract,
[`proto/mouseion/v1/normalized_corpus.proto`](proto/mouseion/v1/normalized_corpus.proto),
with two calls: `Analyze` and `GetCapabilities`
([ADR 0011](doc/adr/0011-grpc-go-python-transport.md)).

## From book to corpus

When a learner adds a book or moves it to *To Read*, the web request enqueues
durable EPUB acquisition and returns. A River worker downloads and imports the
EPUB, then records an analysis run; another River worker does the analysis.

```mermaid
sequenceDiagram
    actor L as Learner
    participant W as Web handler
    participant O as OPDS catalog
    participant DB as PostgreSQL
    participant A as Acquisition worker
    participant R as Analysis worker
    participant N as NLP service

    L->>W: Add, Move to To Read, or Retry
    W->>DB: Insert EPUB acquisition job
    W-->>L: "Downloading and analyzing the text."

    A->>DB: Claim acquisition job
    A->>DB: Confirm Book is still To Read and not current
    A->>O: Download EPUB
    A->>A: Extract reading-order units<br/>with source offsets
    A->>DB: Store content revision and unit snapshot
    A->>DB: Clear the Book's acquisition failure
    A->>DB: Insert analysis run + River job (one transaction)

    R->>DB: Claim job, load unit snapshot
    R->>R: Select main text from EPUB landmarks
    loop each unit, in size-bounded chunks
        R->>N: Analyze(language, text)
        N-->>R: Sentences with tokens, lemmas, UPOS,<br/>morphology, dependencies, offsets
        R->>DB: Update progress
    end
    R->>DB: Persist corpus, sentences, tokens,<br/>and statistics (one transaction)
    R->>DB: Generate vocabulary candidates
    R->>DB: Promote run to the book's current analysis
```

- **Extraction** ([`internal/epub`](internal/epub)) walks the EPUB spine,
  skips non-linear and navigation documents, and keeps each unit's text,
  title, landmarks, and character offsets. The snapshot is immutable; a new
  catalog revision gets a new snapshot.
- **Main-text selection** uses declared EPUB 3 landmarks to drop front and back
  matter, falling back to the whole book when the structure is ambiguous
  ([ADR 0066](doc/adr/0066-main-text-selection-from-epub-structure.md)).
- **Chunking** keeps each gRPC request bounded and shifts offsets so every token
  still points at its exact place in the source
  ([ADR 0013](doc/adr/0013-size-based-nlp-chunking.md)).
- **The normalized corpus** (`corpus_sentences`, `corpus_tokens`) stores every
  token's surface form, raw and canonical lemma, UPOS, morphology, dependency
  relation and head, and source offsets. Coverage metrics, the concordance,
  sentence scoring, and lemma review all read this data instead of calling the
  NLP service again
  ([ADR 0059](doc/adr/0059-persisted-normalized-corpus-for-concordance.md),
  [ADR 0060](doc/adr/0060-persist-dependency-parses.md)).
- **Promotion** points the book at its newest completed analysis. Earlier runs
  stay as history.

### Analysis run lifecycle

```mermaid
stateDiagram-v2
    [*] --> queued: run recorded + job inserted
    queued --> running: worker claims job
    running --> completed: corpus persisted and promoted
    running --> failed: error or timeout
    queued --> cancelled
    running --> cancelled
    failed --> queued: retry (new attempt, same run)
    cancelled --> queued: retry
    completed --> [*]
```

A run's identity and status URL stay stable across retries; each attempt is
recorded separately in `analysis_run_attempts`. Cancellation commits in the
database before the River job is cleaned up, so a late-finishing worker cannot
resurrect a cancelled run.

## From corpus to Study

Study ([`internal/study`](internal/study)) is the only vocabulary workflow
([ADR 0134](doc/adr/0134-study-in-mouseion-vocabulary-review.md),
[ADR 0136](doc/adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md)).
Nothing about a Book's vocabulary is frozen: there is no Current-reading
snapshot, no Reserved or Known vocabulary and no Prepared deck.

```mermaid
flowchart TD
    corpus[Corpus<br/>analysed Books in My Books] --> order[Order new words<br/>Current reading until Target coverage,<br/>then To Read frequency]
    order --> batch[New-word batch<br/>taken or declined on Today]
    batch --> plan[study_plan_preparation]
    plan --> mat[study_prepare_material<br/>one word at a time]
    mat --> card[Study material<br/>context, Gloss, translation]
    card --> today[Today<br/>Again, Got it, Already known, Report]
    today --> fsrs[(Study progress<br/>FSRS state and review log)]
    fsrs --> fluency[study_prepare_fluency<br/>Fluency contexts for Mature words]
    today -. Wrong word .-> remed[Remediation screen<br/>correct or exclude, plus ticked matches]
    remed -. Lemma correction or Occurrence exclusion .-> corpus
```

- **The Corpus** is the analysed Books in My Books in the study language. Study
  material and Fluency contexts draw on all of it, Read Books included. Only To
  Read Books set the priority of new words, and the Current reading's words come
  first until its coverage, counting Mature and in-study words, reaches Target
  coverage (98%). The order is computed from current evidence each time.
- **Candidates** are effective vocabulary identities
  ([`internal/selection`](internal/selection) holds the eligibility rules) and
  Idioms ([ADR 0137](doc/adr/0137-idioms-are-study-items.md)) that are not in
  study; held words stay excluded.
- **Study material** is prepared one word at a time by River jobs shortly before
  Today needs it, from current evidence with the learner's Lemma corrections and
  Occurrence exclusions applied. A provider-route language gets a contextual
  Gloss and translation; a source-evidence language (Ancient Greek, Latin) gets
  a Gloss from the Evidence-assisted provider pass that cites the dictionary
  ([ADR 0133](doc/adr/0133-per-language-card-policy.md),
  [ADR 0138](doc/adr/0138-study-for-source-evidence-languages.md)).
- **Progress** is one FSRS state per learner, language and Study item, with an
  append-only review log of graded answers. **Already known** is answered at
  introduction: it writes review-state progress with an interval of at least 21
  days, so the word is Mature at once, and records the answer in its own event
  table, never as a grade.
- **Coverage** is the share of a Book's analyzable tokens whose effective
  vocabulary identity is Mature, with in-study coverage as a second figure
  (`analysisinsights`). Coverage counts word tokens only.
- **Fixes** start only from a Wrong-word Study report. Its remediation screen
  reuses the lemma-review preview-and-confirm step
  ([`internal/lemmareview`](internal/lemmareview)) for the reported occurrence
  and for matching occurrences the learner ticks, each decided in its own Book's
  analysis; nothing becomes a language-wide rule
  ([ADR 0081](doc/adr/0081-learner-owned-occurrence-lemma-corrections.md)).
- **Finishing a Book** appends a Reading completion and clears the current
  role. It does not read or write vocabulary
  ([ADR 0078](doc/adr/0078-book-dispositions-and-current-reading.md)).

## Background jobs

All background work runs in River inside the Go server process, on named
queues so a long analysis never blocks catalog sync or Study work.

| Queue | Job kinds | Work |
| --- | --- | --- |
| `analysis` | `analyze_corpus`, `rebuild_vocabulary_browse_counts`, `rebuild_idiom_candidate_counts`, `rebuild_construction_occurrences` | Book analysis; cross-book vocabulary, Idiom and Construction projections |
| `catalogue_sync` | `catalogue_sync` (periodic per connection), `book_cover` | Metadata-only catalog reconciliation and cover retrieval ([ADR 0041](doc/adr/0041-catalog-sync-metadata-first.md)) |
| `study` | `study_plan_preparation`, `study_prepare_material`, `study_prepare_fluency` | Plans the next New-word batch's Study material, prepares one word's material, and prepares Fluency contexts ([ADR 0134](doc/adr/0134-study-in-mouseion-vocabulary-review.md)) |
| `source_checks` | `check_source_structure` | Extraction-only comparison of existing EPUB snapshots with the configured extraction contract ([ADR 0103](doc/adr/0103-source-structure-and-poetry-evidence.md)) |
| `classical_preparation` | `classical_preparation` | Serial import of one classical Book's Reading unit: fetch and verify the pinned work file, cut the unit, and publish an ordinary analysis ([ADR 0116](doc/adr/0116-classical-catalog.md)) |
| `classical_translation` | `classical_translation` | Fetch and parse one pinned reference translation, stored once per installation |

Jobs are idempotent: each records its outcome in ordinary tables, so a retry
after a crash resumes from persisted state rather than repeating finished work.

## Data model

The core entities and their relationships. Every table except the shared
reference data (`normalized_corpus_artifacts`, `shared_lemmas`,
`supported_languages`, `enrichment_cache`, `classical_sources`,
`reference_translations`, and the `works`/`editions`/`reading_units` manifest
registry) is owned by a user.

```mermaid
erDiagram
    users ||--o{ opds_connections : configures
    opds_connections ||--o{ catalogue_entries : indexes
    users ||--o{ books : owns
    books ||--o{ source_materials : "acquired as"
    source_materials ||--o{ source_content_revisions : "has revisions"
    source_materials ||--o{ source_material_units : "unit snapshots"
    source_materials ||--o{ analysis_runs : "analyzed by"
    analysis_runs ||--o{ analysis_run_attempts : attempts
    analysis_runs ||--o| corpora : produces
    corpora ||--o{ corpus_sentences : contains
    corpus_sentences ||--o{ corpus_tokens : contains
    books ||--o| book_current_analyses : "current analysis"
    book_current_analyses }o--|| analysis_runs : "points to"
    books ||--o| book_dispositions : "To Read"
    books ||--o{ reading_history : completions
    books ||--o{ occurrence_lemma_corrections : "learner corrections"
    users ||--o{ study_progress : "studies"
    study_progress ||--o{ study_review_log : "graded answers"
    users ||--o{ study_material : "prepared for"
    works ||--o{ editions : contains
    editions ||--o{ reading_units : contains
    books ||--o| book_reading_units : "one Reading unit"
    reading_units ||--o{ book_reading_units : "held by"
    books ||--o| classical_prepared_definitions : "prepared as"
    reading_units ||--o{ classical_prepared_definitions : "defined by"
    classical_prepared_definitions }o--|| classical_sources : "imported from"
    books ||--o| book_preparation_failures : "last failure"
```

Notes on naming: `source_materials` is an acquired EPUB, as distinct from the
catalog-level `books` entry. The Known vocabulary, current-reading snapshot,
reservation and deck tables stay in the schema with no learner-facing use; a
separate, later migration drops them after the Known-to-Study backfill has been
checked ([ADR 0136](doc/adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md)). Vocabulary identity throughout is
`(language, canonical_lemma, upos)`.

The `works`, `editions` and `reading_units` tables are the Classical catalog
registry: the embedded manifest reconciles into them at startup, and a Book's
`book_reading_units` association is immutable. `catalogue_entries` is the
per-connection Catalog index: a local mirror of the entries a connection offers
within the catalog sync scope, holding title, author, language, cover
provenance and the EPUB acquisition link but no learner state
([ADR 0117](doc/adr/0117-one-catalog-my-books-holds-chosen-books.md)).
`classical_sources` keeps one
verified copy of each pinned work file, `classical_prepared_definitions`
records the definition revision and source pin a Book's published analysis came
from, `book_preparation_failures` records one settled Book preparation failure
per Book for either kind, and
`reference_translations` holds one installation-wide English translation per
pinned citation ([ADR 0116](doc/adr/0116-classical-catalog.md),
[ADR 0121](doc/adr/0121-book-preparation-status-vocabulary-and-durable-epub-acquisition.md)).

The schema starts from a single baseline migration with numbered successors
([ADR 0070](doc/adr/0070-migration-and-documentation-reboot.md)); schema
changes follow [ADR 0038](doc/adr/0038-schema-change-governance.md).
Queries are hand-written SQL through `pgx`, with
[sqlc](https://sqlc.dev/)-generated code in [`gen/sqlc`](gen/sqlc).

## Code map

```mermaid
flowchart TB
    subgraph entry[Entry points]
        server[cmd/server]
        fixture[cmd/fixtureserver]
        tools[cmd/classicalmanifest, cmd/glossreview, cmd/greekhitrate]
    end
    subgraph web[Web]
        webapp[webapp]
        readingintent[readingintent]
        webauth[webauth]
        auth[auth]
    end
    subgraph jobs[Durable work]
        analysis[analysis]
        cataloguesync[cataloguesync]
        study[study]
        sourcecheck[sourcecheck]
    end
    subgraph lang[Language and text]
        epub[epub]
        analyzer[analyzer<br/>gRPC client]
        canonicalization[canonicalization]
        selection[selection]
        lexical[lexical]
        gdex[gdex]
        sentencequality[sentencequality]
        dictionary[dictionary]
        enrichment[enrichment]
    end
    subgraph data[Data]
        domain[domain]
        persistence[persistence]
        classical[classical<br/>catalog manifest]
        opds[opds]
    end
    server --> webapp & analysis & cataloguesync & study & sourcecheck
    fixture --> webapp
    webapp --> webauth --> auth
    webapp --> persistence
    webapp --> readingintent
    readingintent --> analysis & cataloguesync & classical & domain
    readingintent --> opds & epub
    analysis --> epub & analyzer & selection & persistence
    sourcecheck --> epub & persistence
    cataloguesync --> opds --> epub
    webapp --> study
    study --> enrichment & sentencequality & persistence
    sentencequality --> gdex
    selection --> lexical
    analyzer --> canonicalization
    persistence --> domain
    persistence --> classical
    analysis --> classical
    webapp --> classical
```

| Package | Responsibility |
| --- | --- |
| [`domain`](internal/domain) | Core corpus, book, and vocabulary models |
| [`persistence`](internal/persistence) | PostgreSQL repositories with explicit ownership boundaries |
| [`classical`](internal/classical) | Embedded Classical catalog manifest: Reading-unit definitions, pinned source layers, and translation pins ([ADR 0116](doc/adr/0116-classical-catalog.md)) |
| [`epub`](internal/epub) | Reading-order extraction with reproducible source locations; main-text selection |
| [`analyzer`](internal/analyzer) | The project-owned analysis boundary and its gRPC client |
| [`analysis`](internal/analysis) | Durable analysis and Book preparation jobs: chunking, persistence, promotion, and Classical and EPUB preparation |
| [`readingintent`](internal/readingintent) | A Book's reading intent: the To Read transition, the Classical-or-EPUB route, and the submission log line ([ADR 0049](doc/adr/0049-reading-intent-triggers-analysis.md)) |
| [`sourcecheck`](internal/sourcecheck) | Extraction-only checks of existing EPUB snapshots against the configured contract |
| [`canonicalization`](internal/canonicalization) | Language-specific canonical lemma profiles |
| [`lexical`](internal/lexical), [`selection`](internal/selection) | Eligibility rules and deterministic vocabulary candidates |
| [`analysisinsights`](internal/analysisinsights) | Coverage and threshold metrics for a learner and a book |
| [`gdex`](internal/gdex) | GDEX-informed example-sentence scoring |
| [`sentencequality`](internal/sentencequality) | The sentence-quality gate and representative-sentence selection for Study material |
| [`dictionary`](internal/dictionary) | Read-only access to the Wiktionary-derived index |
| [`enrichment`](internal/enrichment) | Dictionary and LLM enrichment, caching, and the external-provider privacy boundary |
| [`study`](internal/study) | Study: FSRS scheduling, New-word batches, Study material and Fluency preparation, Study reports, Idioms |
| [`lemmareview`](internal/lemmareview) | The preview-and-confirm step for a Lemma correction or Occurrence exclusion, used by the remediation screen |
| [`opds`](internal/opds), [`cataloguesync`](internal/cataloguesync), [`bookcover`](internal/bookcover) | OPDS client, metadata sync, and cover retrieval |
| [`auth`](internal/auth), [`webauth`](internal/webauth) | Local accounts, Argon2id passwords, sessions |
| [`webapp`](internal/webapp) | Handlers, templ views, styles, and static assets |
| [`fixtures`](internal/fixtures) | In-memory data for the browser acceptance suite |

The Python side is small: [`producer.py`](nlp/src/mouseion_nlp/producer.py)
maps Stanza output to the Protobuf contract and applies the producer-side
normalization, [`server.py`](nlp/src/mouseion_nlp/server.py) serves gRPC,
[`model_config.py`](nlp/src/mouseion_nlp/model_config.py) selects models per
language, and [`provision.py`](nlp/src/mouseion_nlp/provision.py) downloads
them.

## Testing

| Layer | How it is tested |
| --- | --- |
| Domain and language logic | Go unit tests, including a shared parity fixture that keeps German normalization identical in the Go runtime and the Python dictionary-index builder |
| Persistence and jobs | Integration tests against real PostgreSQL via Testcontainers; each test gets its own database cloned from a migrated template |
| NLP producer | pytest with fake pipelines, plus a reproducible precision measurement against real Stanza models ([evidence](doc/evidence/german-separable-verb-precision.md)) |
| Interface | Playwright against [`cmd/fixtureserver`](cmd/fixtureserver), an in-memory server with no database or NLP, in Chromium and WebKit, desktop and compact, light and dark ([ADR 0033](doc/adr/0033-browser-acceptance-harness.md)) |

See the [development guide](doc/development.md) for commands.
