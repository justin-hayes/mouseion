# ADR 0008: Global frequency dataset source and import contract

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

The product's **Global Reference Data** (ADR 0002 §3, admin-managed and language-scoped) needs an initial language-frequency signal. ADR 0005's ranking uses `pct(global_freq)` — a word's percentile rank in the global frequency dataset — and candidate selection uses a top-5% global-frequency cutoff. Issue #30 asks for the **precise source dataset** (with license, version, provenance) and a **reproducible administrative import contract** (format, columns, units, normalization, dedup, refresh/rollback, and how membership vs. numeric rank feeds ranking).

The author's stated v1 target is the **DWDS German corpus frequency data**, consistent with German-first and with the vocabulary-identity model.

## Decision

### 1. Source — the DWDS Lemmadatenbank, imported as a versioned bulk snapshot

- **Dataset:** the **DWDS Lemmadatenbank** (digital dictionary of German lemma inventory with per-lemma frequency class), downloaded as the **CSV snapshot** at `https://www.dwds.de/lemma/csv` (JSON also available at `/lemma/json`).
- **Provenance/versioning:** the download carries an explicit snapshot timestamp (`Stand`, e.g. `2026-08-21 13:00:46 CEST`). This timestamp is the **dataset version identifier**; it is recorded with the import and used for provenance and refresh. The admin imports a snapshot file; the system never mutates it in place.
- **License — verified:** the DWDS Lemmadatenbank is **CC BY-SA 4.0** (Attribution — ShareAlike), author "Digitales Wörterbuch der deutschen Sprache (DWDS)", as stated on the `dwds.de/lemma/list` page. This matches the CC BY-SA 4.0 license already used in the author's related scholarly work.
  - **Consequence:** derived/redistributed forms of the dataset must remain **CC BY-SA 4.0**. This is compatible with a self-hosted personal tool; the admin upload should record the license and attribution so the dataset is never accidentally re-licensed or made proprietary.
- **Not used for v1:** the live Frequenzbarometer **API** (`dwds.de/api/frequency`, per-word query, log-scale 0–6) — it is not a bulk download and is not versioned. The **Goethe-Zertifikat word lists** are **copyright-protected (Goethe-Institut)** and are excluded. The **Leipzig Corpora frequency dictionaries** (CC-BY 3.0) remain a possible future alternative for other languages.

### 2. Import contract — a normalized, versioned frequency snapshot

- **Formats:** accept the DWDS CSV/JSON snapshot directly, plus a normalized internal representation. v1 default: ingest the DWDS CSV.
- **Normalized columns** (the internal table):
  `lemma<TAB>upos<TAB>haeufigkeitsklasse<TAB>global_freq_percentile<TAB>source_version`
  - `lemma` — the DWDS lemma (canonical German form).
  - `upos` — mapped from DWDS `wortklasse` (Substantiv→NOUN, Verb→VERB, …); unknown/unmapped classes are carried as-is or flagged.
  - `haeufigkeitsklasse` — the raw DWDS frequency class (0=selten … 6=häufig), the **stored raw signal**.
  - `global_freq_percentile` — a **derived** value in [0,1] computed at import (see §3).
  - `source_version` — the snapshot `Stand` timestamp; enables provenance and multi-version history.
- **Compression:** `.tsv.gz` supported (the dataset is large).
- **Encoding:** UTF-8.

### 3. Class → percentile derivation at import

DWDS provides **Häufigkeitsklasse** (a logarithmic 7-step scale), **not** a percentile or per-million rank. ADR 0005's ranking expects `pct(global_freq)` in [0,1]. Therefore:

- Store the **raw `haeufigkeitsklasse`** as the authoritative value.
- **Derive `global_freq_percentile` at import** using a deterministic formula that approximates a percentile from the frequency class (relative to the class distribution / corpus size, per the approach DWDS uses for Worthäufigkeit). The exact conversion is implemented and unit-tested in the import adapter.
- The ranking layer (ADR 0005) reads `global_freq_percentile` directly, keeping it **agnostic of the DWDS scale**.

### 4. Membership vs. numeric value — expose both

- **Numeric:** `global_freq_percentile` feeds the ranking formula (ADR 0005 `pct(global_freq)`).
- **Membership:** the imported table supports a **membership lookup** for candidate **selection** — "is this lemma above the top-5% global cutoff?" (ADR 0005 selection rule). Implemented as a derived flag or threshold comparison on `global_freq_percentile`.

### 5. Validation, dedup, refresh, rollback

- **Normalization:** imported lemmas are run through the ADR 0005 normalization profile so they match corpus-analysis candidates. Invalid/unparseable rows are **reported, not silently dropped**.
- **Duplicates:** resolved deterministically (e.g. by lemma+upos); homographs that DWDS does not separate are handled per ADR 0005's sense-agnostic identity.
- **Refresh:** importing a **newer snapshot** supersedes the current one as a **new version** — never an in-place mutation. Prior versions remain available for provenance.
- **Rollback:** the system can roll back to a prior imported version.

## Alternatives considered

- **Live Frequenzbarometer API.** Rejected: per-word, not bulk, not versioned; unsuitable as an admin-loaded reference dataset.
- **Goethe-Zertifikat word lists.** Rejected: **copyright-protected** (Goethe-Institut), cannot be freely redistributed.
- **Leipzig Corpora frequency dictionaries (CC-BY 3.0).** Deferred: possible future alternative for other languages; DWDS is the chosen German v1 source and aligns with the author's stack.
- **Import precomputed ranks/percentiles (no derivation).** Rejected: DWDS only provides Häufigkeitsklasse; deriving a clean percentile at import keeps the ranking layer scale-agnostic.

## Consequences

- The admin frequency-ingestion path (issue #12) ingests DWDS Lemmadatenbank snapshots into a versioned, language-scoped table.
- `pct(global_freq)` in ADR 0005's ranking is satisfied by the derived `global_freq_percentile`.
- Candidate selection's top-5% global cutoff is a membership test on the imported table.
- The dataset is CC BY-SA 4.0; derived forms stay CC BY-SA 4.0, and attribution is recorded.
- Open Question 11 in `product.md` is resolved; #30 can be closed. Issue #12 is updated per the decision.

## Open questions

- Whether a **language-frequency alternative** (e.g. Leipzig CC-BY 3.0 dictionaries) should be added for non-German languages later.
- Whether to **cache the full snapshot** client-side or serve the derived table only (storage/refresh trade-off) once real corpus processing begins.

---

## Related

- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md) — the `pct(global_freq)` ranking input and top-5% selection cutoff this dataset feeds.
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — admin-managed, language-scoped global reference data.
- [Product specification](product.md) — resolves Open Question 11; updates the Decision Register.
