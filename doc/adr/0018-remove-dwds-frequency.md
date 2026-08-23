# ADR 0018: Remove global frequency dataset (DWDS) import

Status: **Accepted** · Date: 2026-08-23 · Author: Justin + Hermes

Amends **ADR 0008** (Global frequency dataset source and import contract).

## Context

ADR 0008 established DWDS as the global frequency dataset source, with an
import contract for loading and caching word frequencies. Milestone 4 removes
corpus-frequency ranking and the DWDS upload flow entirely; the coverage-based
algorithm does not need global frequency data.

## Decision

Remove the DWDS word-list upload, corpus-frequency ranking, and associated
import plumbing from the product.

## Rationale

- Coverage-based selection needs only the learner's known-word list and the
  book's own lemma frequency — no external corpus reference.
- Simplifies the backend and removes a dependency on DWDS data availability.
- Aligns with the Milestone 4 goal of a single-button deck flow.

## Consequences

- `cardexport` and related surfaces no longer expose frequency-ranking options.
- Any persisted DWDS/frequency data is orphaned. Retain in storage for now;
  remove in a later cleanup if coverage selection proves stable.
- The `word_frequencies` or equivalent frequency tables may become unused;
  consider removal in a follow-up migration.
