# ADR 0021: Contextual sentence translation cache and privacy

Status: **Accepted** · Date: 2026-08-24 · Author: Justin + Hermes

## Context

Mouseion's enrichment layer currently returns a concise lemma translation and
a gloss. The selected source sentence is supplied to external providers only
to disambiguate the lemma; a translated example sentence is not currently
returned or cached.

The same lemma can appear in multiple contexts. A cache keyed only by language,
lemma, and UPOS would incorrectly reuse the first contextual translation for
later sentences. External enrichment also has a strict privacy boundary: a
provider may receive the approved sentence and vocabulary identity, but not
owner IDs, book titles, or other private metadata.

## Decision

Treat contextual sentence translation as a distinct enrichment field from:

- lemma translation; and
- concise gloss/sense explanation.

Contextual translations must be cached using a deterministic identity that
includes the source sentence (or a cryptographic hash of its normalized text),
language, lemma, UPOS, provider, and provider version. Different sentences for
the same lemma must never share a cache entry.

The provider request may contain only:

- source language;
- canonical lemma;
- UPOS; and
- the selected source sentence.

It must not contain owner, book, source-document title, or other private
metadata. Provider/version changes invalidate or bypass prior translations.

Sentence translation is performed only after source-sentence quality filtering
has selected an acceptable example. It is asynchronous through River where
possible. An export may proceed with an empty `EnglishSentence` field when the
translation is unavailable, but the artifact must report its completeness
accurately.

## Consequences

- Contextual translation becomes reusable across exports without context
  collisions.
- Translation cost is controlled through owner-independent content caching and
  provider/version identity.
- The existing eight-field Anki contract remains stable; `EnglishSentence` is
  populated opportunistically.
- Missing translation is distinct from poor source evidence. A quality-rejected
  sentence is omitted; a quality-approved untranslated sentence can remain in
  the deck according to export policy.
- Cache schema changes may require a migration and therefore human review.
