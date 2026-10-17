# Faust I: bounded pinned-production NLP validation

Measured: 2026-10-17 · Accepted as bounded evidence, **not feature acceptance**.

This report informed [Basic poetry support](../features/basic-poetry-support.md)
and [Decision: accept the Faust I poetry specification and handoff](https://github.com/justin-hayes/mouseion/issues/2151).
It tests a candidate extraction simulation with the real production German
Producer, plus the actual Go card-quality gate with one rejection removed.
It implements neither v2 extraction nor the accepted per-source-line noise
detector, role-aware selection, persistence, adoption, web presentation or Anki.
No catalog-to-Anki feature run, whole-Book omission rate or NLP accuracy score
is claimed.

## Reproduction and identities

The separate, never-merged
[reproduction asset at `355c5b6f`](https://github.com/justin-hayes/mouseion/tree/355c5b6f835893820086b47b3e032365b2805767/doc/research/faust-pinned-model-validation)
contains ordered executable commands, extraction/sample selectors, Producer
runner, test-only Go overlays, aggregators, tokenizer checks and a checksum
manifest. Run those commands against the pinned application revision, not a
later implementation. No full EPUB, extracted work, Gutenberg header/licence
fixture or raw corpus is committed. Fetch the EPUB locally and fail on a digest
mismatch; a changed upstream download is not the same experiment.

| Input | Identity |
| --- | --- |
| Application source | [`2993f171035ea3e5bed4b1f004f9453828a92115`](https://github.com/justin-hayes/mouseion/commit/2993f171035ea3e5bed4b1f004f9453828a92115) |
| Faust input | `https://www.gutenberg.org/ebooks/21000.epub.noimages`, 179,103 bytes; SHA-256 `93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01` |
| Prose control | Repository `nlp/testdata/german_prose_werther_excerpt.txt`; SHA-256 `e066b0ee1c3fff0bfca6a7f4a45fedf0c9148e7bbea6106df19110aaf9865a1e` |
| Environment | Locked repository environment; Python 3.13.5, Stanza 1.15.0, torch `2.14.1+cpu`, CPU, two torch threads; Go 1.27.2 linux/amd64 for the gate |
| Resource manifest | `stanfordnlp/stanza-resources` commit `569db9d983a47931673d7d265361ca93af85d176`, `resources_1.15.0.json`; SHA-256 `ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463` |
| Model source | `stanfordnlp/stanza-de` revision `6b928a48d4c8d8096b9dc75c887da3652f8ef671`, the measured `v1.15.0` revision; eight artifact SHA-256 values in the asset |
| Actual runtime processors | `tokenize,mwt,pos,lemma,depparse`; Stanza adds German MWT even though the configured processor string omits it |
| Runtime packages | `tokenize/combined_nocharlm`, `mwt/combined`, `pos/combined_charlm`, `lemma/combined_nocharlm`, `depparse/combined_charlm`; `conll17` pretrain and forward/backward `newswiki` charLM |

Eight downloaded runtime artifacts are individually SHA-256-pinned in the
asset. The capability's reported model version is just `1.15.0`; it is not a
substitute for the package/resource/artifact identities. No models or package
configuration were changed to improve the result.

The published probe commands were rerun successfully with the previously
provisioned locked environment and checksum-matching models. The added
cold-provisioning recipe was not rerun: its resource URL/digest and model source
revision's eight LFS SHA-256 values were checked without a second model download.
Raw JSON digests vary with paths, timing and set-derived key order; the asset's
canonicalizer removes volatile metadata and sorts keys. Its verified canonical
Producer-output digests are:

```text
bf674799ce59eb2f91419e230da512546bb00e972357998f463f969229a541a8  out1/producer_results.json
56ccbac336dbec7096879c2825893ea23577317dfea6d9a40414d39f4259788b  out2/producer_results_rebased.json
c8cbeaece303a2aa2435a1ecbc5f1a00339b6f67093afe4eabd685bc93baa461  out2/werther_paragraphs_kept.json
```

## Extraction simulation and provenance correction

The corrected simulation mirrors production readable-unit **selection**: skip
nonlinear/non-XHTML/nav spine items, omit hidden head/script/style/nav text, and
drop empty units. It applies the **candidate**, not current production, break
encoding: `<br>` is LF and block boundaries are double LF.

| Scope: complete readable body, including main-text fallback matter | Count |
| --- | ---: |
| Spine items | 8 |
| Kept nonempty units | 7 |
| Line groups | 2,275 |
| Source lines across groups | 5,952 |
| Multiline groups | 712 |
| `<br>` elements | 3,678 |
| Internal source LFs | 3,677 |

The extra bare `<br>` is outside a text block and creates no nonempty group.
Thus 2,275 groups + 3,677 internal LFs = 5,952 lines. The Director's first
speech is one paragraph with 25 `<br>` elements: **26 lines**, not 26 sentences.
These are whole-readable-snapshot counts, not literary-only yield measures.

The original simulation wrongly included the cover title and XHTML head titles.
The corrected selection removes exactly eight one-line groups. Every measured
EPUB sample was matched by text hash and position to the corrected body groups:
21 individual groups and three combined runs. Only unit-relative offsets/group
ordinals were rebased; sample text and analysis are unchanged. The rebased Go
gate's 1,146 decision rows equal the original rows as parsed JSON.

The earlier structure decision's heuristic **5,887 lines / 2,212 group breaks**
was not reproduced and has no located executable source. Do not use it as an
acceptance oracle. The body-only counts above supersede those measurements,
without changing the accepted break rules. Similarly, the inherited claim that
the `Was fällt …` source line must contain two analyzer sentences is a prediction
contradicted by this real model, not a required boundary to force.

## Exact source alignment and boundaries

- **171/171** measured sentence strings equal their authoritative code-point
  slices: 54 individual EPUB sample sentences, 18 repeated in combined-group
  probes, and 99 from the as-file Werther excerpt. Duplicates are intentional;
  171 is not the quality-gate denominator.
- Token checks found **3,069 exact source spans**, **9 cleaned-edge differences**
  and **66 repeated multiword-token spans**, with no unexplained mismatch in
  these probes. These categories explain the producer's evidence, not a claim
  that every surface token is a unique exact word slice.
- None of the three combined runs produced a sentence crossing a double-LF
  group boundary. Single-LF sentences cross authorial lines as intended. This
  is bounded observation, not general validation of every EPUB or analyzer.
- `Was fällt euch an? Entzückung oder Schmerzen?` stays in **one** Stanza
  sentence when isolated, with the preceding line and in its full group. Test
  the general two-sentences-on-one-line rendering case with supplied analyzer
  evidence, not an inaccurate assertion about this model's punctuation split.
- `<p class="speaker">Valentin <span class="sd">fällt.</span></p>` is a speaker
  label enclosing a direction, not an independently eligible direction. Under
  the accepted role policy, both tokens are non-vocabulary regardless of their
  parsed POS. A genuine separate direction (`Er schlägt das Buch auf …`) supplies
  the positive eligibility example.

## Quality-gate ablation — not the final quality policy

The test-only Go overlay removes only `strings.Contains(text, "\n")` from
`listOrPageFragment`. It leaves the rest of the production scorer untouched.
This diagnoses the cost of the newline veto; it does **not** implement the
accepted content-sensitive checks on every source line or structural role
exclusions.

A **sentence instance** is a sampled corpus sentence, keeping duplicates. It is
accepted here if at least one of its target decisions passes. A **decision** is
one sentence/target gate evaluation; these are not independently selected Book
vocabulary items or final cards.

| Corpus | Instances | With targets | Zero-target | Decisions | Accepted instances: baseline → ablation | Accepted decisions: baseline → ablation |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Sample, including as-file Werther | 155 | 153 | 2 | 1,146 | 9 → 48 | 41 → 424 |
| Same sample plus paragraph-preserving prose control | 254 | 252 | 2 | 1,964 | 36 → 75 | 300 → 683 |

All **39** newly accepted sample instances contained LF; there were no reversals.
They are not all prose:

| Sample | Newly admitted sentence instances |
| --- | ---: |
| Director's first speech | 5 |
| `Was fällt …` surrounding group | 5 |
| Thule stanzas 2–6 | 5 (one each) |
| Visible correction-markup sample | 1 |
| Hard-wrapped as-file Werther excerpt | 23 |

The first four rows total **16 literary sentence instances** with genuine
`<br>`-derived authorial breaks. The 23 Werther flips are a hard-wrapped-file
artifact: contract extraction does not preserve incidental source wrapping.
The `Was fällt …` full-group case is 2/13 → 7/13 accepted sentence instances.
A targeted reprint verifies the selected 211-code-point, four-LF sentence as
**0/17 → 17/17 target decisions**. The first Director sentence (136 code points,
three LFs) is **0/12 → 12/12**. These are separate denominators, not counts of
cards or evidence that the accepted targets' linguistic identities are correct.

## Prose control: keep paragraph boundaries

An interim probe flattened **all** whitespace and incorrectly called that the
contract form. It erased paragraphs. Correct contract-form control D replaces
incidental single LFs with spaces but preserves blank lines as double LF.

| Werther input | Tokenizer paragraphs | Featurized units | Stanza sentences |
| --- | ---: | ---: | ---: |
| A: as-file wrapping and blank lines | 19 | 10,713 | 99 |
| B: wrapped LFs but no blank-line boundaries | 1 | 10,731 | 94 |
| C: incorrectly flattened whitespace | 1 | 10,731 | 94 |
| D: paragraph-preserving contract control | 19 | 10,713 | 99 |

For the pinned tokenizer, A/D have identical featurized sequences, as do B/C.
Single LF becomes space before featurization; the checkpoint has
`start_of_para`/`end_of_para` features. The 99→94 gap was loss of paragraph
boundaries in the probe, not evidence against the contract. Do not generalize
this into an unmeasured promise of identical downstream POS/lemma/parse output
for arbitrary whitespace changes.

Control D's quality outcome is **27/99 accepted sentence instances in both
baseline and ablation** (259/818 target decisions in each). The as-file control
is 4/99 → 27/99. Prose regression acceptance uses D, not the erroneous flat
collapse or as-file newline gains.

## Linguistic limitations and remaining gates

Reviewed problems include `sag’` → `sagsen`, `schwanken` → `schwinken`, a missed
`compound:prt` in `fängt … an`, erroneous `losabmachen`, and `hatt’` parsed as
`PROPN`. The first Director sentence also misparses historical `beyden` as a
finite verb. Exact offsets and a passing quality gate do not establish correct
vocabulary identities or meanings.

The human accepted the existing correction/exclusion workflow and honest
omissions for these bounded historical-language limits. No tuning,
modernization, general accuracy percentage or minimum yield is required.
Curated acceptance cards still require correctly reviewed identities and target
emphasis; the observations above must not become gold linguistic labels.

The feature's acceptance matrix remains future work: implemented extraction,
full selection/count/freeze consistency, real persistence/races, catalog
acquisition, web/accessibility behavior, provider meanings/alignment and actual
Anki import/re-import. Deterministic fixtures, package inspection and a
test-only ablation cannot substitute for those named release checks.
