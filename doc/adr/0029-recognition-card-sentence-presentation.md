# ADR 0029: Recognition-card sentence presentation

Status: **Accepted** · Date: 2026-08-27 · Author: Justin + Hermes

## Context

Mouseion's generated cards are recognition cards. The learner should see the
word under test in its complete source sentence, not reconstruct a sentence
from a hidden cloze or a shortened fragment.

The former long-sentence approach asked the external LLM to select a shorter,
contiguous `context_sentence` for the front of a card. Although the complete
source sentence was retained separately, the shortened context could remove
syntax or discourse needed to understand the target sense. It also created a
second learner-facing representation of the same source evidence.

The generated Anki note currently carries both a front sentence and a separate
`SourceSentence` field displayed again on the back. Once the complete sentence
is the front of a recognition card, that duplicate presentation has no value.

## Decision

1. **Use recognition cards, not cloze cards.** The target word remains visible
   and is deterministically bolded on the front. It is never hidden in a cloze
   deletion.
2. **Preserve the complete selected source sentence.** The front contains the
   complete sentence, with the target form bolded. Mouseion does not ask an
   LLM to select a shorter subset or paraphrase the source sentence.
3. **Keep deterministic long-sentence quality handling.** Sentence selection
   may prefer manageable examples and the export quality gate may reject
   extreme examples. Accepted sentences are not shortened or rewritten.
4. **Remove the duplicate German source presentation.** The back does not
   render a separate `SourceSentence` block. The generated Anki note contract
   should expose one source-sentence field, used for the formatted recognition
   front, rather than a second duplicate German sentence field.
5. **Keep the complete English contextual translation.** It is shown on the
   back. When a provider supplies a validated corresponding English target
   phrase, Mouseion may bold that phrase; uncertain alignments fall back to
   unbolded translation text.
6. **Treat alignment as data, not provider markup.** Providers return plain
   translation text and a target phrase (or equivalent structured alignment).
   Mouseion validates the match and owns HTML rendering and escaping.

## Consequences

- Long accepted examples remain authentic and semantically complete.
- The front is immediately recognizable as a recognition prompt: German text
  is visible, with the tested word emphasized.
- The obsolete `context_sentence` provider field, cache value, validation, and
  front-side fallback logic are removed by this implementation.
- Removing `SourceSentence` from the Anki/TSV note contract is a compatibility
  change requiring updated artifact fixtures and import/export tests.
- English target highlighting is best-effort. Idiomatic, distributed, or
  ambiguous correspondences remain unhighlighted rather than receiving a
  misleading match.
- Source provenance remains available through the book/source metadata and
  application persistence; it is not duplicated as a second sentence block on
  the card back.

## Non-goals

- This ADR does not remove deterministic vocabulary coverage selection.
- This ADR does not remove deterministic representative-sentence selection.
- This ADR does not require every English translation to have a word-level
  alignment.
- This ADR does not introduce automatic paraphrasing of source sentences.

## Related

- [ADR 0020: Anki package output and Mouseion deck hierarchy](0020-anki-package-output.md)
- [ADR 0021: Contextual sentence translation cache and privacy](0021-contextual-translation-cache.md)
- [ADR 0026: Explainable structural text profile](0026-structural-text-profile.md)
