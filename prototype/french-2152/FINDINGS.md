# French resource probe: pinned Stanza 1.15.0 and Kaikki sample

**Date:** 2026-10-17. **Ticket:** [Validate pinned French resources and pronominal identity](https://github.com/justin-hayes/mouseion/issues/2152).
**Decision this feeds:** [Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141).
**Repository base:** `6e02d180b1015536127839e0a45953684c0e5a83`. Unchanged code exercised: `nlp/src/mouseion_nlp/producer.py`
(blob `baac34a5`), `nlp/scripts/derive_dictionary_index.py` (blob `85fa6276`), `internal/textmatch/match.go`
(blob `dcd876e9`, copied verbatim into a scratch module).

This is a throwaway decision artifact. Nothing in the application, shared matchers, persisted vocabulary,
source text or frozen snapshots was changed. No full dictionary bundle was fetched.

Evidence labels: **[O]** observed tool output; **[R]** agent review of that output, pending human
confirmation; **[E]** expectation or inference not run here. The sample is 22 constructed diagnostic
strings, so it supports no accuracy rate.

## 1. Exact resources and verification

| Item | Pin | Verified |
|---|---|---|
| Stanza / torch | `stanza==1.15.0`, `torch==2.14.1+cpu` from `nlp/uv.lock` via `uv sync --locked --no-build` (uv 0.13.0) | [O] imported versions |
| Manifest | `stanfordnlp/stanza-resources@569db9d983a47931673d7d265361ca93af85d176/resources_1.15.0.json` | [O] sha256 `ae9741f2…4463`, md5 `462ea52f…ffd942` |
| Model bytes | `huggingface.co/stanfordnlp/stanza-fr/resolve/caeb7742148cb0368e4a0f2c68a0f7b09bc22880/models/…` | [O] `x-repo-commit: caeb7742…` on every response |

The pinned manifest's `fr.default_processors` are the **combined** packages. The probe pins them explicitly
instead of relying on a moving default. NER and coref were excluded.

| Processor | Package | Bytes | sha256 = HF LFS | md5 = manifest |
|---|---|---:|---|---|
| tokenize | combined_nocharlm | 665,712 | ✓ | ✓ |
| mwt | combined | 544,773 | ✓ | ✓ |
| lemma | combined_nocharlm | 2,813,969 | ✓ | ✓ |
| pos | combined_charlm | 34,563,706 | ✓ | ✓ |
| depparse | combined_charlm | 148,809,357 | ✓ | ✓ |
| pretrain | conll17 (pos and depparse dependency) | 106,908,071 | ✓ | ✓ |
| forward_charlm | newswiki (pos and depparse dependency) | 20,094,072 | ✓ | ✓ |
| backward_charlm | newswiki (pos and depparse dependency) | 20,094,073 | ✓ | ✓ |

All hashes are in `out/artifacts.json`. These eight files are the complete runtime set. Nothing else was
loaded with `download_method=NONE` and `HF_HUB_OFFLINE=1`.

## 2. Cost, bounded [O]

Host: Intel Core Ultra 9 285HX, 4 torch threads, CPU only.

- **Download and disk:** 334,493,733 bytes (319 MiB) of models, fetched in 14 s. The scratch venv (full
  `nlp` lock) was 1.1 GiB.
- **Load and memory:** pipeline load took 2.76 s. Peak RSS was 272 MB after imports, 868 MB after load and
  938 MB after the runs.
- **Latency:** per-string analysis took 29 / 36 / 49 ms (min / median / max). A 797-character paragraph of
  all 22 strings took 0.23 s.

These numbers cover tiny inputs on one machine. They are not throughput figures for whole books.

## 3. Analyzer behaviour on the approved sample

Full token/word/span dump: `out/stanza_results.json`.

### What worked [O] [R]

- **Elision.** All eleven elided clitics split as their own tokens, with exact source spans: `l'`, `d'`,
  `s'` ×2 and `m'`, their five curly copies, and `qu’`. ASCII and curly copies produced identical analyses.
- **Contractions and their ambiguity.** `du marché` and `du café` expand to *de + le*; the partitive is
  `det` + `fixed`, the UD-French convention. `au` → *à + le*, `aux` → *à + les*. `prix des pommes` → *de +
  les*, while `avec des pommes` / `des fruits` stay single `DET` (lemma *un*). All six are right by
  review.
- **Content lemmas and POS.** All 72 eligible content tokens (NOUN/VERB/ADJ/ADV) have plausible lemmas.
  Examples: `vois→voir`, `viendra→venir`, `faut→falloir`, `mains→main`, `mêmes→même`,
  `augmenté→augmenter`. Impersonal `Il faut partir` gives `Il` as `expl:subj`.
- **Pronominal signal.** `s'en va` gives `s'` = `expl:pv` and `en` = `expl:comp`, an inherent-pronominal
  marking. Reflexive `s'habille` / `me lave` / `se lave` give `obj` / `obj` / `iobj`. That is one example of
  each, so it is a usable signal, not an established reliability.
- **Pronoun lemmas** follow a treebank convention: il/elle/le → *lui*, je/me → *moi*, se → *soi*. These
  are not vocabulary (`AllowedPOS` is NOUN/VERB/ADJ/ADV), so this is harmless.

### Failures and risks

1. **Closing guillemets are cut off: systematic [O].** After sentence-final punctuation, `»` becomes its own
   sentence or opens the next one. This held with spaces, U+202F narrow no-break spaces, `\n\n` paragraph
   breaks (as EPUB extraction joins them) and inside one paragraph. `« Tu viens ? » demanda-t-il.` splits
   after `?`. Persisted sentence text is therefore `« Je suis sûr qu’il viendra ce soir.`, an incomplete
   source sentence used for cards. A scratch-only re-anchoring (`scripts/quote_merge_demo.py`, sentence
   text re-sliced from source offsets) restores all three dialogue cases and leaves plain prose unchanged.
   This is a demonstration, not a design.
2. **Contraction parts carry no word offsets [O].** Stanza gives MWT words no character spans, so the
   producer persists `de`/`le` (or `à`/`le`) with the whole `du`/`au` span. The Go explorer found this is
   already handled: the reader marks overlapping spans degraded (`internal/webapp/sentence_study.go:153-177`,
   Italian `del`/`dell'` tests), and the ADP/DET halves are never vocabulary. No invented content words
   appear.
3. **Compounds are lost [O] [R].** `petit déjeuner` → `petit` ADJ + `déjeuner` NOUN. The learner would get
   *déjeuner* ("lunch") as a separate item, and Kaikki's `petit déjeuner` entry is unreachable. `d'accord`
   → `d'` root + `accord` `fixed`, so *accord* becomes vocabulary.
4. **Function-like adverbs are eligible [O] [E].** `ne`, `pas`, `près`, `là`, `où` are ADV and pass the
   current POS filter. Whether `ne`/`pas` should be excluded is a French readiness question.

## 4. Target matching in source sentences

All 72 eligible tokens were run through the unchanged `CleanLexicalSurface` + `FoldedWordSpan` path used by
`cardexport.targetIndex` (`out/match.tsv`).

- **8 of 72 failed [O]:** `homme` (l'homme), `accord` (d'accord), `habille` (s'habille), each in both
  apostrophe forms, plus `va` (va-t-il) and `dit` (dit-elle). The lemma fallback failed as well.
- **Cause [O]:** `isWordRune` counts `'`, `’` and hyphens as word characters, so a form directly after an
  elided clitic or before an inversion hyphen has no word boundary. Ordinary elided targets are lost
  **systematically**: any vowel- or mute-h-initial noun after `l'`/`d'`, and any verb after `j'`/`m'`/`t'`/
  `s'`/`n'`. **[E]** Italian `L'uomo` should hit the same path; the Go explorer found no test covering it.
- **Span check [O]:** all 72 persisted token spans slice exactly to their surfaces. Anchoring the card
  target on the token's own offsets would therefore cover all 72 without apostrophe normalization, and the
  source sentence stays unchanged. This was checked in scratch only. An elision-aware boundary rule is the
  alternative, but it needs a French clitic list.

## 5. Kaikki dictionary evidence

**Input.** I requested 52 titles and fetched 47 per-word French JSONL pages (69 entries) of the
post-processed site data (`out/dict_manifest.json` has URLs, sha256 and entry counts). Five titles do not
exist: `se laver`,
`s'habiller`, `s'apercevoir`, `se lever` and `en aller`. Mouseion's derivation reads the **raw** file, so I
checked the per-word pages against raw records: a capped 3,000,000-byte prefix of
`raw-wiktextract-data.jsonl.gz` (sha256 `d1a32e11…e153`, same as the provenance probe) contains 109 `fr`
records. For 5 words / 10 records (`livre`, `chien`, `minute`, `abdomen`, `gratuit`), every field the
derivation reads matched exactly (`out/raw_vs_post.json`). That justifies using the pages as a proxy for
this sample only, not in general.

**Derivation run [O].** The rows below come from the unchanged derivation functions, with `fr` admitted
only in the scratch process (`out/dict_report.json`).

### Pronominal entry layouts [O]

| Lemma | Base entry | Separate headword |
|---|---|---|
| laver | "to wash"; "to wash oneself" tagged `reflexive`. `se laver` appears only among its forms. | none |
| habiller | "to get dressed" and "to dress in a certain fashion" tagged `pronominal` | none |
| aller | 7 senses, **none** meaning "go away" | `s'en aller` "to go away, to leave", with its own IPA and 62 forms |
| souvenir (verb) | single sense "to remember" tagged `pronominal` | `se souvenir`, which duplicates it |
| apercevoir | "to realize, notice" tagged `reflexive` | none |
| lever | 4 of 5 senses tagged `reflexive` | none (only idioms) |
| falloir | "to be missing" tagged `reflexive`, raw tag "with en" | none (`s'en falloir` is only listed as derived) |

The tags are inconsistent (`reflexive` vs `pronominal`), and headword pages exist irregularly. So **a
separate page is not a reliable lexicalization signal**. With base-lemma lookup, `aller` cannot reach the
"go away" meaning of `s'en aller`.

### Lexical fields: isolated vs systematic

| Finding | Kind | Evidence |
|---|---|---|
| No French article derived; gender only | **Systematic** | `article_for` has no `fr` branch: every row has article `""` |
| Mute h is never positively marked; absence only means "not known to be aspirated" | **Systematic** | `hache`, `haricot`, `hasard` carry "French terms with aspirated h"; `homme`, `hôtel`, `huile` carry nothing |
| `héros` (aspirated: *le héros*) lacks the aspirated-h category | Isolated misleading | Treating absence as mute would yield *l'héros* |
| `(ə)` optional schwa is stripped, corrupting IPA: `/su.v(ə).niʁ/` → `/su.v.niʁ/` | **Systematic risk** | `OPTIONAL_SEGMENT_PATTERN`; French entries use `(ə)` routinely **[E]** |
| IPA dropped when every transcription is region-tagged | Systematic for that layout | `sûr`, `sur`, `d'accord` (Europe/Canada tags) get no IPA |
| Invariable plural is indistinguishable from a missing plural | Systematic | `prix`, `héros` → plural `""`; head says "invariable" |
| A merged entry takes the first plural | Isolated | `ciel` (sky *cieux*, canopy *ciels*) → `cieux` for both |
| Page categories leak gender onto non-noun rows | Systematic, low impact | `café` ADJ → Masc; `de` DET → Fem (from the abbreviation *dᵉ*); article logic is NOUN-only |
| English meanings, gender and regular or irregular plurals for common nouns | Present | all 13 sampled common nouns (`yeux`, `chevaux`, `travaux` included) |
| Verb inflection tables | Present | 62–125 forms per verb, reflexive forms included; principal parts empty (pattern is English-only) |
| Contractions `du`/`au`/`aux`/`des` | Present but unreachable | Rows exist; Stanza emits *de*/*le*, and ADP/DET are not vocabulary anyway |

Contextual gloss generation was not exercised and does not establish dictionary coverage.

## 6. Pronominal identity options, applied to observed output

`scripts/identity_policies.py` applies three illustrative rules to the 24 observed VERB tokens
(`out/identity_policies.txt`). The current identity is `{Language, CanonicalLemma, UPOS,
LexicalDiscriminator, Headword}`, and Greek already uses the discriminator and headword end to end. A
pronominal identity could therefore travel in the existing shape **[E]**. Known/lemma-review fallbacks
preserve the discriminator (`internal/fixtures/lemma_review.go:228`), so marking *laver* Known would
**not** cover a discriminated *se laver*.

| | A. Base lemma (today) | B. Blanket pronominal split | C. Selective (lexicalized only) |
|---|---|---|---|
| Sample verb identities | 16 | 18 (`se laver`, `s'habiller`, `s'en aller`, **`se parler`**) | 17 (`s'en aller` only) |
| `il s'en va` | merged into *aller*; lookup misses "go away" | `s'en aller`, own entry | `s'en aller`, own entry |
| `je me lave` / `elle se lave` | *laver*, whose base entry has the reflexive sense | `se laver`: **no entry**, needs a fallback to *laver* | *laver* |
| `elle ne m'en parle pas` | *parler* (correct) | **wrong `se parler`**: `m'` is an object, not reflexive | *parler* |
| Known vocabulary | Knowing *aller* hides every *s'en aller* | Knowing *laver* leaves *se laver* unknown; noisy | Only lexicalized uses need separate learning |
| Reserved vocabulary and counts | Fewest items; meaning collapsed | More items; reflexive-of-a-known-verb inflates counts | Small, targeted increase |
| Frozen snapshots | No change | New identities; existing ones unaffected, but re-analysis moves occurrences | Same as B, for fewer identities |
| Dictionary lookup | Base headword only | Needs headword→base-sense fallback for most rows | Own headword where present, else base sense tagged `reflexive`/`pronominal` |
| Detection required | none | Reflexive vs object clitic, which needs person agreement with the subject (the rule failed on S05) | `expl:pv` (observed once) plus a curated or derived list of lexicalized verbs |

None of these is adopted. This is for the human to decide, and the final decision returns to the
resource-selection ticket.

## 7. What this does and does not show

- **Supported:** the pinned combined pipeline is reproducible from exact bytes, cheap to run, and handles
  elision, contraction expansion and clitics well on this sample. Kaikki has usable meanings, gender,
  plurals and inflection for common words. The real defects found are on Mouseion's side: guillemet
  sentence boundaries, card-target matching after elision and inversion, French article/h-status/IPA
  derivation, and pronominal identity.
- **Not shown:** accuracy on real books, behaviour on literary text, the reliability of `expl:pv`,
  dictionary coverage beyond 47 titles, or full-corpus size. spaCy was not investigated. No observed
  weakness here is a Stanza weakness that a different model would obviously fix; the failures are
  segmentation conventions and downstream handling.

## Reproduce

See `README.md`. Scripts take every path as an argument and are run with `python -I`, outside downloaded
directories.

## 8. Extended diagnostics (human-requested, 20 more strings)

These strings were added after review: `extra_sentences.json`; outputs in `out/extra_stanza_results.json` and `out/extra_match.tsv`.

- **Base lemmas for pronominal forms [O] [R]:** all correct. `m'aperçois→apercevoir`,
  `me souviens→souvenir`, `s'est levée→lever`, `nous en allons` / `m’en vais` / `s'en vont` / `Va-t’en→aller`,
  `te trompes→tromper`, `se lave→laver`.
- **Pronominal relations are inconsistent [O]:**
  - `expl:pv` appeared on `souviens`, `levée`, `allons`, `vais` and `vont`.
  - Reciprocal `se parlent` also got `expl:pv`.
  - `m'aperçois` got `iobj`, and reflexive `te trompes` / `se lave` got `obj` / `iobj`.
  - Non-reflexive `Il me lave` got `obj`, the same relation as reflexive `Je me lave`.

  So the relation alone separates neither lexicalized nor reflexive use.
- **Elision and hyphenation [O]:**
  - `j'`, `l'` (DET and PRON, *Elle l'a vu*), `d'`, `qu'` and `n'` all tokenized correctly.
  - `Aujourd'hui` and `petit-déjeuner` stayed single tokens. Only the spaced `petit déjeuner` splits.
  - `Va-t’en` gave the token `-t’en` the nonsense lemma *von* (PRON, so not vocabulary).
- **Dialogue [O]:**
  - Em-dash dialogue `— Tu viens ? demanda-t-il.` split the incise into its own sentence.
  - `« Tu viens ? » demanda-t-il.` stayed whole when analyzed alone, though it split inside a paragraph (§3). Segmentation depends on context.
- **Matching [O]:**
  - 9 of 36 eligible targets failed under the unchanged matcher: `aperçois`, `aime`, `huile`, `olive`, `hôtel` after elision; `Va`, `demanda` ×2 before hyphens; and `n'`.
  - All 36 persisted spans slice exactly to their surfaces.
  - Across both runs, **17 of 108 targets failed and 108 of 108 spans were exact.**
- **Function words as vocabulary [O]:** `n'` (ADV, lemma *ne*) is an eligible content token, which confirms the
  `ne`/`pas` risk in §3.

## 9. Human review outcome

- **Reviewed readings:** the human confirmed the agent's readings of the observed output (§3–§5).
- **Pronominal identity:** the human chose **A, base-lemma grouping**. French pronominal and non-pronominal uses share
  the base verb's identity (`laver`, `aller`), with no pronominal discriminator. Consequences accepted with this choice:
  - Known *aller* covers *s'en aller* occurrences.
  - Counts and Reserved vocabulary do not grow.
  - Snapshots keep the current identity shape.
  - Dictionary lookup uses the base headword, so lexicalized meanings that exist only under their own headword (*s'en aller* "go away") are not
    reached by lookup.
- **Supporting evidence:** the extended run supports A's prerequisite, correct base lemmas, and undercuts selective detection.
- **Where the decision lands:** final resource adoption and recording this identity policy belong to
  [Select the French launch resources and support boundary](https://github.com/justin-hayes/mouseion/issues/2141).
