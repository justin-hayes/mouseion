# Reviews

Dated operator reviews of provider output that cannot run in CI. Each report
records the date, model, reasoning setting, prompt contract, the exact cases
checked and the operator verdicts. Reports are evidence for a rollout decision,
not automated pass/fail gates.

## Contextual Gloss review

`cmd/glossreview` runs the frozen case set in
`cmd/glossreview/testdata/gloss_review_cases.json` through the real contextual
Gloss codec and the configured OpenAI-compatible provider, at the deployed model
and reasoning setting. It writes a dated Markdown report under `doc/reviews/`.
The command never runs in CI and the case set is repository test data, so a
review is tied to the code that produced it.

The command sends only what normal preparation sends: the lemma, target form,
one representative sentence and public lexical evidence. It sends no learner
identity, Book title, whole Book or reading history.

### Usage

Run from the repository root with the deployed provider configuration:

```sh
export MOUSEION_LLM_ENABLED=true
export MOUSEION_LLM_API_KEY=...
export MOUSEION_LLM_MODEL=...
# Optional, matching the deployment:
export MOUSEION_LLM_BASE_URL=...
export MOUSEION_LLM_REASONING_EFFORT=low
export MOUSEION_LLM_SUPPORTS_REASONING_EFFORT=false

go run ./cmd/glossreview
```

The default report path is
`doc/reviews/contextual-gloss-<YYYY-MM-DD>.md`. Choose a different path or a
fixed report date when reproducing a run:

```sh
go run ./cmd/glossreview \
  -out doc/reviews/contextual-gloss-v15-baseline-2026-10-29.md \
  -date 2026-10-29
```

The date defaults to today in UTC. The command refuses to run when no provider is
configured, rather than writing a report with empty provider output.

### Case set

The case set is versioned (`gloss-review-v2`) and carries the v15 failure and win
classes from #2325:

- `plain_equivalent` — must become the plain lemma equivalent.
- `drop_sentence_material` — must drop the sentence's inflection, number and
  arguments.
- `protected_win` — a genuine contextual improvement that must be kept.
- `lemma_mismatch` — the sentence uses a different lexeme, so the card must be
  omitted instead of glossed.
- `spot_check` — function words, idioms, and Italian or Modern Greek coverage.

`gloss-review-v2` keeps every v1 case unchanged and adds harder German cases:

- **Missed-particle mismatches** — `de-ruft-an`, `de-steht-auf` and
  `de-kauft-ein` freeze the bare base-verb lemma (`rufen`, `stehen`, `kaufen`)
  although the sentence uses the separable verb (*anrufen*, *aufstehen*,
  *einkaufen*). Each must return `lemma_mismatch`.
- **Lookalike controls** — `de-liegt-fern` is a *fern* homograph where the word
  is an adverb ("far from"), not a separable prefix, and it must be glossed as
  *liegen* ("to lie"). `de-fernsehen` is the *sieht … fern* sentence with the
  correctly analysed lemma *fernsehen*, which re-analysed books produce since
  #2329; its Gloss must be "to watch television". Both controls fail the
  automatic `lemma_mismatch` check if a provider flags them.

`cmd/glossreview/testdata/gloss_review_cases_v1.json` preserves the v1 set, and
a test asserts every v1 case still appears unchanged in v2.

Every case carries language, lemma, UPOS, target form, representative sentence,
frozen meaning evidence, an expected verdict class and, unless it is a mismatch
case, an expected Gloss. `loadCaseSet` rejects a case set that misses a language
or verdict class.

### What the report contains

For each case the report records the request summary, the raw Gloss, the
translation, the outcome, and two automatic checks:

- **Gloss equals translation** — compares the resolved Gloss with the plain
  translation, ignoring case and surrounding whitespace.
- **outcome is `lemma_mismatch`** — compares the provider's `unresolved_kind`
  with the case's expected verdict. A `lemma_mismatch` case passes only when the
  provider returns `lemma_mismatch`; any other case fails if the provider does.
  Each case also shows the unresolved kind it returned.

The command always runs the contract compiled into the binary (currently
`translation-v17-clause-lemma-mismatch`). To reproduce a v15 run, check out a
commit from before v16 and run the command there.

The summary reports the full review-corpus "Gloss contains translation" rate
for information only. It does not gate rollout, and it does not read the
operator's `enrichment_cache`; the library-wide rate remains the separate
#2325 measurement.

The report leaves a blank reviewer verdict for every judgement-based rubric item:

- citation form;
- no definition paraphrase;
- no sentence inflection or arguments;
- protected win kept.

Automatic checks never fill these in. A report is complete only when the
operator has recorded every verdict.

## Idiom card review

`cmd/glossreview -review idiom` runs the frozen idiom case set
`idiom-review-v3` through the real idiom codec and the configured provider, at
the deployed model and reasoning setting. The reasoning setting is
`MOUSEION_LLM_IDIOM_REASONING_EFFORT` when set, otherwise
`MOUSEION_LLM_REASONING_EFFORT`; set the same value the deployment uses. It
records the date, model, reasoning setting, prompt contract, matcher version
and matcher gap bound, and writes a dated Markdown report under
`doc/reviews/`. The command never runs in CI.

The idiom request carries only the headword, the matched forms, one
representative sentence and the idiom's bounded public senses. When the
dictionary lists one headword under several parts of speech (*di fronte* is
both an adjective and an adverb), the senses of every part of speech are pooled
in part-of-speech order before the bound applies (`idiom-matcher/2`, #2378). It
sends no learner identity, Book title, whole Book or reading history.

### Usage

Run the case set and a 30-card sample from one real analyzed Book:

```sh
export MOUSEION_LLM_ENABLED=true
export MOUSEION_LLM_API_KEY=...
export MOUSEION_LLM_MODEL=...
export MOUSEION_DATABASE_URL=...
export MOUSEION_DICTIONARY_INDEX=...

go run ./cmd/glossreview -review idiom \
  -book <Book-ID> \
  -language it \
  -seed 2026-11-08
```

The default report path is `doc/reviews/idiom-review-<YYYY-MM-DD>.md`. Choose a
different path or a fixed report date when reproducing a run:

```sh
go run ./cmd/glossreview -review idiom \
  -out doc/reviews/idiom-review-2026-11-08.md \
  -date 2026-11-08 \
  -book <Book-ID> -language it -seed 2026-11-08
```

`-book` is optional: without it the report contains only the case set and says
that the gate cannot pass without a sample. `-seed` defaults to the report date.
`-book` needs `MOUSEION_DATABASE_URL` and `MOUSEION_DICTIONARY_INDEX`, and the
index must carry the idiom lookup tables (`idiom_entries` and `idiom_words`). An
index derived before those tables existed is refused; rebuild it first. An
index from an older derivation that already has those tables is accepted
without warning, so rebuild it before a review whenever the deployed change
touches the derivation script or the reviewed idiom list
([Dictionary index](../operations.md#dictionary-index)). The
sample does not need a ready deck or a recorded idiom assessment: recording the
assessment is what the review is for.

### Case set

`idiom-review-v3` contains four review classes. It keeps every
`idiom-review-v2` case unchanged (`testdata/idiom_review_cases_v2.json`), which
in turn kept every `idiom-review-v1` case (`testdata/idiom_review_cases_v1.json`).
v2 added cards 4, 13 and 25 of the failed
[2026-11-08 Italian review](idiom-review-2026-11-08.md) (#2378); v3 adds cards 8
and 18 of the failed v2 re-run (#2389).

- **Positive cases** — *in gamba*, *in bocca al lupo* ("good luck"), *macchina
  da scrivere* ("typewriter"), *κάνω τα στραβά μάτια* ("to turn a blind eye")
  *di fronte* with its pooled adjective and adverb senses, where the
  sentence uses the adverb ("in front", not the noun "front"), and *in giro*
  in *andare in giro per la città*, whose Gloss must be "around", not the
  sentence's verb ("to go around"). Each must be confirmed with a
  citation-form Gloss.
- **Literal-use controls** — *Tagliò la corda del pacco.* against *tagliare la
  corda*, *Sie nimmt das Kind auf den Arm.* against *jemanden auf den Arm
  nehmen*, and *fare le cose fatte bene* against *fare bene*, whose senses
  include Wiktionary's "Used other than figuratively or idiomatically" marker.
  The words are meant literally, so each must return `literal_use`.
- **Ambiguous controls** — the comparative *rispetto a* ("compared with"),
  whose dictionary senses list only "with respect to" and "in relation to", and
  *casa isolata* for a remote house beyond the hills, whose senses list only
  "detached house" and "homestead". No candidate sense fits, so each must
  return `ambiguous` rather than a Gloss from context.
- **Lemma-card controls** — *gamba* and *Arm*. Each records the idiom sentence
  and the idiom-free sentence the anchor's own card must use instead.

Every provider case carries language, headword, matched forms, one
representative sentence, frozen dictionary senses, and its expected outcome. A
positive case also carries an expected Gloss; a literal-use or ambiguous
control must not. A positive case may list `gloss_excludes`: sentence words,
such as the verb that governs the idiom, its confirmed Gloss must not
contain. `loadIdiomCaseSet` rejects a set that misses a language,
review class or required field.

### Sample mode

The sample is 30 distinct confirmed idiom cards from one Book in the requested
language. The command re-freezes the Book's current deck preparation in memory,
in a read-only transaction that is always rolled back, with idiom cards on for
that language and no per-deck bound. It orders the idiom items by a SHA-256 of
the Book ID, seed and item identity, so the same Book and seed give the same
order regardless of database order. It then sends each item to the provider,
using the request a prepared deck would send, until 30 are confirmed. An item
counts as confirmed only when the provider resolves a Gloss; items returned as
`literal_use`, `ambiguous` or unresolved are listed under "Not confirmed" in the
report. Nothing is written and no assessment is recorded.

The report quotes each sampled item's representative sentence; commit it under
the same source-rights review as any other review excerpt.

### What the idiom report contains

The report header pins the provider, prompt contract, matcher version and gap
bound. For each case it shows the headword, matched words, representative
sentence, provider outcome, raw Gloss, and automatic checks:

- **outcome is `confirmed`** — a positive case passes only when the provider
  resolved a Gloss.
- **Gloss leaves out the excluded sentence words** — a positive case with
  `gloss_excludes` passes only when its confirmed Gloss contains none of them
  as a whole word, ignoring case. It is not applicable when the case was not
  confirmed.
- **outcome is `literal_use`** — a literal-use control passes only when the
  provider rejected the idiom.
- **outcome is `ambiguous`** — an ambiguous control passes only when the
  provider abstained because no candidate sense fits.
- **lemma card moves off the idiom sentence** — a lemma-card control passes when
  its recorded card sentence differs from the idiom sentence and still carries
  the anchor form.

The sample header counts the idiom items the freeze found, how many were sent
and how many were confirmed. Each confirmed item shows its headword, anchor,
matched words, representative sentence, Gloss and translations; each item the
provider did not confirm shows its outcome and reason. Every case and confirmed
sample item leaves a blank reviewer verdict; the command never judges citation
form or meaning.

### Gate

Italian idiom cards stay off until a committed report shows all of:

- every positive case is confirmed with a citation-form Gloss;
- no literal-use, ambiguous or lemma-card control is confirmed;
- the sample contains 30 confirmed cards and none teaches a wrong meaning.

The report states this gate and leaves the final verdict to the operator.

### Reports

- [Contextual Gloss v15 baseline](contextual-gloss-v15-baseline-2026-10-29.md) —
  baseline pending; the implementation environment had no configured provider.
- [Contextual Gloss v16 review, 2026-11-07](contextual-gloss-v16-2026-11-07.md) —
  gate failed on the *fern-sehen* `lemma_mismatch` case; v16 does not roll out.
- [English target alignment quality review](english-target-alignment.md) —
  configured-model review pending.
- [Idiom card v1 review, 2026-11-08](idiom-review-2026-11-08.md) — gate failed
  on three wrong-meaning sample cards; Italian idiom cards stay off (#2378).
- [Idiom card v2 review, 2026-11-08](idiom-review-v2-2026-11-08.md) — gate
  failed on four case checks and a sample of 1 confirmed card; Italian idiom
  cards stay off (#2385).
