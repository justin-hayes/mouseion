# Book 1 non-LLM alignment: measured prototype findings

2026-10-17. Throwaway evidence, **awaiting final live acceptance**. No production
behaviour or accepted ADR changes. For
[Prototype: aligning public-domain English to Book 1 without an LLM](https://github.com/justin-hayes/mouseion/issues/2129).

## Recommendation for human review

**Do not accept any of these cheap automatic baselines as an unattended source
of exact sentence translations and target emphasis.** The reviewed examples look
good because their source spans and highlights were proposed and then confirmed
by Justin. That is not what the automatic systems reproduced.

Punctuation, length, proper-name anchors and dictionary anchors each achieve
**5/8 exact sentence-span sets**, with different failures. Dictionary links are
**11/14 precise (78.6%) but recover only 11/41 approved token links (26.8%)**;
only **8/24 complete target highlights** match the reference. Statistical learning
from this tiny coarse parallel corpus is weaker. Strict agreement abstains on
almost everything and still shares incorrect sentence boundaries.

This is a negative result for these implementations/data, **not proof that all
non-LLM alignment, fast_align, eflomal, a larger corpus, a richer lexicon or an
expert-curated alignment could not work**. The sample cannot establish a safe
confidence threshold. These methods can make review suggestions; the evidence
does not justify advertising their unreviewed output as a sentence translation.

Proposed uncertainty contract, **not yet accepted**:

- With these prototypes, an unreviewed automatic alignment remains uncertain
  even when heuristics agree. There is no validated automatic acceptance gate.
- Confirmed sentence span but uncertain target correspondence: show the complete
  confirmed English translation **without target emphasis**.
- Uncertain sentence extent: show an explicitly labelled **wider passage
  translation**, without target emphasis; never label the whole section as this
  sentence's exact translation. The demo also offers withholding English for
  Justin to compare. A neighbouring-section window may be necessary for a crossing.
- Known edition mismatch or no trustworthy passage: withhold exact translation
  and explain that it is unavailable. Do not silently rewrite Brownson or drop
  every bracketed passage.
- No raw model probability, heuristic agreement or observed 100% on two/four
  links is a calibrated confidence guarantee. Sentence and word-link uncertainty
  are separate gates; correct word emphasis does not validate its wider sentence.

Open [`evidence.html`](evidence.html) directly in a browser for the measured
strategy comparison, per-target reference/output, and uncertainty experiments.

## Human checkpoint and frozen assets

Gold was committed at **`6455853e` before automatic evaluation**:
[human-confirmed gold](https://github.com/justin-hayes/mouseion/blob/6455853e/doc/research/prototypes/anabasis-alignment/gold.json),
[verbatim live-review ledger](https://github.com/justin-hayes/mouseion/blob/6455853e/doc/research/prototypes/anabasis-alignment/review-status.json).
Justin explicitly confirmed all eight sentence-span sets and all 24 selected
target highlights in four batches, after clarification and card-style examples.
No corrections were requested. The comma-ending extent and cross-section extent
were explicitly accepted for their examples. This does not accept a general
fallback policy or an automatic system.

The sample spans 6–39 printed words, sections containing 1–4 and 10 sentences,
one crossing, an editorial mismatch and a Brownson-attributed existing alignment.
It is purposive and proposal-biased, not independently annotated or expert
certified. It contains **41 Greek-target/English-token pairs**, all marked sure
by the accepted highlighting; no NULL/possible/unresolved examples were added
by the agent. It does not cover all words in these sentences. English links are
not dictionary-sense labels for
[Prototype: word sense in context for Ancient Greek without an LLM](https://github.com/justin-hayes/mouseion/issues/2130).

The initial implementations and extracted inputs were frozen in **`ee3cb053`
before their first scoring run**. After observing that seven target occurrences
were absent from held-out training, a separately labelled all-section training
diagnostic was added, without changing algorithm/scoring parameters. The main
run was repeated unchanged. Development was not blinded: reviewed examples were
visible while writing the prototype. There was no hyperparameter search.

## Sources and import correspondence

The [source research](https://github.com/justin-hayes/mouseion/issues/2128)
selected Brownson and OGA; exact URLs, sizes/digests and extraction are retained
in the JSON assets and [checkpoint README](README.md).

- OGA/`oga_src` v0.2.0: commit `223d2258745ecf35e7659191080f93640fe900c1`,
  whole-work CoNLL-U SHA-256
  `c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61`.
- Brownson: canonical-greekLit commit
  `0189b9c8d8d1f148133f2f0af309b4ee90ea4332`, `perseus-eng2.xml` SHA-256
  `f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7`.
- The real no-database import returned **607 sentences, 10,462 printed tokens,
  110 omitted ellipsis nodes**. Every sentence text and printed token surface,
  raw lemma and codepoint offset matches the extraction sidecar.
- 205 section keys match. First-section ownership gives 32 one-sentence sections,
  but counting every touched section gives **31**. Sentences 103, 155 and 445
  cross 1.2.17–18, 1.3.5–6 and 1.8.5–6. Section 1.4.8 contains ten sentences.
  Most sections own 2–4 (146/205). These are not one-to-one bilingual sentence pairs.

No database, queue, NLP service or external provider was used. Source Greek is
preserved with diacritics; offsets index the actual import reconstruction.
English offsets index note-free whitespace-collapsed TEI section strings,
retaining note tails and main-text brackets; milestones supply a word boundary.

## What was actually compared

All implementations are Python standard-library, throwaway code in `compare.py`.
The statistical model is an explicit small **IBM Model 1 EM baseline**, not the
fast_align/eflomal package or a reproduction of its reported results.

| Strategy | Behaviour |
| --- | --- |
| Whole-section control | Use the entire touched section(s); deliberately demonstrates why section correspondence is not sentence alignment. No bilingual word-link output. |
| Punctuation/order | Split at `. ! ? ; :`, pair by ordinal within the first section; abstain if no corresponding English unit. No bilingual vocabulary links. |
| Length | Monotonic dynamic programming over English token boundaries, preferring punctuation, with a character ratio estimated on training sections. Join connected section components for real OGA crossings. No bilingual vocabulary links. |
| Length + names | Add uppercase proper-name transliteration-similarity anchors. These are segmentation hints, not selected vocabulary links. |
| Dictionary | Add POS-compatible dictionary-gloss word anchors to length allocation; link target to all matching non-stopword gloss words with a generic suffix heuristic. Union senses, not contextual disambiguation. No hand-authored bilingual seeds. |
| Statistical top word | Length segmentation, then best lexical probability for the Greek lemma among English words in the assigned extent. From-scratch forward IBM 1, with NULL, 20 EM iterations. |
| Statistical mutual | Require the best forward word and best reverse lemma to agree; 20 EM iterations each direction. This is deliberately sparse, not full phrase/fertility alignment. |
| Agreement | Require dictionary/name-length span agreement, then intersect dictionary word matches with mutual statistical links. No calibrated confidence interpretation. |
| Existing Ugarit | One pinned Brownson-attributed contributor record, bound by occurrence to OGA/Brownson; native link groups expand to many-to-many links. Infer sentence extent from linked word hull plus terminal punctuation. No copying gold boundaries. |

Length costs and fixed rewards are recorded in `results.json`. The methods
retain source wording rather than generating or paraphrasing English. No learned
model, dictionary sense or source-edition repair oracle is smuggled in.

## Primary measurements: reviewed sections excluded from statistical training

Train on **196 whole-section pairs**, excluding all nine sections touched by
the gold sample: 8,854 Greek and 13,284 English word occurrences, 1,408 Greek
lemma types and 2,039 English types. Seven of the 24 selected target occurrences
have **zero** training examples. All section pairs are coarse/noisy, not verified
sentence pairs. Full per-case predictions: [`results.json`](results.json).

| Strategy | Exact sentence spans | Word precision | Word recall | Complete target highlights |
| --- | --- | --- | --- | --- |
| Whole section | 1/8 (12.5%) | —, no links | 0/41 (0%) | 0/24 |
| Punctuation/order | 5/8 (62.5%); predicts 7/8 | —, no links | 0/41 (0%) | 0/24 |
| Length | 5/8 (62.5%) | —, no links | 0/41 (0%) | 0/24 |
| Length + names | 5/8 (62.5%) | —, no links | 0/41 (0%) | 0/24 |
| Dictionary | 5/8 (62.5%) | 11/14 (78.6%) | 11/41 (26.8%) | 8/24 |
| Statistical top word | 5/8 (62.5%) | 5/17 (29.4%) | 5/41 (12.2%) | 2/24 |
| Statistical mutual | 5/8 (62.5%) | 4/4 (100%; only four links) | 4/41 (9.8%) | 2/24 |
| Agreement | 5/8 (62.5%) | 2/2 (100%; only two links) | 2/41 (4.9%) | 2/24 |
| Existing Ugarit | 1/8 overall; **1/1 covered** | 4/5 (80%) | 4/41 (9.8%) overall; **4/4 covered** | 2/24 overall; **2/3 covered** |

All methods except punctuation and Ugarit predict an English extent for all
eight examples, including incorrect extents. Sentence accuracy requires the
exact **set of section-relative spans and punctuation**, not merely containing
the right phrase somewhere in a larger passage. Link precision/recall use fixed
English token identities for the selected Greek occurrences. Abstentions remain
in recall; no predictions makes precision undefined, not 0% or 100%. Whole-section
word recall is zero because it offers no target links, not because a bilingual
aligner was run and mislinked every word. Missing Ugarit coverage is not seven
observed alignment errors.

## Data-availability diagnostic: all 205 unlabelled section pairs

Since the intended task aligns a known book, it is legitimate to let an
unsupervised model see **all source/translation text**, including the evaluation
sections, without their human links. This is **not held-out evaluation** and was
added after the initial run; it prevents confusing held-out vocabulary absence
with an unavoidable limitation of the intended deployment.

Full output: [`results-transductive.json`](results-transductive.json).
205 coarse pairs, 9,305 Greek / 14,027 English word occurrences; 1,449 / 2,110
types. All selected targets now occur in training. No gold annotations are
training inputs. The length ratio is re-estimated on the larger text; all four
automatic segmentation variants still score 5/8.

| Word strategy | Precision | Recall | Complete targets |
| --- | --- | --- | --- |
| Statistical top word | 7/24 (29.2%) | 7/41 (17.1%) | 4/24 |
| Statistical mutual | 6/8 (75%) | 6/41 (14.6%) | 4/24 |
| Agreement | 4/4 (100%; only four links) | 4/41 (9.8%) | 4/24 |
| Dictionary | unchanged, 11/14 (78.6%) | unchanged, 11/41 (26.8%) | 8/24 |

The all-text diagnostic removes OOVs but does not rescue the baseline. More
**independent occurrences and cleaner parallel units**, not just more copies of
the same section, are needed to distinguish co-occurring rare lemmas. These runs
do **not** measure how many additional pairs would be sufficient. No learning
curve or “N pairs guarantees reliability” claim is justified. Wider books or
external training corpora require their own correspondence/licence checks.

Separately labelled **oracle-boundary diagnostics** feed each word method the
confirmed sentence spans: these are not automatic sentence results. In the
primary run, dictionary/top/mutual link counts happen to be unchanged. On all
text, top-word rises to 8/24 precision, 8/41 recall; mutual to 7/9, 7/41. Even
perfect human boundaries do not make the word baseline adequate. The oracle
8/8 sentence score is supplied truth, not an alignment achievement.

## Concrete failures, with approved references

1. **Whole sections:** 1.1.1 contains two imported Greek sentences. Giving all of
   its English for sentence 1 includes the following illness/death context;
   section-level correspondence is not the card's exact sentence translation.
2. **Ordinal punctuation drift:** in ten-sentence section 1.4.8, Greek sentence
   238 (“ἔχω γὰρ τριήρεις…”) is paired with “To be sure, I have their wives and
   children under guard…”, not the approved “for I have men-of-war…”. At the
   1.2.17–18 crossing, the ordinal method has no fourth English unit in the first
   section and emits no span. Length can recover these two cases.
3. **Length/name/dictionary boundaries:** sentence 15 wrongly starts with
   “that he gathered this force:” before the approved “In the first place…”.
   Sentence 536 wrongly includes “he secured the services of an army worthy of
   the name.” from the preceding Greek sentence. Name and gloss anchors do not
   change these outcomes; common anchors do not uniquely identify a boundary.
4. **Editorial mismatch, sentence 428:** all automatic splitters include at least
   part of the canals aside absent from OGA. Length/name/dictionary include through
   “each a plethrum wide and exceedingly deep,” rather than ending at “Media,”.
   Dictionary/statistical agreement returns the correct **trench** and **plain**
   word links inside this incorrect wider span. Its 100% word-link precision on
   two predictions is therefore especially misleading as a sentence gate.
5. **Discontinuous/multiword highlighting:** dictionary links γίγνονται only to
   “born”, not the confirmed “had … born to them”; παραγγέλλω only to “orders”,
   not “sent orders”. Mutual statistics recover “chanced” but not the full
   “as it chanced”. Token precision can look good while complete highlights fail.
6. **Dictionary senses/wording:** παῖς has “child/slave/servant” but the selected
   English says “sons”; πλοῖον has “ship/boat”, but Brownson says “craft”.
   φόβος matches “flight”, whereas the reviewed correspondence is “frightened”.
   φεύγω matches “run” and “direction” in an earlier clause instead of “took to
   flight”. Literal gloss overlap is not contextual word/sense alignment.
7. **Sparse statistics:** in the primary run γίγνομαι chooses “was”, παῖς “them”,
   τριήρης “with” and αἱρέω “I”. Seven targets are unseen. All-text training fixes
   vocabulary coverage, but still has 17 incorrect top-word links among 24 guesses.
8. **Partial existing alignment:** Ugarit gives κερδαλεώτερον → “worth more” and
   πειθαρχεῖν → “obedience”, matching gold, but κέρδος → **“their pay”** rather
   than the accepted **“pay”**. The extra “their” is an overbroad highlight under
   this reference, not proof that the contributor's interpretation is semantically
   wrong. It covers only one gold sentence, so it cannot replace a Book 1 corpus.

Every strategy's eight cases and each target's proposed/gold token positions,
words, TP/FP/FN are in the result JSON and the standalone demo. These are measured
failures against the confirmed markup, not agent-invented human labels.

## Dictionary and existing-source provenance / non-NC checks

No pretrained model was downloaded or used. The statistical tables are trained
from scratch in memory on the pinned OGA/Brownson text; both digitisations carry
CC BY-SA 4.0, not NC. No PROIEL, UD Perseus, Gorman or uncleared training resource
is used. The previously researched Ugarit pretrained checkpoint remains excluded
because its identified inputs have NC/conflicting notices. Treebank provenance
alone is not the exclusion under Justin's clarified rule.

**Dictionary:** [`dictionary-subset.json`](dictionary-subset.json) retains 1,368
separate raw-Kaikki entries selected from the 68,196 Ancient Greek rows of the
existing local raw cache. Compressed cache SHA-256:
`42e0bfe1669513cb89bd0a12f2e85d1a8f4ebedc70c65ec88c9509d0f5d392f9`,
2,863,667,287 bytes. Each selected source row is hashed and attributed to its
English Wiktionary page/contributor history. The local cache does not retain its
original dump date/extractor revision; this is explicitly **not** inferred from
the current Kaikki website. The committed reduced bytes are the reproducible
prototype input; a future refresh is a new experiment, not silent drift.

The [raw-data source](https://kaikki.org/dictionary/rawdata.html) describes
English Wiktionary extraction. The pinned
[Wiktionary copyright notice](https://en.wiktionary.org/w/index.php?title=Wiktionary:Copyrights&oldid=87545419)
gives CC BY-SA 4.0/GFDL and cautions that quotations/media can have other terms.
This prototype retains **glosses/metadata only**, no examples, quotations, media
or translated example sentences. It follows the CC BY-SA 4.0 attribution and
share-alike route. No NC resource is introduced. This is a narrow existing-source
anchor baseline, **not** resolution of
[Research: openly licensed Ancient Greek lexica for contextual word sense](https://github.com/justin-hayes/mouseion/issues/2133).
It makes no numbered-homograph/sense binding: entries remain separate and all
POS-compatible glosses are candidate evidence. Three distinct selected headwords
have no entry (ἐγκέλευστος, κερδαλέος, πειθαρχέω); ὤνιος is adjectival in the
dictionary but noun-tagged in this occurrence and thus has no POS-compatible
anchor. Proper names are segmentation anchors, not vocabulary cards.

**Existing alignment:** [`existing-ugarit.json`](existing-ugarit.json) is the
bounded extraction from [Jacob Schwartz's Brownson-attributed 1.9.17–19 record](https://www.ugarit.ialigner.com/text.php?id=34674).
Fetched HTML SHA-256
`e5d891142483c7243fbf1869896cb4668eed8ceb9f57ca8c75d62025a36e0995`
matches the source research pin. All 129 Greek / 220 English native tokens bind
by letter-stream occurrence to the pinned source sections, preserving Greek
diacritics; punctuation/token boundaries and the U+02BC/U+1FBD elision-mark
difference are explicitly accounted for. Native IDs/groups, not aggregated TSV
phrase counts, preserve occurrence identity.

Ugarit's site-level CC BY-SA 4.0 notice is not independent proof of contributor
authority. Brownson identity is credited and the underlying text binding is
checked, so this is a useful bounded comparison, not blanket licence/source
acceptance. Retain attribution to Schwartz, Brownson, Perseus and Ugarit with the
derived data. The other researched records were not ingested: uncredited 1.5.12
has unresolved translation provenance; 1.9.24 is partial and not in this gold
sample. They are unavailable as scored comparators here, **not measured failures**.

## Reproduction and checks

Follow the checkpoint README to download/hash the two sources and run the import
probe. For evaluation, use the committed dictionary/existing subsets; no need
to download or rescan the multi-gigabyte cache. Then, from the repository root:

```sh
python3 doc/research/prototypes/anabasis-alignment/prepare_review.py \
  "$HOME/.cache/scratch/anabasis-alignment-2129"
python3 - <<'PY'
import json
from pathlib import Path
import runpy, sys
root = Path('doc/research/prototypes/anabasis-alignment')
scratch = Path.home()/'.cache/scratch/anabasis-alignment-2129'
sys.argv = [str(root/'prepare_review.py'), str(scratch)]
data = runpy.run_path(str(root/'prepare_review.py'))
(scratch/'corpus.json').write_text(json.dumps({
    'sentences': data['sentences'], 'sections': data['sections'], 'pins': data['PINS']
}, ensure_ascii=False))
PY
python3 doc/research/prototypes/anabasis-alignment/compare.py \
  "$HOME/.cache/scratch/anabasis-alignment-2129"
python3 doc/research/prototypes/anabasis-alignment/compare.py \
  "$HOME/.cache/scratch/anabasis-alignment-2129" --transductive
python3 doc/research/prototypes/anabasis-alignment/render_evidence.py
```

`prepare_inputs.py` documents optional dictionary re-extraction from the exact
cached gzip. `prepare_existing.py` documents the digest-checked Ugarit extraction.
Those are provenance tooling, not requirements to use the committed input bytes.
Do not put source/work downloads, model tables or local private import output in
production paths or CI. Scratch is under `~/.cache/scratch`, not RAM-backed `/tmp`.

Gold/source digests, per-case predictions, training regimes and exact prototype
hashes are recorded in the result JSON. Repeated results/demo generation must be
byte-identical. Python uses no external runtime dependency. The initial full Go
unit/lint runs passed; full project Python lint/coverage remain blocked by this
worktree's missing `.venv` as recorded in the checkpoint README. No production
Go/Python source was changed and no prototype tests were added to CI.

Validation passed: independent set-based recount of all nine strategies in both
regimes, unchanged pre-evaluation gold bytes and input/source hashes, byte-identical
comparison/demo regeneration, explicit Ruff checks of every prototype script,
and standalone Chromium inspection of all **432** regime/example/method/target
combinations plus uncertainty actions, no JavaScript errors and no 390px mobile
overflow. Full `make lint` again passed Go lint before stopping at missing
`.venv/bin/ruff`; `make coverage-new` again stopped at missing Python/pytest.

Extracted/adapted source text, dictionary subset, existing links and reviewed
alignment data retain their CC BY-SA 4.0 attribution/share-alike notices; code
remains under the repository licence. Personal review/evaluation does not settle
worldwide copyright status or a future exported-deck redistribution contract.
