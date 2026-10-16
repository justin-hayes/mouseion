# ADR 0099: Ancient Greek through imported analyses in the existing pipeline

Status: **Accepted** · Date: 2026-10-16 · Author: Justin + opencode

Tracked by [#2103](https://github.com/justin-hayes/mouseion/issues/2103), under
the Ancient Greek MVP spec [#2098](https://github.com/justin-hayes/mouseion/issues/2098)
(the "no treebank data" restart). Implements the import seam and the
syntax-storage decision for "Book 1 is prepared".

## Context

Mouseion gets its analyses from the NLP service, but there is no acceptable
Ancient Greek model: the available models are trained on non-commercial
treebanks. The MVP therefore imports a pre-annotated text of Xenophon's
*Anabasis* Book 1 from a pinned file and makes it an ordinary analysis of an
ordinary Book: My Books shows it prepared, Browse lists its vocabulary,
Reading works, and cards can be prepared, all with the NLP service stopped.

The pinned source is the single-file CoNLL-U packaging of `oga_src` v0.2.0
(Zenodo DOI 10.5281/zenodo.15199546), which is a packaging of OGA v0.2.0. Its
size and SHA-256 are reviewed commitments. It carries Greek text, so logs and
operator output must never contain source text. Several decisions have to be
settled before writing the import (spec "Implementation Decisions").

## Decision

**One import seam, one adapter.** The import is a bounded, checked read → a
format adapter for the `oga_src` CoNLL-U dialect → one source-neutral candidate
→ validation → a writer that reuses the analysis pipeline's persistence. A later
source format adds only an adapter. The import never calls the NLP service and
never enqueues an `analyze_corpus` job.

**The file read is bounded and pinned.** The reader loads at most the pinned size
plus one byte; a larger file is `source_oversize`, a different size or digest is
`source_pin_mismatch`. A malformed line, an unexpected comment, a missing
citation reference, an unknown native part of speech or a sentence that straddles
the citation boundary stops the import with a stable code before anything is
written.

**The interpretation is versioned and recorded on the run.** `oga-agdt-upos/4`
maps the native AGDT part of speech to UPOS, with `c` split by `AuxC` and εἰμί as
VERB. `oga-agdt-feats/1` maps the AGDT `postag` positions to UD FEATS; a value
with no UD counterpart is omitted and counted. `ancient-greek/1` derives the
canonical lemma from the citation headword; a trailing source number becomes the
`morpheus:N` discriminator and the headword loses it. The proper-name rule is
computed per lemma over the whole pinned work. The run records the versioned
interpretation and the syntax decision.

**Syntax is recorded as absent, not stored as UD.** The existing `corpus_tokens`
`dependency`/`head` fields are read as UD (eligibility reads `dependency`; sentence
study reads both), and a native AGDT head cannot be stored faithfully in a
0-based kept-token index after ellipsis tokens are dropped. An imported run
therefore carries `analysis_runs.syntax_state = 'absent'` and a non-committal
`dependency = 'dep'`, `head = 0` skeleton that satisfies the token checks.
Greek consumers show "Syntax unavailable" from the run's syntax state, never the
stored skeleton. Native AGDT syntax is not stored and not shown.

**The identity travels with the data, additively.** `corpus_tokens` gains a
`lexical_discriminator` (empty for existing rows) and a citation `headword`;
`vocabulary_browse_counts` gains the discriminator in its key. The migration
rebuilds keys without changing any existing value. So homographs that share a
spelling and part of speech (δέω 1 and δέω 2) stay distinct throughout, and
proper names, though stored, are excluded from vocabulary by the existing
eligible part-of-speech set.

**The writer is the existing pipeline.** It stores the source material, the
analysis run, the corpus, its sentences and tokens through the same persistence
the analysis job uses, and publishes through the same publication step, which
builds the Browse projection in the same transaction. A repeat with the same pin
and interpretation version finds the completed run and is a no-op; a new
interpretation version is a new run.

**To Read changes only the disposition.** A prepared imported Book already
carries its analysis, so To Read never acquires a source and never submits EPUB
analysis.

## Consequences

- Every migration is additive; modern-language analyses, publication and
  outcomes are byte-for-byte unchanged (complete-loop characterization).
- The import is the only path that writes `grc` rows; registration still refuses
  when legacy `grc` rows exist.
- Native AGDT syntax is deferred. A later feature that needs it can add a
  `native_agdt` syntax state and fields that are never read as UD, without
  changing this run's state.
- Adding *Anabasis* Book 2, another work or another source format is a new
  definition or adapter, not new pipeline code.
- Migrations and this ADR are human-reviewed under CODEOWNERS.

## Related

- [ADR 0091: Go package layering](0091-go-package-layering.md) — the import seam
  stays in `internal/ancientgreek`; the writer lives in the `internal/analysis`
  job package, which owns pgx and analysis persistence.
- [ADR 0087: Vocabulary counts come only from selection](0087-vocabulary-counts-come-only-from-selection.md)
  — the Browse projection runs `selection.Project`; the discriminator is part of
  the projected identity.
