# ADR 0017: Replace frequency-based ranking with coverage-based selection

Status: **Accepted** · Date: 2026-08-23 · Author: Justin + Hermes

Amends **ADR 0005** (Vocabulary identity, normalization, and initial ranking defaults).

## Context

ADR 0005 established frequency-based ranking (in-text frequency, corpus
frequency, priority, cross-text) to decide which unknown lemmas belong in an
Anki deck. Milestone 4 reframes the goal: the learner wants to read a book
with minimal interruption, so the deck should contain the smallest set of
unknown words that achieves high text coverage — not a frequency-sorted
long list.

## Decision

Replace the ranking/weight system with a **coverage-based selection algorithm**:

1. Tokenize + lemmatize the book.
2. Remove proper names, punctuation, stop words.
3. Compare lemmas against the learner's known-word list.
4. Count unknown-lemma frequency.
5. Pick the minimal unknown-lemma set needed for **≥ 97%** text coverage.
6. Export the deck with words ordered by first encounter in the text.

DWDS/corpus-frequency ranking is discarded entirely.

## Rationale

- Simpler UX: one "Generate deck" button instead of filter/ranking/review flow.
- Coverage directly maps to reading fluency — the learner only studies words
  that matter for this book.
- Removes the DWDS dependency and corpus-frequency plumbing.

## Consequences

- The per-book review decision flow (include / known / ignore) is removed.
- The ranking columns (`ranking_global_pct`, `ranking_corpus_pct`,
  `ranking_priority`, `ranking_cross_text`, `ranking_score`) become unused
  in the deck path. They are retained in storage for now to avoid a
  destructive migration; remove in a later cleanup if coverage selection
  proves stable.
- The deck export config page (`/deck?book=`) with filter-known + ranking
  options is replaced by a single "Generate deck" button on the book detail
  page.
- **ADR 0019** amends this selection policy: explicit known vocabulary and
  owner-scoped vocabulary generated for other books are removed before the
  fixed 97% denominator is calculated.
