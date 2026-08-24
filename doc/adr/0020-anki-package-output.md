# ADR 0020: Anki package output and Mouseion deck hierarchy

Status: **Accepted** · Date: 2026-08-24 · Author: Justin + Hermes

## Context

Mouseion currently exports Anki-compatible notes as a TSV artifact. TSV is
portable but requires manual note-type and deck configuration, and it does not
carry the deck hierarchy or note-model metadata needed for a seamless import.
The generated deck should be immediately useful in Anki while retaining
owner/book provenance and stable duplicate behavior.

The detailed note fields, card templates, and styling should be informed by an
audit of Justin's existing effective Anki deck rather than guessed. That audit
is tracked separately in issue #132.

## Decision

The primary generated artifact will be an Anki deck package (`.apkg`). Each
package will contain a deck named exactly:

```text
Mouseion::<language code>::<book title>
```

The download filename will be derived from the book title, safely sanitized
for the target filesystem, and end in `.apkg`. Empty or unsafe titles must use
a deterministic fallback rather than exposing a path or producing an empty
filename.

The note model and card templates will be stable across exports. Their final
field order and presentation will be captured in the card contract produced by
issue #132, then implemented by issue #134. The package must preserve the
existing semantic content: cloze target, lemma, translation when available,
morphology, source context, and provenance.

TSV may remain temporarily as an explicitly labeled compatibility artifact if
it helps existing users migrate, but it is not the primary product flow.

## Consequences

- Anki users can import a generated package without manually creating the
  Mouseion/language/book deck hierarchy.
- Book titles become part of the user-visible Anki deck name and filename, so
  sanitization and deterministic naming are required.
- Anki's SQLite/ZIP package format becomes a compatibility surface that needs
  import tests and a stable note-model contract.
- Existing generated-vocabulary and card identity rules remain owner-scoped;
  changing the container format must not change exclusion semantics.
- The existing TSV renderer can be retained during migration but should not
  cause two divergent card-content implementations.

## Non-goals

- This ADR does not define sentence-quality scoring; that is issue #135.
- This ADR does not automatically mark exported words as known.
- This ADR does not choose a final third-party package library until the
  implementation evaluates compatibility and maintenance tradeoffs.
