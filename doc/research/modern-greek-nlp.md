# Modern Greek NLP Pipeline Research

**Research date:** 2026-09-19

**Scope:** Compare self-hostable Modern Greek (`el`) NLP options for sentence
segmentation, tokenization, lemmatization, POS/morphological tagging, and
dependency parsing, with special attention to lemma quality for learner
vocabulary lookup.

## Executive Recommendation

Do not replace the existing Stanza architecture solely to add Modern Greek.
Stanza is the lowest-risk full-pipeline addition and has a maintained Greek
resource bundle. However, Greek should be provisioned with the `mwt` processor
as well as `tokenize`, `lemma`, `pos`, and `depparse`. The Greek UD treebank has
1,668 multi-word tokens, including common preposition/article contractions, and
the Stanza resource bundle includes a dedicated Greek MWT model.

If lemma quality is the deciding factor, benchmark Dilemma as a serious
alternative before implementation. Dilemma is a Greek-specific package with a
Modern Greek (`el`) ONNX tagger/parser and a large lookup-plus-rules-plus-model
lemmatizer. Its published Modern Greek benchmark is substantially better than
the published Stanza and spaCy numbers, but the benchmark is its own Demotic
set rather than the directly comparable UD evaluation. Treat it as a strong
candidate requiring an application-corpus bake-off, not as a proven universal
winner.

Trankit is the strongest directly comparable older benchmark result for Greek,
but its large XLM-R dependency, old dependency constraints, and weaker
maintenance signals make it a poor default for this service. spaCy is a
reasonable general-purpose alternative with Greek NER and vectors, but its
Greek lemmatization scores and model licensing are less attractive here.

## Existing Architecture Constraints

- The app already uses Stanza for German and Italian.
- The NLP service is authoritative for language capabilities; language support
  should remain a deployment/model capability rather than a web-app allowlist.
- Existing deployment documentation provisions `tokenize,pos,lemma,depparse`.
  That set is sufficient for the current languages but should not be assumed
  sufficient for Greek because Greek has MWT contractions.

Sources: [ADR 0023](../adr/0023-nlp-capabilities.md),
[language support](../features/language-support.md),
[Stanza resources 1.14.0](https://raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.14.0.json).

The implementation touchpoints for a future Greek change are:

- `nlp/src/mouseion_nlp/provision.py`: the shared `PROCESSORS` download set.
- `nlp/src/mouseion_nlp/producer.py`: the serving pipeline processor set.
- `nlp/tests/test_provision.py`: provisioning assertions that currently expect
  the shared set.
- `doc/features/language-support.md` and
  `doc/features/dependency-parse-foundation.md`: the documented processor
  contract.

Because provisioning and serving currently use one shared processor string,
adding `mwt` for Greek should not be done by assumption as a global append.
The implementation should either verify MWT behavior for every configured
language or introduce a language-specific processor selection.

## Stanza

The current Stanza resource manifest identifies `el` as Greek and provides:

- tokenizer: `gdt`
- MWT expansion: `gdt`
- lemmatizer: `gdt_nocharlm`
- POS/morphology: `gdt_nocharlm`
- dependency parser: `gdt_nocharlm`
- accurate dependency parser option: `gdt_greek-bert`
- alternate GUD tokenizer, lemmatizer, POS, and parser package
- no packaged Greek NER model

The manifest's default Greek package is therefore `tokenize + mwt + lemma +
pos + depparse`; omitting MWT is a deliberate change from the upstream default.

### Stanza API and MWT resolution

The v1.14.0 `Pipeline` constructor accepts `package` as a string, and accepts a
processor-to-package mapping through `processors`:

```python
stanza.Pipeline(
    "el",
    package=None,
    processors={
        "tokenize": "gud",
        "mwt": "gdt",
        "lemma": "gdt_nocharlm",
        "pos": "gdt_nocharlm",
        "depparse": "gdt_nocharlm",
    },
)
```

`stanza.download()` uses the same selection rules. A `package` mapping is also
supported when `processors` is a string or list, for example
`processors="tokenize,pos"` with
`package={"tokenize": "gud", "pos": "gud_nocharlm"}`. It is not accepted
alongside a dictionary-valued `processors` argument; in that form `package`
must be a string or `None`.

Stanza automatically adds MWT only when the selected tokenizer/package has a
matching MWT model in the resource manifest. Selecting GUD does not add MWT:
the `gud` bundle has no MWT entry, while the `gdt` bundle maps `mwt` to `gdt`.
This is why Greek provisioning should select `mwt` explicitly rather than
assuming that every Greek tokenizer implies expansion.

Sources: [v1.14.0 pipeline source](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.14.0/stanza/pipeline/core.py),
[v1.14.0 resource-resolution source](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.14.0/stanza/resources/common.py),
[Greek resource manifest](https://raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.14.0.json).

Stanza's official performance page reports the following Greek-GDT end-to-end
scores for Stanza v1.5.1 on UD v2.12: token 99.90, sentence 93.66, UPOS 97.71,
UFeats 94.72, lemma 96.05, UAS 91.22, LAS 88.87, MLAS 77.99, and BLEX 79.08.
These are not measurements of the current 1.14.0 model bundle; they are the
latest official table currently published by Stanza and should be labeled as
historical when used in a comparison.

The same official table also includes GUD, so a direct historical comparison is
available:

| Metric | GDT | GUD |
| --- | ---: | ---: |
| Token | 99.90 | 99.89 |
| Sentence | 93.66 | 98.16 |
| Words | 99.90 | 99.89 |
| UPOS | 97.71 | 96.52 |
| Lemma | 96.05 | 92.63 |
| UAS | 91.22 | 89.81 |
| LAS | 88.87 | 85.33 |

The page does not provide a current 1.14.0-specific GDT-vs-GUD experiment, and
its token/word metrics are not a separate MWT-expansion score.

Stanza code is Apache 2.0. Stanza states that language packs built from UD are
made available under ODC-By to the extent Stanford owns them, while the
underlying data licenses must be checked separately. UD Greek-GDT is
CC BY-NC-SA 3.0, so redistribution and commercial use need explicit review.

Sources:

- [Stanza available models](https://stanfordnlp.github.io/stanza/available_models.html)
- [Stanza performance](https://stanfordnlp.github.io/stanza/performance.html)
- [Stanza license](https://raw.githubusercontent.com/stanfordnlp/stanza/main/LICENSE)
- [Stanza Greek manifest](https://raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.14.0.json)
- [UD Greek-GDT](https://universaldependencies.org/treebanks/el_gdt/index.html)

## UD Greek-GDT and Evaluation Caveats

UD Greek-GDT is a relatively small treebank: 2,521 sentences, 61,773 surface
tokens, and 63,441 syntactic words. Lemmas are manually annotated; UPOS,
features, and relations were converted from manual annotations with some
corrections. The treebank contains 1,668 MWTs, averaging two syntactic words
per MWT.

The current repository changelog shows continued annotation fixes, including
lemma fixes in v2.15 and further annotation changes in v2.16 and v2.17. This
means benchmark values tied to a specific UD release are not automatically
interchangeable across tools.

Sources:

- [UD Greek-GDT metadata](https://universaldependencies.org/treebanks/el_gdt/index.html)
- [UD Greek-GDT repository](https://github.com/UniversalDependencies/UD_Greek-GDT)
- [Greek UD guidelines](https://universaldependencies.org/el/index.html)

## Dilemma

Dilemma is a Greek-specific package, currently version 1.3.0, first released in
2026. It provides:

- Modern Greek, Ancient Greek, and Medieval/Byzantine modes
- Modern Greek tokenization and MWT handling
- contextual POS and UD-feature tagging
- biaffine dependency parsing
- a dedicated Modern Greek lemmatizer integrated into the tagger
- a torch-free ONNX runtime for inference

Its Modern Greek tagger uses Greek-BERT and openly licensed UD Greek-GUD plus
dialect treebanks, not the non-commercial UD Greek-GDT. Its lemmatizer combines
an 8.6M-form SQLite lookup, deterministic rules, dialect normalization, and a
small character-level transformer for unseen forms. The package supports a
Triantafyllidis convention intended for Modern Greek citation forms.

There is an integration distinction: the direct `Dilemma(lang="el",
convention="triantafyllidis")` API supports that convention, while the current
`Tagger(lang="el")` constructor initializes its integrated lemmatizer without
passing a convention. A full tagger integration must therefore either accept
the tagger's default lemma convention, add an explicit mapping step, or obtain
upstream support for configuring the convention.

Dilemma's published cross-tool Demotic Modern Greek benchmark reports:

| Tool | Demotic MG lemma agreement |
| --- | ---: |
| spaCy `el` | 79.9% |
| Stanza `el` | 87.0% |
| Dilemma, recommended Modern Greek convention | 94.8% |

The benchmark is useful because it targets Modern Greek text outside the usual
UD comparison, but it is maintained by Dilemma and is not directly comparable
to the UD end-to-end scores above. It should be independently reproduced on
the app's learner-reading corpus before adoption.

The Dilemma README also reports a directly evaluated Modern Greek tagger on a
held-out UD Greek-GUD test split of approximately 38K tokens: UPOS 97.9%,
features 99.4%, UAS 89.5%, and LAS 86.1%. This supports the claim that Dilemma
is a full pipeline, but it is still a different treebank and model-training
setup from Stanza's Greek-GDT result.

Dilemma source code is MIT. Its generated lookup and model artifacts retain
the licenses of their upstream sources. The project explicitly documents its
exclusion of non-commercial sources from shipped artifacts, but this needs a
legal review if the app redistributes its downloaded artifacts.

Operational costs are material: the documented full download is approximately
5.5 GB, or approximately 4.5 GB without tagger weights. It is therefore not a
drop-in lightweight replacement for the current Stanza image.

Sources:

- [Dilemma repository and API](https://github.com/open-greek/dilemma)
- [Dilemma tagger evaluation](https://github.com/open-greek/dilemma#pos-tagger-and-dependency-parser)
- [Dilemma package metadata](https://raw.githubusercontent.com/open-greek/dilemma/main/pyproject.toml)
- [Dilemma tagger runtime](https://raw.githubusercontent.com/open-greek/dilemma/main/dilemma/tagger/__init__.py)
- [Dilemma changelog](https://raw.githubusercontent.com/open-greek/dilemma/main/CHANGELOG.md)
- [Dilemma license](https://raw.githubusercontent.com/open-greek/dilemma/main/LICENSE)
- [Dilemma third-party notices](https://raw.githubusercontent.com/open-greek/dilemma/main/NOTICE)

## Trankit

Trankit provides a pretrained `greek` pipeline trained on UD Greek-GDT. Its
documented task set covers sentence segmentation, tokenization, MWT expansion,
POS, morphology, dependency parsing, and NER. It uses XLM-R Large for its
large pretrained pipelines and is Apache 2.0 software.

On the project's UD v2.5 comparison, Greek-GDT scores were:

| System | Lemma | UAS | LAS |
| --- | ---: | ---: | ---: |
| Trankit large | 96.73 | 95.25 | 93.87 |
| Trankit base | 96.55 | 94.16 | 92.80 |
| Stanza v1.1.1 | 96.49 | 91.12 | 88.78 |

These results are old, are from UD v2.5, and are reported by the Trankit
project. They are not proof that the current package outperforms current
Stanza. The repository README also records a July 2025 installation problem,
and the package declares an old `torch <= 2.0.1` constraint. Those are
significant deployment risks for this Python 3.11 service.

Sources:

- [Trankit supported languages](https://trankit.readthedocs.io/en/latest/pkgnames.html)
- [Trankit performance](https://trankit.readthedocs.io/en/latest/performance.html)
- [Trankit README](https://raw.githubusercontent.com/nlp-uoregon/trankit/master/README.md)
- [Trankit setup metadata](https://raw.githubusercontent.com/nlp-uoregon/trankit/master/setup.py)
- [Trankit license](https://raw.githubusercontent.com/nlp-uoregon/trankit/master/LICENSE)

## spaCy

spaCy provides `el_core_news_sm`, `el_core_news_md`, and `el_core_news_lg`, all
at version 3.8.0. They include tokenization, morphology, parsing,
lemmatization, sentence segmentation support, and NER. The medium and large
models additionally provide 300-dimensional Greek vectors, with 20,000 and
500,000 vectors respectively.

The official model metadata reports lemma accuracy of 88.93% for small, 89.45%
for medium, and 89.83% for large on the model's UD Greek-GDT v2.8 evaluation.
The model packages are licensed CC BY-NC-SA 3.0 and include UD Greek-GDT plus
a Greek NER corpus. The non-commercial model license is a poor fit for a
general redistributable self-hosted product.

Sources:

- [spaCy Greek models](https://spacy.io/models/el)
- [small model metadata](https://raw.githubusercontent.com/explosion/spacy-models/master/meta/el_core_news_sm-3.8.0.json)
- [medium model metadata](https://raw.githubusercontent.com/explosion/spacy-models/master/meta/el_core_news_md-3.8.0.json)
- [large model metadata](https://raw.githubusercontent.com/explosion/spacy-models/master/meta/el_core_news_lg-3.8.0.json)

## UDPipe

UDPipe remains a useful small, self-hosted baseline. Its official site
describes UDPipe 1 as a C++ application with Python bindings, approximately
1,000 words/sec on CPU, and weaker morphosyntactic performance than deep
neural systems. UDPipe 2 is a more expensive Python prototype.

It is attractive when footprint and CPU throughput dominate, but the official
documentation does not provide a current Greek-specific comparison suitable
for selecting it over Stanza or Dilemma for lemma accuracy.

Source: [UDPipe](https://ufal.mff.cuni.cz/udpipe).

## GreekBERT and Other Backbones

`nlpaueb/bert-base-greek-uncased-v1` is a Greek BERT fill-mask/backbone model,
not a complete tokenizer, lemmatizer, POS tagger, or dependency parser. Its
paper reports downstream POS and NER experiments after fine-tuning, but using
the base model directly would require building and maintaining task heads and
training data.

It is therefore not a replacement candidate by itself. It is relevant as a
backbone, and Dilemma uses Greek-BERT for its Modern Greek tagger.

For Stanza's `gdt_greek-bert`, the resource manifest lists only the Stanza
parser checkpoint and its `conll17` pretrain dependency. When the dependency
parser is constructed, the checkpoint configuration selects
`nlpaueb/bert-base-greek-uncased-v1`; Stanza then calls Hugging Face
`AutoModel.from_pretrained()` and `AutoTokenizer.from_pretrained()`. Thus
`stanza.download("el", package="default_accurate")` does not itself download
GreekBERT, but constructing the pipeline does unless the Hugging Face cache is
already populated. The parser checkpoint is approximately 110.6 MB, the
ordinary GDT parser checkpoint is approximately 95.9 MB, and the GreekBERT
PyTorch weights are approximately 454.2 MB, excluding small configuration and
tokenizer files.

Licenses must be treated as separate layers: Stanza source and its model
repository are Apache-2.0; the GreekBERT upstream repository is MIT; and the
GDT training data is CC BY-NC-SA 3.0. The GreekBERT model card itself does not
replace the upstream repository's license notice.

Sources:

- [GreekBERT model card](https://huggingface.co/nlpaueb/bert-base-greek-uncased-v1)
- [GreekBERT paper](https://arxiv.org/abs/2008.12014)
- [Stanza Greek transformer selection](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.14.0/stanza/resources/default_packages.py)
- [Stanza BERT loader](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.14.0/stanza/models/common/bert_embedding.py)
- [Stanza dependency-parser loader](https://raw.githubusercontent.com/stanfordnlp/stanza/v1.14.0/stanza/models/depparse/trainer.py)
- [GreekBERT upstream license](https://raw.githubusercontent.com/nlpaueb/greek-bert/master/LICENSE.md)

### Issue search

The relevant Stanza issues found do not establish a Modern Greek quality bug:

- [#1324](https://github.com/stanfordnlp/stanza/issues/1324) asks why GUD was
  shown on the performance page but was not downloadable at the time. The
  current 1.14.0 manifest now includes `gud`.
- [#1506](https://github.com/stanfordnlp/stanza/issues/1506) reports a PyTorch
  2.6 failure when training with an external Greek FastText embedding file; it
  is not an inference-quality finding.
- [#1311](https://github.com/stanfordnlp/stanza/issues/1311) reports punctuation
  failures in the Ancient Greek `grc` PROIEL parser, not Modern Greek `el`.

The repository search did not return a Modern Greek-specific issue about final
sigma, accents, tokenization, or lemmatization. That is absence of a documented
issue, not evidence that those edge cases are error-free.

## Decision Matrix

| Option | Full pipeline | Lemma evidence | Deployment fit | Main concern |
| --- | --- | --- | --- | --- |
| Stanza `el` | Yes, add MWT | Strong direct UD result | Best fit | UD-GDT non-commercial license; historical official scores |
| Dilemma `el` | Yes | Stronger self-reported Modern Greek result | Promising | Young project, large artifacts, separate bake-off needed |
| Trankit `greek` | Yes | Strong older UD result | Moderate/poor | XLM-R/torch footprint and maintenance risk |
| spaCy Greek | Yes, plus NER/vectors | Lower published lemma result | Moderate | CC BY-NC-SA model license |
| UDPipe | Yes | No current Greek evidence found | Best footprint | Likely lower accuracy |
| GreekBERT alone | No | Not applicable | Not applicable | Requires task heads and training |

## Recommended Next Experiment

Run the same representative Modern Greek corpus through:

1. Stanza `el` with `tokenize,mwt,pos,lemma,depparse`.
2. Dilemma `Tagger(lang="el")` for the full tag/POS/dependency output, plus
   `Dilemma(lang="el", convention="triantafyllidis")` as the lemma-only
   comparison and canonical-form reference.
3. Trankit `greek`, only if its current dependencies can be isolated.

Measure exact-match lemma accuracy against a manually reviewed sample, lemma
coverage, false lemma rate, MWT/token alignment, sentence boundaries, CPU
memory, cold-start time, and throughput. Give extra weight to inflected verbs,
ambiguous forms, proper names, accentless text, punctuation, and contractions.

Until that experiment is complete, the pragmatic choice is Stanza with Greek
MWT enabled, while keeping Dilemma as the leading candidate for a later
lemma-focused provider or fallback.
