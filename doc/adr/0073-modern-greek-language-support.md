# ADR 0073: Modern Greek language support

Status: **Accepted** · Date: 2026-09-19 · Author: Justin + opencode

Builds on **ADR 0023** (the NLP service owns language capabilities) and
**ADR 0063** (Stanza models provisioned on a volume, not the image). Amends
ADR 0063's provisioned package and processor set, ADR 0005's pluggable
normalization profiles, and ADR 0064's dictionary-index language set.

## Context

Mouseion's language support is capability-driven and language-generic: the NLP
service advertises ready languages, the schema stores language codes as plain
text, and selection, coverage, deck naming, and persistence key on
`(language, canonical lemma, upos)`. German and Italian are provisioned by the
standard deployment.

Adding Modern Greek (`el`) is not a pure configuration change:

- **The shared processor set is insufficient.** Provisioning and serving use one
  `tokenize,pos,lemma,depparse` string. Greek has multi-word tokens (candidate
  contractions such as `στο`, `στην`), and Stanza ships an `mwt` model only in
  its GDT package. Appending `mwt` globally risks every other language.
- **The most accurate Stanza parser needs external weights.** Stanza's Greek
  treebank ships a `gdt_greek-bert` dependency parser. It pulls
  `nlpaueb/bert-base-greek-uncased-v1` (~454 MB) through `transformers`
  *at pipeline construction*, which `stanza.download` does not fetch. The
  serving container currently has no `transformers` dependency and no persistent
  Hugging Face cache, so weights would be re-downloaded into an ephemeral layer
  and serving would depend on runtime egress.
- **Greek canonicalization is undefined.** The Python producer falls back to
  `unicode-casefold`; Go's canonicalization registry has only a German profile,
  so known-vocabulary import for any non-`de` language errors. Greek has
  language-specific form questions (final sigma, monotonic accents/tonos).
- **Provisioning cannot express per-language packages.** The provision marker
  records only the Stanza version and the language set, so changing one
  language's package or processor set after first provision is a silent no-op.

Two facts narrowed the package choice. The published GDT-vs-GUD comparison is
not apples-to-apples (each model evaluated on its own treebank split, from the
historical v1.5.1 table): GDT leads on lemma (96.05 vs 92.63), UPOS (97.71 vs
96.52), and LAS (88.87 vs 85.33), while GUD leads only on sentence segmentation
(98.16 vs 93.66). GUD also has no `mwt` model, and vocabulary identity in
Mouseion is lemma-driven. Licensing is unchanged from the established posture:
models are downloaded at deployment time and never redistributed, so the GDT
training data's CC BY-NC-SA 3.0 terms are not redistributed by this repository;
Stanza is Apache-2.0 and GreekBERT is MIT.

## Decision

### Stanza package: GDT with explicit MWT and the accurate parser

Modern Greek is provisioned and served with Stanza's `default_accurate` package:

```
tokenize = gdt
mwt      = gdt
lemma    = gdt_nocharlm
pos      = gdt_nocharlm
depparse = gdt_greek-bert
```

This keeps GDT's stronger lemmatization and morphology for vocabulary identity
and pays the accurate Greek-BERT parser for parse-driven surfaces (concordance,
sentence quality).

### Per-language processor and package selection

Provisioning (`provision.py`) and serving (`producer.py`) select processors and
package per language. German and Italian keep their existing default package and
processor set; Greek additionally runs `mwt`. Capabilities continue to advertise
`tokenize,pos,lemma,depparse`; `mwt` is internal preprocessing and is not a
learner-facing feature.

### Offline provisioning of the accurate parser

- `nlp-init` pre-downloads `nlpaueb/bert-base-greek-uncased-v1` and the GDT
  parser checkpoint into a named Hugging Face cache volume (`HF_HOME`) mounted on
  both `nlp-init` and the serving `nlp` service.
- The serving pipeline is constructed in offline mode
  (`download_method=DownloadMethod.NONE`, propagated as `local_files_only`), so
  serving never depends on network egress.
- The provision marker records the per-language package/processor selection in
  addition to the Stanza version and language set, so a selection change
  re-provisions instead of silently serving a stale package.
- `transformers` becomes a pinned runtime dependency of the NLP package.

### Greek canonicalization

A Greek normalization profile defines the canonical lemma as NFC + Unicode
case folding while preserving monotonic accents (tonos). Case folding naturally
folds final sigma (`ς`) to medial sigma (`σ`) and uppercases to a stable form;
accents remain significant so pairs such as `πού`/`που` and `ή`/`η` stay
distinct. The profile is applied consistently in the Python producer, the
dictionary-index derivation, Go's runtime dictionary lookup, and Go's
canonicalization registry. A language-neutral fallback profile is added so
languages without a dedicated profile (such as Italian) no longer error in
synchronous known-vocabulary import.

### Language-specific surfaces

- **Dictionary index.** `el` is added to the Kaikki/Wiktextract derivation:
  glosses, gender, nominative definite article (`ο`/`η`/`το`), plural, and IPA.
  Verb principal parts are omitted in v1 because the existing extractor's
  Germanic head-template expansion does not apply to Greek.
- **Card export.** Greek nouns render the nominative definite article by gender,
  matching the German and Italian behavior.
- **Enrichment.** Greek function words are added to the contextual stopword set
  (the existing `el` key is an archaic Italian article, not Greek).
- **Sentence quality.** Greek uses the generic rubric; the German GDEX
  finite-verb-and-subject knock-out is deferred, matching Italian. Because the
  dependency parse is persisted, the knock-out can be added later without
  re-analysis.
- **Lemma display.** `lemmadisplay` remains German-only; Greek common nouns are
  not capitalized.

### Deployment and observability

`el` joins the Compose default `MOUSEION_NLP_WARM_LANGUAGES` (`de,it,el`). No
database migration is required: all language columns are plain text and
`supported_languages` is populated from capabilities. The Greek capability's
`model_version` distinguishes the accurate package so capability observation
remains meaningful.

## Consequences

- Greek gains sentence segmentation, MWT expansion, GDT morphology/lemmas, and a
  Greek-BERT dependency parse, served fully offline.
- The NLP image and the provisioner gain a `transformers` dependency, a second
  persistent cache (Hugging Face weights) alongside the Stanza volume, and a
  richer marker; `compose.yaml`, `nlp/Dockerfile`, and `nlp/pyproject.toml` cross
  the human-review boundary.
- Greek lemmatization and parsing adopt whatever conventions the GDT/Greek-BERT
  checkpoints use; the canonical profile normalizes case and sigma but does not
  rewrite lemmas.
- Greek verb principal parts and the German GDEX knock-out are explicitly
  deferred, not silently missing.
- Non-German known-vocabulary import (Italian included) stops erroring due to the
  added fallback profile.

## Alternatives considered

- **The GUD package.** Rejected: no `mwt` model and weaker published
  lemmatization/parsing; its better sentence segmentation does not outweigh
  lemma quality for a vocabulary-identity-driven product.
- **Replacing Stanza with a Greek-specific provider (e.g. Dilemma).** Rejected
  for this decision: out of scope by the product owner, a young project, and a
  much larger artifact footprint, for a language the Stanza pipeline already
  covers.
- **A package bake-off before choosing.** Deferred: GDT + `mwt` is the
  conservative choice on the evidence; a future bake-off can revisit the package
  without changing the surrounding architecture.
- **Appending `mwt` globally.** Rejected: unverified for other languages; the
  per-language seam is required and cheap.
- **Advertising `mwt` as a supported feature.** Rejected: it is an internal
  tokenization step, not a capability the web application acts on.
- **Downloading GreekBERT at runtime.** Rejected: breaks the offline serving
  posture and re-downloads on every container recreation.
- **Baking GreekBERT into the image.** Rejected: bloats the image and reverts
  ADR 0063's rationale.
- **Enabling the German GDEX knock-out for Greek now.** Rejected: the deixis
  term list is language-specific; the generic rubric is consistent with the
  Italian deferral.

## Related

- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md) — pluggable normalization profiles.
- [ADR 0023: NLP service owns language capabilities](0023-nlp-capabilities.md)
- [ADR 0062: Sentence-quality scoring derived from the persisted corpus](0062-derived-sentence-quality-scoring.md) — the knock-out deferred here.
- [ADR 0063: Stanza model provisioning on a Docker volume instead of the image](0063-stanza-models-on-volume.md) — its provisioned package/processor set is amended here.
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md) — its language set is extended here.
- [Language Support](../features/language-support.md)
- [Dependency Parse Foundation](../features/dependency-parse-foundation.md)
- [Modern Greek NLP pipeline research](../research/modern-greek-nlp.md)
