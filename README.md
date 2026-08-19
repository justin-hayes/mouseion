# mouseion

A self-hosted **reading environment** for learning foreign languages at an advanced reading level. It turns books you actually want to read — browsed from your own Calibre-Web/OPDS library — into vocabulary you can study, with an Anki deck as the output and, eventually, in-text reading aids.

> **Working title.** "mouseion" is the project name for the reading app discussed in Obsidian (see the 2026-08-17 braindump and the Vocabulary Acquisition Tool sessions). The repo is currently empty; these docs capture the agreed direction.

## What it does

- Browse an OPDS library (self-hosted Calibre-Web) and pick a book for analysis
- Ingest EPUBs and analyze them linguistically in the background
- Rank the text's vocabulary by frequency, each word with a representative example sentence from the book
- Let you curate the result (mark words as known, choose the best example sentence)
- Export the curated list as an **Anki-compatible deck**
- (Roadmap) reading progress sync with KOReader, corpus-wide concordance, and an in-text reading environment with statistics and reading aids

## Design principles

- **Reading first.** The focus is learning to read at an advanced level, not gamified vocabulary drilling.
- **Go core.** As much of the application as possible is implemented in Go, with calls out to a Python service only for NLP.
- **Corpus-agnostic.** The pipeline works on any EPUB; the linguistic and vocabulary layers are separate from any single text.

## Project status

**Pre-alpha / empty.** Documentation-only so far. See [`doc/product.md`](doc/product.md) for the product specification and [`doc/adr/`](doc/adr/) for architecture decision records.

## Repository layout

```
doc/
  product.md        # Product specification
  adr/              # Architecture decision records
```

## Relationship to other projects

- **schwab-edition** (github.com/justin-hayes/schwab-edition) is a separate scholarly digital edition of Gustav Schwab, built on eXist-db and TEI. It serves the same audience but is its own infrastructure, not a mouseion corpus — at least for now. See the Open Questions in the product spec.

## Getting started

To be added once there is a runnable CLI or server.

## License

To be decided.
