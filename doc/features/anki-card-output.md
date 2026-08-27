# Anki card output milestone

Status: **Superseded by ADR 0029** · Date: 2026-08-27

The recognition-card contract is defined by [Recognition-card sentence
presentation](recognition-card-sentence-presentation.md) and
[ADR 0029](../adr/0029-recognition-card-sentence-presentation.md).

New exports use the complete selected source sentence in `Text`, with the
tested German target visibly bolded. The note fields are `Text`, `Lemma`,
`POS`, `Morph`, `English`, `EnglishSentence`, and `BookTitle`, followed by TSV
tags. `SourceSentence` is not exported because it duplicates `Text`.

The source sentence is selected deterministically and long examples are
quality-gated deterministically. Mouseion does not ask a provider to shorten,
rewrite, or choose a front-side context. Contextual translations remain tied
to the complete source sentence and may contain Mouseion-owned bold markup for
a uniquely validated plain-text English target phrase.

Existing generated-vocabulary records and imported Anki notes are not
rewritten. New APKG and TSV artifacts use the recognition contract; their
stable Anki GUID and persistence deduplication key remain the owner-scoped
language/lemma/UPOS identity.
