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
`translation-v16-citation-form-gloss`). To reproduce a v15 run, check out a
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

### Reports

- [Contextual Gloss v15 baseline](contextual-gloss-v15-baseline-2026-10-29.md) —
  baseline pending; the implementation environment had no configured provider.
- [Contextual Gloss v16 review, 2026-11-07](contextual-gloss-v16-2026-11-07.md) —
  gate failed on the *fern-sehen* `lemma_mismatch` case; v16 does not roll out.
- [English target alignment quality review](english-target-alignment.md) —
  configured-model review pending.
