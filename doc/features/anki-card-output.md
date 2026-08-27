# Anki card output milestone

Status: **Accepted and rolled out** · Date: 2026-08-27

## Problem

The generated Anki cards currently expose several analyzer/output contract
defects:

- persisted targets can retain punctuation (`Souveränität›`, `die Besten›`,
  `‹die`);
- Stanza can emit a pipe-separated lemma such as `geleiten|leiten`;
- German article display can copy a declined context article into the lemma;
- Anki duplicate detection can collide when two targets share a sentence.

The current note is also shaped around a production-oriented Cloze workflow.
The intended product is now a recognition-oriented card with a compact,
readable front and back. Long but otherwise useful source sentences should not
be discarded solely because the complete sentence is too long to scan.

## Goals

- Ensure extracted word forms do not include surrounding punctuation while
  preserving the original source sentence exactly.
- Establish deterministic handling for pipe-separated analyzer lemmas.
- Replace the user-facing morphology JSON/card field with a separate German
  noun article derived from stored gender and number morphology.
- Change the primary Anki note model from Cloze production to recognition:
  context with the tested word bolded on the front; lemma, concise English
  definition, and English translation of the sentence on the back.
- Retain useful vocabulary from long sentences by asking the translation/enrichment
  LLM for a shorter, sufficient context clause, with a safe deterministic
  fallback.
- Preserve stable identity, provenance, escaping, deterministic exports, and
  owner-scoped generated-vocabulary behavior.
- Give each concrete card a deterministic, unobtrusive Anki duplicate field so
  different targets in the same sentence do not collide.

## Non-goals

- Do not infer articles for languages other than German or use declined source
  context as an article source.
- Do not make the LLM responsible for lexical eligibility, target matching, or
  HTML safety.
- Do not silently change historical cards or rewrite existing generated-vocabulary
  provenance. New exports use the new contract; migration/compatibility behavior
  must be explicit.
- Do not add a second divergent TSV card-content implementation.

## Proposed card contract

The note model contains these fields, in this order:

1. `Identity` — a hidden SHA-256 value derived from the owner, language,
   canonical lemma, UPOS, normalized tested surface, complete source sentence,
   source document, and source location;
2. `Front` — source context with the tested surface form rendered in bold;
3. `Article` — dictionary-form German definite article when morphology is
   sufficient and unambiguous, otherwise empty;
4. `Lemma` — the bare display lemma;
5. `English` — concise English definition/translation of the lemma;
6. `EnglishSentence` — English translation of the complete source sentence;
7. `BookTitle` — source/book provenance;
8. `SourceSentence` — complete original source sentence.

`POS` and `Morph` should no longer be user-facing card fields. POS may remain a
stable tag and internal selection attribute if needed for identity and filtering.
Morphology remains internal persisted analysis data. The back conditionally
renders `Article` plus a space before `Lemma`, while both remain separate note
fields. `Identity` is the Anki model's sort field and the serialized note's
`sfld`/first-field checksum source, but is not referenced by either card
template. The existing owner/language/lemma/UPOS key continues to produce the
Anki GUID and persistence dedup key, preserving lemma-level generated-vocabulary
semantics. The front is a normal field template, not a Cloze template.

The front must HTML-escape all source text first and then add the bold wrapper
only around the matched target span. Matching must be Unicode-aware and must
not allow a target to match inside a larger word.

## Proposed normalization rules

### Punctuation-bearing surfaces

At the analyzer boundary, derive a lexical surface form by removing only
leading/trailing Unicode punctuation and symbol characters. Standalone
punctuation tokens and lexical apostrophes in elided forms such as Italian
`L'` remain intact. Preserve internal punctuation (for example apostrophes or
hyphens) and preserve the original sentence text and token offsets for
display/provenance. Use the cleaned surface for target matching and extraction.
Add regression fixtures for `‹die` and `Besten›`, plus quotes, parentheses, em
dashes, symbols, apostrophes, and hyphenated forms.

This is implemented in the analyzer normalization path for new analysis. Card
selection/export also reapplies the identical rule immediately before matching
persisted observed forms. That last-boundary guard covers legacy analysis data
without changing canonical identity or rewriting historical records. It also
feeds the cleaned target to sentence scoring, long-context validation,
enrichment candidates, card rendering, and card-specific identity generation.

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

For German `NOUN` entries, derive the dictionary-form definite article from all
persisted morphology variants for the lemma:

- an unambiguous masculine gender produces `der`;
- an unambiguous feminine gender produces `die`;
- an unambiguous neuter gender produces `das`;
- when gender is absent, morphology that is unambiguously plural produces
  `die` (for plural-only lemmas);
- missing, malformed, unsupported, or conflicting morphology produces an empty
  `Article` field.

Gender takes precedence over an observed token's number because the displayed
lemma is the dictionary lemma: plural `Häuser` still displays `das Haus`, while
a plural-only lemma without gender can display `die Eltern`.

Sentence context is never an article source, so a dative phrase such as `der
Iteration` exports bare lemma `Iteration`, article `die`, and displays `die
Iteration` on the back. `Ruderblatt` can display `das Ruderblatt` even when no
article precedes it in the source. The source sentence and internal morphology
remain unchanged, and article display does not alter canonical lemma identity.

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
   context must be an exact substring (or a precisely validated token span) of
   the original sentence, contain the target, contain at least three
   whitespace-delimited words, fall at or below both long-context limits, and
   pass the same escaping/boundary checks as ordinary context.
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
The current operation/version is `translation-v5-concise-json-50w-400c` with
low reasoning effort on supported reasoning models; changing it bypasses older
cached provider results. The cache stores the validated
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
2. **Lean note/model contract** — complete: the recognition `Front`, separate
   morphology-derived `Article`, bare `Lemma`, and hidden card-specific
   `Identity` replace the user-facing morphology fields.
3. **Long-context enrichment** — complete: versioned structured provider
   response, cache-key update, exact-span validation, deterministic fallback,
   and prompt/privacy tests.
4. **Artifact compatibility** — complete: APKG/TSV renderers, deterministic
   package tests, field-order/template assertions, and import-level validation.
5. **Migration and rollout** — complete: no migration rewrites historical
   generated-vocabulary provenance or imported Anki notes.

## Acceptance criteria

- The examples `Souveränität›`, `die Besten›`, and `‹die` produce clean tested
  forms while the displayed source sentence remains unchanged.
- `geleiten|leiten` never appears as a learner-facing lemma; raw analyzer data is
  still diagnosable.
- No morphology JSON or `Morph` card field is emitted by the new export contract.
- A German noun with unambiguous gender/number morphology exports a separate
  dictionary-form article and bare lemma; missing or ambiguous morphology leaves
  `Article` empty.
- Declined context articles do not affect the dictionary-form article.
- Front cards show a bold target in ordinary sentence context; backs show lemma,
  English definition, and full-sentence English translation.
- Validated short context is used only when needed for a long sentence; invalid
  or unavailable LLM output falls back safely.
- APKG output is deterministic, HTML-safe, stable-GUID preserving, and
  importable. `Identity` is its hidden sort/duplicate field, differs for
  different targets in one sentence, and is stable for identical card inputs.
  The retained TSV has the same eight semantic fields.

## Accepted decisions

- For pipe-separated lemmas, validate representative Stanza output, then use the
  first non-empty alternative as canonical/display lemma and preserve the raw
  analyzer value for diagnostics.
- Remove the Cloze model immediately; recognition cards become the sole export
  model rather than carrying two templates indefinitely.
- Remove morphology from transport-facing/card output, but retain internal
  morphology and its current persistence identity semantics. Aggregate stored
  variants at read time only to detect ambiguous article display; no migration
  or historical rewrite is required.
- Preserve the existing lemma-level owner-scoped key for Anki GUIDs and stored
  card/generated-vocabulary semantics. Use a separately versioned card-specific
  `Identity` hash as Anki's first/sort field and first-field checksum source.
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
produce the `Mouseion Vocab Recognition` APKG model; the optional TSV is a
compatibility artifact with the same `Identity`, `Front`, `Article`, `Lemma`,
`English`, `EnglishSentence`, `BookTitle`, and `SourceSentence` fields. New
recognition fronts bold only the normalized tested surface, and the back shows
the optional article with the bare lemma, definition, complete-sentence
translation, and complete source sentence.

Existing imported Anki notes are not rewritten. Existing generated-vocabulary
rows and their first-deck/source provenance are not migrated or reinterpreted;
the recognition contract applies to newly prepared artifacts. Users who want
to remove old Cloze cards must do so in Anki. A prepared export can still be
ready when optional translation or shortening is unavailable: accepted short
cards may have empty English fields, while long cards without a validated span
are reported as quality omissions and are not assigned as generated
vocabulary.

The analyzer contract revision is identified by scoped analyzer version `3`,
German normalization profile version `4`, and language-neutral profile version
`1.2.0`. Re-submitting a reviewed scope creates a new immutable analysis run
under those versions instead of rewriting an earlier corpus.
