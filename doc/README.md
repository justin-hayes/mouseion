# Mouseion Documentation

## Start here

1. [README](../README.md) — what Mouseion is and why it exists.
2. [Architecture](../ARCHITECTURE.md) — processes, the analysis and deck
   pipelines, background jobs, the data model, and a code map, with diagrams.
3. [Linguistic design notes](linguistics.md) — how the text is selected and
   annotated, what counts as a word, how coverage, vocabulary, and example
   sentences are chosen, and how the results are evaluated.
4. [Product summary](product.md) — the current pipeline, stack, and the index of
   every architecture decision.
5. [Domain glossary](../CONTEXT.md) — the vocabulary used consistently in code,
   interface, and documentation (Book, Analysis, Lemma, Known, Reserved,
   Concordance, and so on).

## Decisions worth reading first

Mouseion records its consequential decisions as
[Architecture Decision Records](adr/). They are written with context, the
alternatives considered, and consequences, and later decisions amend earlier
ones rather than rewriting them. A short path through the most representative:

**Text and language processing**

- [ADR 0066](adr/0066-main-text-selection-from-epub-structure.md) — identify a
  book's main text from declared EPUB structure, with a fail-safe fallback.
- [ADR 0059](adr/0059-persisted-normalized-corpus-for-concordance.md) and
  [ADR 0060](adr/0060-persist-dependency-parses.md) — persist the normalized
  token stream and dependency parses so the corpus can be queried without
  re-running NLP.
- [ADR 0061](adr/0061-german-separable-verb-lemmatization.md) — reattach German
  separable-verb particles so vocabulary identity is the full lexeme.
- [ADR 0065](adr/0065-german-pre-1996-sharp-s-canonicalization.md) — canonicalize
  documented pre-1996 German ß spellings without collapsing distinct lexemes.
- [ADR 0073](adr/0073-modern-greek-language-support.md) — Modern Greek with
  per-language model selection and Greek canonicalization.
- [ADR 0062](adr/0062-derived-sentence-quality-scoring.md) — GDEX-informed
  example-sentence scoring over the persisted corpus.
- [ADR 0081](adr/0081-learner-owned-occurrence-lemma-corrections.md) — learner
  corrections layered over immutable analyzer output.

**System architecture**

- [ADR 0001](adr/0001-go-core-python-nlp-service.md) and
  [ADR 0011](adr/0011-grpc-go-python-transport.md) — a Go core with Python NLP
  behind a typed gRPC boundary.
- [ADR 0010](adr/0010-river-job-queue.md) — durable background work in
  PostgreSQL with River instead of a separate broker.
- [ADR 0023](adr/0023-nlp-capabilities.md) — the NLP service is authoritative
  for which languages and features exist.
- [ADR 0071](adr/0071-decouple-deck-data-from-presentation.md) — a frozen deck
  manifest that can be re-rendered without re-analysis.
- [ADR 0070](adr/0070-migration-and-documentation-reboot.md) — consolidating
  migration history and documentation into a present-state baseline.

## Guides

- [Operations guide](operations.md) — running, configuring, and maintaining an
  installation.
- [Development guide](development.md) — toolchain, tests, generated code, and
  conventions.
- [Browser acceptance suite](../e2e/README.md) — the Playwright harness.

## Reference

| Directory | Contents |
| --- | --- |
| [`adr/`](adr/) | Architecture decision records, indexed in [`product.md`](product.md#architecture-decision-records). |
| [`features/`](features/) | Feature specifications: behavior, motivation, and scope. |
| [`design/`](design/README.md) | Design principles, design system, information architecture, and workflows. |
| [`research/`](research/) | Source surveys, for example [Modern Greek NLP options](research/modern-greek-nlp.md). |
| [`evidence/`](evidence/) | Reproducible measurements, for example [German separable-verb precision](evidence/german-separable-verb-precision.md). |
| [`reviews/`](reviews/) | Quality reviews of model-assisted features. |
| [`agents/`](agents/) | Conventions for the coding agents that implement changes; see the [autonomous change policy](autonomous-change-policy.md). |
| [`archive/`](archive/) | Retired feature documents and dated operational records. |

[Documentation governance](documentation-governance.md) explains where each kind
of document lives and which source is authoritative.
