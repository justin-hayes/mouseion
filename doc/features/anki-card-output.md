# Anki card output milestone

Status: **Accepted and rolled out** · Date: 2026-08-27

## Problem

The generated Anki cards currently expose two small classes of analyzer/output
quality defects:

- punctuation can become part of the extracted word (`‹die`, `Besten›`);
- Stanza can emit a pipe-separated lemma such as `geleiten|leiten`.

The current note is also shaped around a production-oriented Cloze workflow.
The intended product is now a recognition-oriented card with a compact,
readable front and back. Long but otherwise useful source sentences should not
be discarded solely because the complete sentence is too long to scan.

## Goals

- Ensure extracted word forms do not include surrounding punctuation while
  preserving the original source sentence exactly.
- Establish deterministic handling for pipe-separated analyzer lemmas.
- Replace the user-facing morphology JSON/card field with a compact noun article
  display where a definite article is present directly before the tested noun.
- Change the primary Anki note model from Cloze production to recognition:
  context with the tested word bolded on the front; lemma, concise English
  definition, and English translation of the sentence on the back.
- Retain useful vocabulary from long sentences by asking the translation/enrichment
  LLM for a shorter, sufficient context clause, with a safe deterministic
  fallback.
- Preserve stable identity, provenance, escaping, deterministic exports, and
  owner-scoped generated-vocabulary behavior.

## Non-goals

- Do not infer a noun's article from morphology or an external grammar source in
  this milestone. The initial rule is limited to an article actually present in
  the source immediately before the noun.
- Do not make the LLM responsible for lexical eligibility, target matching, or
  HTML safety.
- Do not silently change historical cards or rewrite existing generated-vocabulary
  provenance. New exports use the new contract; migration/compatibility behavior
  must be explicit.
- Do not add a second divergent TSV card-content implementation.

## Proposed card contract

The note model should contain these display fields, in this order:

1. `Front` — source context with the tested surface form rendered in bold;
2. `Lemma` — canonical lemma, with a directly preceding German definite article
   when one is present (for example, `das Buch`);
3. `English` — concise English definition/translation of the lemma;
4. `EnglishSentence` — English translation of the complete source sentence;
5. `BookTitle` — source/book provenance;
6. `SourceSentence` — complete original source sentence.

`POS` and `Morph` should no longer be user-facing card fields. POS may remain a
stable tag and internal selection attribute if needed for identity and filtering.
The exact Anki model name/template should be versioned as a deliberate contract;
the front should be a normal field template, not a Cloze template.

The front must HTML-escape all source text first and then add the bold wrapper
only around the matched target span. Matching must be Unicode-aware and must
not allow a target to match inside a larger word.

## Proposed normalization rules

### Punctuation-bearing surfaces

At the analyzer boundary, derive a lexical surface form by removing only
leading/trailing Unicode punctuation and symbol characters. Preserve internal
punctuation (for example apostrophes or hyphens) and preserve the original
sentence text and token offsets for display/provenance. Use the cleaned surface
for target matching and extraction. Add regression fixtures for `‹die` and
`Besten›`, plus quotes, parentheses, em dashes, apostrophes, and hyphenated
forms.

This should be implemented once in the shared normalization path rather than
as an export-only workaround, so selection and card rendering agree about the
word under test.

### Pipe-separated lemmas

Treat a pipe as an analyzer alternative separator, not as literal lemma text.
The proposed first-pass policy is:

- preserve the exact analyzer value in `RawLemma` for diagnostics;
- split on `|`, trim each alternative, discard empty alternatives;
- use the first non-empty alternative as the canonical/display lemma;
- if alternatives differ, attach a non-fatal normalization warning/metric;
- reject the token only when no usable alternative remains.

Before implementation, validate the first-alternative rule against a small set of
real Stanza outputs. If Stanza's ordering is not a preference ordering, retain
all alternatives in an internal diagnostic field and define a later lexical
selection policy rather than presenting a pipe in a learner-facing card.

## Article handling

For German NOUN entries, inspect the source token immediately preceding the
selected surface form. If it is a definite article (`der`, `die`, or `das`,
case-insensitive), prefix that article to the displayed lemma. Do not include
an article that is merely elsewhere in the sentence, and do not infer one from
morphology in this milestone. The source sentence remains unchanged.

The implementation should carry article information as explicit enrichment/card
input rather than overloading the canonical lemma identity. This keeps `Buch`
and `das Buch` as the same vocabulary item.

## Final long-context contract

A source sentence is long when either of these limits is exceeded after
trimming outer whitespace:

- more than **50 whitespace-delimited words**; or
- more than **400 Unicode code points**.

The limits are an export contract shared by enrichment validation and card
rendering. A sentence at or below both limits never uses a provider-supplied
short context. Shortening is a controlled fallback rather than an LLM rewrite
of every card:

1. Keep the complete source sentence as the canonical context and translation
   input/output.
2. For a sentence exceeding the limits, ask the translation provider for a
   shorter contiguous clause/span that contains the tested word and is
   sufficient to identify its sense.
3. Require structured output containing `context_sentence`; the returned
   context must be an exact substring (or a precisely
   validated token span) of the original sentence, contain the target, and pass
   the same escaping/boundary checks as ordinary context.
4. Use the shorter context only on the front. Show the complete sentence and its
   English translation on the back.
5. If the provider is unavailable, returns malformed output, omits the target,
   proposes an inadequate fragment, or returns a span for a short source,
   discard the proposed context. The complete source is then subject to the
   ordinary deterministic quality gate: an otherwise usable short source can
   export without English enrichment, while a long source without a validated
   short span is omitted and contributes to `QualityOmitted`.

This preserves privacy and reproducibility better than asking the LLM to
paraphrase. It also means the cache key and prompt version must include the
context-selection operation and its version. The full sentence translation
should remain tied to the complete source sentence, not the shortened clause.
The current operation/version is `translation-v4-long-context-50w-400c`; changing
it bypasses older cached provider results. The cache stores the validated
context with the provider, provider version, and complete-sentence hash. A
confidence or rationale is not persisted in this rollout.

The quality gate therefore distinguishes a successfully shortened long source
from a genuine quality omission. A validated short context makes an otherwise
eligible long source exportable, but appears only in `Front`;
`SourceSentence` and `EnglishSentence` retain the complete original sentence
and its translation.

## Delivery slices

1. **Normalization fixtures** — complete: punctuation-trimmed surfaces, pipe
   lemmas, Unicode boundaries, raw-value preservation, and selection/card
   regressions.
2. **Lean note/model contract** — complete: the recognition `Front` and
   article-aware lemma display replace the user-facing morphology fields.
3. **Long-context enrichment** — complete: versioned structured provider
   response, cache-key update, exact-span validation, deterministic fallback,
   and prompt/privacy tests.
4. **Artifact compatibility** — complete: APKG/TSV renderers, deterministic
   package tests, field-order/template assertions, and import-level validation.
5. **Migration and rollout** — complete: no migration rewrites historical
   generated-vocabulary provenance or imported Anki notes.

## Acceptance criteria

- The examples `‹die` and `Besten›` produce clean tested forms while the displayed
  source sentence remains unchanged.
- `geleiten|leiten` never appears as a learner-facing lemma; raw analyzer data is
  still diagnosable.
- No morphology JSON or `Morph` card field is emitted by the new export contract.
- A noun with a directly preceding `der`, `die`, or `das` displays the article in
  the lemma field; other nouns are unchanged.
- Front cards show a bold target in ordinary sentence context; backs show lemma,
  English definition, and full-sentence English translation.
- Validated short context is used only when needed for a long sentence; invalid
  or unavailable LLM output falls back safely.
- APKG output is deterministic, HTML-safe, stable-ID preserving, and importable;
  the retained TSV has the same six semantic fields.

## Accepted decisions

- For pipe-separated lemmas, validate representative Stanza output, then use the
  first non-empty alternative as canonical/display lemma and preserve the raw
  analyzer value for diagnostics.
- Remove the Cloze model immediately; recognition cards become the sole export
  model rather than carrying two templates indefinitely.
- Remove morphology from transport-facing/card output, but retain internal
  morphology and its current persistence identity semantics until a separate
  migration decision.
- Include long-sentence shortening in the first implementation as a validated
  exact-span LLM fallback with full-sentence back context and deterministic
  fallback.
- Use a 50-word or 400-Unicode-code-point limit for long-context shortening;
  do not persist provider confidence or rationale.

## Decisions still needed

- Decide whether a later milestone should remove morphology from the internal
  shared-lemma schema and uniqueness key (the latter requires a migration and
  changes identity semantics).

## Rollout and compatibility

The Cloze model is removed immediately from new exports. Prepared-deck workers
now produce the `Mouseion Vocab Recognition` APKG model only; the optional TSV
is a compatibility artifact with the same `Front`, `Lemma`, `English`,
`EnglishSentence`, `BookTitle`, and `SourceSentence` fields. New recognition
fronts bold the tested surface, and the back shows the lemma, definition, and
complete-sentence translation plus the complete source sentence.

Existing imported Anki notes are not rewritten. Existing generated-vocabulary
rows and their first-deck/source provenance are not migrated or reinterpreted;
the recognition contract applies to newly prepared artifacts. Users who want
to remove old Cloze cards must do so in Anki. A prepared export can still be
ready when optional translation or shortening is unavailable: accepted short
cards may have empty English fields, while long cards without a validated span
are reported as quality omissions and are not assigned as generated
vocabulary.
