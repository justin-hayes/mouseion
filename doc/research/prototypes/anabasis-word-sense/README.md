# Throwaway: Book 1 dictionary sense choice

For [Prototype: word sense in context for Ancient Greek without an LLM](https://github.com/justin-hayes/mouseion/issues/2130),
on the **unmerged** `prototype/anabasis-word-sense` branch. No production changes,
LLM calls, model weights, database, queue, or NLP service. No PR or merge intended.

**Status: live human sense review in progress (6/24 occurrences); no sense accuracy measured.**

Open [`evidence.html`](evidence.html) directly in a browser. It contains all inputs,
fixed baseline outputs, native dictionary senses and memory-only review controls.
Downloads from that page are **drafts**, never automatic human confirmation. The
suggestions are hidden initially, but this is not a blinded experiment: the
sample is already visible and any agent sense proposals are explicitly marked.

## Checkpoint and evaluation contract

- `alignment-gold.json` is byte-for-byte the human-confirmed alignment checkpoint
  at `6455853e`, from the separately accepted alignment prototype. Eight sentences,
  24 selected targets, 41 approved English-token links, not full sentence annotation.
  Its provenance and limitations remain authoritative. It supplies **no sense labels**.
- `inputs.json` preserves each occurrence's OGA identity and native morphology,
  separate dictionary entries, pinned LSJ native IDs and Kaikki record/path IDs, gloss breadcrumbs, labels,
  LSJ hierarchy and sense-local citation attributes. Source text is not repaired.
- `predict.py` reads only these inputs. It cannot read human sense gold. Freeze
  this code and its predictions before scoring; post-checkpoint changes must be
  labelled a new exploratory run, not a held-out improvement.
- `evaluate.py` refuses to score until a complete, explicitly confirmed
  `sense-gold.json` exists. Every target must have a separate reviewed acceptable
  source-identity set for **each** dictionary. Empty means no accepted sense, not an
  unreviewed omission. Never infer a sense equivalence between lexica.
- Chosen-sense accuracy = accepted native IDs / non-abstained choices; with no
  choices it is undefined. Abstention rate = no choice / all 24 occurrences.
  Correct yield includes abstentions in its denominator. Wrong choices and
  abstention examples are retained per dictionary and strategy. A complete short
  entry or evidence-only display is **not** scored as a contextual Gloss.
- Translation projection receives the **approved human links**. This is an
  oracle-alignment diagnostic, not validation of unattended alignment. The previous
  prototype did not accept its automatic baselines for unattended cards.

## Fixed cheap baselines

All methods keep separate native entries. Kaikki requires explicit UPOS agreement;
LSJ POS is often unknown and that weakness is disclosed. Multiple remaining entries
or any numbered OGA identity without a reviewed crosswalk trigger identity
abstention. Equal homograph digits are never used. Unique retrieval is only a
candidate identity, not proof of semantic equivalence.

1. **Morphology / labels / dictionary order:** apply explicit Kaikki number/voice,
   tense, participle and degree labels
   against the native OGA postag; rank matches, disprefer marked non-Attic/poetic
   labels, then source sense order. LSJ grammar mentions in prose are not assumed
   to be exclusive constraints, and missing labels are unknown. No syntax or case
   government is inferred from the production placeholder dependency skeleton.
2. **Translation projection:** lexical content-word overlap between the approved
   English realization and the candidate node's gloss (LSJ own glosses), with a
   small generic suffix heuristic. No overlap → abstain. More overlap wins;
   morphology and source order break ties. No hand-made bilingual synonyms.
3. **Unique projection:** require exactly one compatible source-sense/path candidate
   with positive overlap. Repeated Kaikki paths can make this conservative gate
   abstain; they are not claimed to be distinct meanings. This is an explicit
   abstention heuristic, **not calibrated confidence**.
4. **Evidence only / no contextual Gloss:** always abstain from contextual sense
   choice. Source-attributed dictionary evidence may be displayed separately;
   whether that is useful on cards remains a human decision.

Kaikki preserves every raw source sense row and creates candidate nodes for exact
gloss-prefix paths, including parents otherwise present only as breadcrumbs.
Every source-row/depth path remains distinct even when gloss text repeats: raw
construction notes may differ. Their local IDs are **not**
source-provided universal sense IDs. A supported parent does not select one of its
narrower children. Child-only tags are not imposed on parent candidates.

These are narrow baselines, not a claim to exhaust non-LLM WSD. LSJ prose grammar,
citations and inherited labels are retained for inspection, but not automatically
interpreted as token-level truth. Parent and leaf senses remain distinct. There
is no automatic short-Gloss writing step: an accepted native dictionary sense
would not by itself establish that its verbatim cue is good card wording.

## Exact source pins and rights

The prerequisite [lexicon research](https://github.com/justin-hayes/mouseion/blob/c3fdfbd7eec9aa4cf7a73cdf06dec0e9046ca9e4/doc/research/ancient-greek-lexica.md)
and its SOURCES/NOTICE files own the source survey. This experiment does not repeat
that survey or settle global shipping rights.

| Input | Pin / exact SHA-256 |
| --- | --- |
| OGA `oga_src` v0.2.0 CoNLL-U | `mgbilby/oga_src@223d2258745ecf35e7659191080f93640fe900c1`, Anabasis member; `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61` |
| Brownson English | `PerseusDL/canonical-greekLit@0189b9c8d8d1f148133f2f0af309b4ee90ea4332`, `perseus-eng2.xml`; `f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7`; reviewed extraction reused, no fresh source normalization |
| Raw Kaikki gzip | `https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz`; `3dac8a09e57827bef493e2e6b552fe917bf3b1fc55dee05c4e4a3702aff5dbdb`; dump 2026-09-02, extraction 2026-10-03, Wiktextract `1a05e46f9efbccda6a2b2f8e21b30a9c0c46513a`, wikitextprocessor `e3d6d4edb77618f4d6680edc66e3f774bea59820` |
| Derived raw `grc` subset | 68,196 rows; `65c795d9ca6e1df91dfbdbe288697d576afa7c289a361512431c8a0c24727a12`; ordinal IDs refer to this exact ordering, never a future mutable download |
| LSJ archive | `https://codeload.github.com/PerseusDL/lexica/zip/56061ca127f4a2844980baffc5f2b6d1332897b3`; `8d5bb101f912a9b0a8e09ecaa622b6a31d0dc895803a36e926ddcb63d0dfa910` |

Greek text/annotations: OGA, `oga_src`, Perseus/canonical-greekLit, CC BY-SA 4.0.
English: Xenophon, translated by Carleton L. Brownson (1921–22), Perseus/Tufts
digitisation, CC BY-SA 4.0. Wiktionary definitions: English Wiktionary contributors,
CC BY-SA 4.0; each selected record links its source page and preserves a raw-line
hash. The bounded extract retains **no Kaikki examples/English example translations**.
LSJ: H. G. Liddell, R. Scott, revised H. S. Jones / R. McKenzie, Clarendon 1940;
Tufts/Perseus digitisation (Gregory Crane, Lisa Cerrato, David Smith and contributors).
Its component README grants BY-SA 4.0 with preservation/credit/offer-back wording:
preserve its notice, credit Tufts/Perseus and offer modifications to the Perseus
Project. The underlying 1940 edition's applicable territorial rights and extra
wording still require a recorded shipping review. This prototype does not approve
new deck redistribution. Middle Liddell and other uncleared candidates are not used.

Bounded source extracts and derived alignment/sense review data retain
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) attribution and
share-alike obligations. Extraction and proposed correspondences by the OpenAI
assistant for Justin Hayes, 2026-10-17. Record human confirmation only when it
actually occurs. Code remains under the repository licence.

## Reproduce

Open the committed HTML without installing anything. Rebuild predictions/demo:

```sh
python3 doc/research/prototypes/anabasis-word-sense/predict.py
python3 doc/research/prototypes/anabasis-word-sense/render_demo.py
```

`source_paths.py` can rebuild just the derived Kaikki candidate paths from the
retained raw `source_senses`, without downloading or changing dictionary sources.
The original checkpoint `03207830` shared identical gloss prefixes; source review
for the second batch revealed distinct “depart” construction rows. Before any
scoring, this was corrected to retain every raw row/depth. The first six confirmed
IDs and their source meanings are unchanged. Original predictions remain available
at that commit; the revised checkpoint is exploratory, not a held-out improvement.

Optional re-extraction needs the exact pinned source files in
`~/.cache/scratch/anabasis-word-sense-2130/{oga.conllu,lexica.zip,raw.jsonl.gz}`.
Fetch the URLs above (OGA's raw URL is also in `alignment-gold.json`). The sources
are large and the raw Kaikki URL is mutable: a hash mismatch must stop the run,
not silently substitute a newer extract. `prepare_inputs.py` verifies all pins,
uses the real Go `ancient-greek/1` normalizer/Unicode 17, reconstructs LSJ sibling
levels, and discloses unsupported key notation and unrecognised named entities.
It never fetches external DTD/entities. Then run the two commands above again:

```sh
python3 doc/research/prototypes/anabasis-word-sense/prepare_inputs.py \
  "$HOME/.cache/scratch/anabasis-word-sense-2130"
```

After the separate live checkpoint only:

```sh
python3 doc/research/prototypes/anabasis-word-sense/evaluate.py
```

No prototype tests or production code are added. Reproduction/hash checks are
plumbing checks, not semantic validation or a statistical reliability guarantee.
