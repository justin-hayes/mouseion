# OGA Anabasis Book 1 import proof: all required gates pass with an explicit research-repair layer

## Current result: authorized research-repair pass (#2052, 2026-10-15)

**With 7 citation repairs and 6 εἰμί POS repairs, every whole-unit gate
passes.** No printed node is left unresolved, and the measured dry run is
**1,426 identities, 438 selected, and 434 of 438 with sentence inputs**. The
real Go selection matches the independent Python model byte for byte.

Justin authorized this bounded exception on
[#2052](https://github.com/justin-hayes/mouseion/issues/2052#issuecomment-6186557784).
Two things therefore change:

- **This is not the strict uncorrected proof.** It applies an explicit,
  versioned research-repair layer. The uncorrected v2 result (7 required
  identities unsupported, 437/433) and the v1 dry run (437/432) stay
  reproducible and unchanged.
- **The repairs are not learner state.** Known, Reserved, corrections and
  exclusions are all still empty. Raw OGA predictions, surfaces, native POS and
  spans are untouched.

Source acceptance and practical usefulness remain #2030 decisions, and #2031
stays downstream.

### A. Reproduce

From `doc/research/ancient-greek-oga-anabasis1-import-proof/`, with Go on
`PATH` and `GOTMPDIR=$HOME/.cache/gotmp`:

```sh
python3 -I opus_repair.py                     # read-only: all gates, repairs, Go parity, compare
python3 -I opus_completion.py                 # uncorrected v2 still reproduces exactly
python3 -I mapping_completion_v2.py           # earlier evidence still reproduces exactly
python3 -m unittest discover -p 'test_*.py'   # 164 tests (139 earlier + 25 repair tests)
```

`opus_repair.py` (`oga-anabasis1-opus-repair/1`) works in four steps:

1. it runs the unchanged v2 interpreter;
2. it applies the εἰμί rule v3 to all 178 εἰμί nodes;
3. it applies only the whitelisted records in
   [`opus-repair-records.json`](ancient-greek-oga-anabasis1-import-proof/opus-repair-records.json),
   which are bound to the OGA v0.2.0 component hashes;
4. it reruns every gate, the real Go probe, the 3/15 model and the sentence
   inputs.

It compares everything with
[`opus-repair-summary.json`](ancient-greek-oga-anabasis1-import-proof/opus-repair-summary.json)
and the five private outputs in `private/opus-repair/out/`. Only `--write`
regenerates, and it can write only those. A test checks that it never writes the
v1 or v2 outputs, `private/opus-completion/`, or the repair records.

**Each repair fails closed.** The run stops on:

- a wrong component revision;
- a node that is unknown or in the wrong section;
- any raw lemma, surface, native POS, relation, prior rule or prior UPOS that
  differs from the record, including by one diacritic;
- a node that is no longer in its recorded prior state;
- a Morpheus row, stem line or UD Perseus lemma/form count that differs from its
  cited witness;
- a repaired key that does not decode exactly to the recorded headword, or that
  carries a homograph number;
- a POS repair that would not change the UPOS.

A missing repair leaves its node unresolved, and the vocabulary-ready gate then
fails.

**Review fix after parent review.** The parent found that the `t_7640` witness
in `230decab` named a native edge that does not exist: it said ἐλῶντα attaches
PNOM directly to ὡς. Nothing had checked the POS witnesses. The fix leaves every
mapping rule, repair target and count unchanged. Only the `t_7640` uncertainty
text changes, which in turn changes the node-ledger hash. It adds these checks:

- **Edge witnesses.** Each of the 6 POS records now lists structured edges
  ([node, exact form, relation, head]); `t_3351` also lists its 5 possessive-dative
  parallels. All are checked against the pinned CoNLL-U groups before any POS
  repair applies. These checks confirm the cited parse, not the Greek reading.
- **OGA parallels.** Cited parallel nodes must carry the repaired headword, and
  convention nodes their stated lemma and surfaces.
- **Exact whitelist.** Records must name exactly the authorized 7 citation and 6
  POS IDs, once each, with no node in both lists.
- **Exit status.** `opus_repair.py` now exits 1 whenever any required gate fails,
  even when every output reproduces. `--write` records the failing gates and
  also exits 1.

Mutation tests cover:

- a wrong head, relation or diacritic, or a changed iota subscript, in an edge;
- a missing witness or node;
- a wrong parallel lemma;
- duplicate, missing, extra or overlapping repair IDs;
- a mocked run where every output reproduces but one gate fails.

**Checks on 2026-10-15.** All of these passed:

- the read-only repair reproduction, with no problems and Go parity;
- the uncorrected v2 reproduction;
- the `mapping_completion_v2.py` reproduction;
- all 164 companion tests;
- `git diff --check`;
- `make lint`;
- `make coverage-new`.

**No new input was acquired.** The evidence is already retained and pinned:

- the Morpheus index and stem files;
- the UD Ancient Greek-Perseus copies;
- the OGA component itself.

There were no network requests, and the Gorman files were not used.

### B. Citation repairs: seven occurrences, five labels, all resolved

The decisive new evidence comes from the AGDT lineage that trained the producer,
as seen in UD Ancient Greek-Perseus.

- That lineage writes compound lemmas in Morpheus's **hyphenated derivation
  notation**: 88 such lemma types over 125 tokens, e.g. `ἐν-εἰμί`, `σύν-εἶδον`,
  `κατά-καθέζομαι`, `σύν-λέγω`.
- It also writes the **plain citation form for forms of the same paradigms**:
  `ἔνεστι` occurs both as `ἐν-εἰμί` and as `ἔνειμι`; `συλλέξας` both as
  `σύν-λέγω` and as `συλλέγω`; `καθεζόμενος` both as `κατά-καθέζομαι` and as
  `καθέζομαι`.
- The anomalous OGA labels are that notation with the hyphen removed. That is
  not a guess about shape: the notation and the citation demonstrably name the
  same lexeme in the producer's own training lineage.

The same evidence also strengthens the existing `t_302` decoding (`σύνλέγω2` →
συλλέγω), which this pass leaves unchanged.

Every repair is an explicit record. Each holds the before/after values, the
witnesses, the reason and the uncertainty. The `ancient-greek/1` profile is
unchanged.

| Node(s) | Raw label → repaired citation | Kind | Key evidence | Uncertainty |
| --- | --- | --- | --- | --- |
| `t_437` (συνέλεξεν, 1.1.9) | `σύλλέγω` → **συλλέγω** | malformed compound spelling | Morpheus `sulle/gw` (`vbs.cmp.ml:6459`); UD Perseus `συλλέγω` for συνέλεγεν/συνελέγοντο/συλλέξας; OGA `t_402`, `t_4272` συλλέγω | Low |
| `t_4624`, `t_4638`, `t_5775` (ἐνῆν, 1.5.1/1.5.2/1.6.3) | `ἐνεἰμί` → **ἔνειμι** | notation → citation | UD Perseus `ἐν-εἰμί` (ἐνῆν ×2, ἔνεστι) and `ἔνειμι` (ἔνεστι ×3, ἐνείη); OGA `t_4654` ἐνῆσαν = ἔνειμι; single Morpheus key `e)/neimi` | Morpheus's one record derives `e)/neimi` from εἶμι, so it is **not** used as base evidence. The εἰμί base comes from the surface and the producer's notation. There is no homograph number, so none is guessed. |
| `t_4930` (ἑπτὰ, 1.5.6) | `ἑπτὰ` → **ἑπτά** | node-specific: the surface's sandhi grave was copied into the lemma | Morpheus `e(pta/`; UD Perseus lemmatizes ἑπτὰ as ἑπτά ×9; OGA `t_935`, `t_3765`, `t_5824` ἑπτά | Low. This is **not** a global grave/acute equivalence. |
| `t_5121` (ἐκαθέζετο, 1.5.9) | `κατάκαθέζομαι` → **καθέζομαι** | redundant prefix notation | Morpheus `kaqe/zomai` with its κατά-prefixed stem and imperfect ending (`vbs.simp.ml:9780–9782`); UD Perseus `καθέζομαι` (καθέζετο ×6) and `κατά-καθέζομαι` for the same participle | Low |
| `t_5145` (συνιδεῖν, 1.5.9) | `σύνεἶδον` → **συνεῖδον** | notation → compound citation, formed by the scheme's own pattern | UD Perseus `σύν-εἶδον` (συνιδοῦσα, συνεῖδε) and sibling `προσεῖδον`; Morpheus prefix + εἶδον keys (`e)nei=don`, `prosei=don`, `parei=don` …) and σύν keeping ν before a vowel (`su/neimi1`, `su/noida`); OGA lemmatizes ἰδεῖν as εἶδον | **Medium.** συνεῖδον is formed, not itself an attested key. The LSJ-derived list has συνοράω instead, which is not adopted because it would cross the producer's εἶδον/ὁράω separation. Single occurrence, not selected either way. |

The seven wrong-homograph predictions (`t_1590`, `t_3153`, `t_3251`, `t_3800`,
`t_5625`, `t_5642`, `t_5708`) are outside this scope. They stay as recorded
upstream prediction errors.

### C. εἰμί: rule v3 plus reviewed repairs, checked across all 178

**Rule v3 changes 3 nodes**, each confirmed by reading the context:

- `t_5030`: ἦν θεάσασθαι, with the infinitive labelled PNOM, means "it was
  possible to observe": impersonal **VERB**.
- `t_5147`: συνιδεῖν δ' ἦν, with an empty-node PNOM, is likewise impersonal:
  **VERB**.
- `t_6214`: σχολὴ ᾖ ἡμῖν, with a nominal subject and a possessive dative, means
  "that we may have leisure": possession, **AUX**.

The impersonal reading now needs either an infinitive or empty PNOM, or an
infinitive subject without a nominal subject. In both cases there must be no
lexical PNOM and no PP.

**Six reviewed POS repairs.** Each names the native-parse witness that it
overrides:

| Node | Rule → repaired | Positive classification and evidence |
| --- | --- | --- |
| `t_2739` | VERB → **AUX** | οὐχ ὥρα εἶναι … καθεύδειν: ὥρα is the predicate. The native parse attaches its PNOM to impersonal δοκεῖ. |
| `t_3351` | VERB → **AUX** | τοῖς δὲ ὑποψία ἦν: a possessive dative, parsed as an ATR of the subject. The producer itself labels possessive datives PNOM in five other εἰμί clauses (`t_662`, `t_976`, `t_6764`, `t_8982`, `t_9036`), which become AUX. |
| `t_7640` | VERB → **AUX** | ἡ γνώμη ἦν ὡς … ἐλῶντα: the native parse labels ἐλῶντα PNOM, but attaches it indirectly: `t_7647` PNOM → `t_7648` καί (COORD) → `e_88` [0], an elided node (ADV) → `t_7641` ὡς (AuxC) → `t_7640` ἦν. Reading the ὡς-participle phrase as the predicate of ἦν is a review judgment, since the parse does not attach the PNOM to ἦν itself. Uncertainty is low-medium. |
| `t_10265` | VERB → **AUX** | τὰ ὑπὲρ τοῦ λόφου τί ἐστιν: proleptic subject. The clause is copular whether τί or the locative PP is the predicate. |
| `t_10412` | VERB → **AUX** | ἦσαν δ' αὗται τετρακόσιαι … ἅμαξαι: bare αὗται, without the article, cannot be attributive. It is the pronoun subject, and the numeral + noun is the predicate. |
| `t_5989` | AUX → **VERB** | μετὰ ταῦτα … ἔστιν ὅ τι σε ἠδίκησα;: a temporal PP, not a predicate. ἔστιν ὅ τι is existential, with the relative clause as subject. |

**Reviewed and kept without repair:**

- `t_6641` keeps **AUX**: a possessive dative ἡμῖν, consistent with `t_3351`.
- `t_5651` and `t_9711` keep **VERB**: the native attributive parse gives a
  grammatical existential reading, and there is no positive evidence that it is
  wrong. These are the only open readings. If either were AUX, εἰμί|VERB would
  fall from 20 to at most 18 occurrences; selection and inputs would not change.

**Result:**

| Rule | Nodes |
| --- | ---: |
| PNOM copula | 122 |
| Adverbial/PP predicate | 31 |
| Impersonal infinitive | 8 |
| Existential | 11 |
| Reviewed repair | 6 |

That gives **AUX 158, VERB 20**. The other UPOS classes remain
producer-compatible conversions, not strict UD gold, as in v2.

### D. Whole-unit rerun

**Integrity gates.** Every gate below passes:

| Gate | Result |
| --- | --- |
| Native join | 10,462 printed + 110 empty nodes |
| Reading coverage | 48,892 non-whitespace characters, exact, no overlaps |
| Sentence coverage | 381 complete sentences, whitespace-only tail |
| Native groups | all 607 nest |
| Canonical POS | determinate for all printed nodes |
| Required identity | **usable for every printed lexical node** (0 unsupported) |
| Repair whitelist | all 13 applied |
| Vocabulary-ready accounting | complete |

**Denominators.**

| Count | Value |
| --- | ---: |
| Eligible occurrences | 6,037 |
| Non-vocabulary lexical | 3,268, including AUX 158 |
| Punctuation | 1,157 |
| Unresolved | 0 |

Lemma rules over the printed non-punctuation nodes:

| Lemma rule | Nodes |
| --- | ---: |
| `I-PLAIN` | 9,156 |
| `I-NUM-MORPHEUS-KEY` | 141 |
| `I-COMPOUND-RECORD` | 1 |
| `R-CITATION-REPAIR` | 7 |

**Selection.** One analyzed Book; Known, Reserved, corrections, exclusions and
external frequencies are all empty.

| Measure | Repaired | Uncorrected v2 |
| --- | --- | --- |
| Identities | **1,426**: NOUN 489, VERB 520, ADJ 278, ADV 139 | 1,424 |
| Selected | **438**: NOUN 146, VERB 154, ADJ 79, ADV 59 | 437 |
| Selected occurrences | 4,819 / 6,037 (79.8%) | — |
| Known-token coverage | 0% | 0% |

The Go probe printed `candidates=1426 selected=438 analyzable_tokens=6037
distinct_identities=1426 sentences=381`. Its TSV is byte-identical to the model.

**Changes from the uncorrected v2.** 16 nodes have a changed interpretation.
Identity counts move as follows:

| Identity | Occurrences | Selected |
| --- | --- | --- |
| ἔνειμι VERB | 1 → 4 | **newly selected** |
| συλλέγω VERB | 3 → 4 | yes |
| ἑπτά ADJ | 3 → 4 | yes |
| εἰμί VERB | 23 → 20 | yes |
| καθέζομαι VERB | new, 1 | no |
| συνεῖδον VERB | new, 1 | no |

**Sentence inputs** (same rules as v2: complete sentence, at most 50 units and
400 code points after trimming, verified target span, usable headword; earliest
occurrence first; nothing shortened):

- **434 of 438** selected identities get an input. The 4 omissions are the same
  as in v2: οἴχομαι, πατήρ, πρότερος|ADJ and `ἄπειμι#morpheus:1`. Every
  occurrence of each is over a cap.
- 58 inputs use a later occurrence. The earlier rejections were 79 over the unit
  cap and 19 over the code-point cap.
- The chosen sentences run 4–50 units (median 27.5) across 179 distinct
  sentences.

Inputs for repaired identities, as private spans:

| Identity | Chosen input |
| --- | --- |
| ἔνειμι | `t_5775` (reading span 33056–33060); `t_4624`, `t_4638` and `t_4654` are over the unit cap |
| συλλέγω | `t_302` |
| ἑπτά | `t_935` |
| εἰμί\|VERB | `t_3631` (was `t_2739`, now AUX); `t_3146` is over the cap |

Sentence extents and length distributions are unchanged from v2. Repaired
ledgers and inputs stay private.

### E. Verdicts

| Layer | Verdict |
| --- | --- |
| Binding and whole-unit gates | **Pass** |
| Identity | **Pass with the research-repair layer.** 7 occurrences use explicit citation repairs; `t_5145` carries a medium-uncertainty formed citation. Without repairs, v2 still fails on these 7. |
| POS | **Determinate.** εἰμί follows rule v3 plus 6 reviewed repairs; 2 readings stay open with stated impact. Other classes are producer-compatible, not strict UD |
| Vocabulary-ready accounting | **Complete under the repair layer.** This is not an uncorrected proof, and it is not source acceptance. |
| Dry run | 438 selected, 434 inputs, real Go parity |
| Prediction quality | 7 wrong-homograph predictions, the function-word ADV and name findings from v2, and the PAULA spacing all remain |

**For #2030 (practical, not Greek-factual).** The repaired citation failures
were content verbs and a numeral; the residual open εἰμί readings are function
words.

- No stopword strategy was needed to reach a passing gate.
- Justin's stopword idea stays a recorded fallback, not adopted.
- The practical questions remain from v2:
  - particles as ADV: 24 selected identities, 1,041 occurrences;
  - names as lowercase NOUN headwords;
  - PAULA spacing.

## v2 review revision (historical; superseded by the research-repair pass above)

**The integrity gates and the canonical POS gate pass.** All 10,572 native nodes
join their CoNLL-U rows, every non-whitespace reading character is covered, all
381 sentences are complete, and native groups nest. Every printed node gets a
determinate UPOS.

**The required-identity gate does not pass.** Seven printed nodes have no
usable citation representation: `t_437`, `t_4624`, `t_4638`, `t_4930`,
`t_5121`, `t_5145`, `t_5775`. Under ADR 0094 this blocks vocabulary-ready
publication of the Book.

The downstream workflow was run anyway, as a **conditional dry run** with those
seven nodes counted as unresolved rather than as vocabulary:

- 1,424 identities, of which 437 are selected, with empty Known and Reserved;
- real Go output byte-identical to the Python model;
- 433 of the 437 have deterministic sentence inputs.

These are dry-run measurements, not a ready Book. The first pass at `0e839900`
(437 selected, 432 inputs) is kept below as an immutable historical result. Its
corrected claims are listed there.

### Independent parent review

The parent independently reproduced commit `80a3c986`, all 139 companion tests,
the real Go/model comparison, `make lint`, and `make coverage-new`. It also
independently checked all 46 new input sizes/hashes, all 10,572 native joins,
10,462 printed spans, 48,892 non-whitespace reading characters, all 381 sentence
extents and 607 nested native groups. A separate recount verified 6,033 eligible
occurrences, 1,424 identities, 437 selected identities and all 433 earliest
cap-qualified input targets, **under the reported interpretation**. The seven
unsupported nodes remain explicitly accounted for; their removal from this
conditional scenario does not make a smaller ready Book.

The verdicts below are the research adapter's verdicts, not adopted canonical
rules or source acceptance. In particular, the POS gate's passing value establishes
that the evaluator produces a determinate UPOS, not that every conversion is
correct. The seven reported context disagreements and two ambiguous readings in
§C remain qualifications. A native `ADV` dependent can be an adjunct rather than
a predicate, so a conversion mistake cannot always be attributed to an upstream
parse error. No blanket εἰμί exclusion is accepted by this review.

The 141 direct numbered-key spellings/discriminators and the single compound
record were independently cross-checked. The `t_302` interpretation remains an
evidence-supported inference with the documentary limit stated in §B; it is not
an explicit OGA declaration. The seven unsupported citation forms are a concrete
failure of this adapter's required identity gate, sufficient to block publication,
not proof that OGA cannot support a usable adapter or that every other field has
been certified. No spelling repair, accent equivalence or learner exclusion was
introduced.

The Gorman acquisition exceeded the explicit no-new-training-data acquisition
boundary; calling those files lineage evidence does not retroactively authorize
it. They remain private and are not substituted for OGA. Its licence conflict and
the historical v1 label projections are recorded in `NOTICE.md`; this review
does not grant redistribution, bundling, export or provider-use permission. No
further acquisition, corrective annotation project or fallback is launched.

This review revision corrects four things in `0e839900`:

- Seven unusable predicted citation forms were silently counted as identities.
- εἰμί was AUX in every context.
- The training-overlap wording was too strong.
- Several integrity checks did not fail closed.

### A. Reproduce

From `doc/research/ancient-greek-oga-anabasis1-import-proof/`, with Go on
`PATH` and `GOTMPDIR=$HOME/.cache/gotmp`:

```sh
python3 -I opus_completion.py                 # read-only: hashes, gates, rebuild, Go parity, compare
python3 -m unittest discover -p 'test_*.py'   # 139 tests: 97 earlier plus 42 for this pass
python3 -I mapping_completion_v2.py           # earlier v2 evidence still reproduces exactly
```

`opus_completion.py` is now `oga-anabasis1-opus-completion/2`. It compares
against
[`opus-completion-v2-summary.json`](ancient-greek-oga-anabasis1-import-proof/opus-completion-v2-summary.json)
and the seven private outputs in `private/opus-completion/out-v2/`. `--write`
can write only those, and the write guard and a test protect the v1 summary and
`private/opus-completion/out/`.

The integrity gates raise and stop the run: native join, reading coverage,
sentence coverage and native-group nesting. The lexical gates are recomputed and
reported under `required_gates`. The run exits 1 on any input-hash, output or
Go-parity mismatch.

The v1 summary (`opus-completion-summary.json`) reproduces only from commit
`0e839900`.

**Checks on 2026-10-15.** All of these passed:

- the read-only reproduction, with no problems and Go parity;
- all 139 companion tests;
- `mapping_completion_v2.py`, which still compares exactly;
- `git diff --check`;
- `make lint`: golangci-lint 0 issues, ruff and mypy clean;
- `make coverage-new`: NLP 90 passed and 3 skipped, plus the Go unit and
  integration runs. The research Python is outside the coverage reports, so it
  reports "No lines with coverage information".

Tests write temporary files only under
`~/.cache/scratch/greek-oga-anabasis1-import-proof/opus-completion-tests/`,
never `/tmp`. The fail-closed mutation tests cover:

- a missing, duplicate or extra CoNLL-U join;
- a duplicate native ID;
- a surface, lemma or POS mismatch, including a mismatch in one diacritic or one
  letter form;
- uncovered, overlapping or wrong spans;
- a native group crossing a sentence;
- a sentence gap or a non-terminal tail;
- unreviewed terminals.

**Inputs.** The v2 revision fetched nothing.
[`opus-completion-inputs.json`](ancient-greek-oga-anabasis1-import-proof/opus-completion-inputs.json)
now has an `acquisition` field for each of the 46 entries:

- **Fetched over the network on 2026-10-15:**
  - Morpheus: 8 stem-source files and 3 documentation files;
  - **Gorman: two complete Book 1 annotation XML files** (710,529 + 619,008
    bytes, sections 1.1–1.10), plus the README and the licence file;
  - 26 UD documentation pages.
- **Copied locally:** UD Ancient Greek-Perseus, as byte-identical copies of the
  #2050 retained files.

The Gorman files are training-data lineage material, not short lemma extracts.
That goes beyond the brief's "narrow lexical record" wording and is recorded as
a scope qualification for the parent to assess. Licence terms, including the
Gorman README (NC) versus licence-file (BY-SA) conflict, are listed per source in
[NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md). Nothing is
relicensed in bulk.

### B. Lexical interpretation (`oga-greta-lemma/2`)

**Namespace (unchanged from `0e839900`).** A numbered label `H` + `n` is the
Perseus Morpheus lemma key for that headword and number, with every diacritic
kept. It is recorded as discriminator `{scheme: morpheus, value: n}` (pinned
`b1b33c56`). The witnesses are the same as in §B of the `0e839900` section:

- the AGDT2 guidelines §2 and §2.5 on Morpheus numbers in the lemma field;
- the Gorman trees carrying the same strings on the same tokens, e.g. `λέγω3`
  on all 49 aligned `λέγω3` tokens;
- the Morpheus stem-file format and `lemmaRegistry.xsl`;
- the declared stem class or base of each key.

Native POS agrees with the declared stem class on 101 of 106 simplex
occurrences. The other five are `ἤ1` with `d`.

Of the εἶμι/εἰμί compound occurrences, 22 match the base their key declares, 7
do not, and 6 cannot be checked because the scheme itself is ambiguous. The 7
mismatches are `t_1590`, `t_3153`, `t_3251`, `t_3800`, `t_5625`, `t_5642` and
`t_5708`. They are wrong-homograph predictions. They are **kept**, because a
valid decoding of a wrong prediction is still a usable identity.

**`πρόσειμι1`** is a valid key, and its six occurrences stay resolved. Morpheus
derives both `pro/seimi1` and `pro/seimi2` from both εἶμι and εἰμί
(`vbs.cmp.ml:5639–5644`), so the scheme does not say which base the number
means. That ambiguity belongs to the scheme. It is not a decoding failure and
not a measure of prediction accuracy.

**`t_302` `σύνλέγω2` decodes to συλλέγω (`I-COMPOUND-RECORD`).** The rule is
supported by structure, not just by appearance:

1. **The label is exactly the derivation.** Its characters are the Unicode
   decoding of `su/n` followed by `le/gw2`. That includes the digit, and the
   digit belongs to the **base**: `le/gw2` has its own entry at
   `vbs.simp.ml:12226`, with the perfect stem εἰλοχ-, and is distinct from
   `le/gw3` (say).
2. **Exactly one record has that derivation.** `vbs.cmp.ml:6459` reads
   `su/n-le/gw2 sulle/gw`, a record mapping a derivation to a key. The label
   itself is not a Morpheus key.
3. **The model produces this shape regularly.** Four other Book 1 labels are
   prefix + base without a hyphen: `ἐνεἰμί` (ἐν + εἰμί, three times),
   `σύνεἶδον` and `κατάκαθέζομαι`.
4. **Deleting the hyphen is an established step in this toolchain.** The
   Morpheus Greek makefile builds keys from hyphen-marked LSJ lemmata with
   `sed -e 's/\-//'`. `morpheus.html` describes analysis as finding "a stem (or
   a stem with a prefix)".
5. **Gorman's label on the same token is συλλέγω.** This agrees with the result
   but is not part of the rule.

The rule applies only when all of these hold:

- the label is not an exact key;
- exactly one single-prefix record matches;
- the numbered base is itself an exact key.

Two cases are excluded by that ordering and uniqueness. Across the index, 642
prefix + base strings coincide with real keys, and 172 come from several
records.

**One gap remains:** no OGA document states that labels may carry Morpheus
derivation notation.

**Usable-headword rule (new).** A plain label is a usable citation
representation unless both of these hold:

- it breaks a Greek citation-form invariant: more than one accent, a breathing
  after the second letter except on ρ, a grave accent, a stray mark, or a
  non-Greek letter;
- no Morpheus key attests that exact spelling.

The invariant alone is not enough. 285 of the 187,682 decodable Morpheus records
break it and are still valid: enclitic `-δε` forms such as οἶκόνδε, koronis
crasis such as καλοκἀγαθία, and starred names. Such attested spellings stay
usable. Unattested well-formed labels such as κῦρος stay usable without a key,
because no dictionary validation is required. Nothing is repaired,
accent-folded or rebuilt.

| ID | Cts | Surface | Predicted label | Why the representation is unusable |
| --- | --- | --- | --- | --- |
| `t_437` | 1.1.9 | συνέλεξεν | `σύλλέγω` | Two accents. The assimilated prefix σύλ- is not a Morpheus prefix, and no record or key matches. It is not the hyphenless derivation `σύνλέγω` either. |
| `t_4624`, `t_4638`, `t_5775` | 1.5.1, 1.5.2, 1.6.3 | ἐνῆν | `ἐνεἰμί` | ἐν + εἰμί notation with a medial breathing. The only ἔνειμι record (`vbs.cmp.ml:2556`, `e)n-ei)=mi`) derives from **εἶμι**, not εἰμί. Using it would substitute a different base. |
| `t_4930` | 1.5.6 | ἑπτὰ | `ἑπτὰ` | Grave accent in a citation form. Key `e(pta/` exists only with the acute accent, and the profile forbids grave/acute equivalence. |
| `t_5121` | 1.5.9 | ἐκαθέζετο | `κατάκαθέζομαι` | κατά + καθέζομαι notation with two accents. No record or key. |
| `t_5145` | 1.5.9 | συνιδεῖν | `σύνεἶδον` | σύν + εἶδον notation with two accents and a medial breathing. No record or key. |

These seven labels are not just wrong predictions of a valid form; they have no
usable citation representation. The wrong-homograph predictions above, by
contrast, stay resolved. The seven are `I-UNSUPPORTED-CITATION-FORM`, recorded
with raw evidence and exact IDs.

**Verdict:** required identity is unsupported for these 7 nodes (5 labels),
which blocks vocabulary-ready publication. All other 9,298 lexical nodes have a
usable identity:

| Rule | Nodes |
| --- | ---: |
| `I-PLAIN` | 9,156 |
| `I-NUM-MORPHEUS-KEY` | 141 |
| `I-COMPOUND-RECORD` | 1 |

### C. POS interpretation (`oga-agdt-upos/2`)

**Contract.** Lexical classes follow the producer-lineage conversion. UD
distinctions that are defined by function, namely AUX versus VERB for εἰμί,
follow the UD definitions applied to native AGDT syntax.

The native tag letters (a, d, i, l, n, p, r, u) and the conjunction pairs use
the UD Perseus conversion, as in `0e839900`; letter counts are in the summary.
This is a **producer-compatible mapping, not a claim of strict UD category
correctness**. The UD grc pages define DET, NUM and PART more narrowly, for
example δή and αὖ as PART and cardinals as NUM. AGDT2 §2.9 has its own numeral
tag, but OGA never predicts it. Applying the UD grc example lemmas gives
diagnostic `D_strict_ud_grc_DET_NUM_PART_examples` in §G. Nominals and
adjectives with an adverbial relation stay NOUN/ADJ (AGDT2 §2.6; UD Perseus
v2.11 changelog).

**εἰμί.** 178 printed nodes; the rules are listed in order of precedence:

| Rule | Native-syntax condition | UPOS | Nodes | Basis |
| --- | --- | --- | ---: | --- |
| `P-V-EIMI-COP-PNOM` | PNOM child, directly or through COORD/APOS | AUX | 124 | AGDT2 §3.7 (PNOM is the complement of a copulative verb) and §4.1.9.4 (periphrasis); UD cop says "verbal copulas are tagged AUX"; UD grc AUX covers periphrasis |
| `P-V-EIMI-IMPERSONAL-INFINITIVE` | Infinitive subject and no PP | VERB | 7 | ἔστι + infinitive has no nonverbal predicate; the grc AUX page admits no modal auxiliary |
| `P-V-EIMI-COP-ADVERBIAL-PREDICATE` | AuxP or ADV dependent | AUX | 31 | UD simple-syntax: a one-verb language assimilates location, possession and benefaction, and existentials with a location, to the copula; UD cop: "also when the predicate is a prepositional phrase" |
| `P-V-EIMI-EXISTENTIAL` | None of the above | VERB | 16 | No predicate other than the verb (simple-syntax); UD AUX covers only auxiliaries and narrow-sense copulas |

Totals: AUX 155, VERB 23. The node IDs for each rule are in the summary. The
rule is determinate for every node and fails closed on `AuxV` attached to another
verb.

**What the conversion does and does not show.** UD Perseus has εἰμί as AUX
1,864 times (1,863 cop and 1 conj) and as VERB twice. Both VERB witnesses are
copular when read: ὤν in "…δίκαιος ὤν…" (xcomp) and ἔσκεν with οἵη (root). They
are therefore conversion inconsistencies, not evidence of a VERB category. The
conversion data offer no contrast for existential uses, and I did not check
whether its AUX tokens include pure existentials. The `0e839900` claim that the
converter keys on the lemma is withdrawn: a correlation with the misspelt
`είμί` proves nothing. AUX for every εἰμί stays only as the diagnostic
`D_eimi_all_AUX_ud_perseus_conversion_compatible`.

**Residual uncertainty, as concrete IDs.** I read all 54 contexts without PNOM.
This reading is a diagnostic and never changes the rule.

- 45 agree with the rule.
- 7 disagree:
  - `t_2739`, `t_5651`, `t_7640`, `t_9711`, `t_10265`, `t_10412`: the rule
    gives VERB, but each is copular. The native parse does not attach the
    predicate (ὥρα, χρήσιμον, a ὡς-clause, a numeral, τί).
  - `t_5989`: the rule gives AUX, but it is a pure existential; its PP is a
    temporal adjunct.
- 2 are open between possession and existence: `t_3351`, `t_6641`.

These are upstream syntax-prediction effects or limits of the native labels.
They are not undetermined mappings. **The canonical POS gate passes:** UPOS is
determinate for all 10,462 printed nodes.

### D. Conditional dry run

The seven unsupported nodes are counted as `unresolved-identity`, not as
vocabulary.

| Count | v2 | `0e839900` |
| --- | ---: | ---: |
| Eligible occurrences | 6,033 | 6,017 |
| Non-vocabulary lexical | 3,265 (AUX 155) | 3,288 (AUX 178) |
| Unresolved identity | 7 | 0 (the 7 were counted as identities) |
| Identities | 1,424: NOUN 489, VERB 518, ADJ 278, ADV 139 | 1,428 |
| Selected (Go = model) | 437: NOUN 146, VERB 153, ADJ 79, ADV 59 | 437 |
| Selected occurrences | 4,816 / 6,033 (79.8%) | 4,796 / 6,017 |
| Sentence inputs | 433 / 437 | 432 / 437 |

- `εἰμί|VERB` (23) is now an identity and is selected.
- `ἐνεἰμί` is no longer an identity.
- The Go probe printed `candidates=1424 selected=437 analyzable_tokens=6033
  distinct_identities=1424 sentences=381`.
- The 4 omissions are οἴχομαι, πατήρ, πρότερος|ADJ and `ἄπειμι#morpheus:1`. In
  each case every occurrence is over a cap.
- 56 inputs use a later occurrence.

The sentences are unchanged: 381 complete, 10 with at most 5 units, 36 over 50
units, 4 over 400 code points.

### E. Lineage wording (corrected)

**Training overlap.** Paper §3 describes one dataset for training, validation
and testing. Appendix A lists `tlg0032.tlg006` (*Anabasis*) at work level. The
token count is MathML in the retained HTML; the parent reports 18,737. This is
**a known work-level overlap with the reported development and evaluation data.
No held-out claim is established.**

**What was not verified:**

- which Book 1 passages fell into which split;
- the training set of the released model.

The interpretation of the lemma namespace does not depend on either.

**The Gorman comparison.** It is a local same-section surface alignment
(difflib), not a complete binding and not an accuracy gold standard.

- Gorman's XML has 207 subdoc labels, including the out-of-range `1.5.171`,
  `3.4.1` and `19.31`.
- OGA section 1.7.20 has no Gorman counterpart.
- On 8,926 aligned lexical nodes, the lemma string agrees on 8,636 (96.8%) and
  the POS letter on 8,839. These are rates of agreement between aligned labels
  for the same form, not certified prediction accuracy.
- Gorman strings are now projected publicly only as agreement categories.

### F. Separate verdicts

| Layer | Verdict |
| --- | --- |
| Binding, join, coverage, sentence and nesting gates | **Pass**, failing closed |
| Representation, identity | **Fails for 7 nodes** (IDs in §B). All others are usable under `oga-greta-lemma/2`, including the 141 numbered keys and the `t_302` compound record |
| Representation, POS | **Pass** (determinate). Producer-compatible lexical classes; εἰμί from UD definitions; residual readings in §C |
| Vocabulary-ready publication | **Blocked** by the 7 unsupported identities |
| Downstream measurements | **Conditional dry run** (§D), with real Go parity |
| Prediction quality | Measured, not certified: wrong homographs, unusable forms, missing predicate attachments, the practical findings in §G |
| Unperformed | Gloss, translation, dictionary coverage, Anki/client rendering, human example review |

### G. Practical quality and diagnostics

Practical usefulness, including function words, names and spacing, is for #2030
to decide. These are facts, not Greek-classification questions:

- 24 selected ADV identities (1,041 occurrences) are particles or conjunction
  uses tagged `d`.
- 40 selected NOUN identities (453 occurrences) are always capitalized names
  with lowercase headwords.
- The PAULA text has 47 word-internal splits, in 33 sentences, and adds a space
  before punctuation.
- Nine headwords exist as both numbered and plain identities.

The diagnostic scenarios compare interpretation contracts. None is a choice for
Justin.

| Scenario | Eligible | Identities | Selected | Unresolved |
| --- | ---: | ---: | ---: | ---: |
| Adopted v2 | 6,033 | 1,424 | 437 | 7 |
| The 7 counted as their predicted strings (`0e839900` handling) | 6,040 | 1,429 | 438 | 0 |
| εἰμί always AUX (conversion-compatible) | 6,010 | 1,423 | 436 | 7 |
| Strict UD grc DET/NUM/PART examples | 5,739 | 1,399 | 420 | 7 |
| Without the compound-record rule | 6,032 | 1,424 | 436 | 8 |

## First fresh-owner dry run at `0e839900` (historical; superseded by the v2 review revision above)

Kept unchanged below as the immutable conditional dry run (437 selected, 432
inputs). The v2 review revision corrects these claims in it:

- "0 unresolved / established for all nodes": 7 nodes have no usable citation
  representation.
- εἰμί AUX in all 178 contexts, and "the converter keys on the lemma".
- "Book 1 is in the training data / not held out".
- The S1, S3 and S4 rows "for #2030 to decide": these are interpretation
  questions, settled in v2.
- §A's description of every new input as narrow lineage evidence:
  two complete Gorman annotation files were fetched.

**Every one of the 10,462 printed nodes now has a versioned lemma and UPOS
interpretation, and no node is left unresolved.** The 142 numbered labels
(21 distinct) decode against Perseus Morpheus lemma keys. The whole Book 1
workflow then ran end to end:

- 381 complete sentences;
- real Go selection, byte-identical to an independent Python model: **437 of
  1,428 identities** selected, with empty Known and Reserved;
- deterministic sentence inputs for **432 of the 437**.

This is research evidence for
[#2030](https://github.com/justin-hayes/mouseion/issues/2030). It does not
accept OGA, a mapping, a deck or the MVP. Practical quality has large, measured
problems: function words become ADV vocabulary, names are NOUN, several
predictions pick the wrong homograph, and seven predicted citation forms are
malformed (§F). The earlier stops below stay as history.

The binding baseline (`oga-anabasis1-native-binding/1`) is unchanged.

### A. Reproduce

From `doc/research/ancient-greek-oga-anabasis1-import-proof/`, with Go on
`PATH` and `GOTMPDIR=$HOME/.cache/gotmp`:

```sh
python3 -I opus_completion.py                 # read-only: verify, rebuild, run the Go probe, compare
python3 -m unittest discover -p 'test_*.py'   # 127 tests: 97 earlier plus 30 for this pass
```

The default run:

1. checks every input against its pin. That covers the original pins, the 46
   new entries in [`opus-completion-inputs.json`](ancient-greek-oga-anabasis1-import-proof/opus-completion-inputs.json),
   the Morpheus manifest and the producer documents;
2. rebuilds everything in memory;
3. compares the result with the committed
   [`opus-completion-summary.json`](ancient-greek-oga-anabasis1-import-proof/opus-completion-summary.json)
   and the seven private outputs under `private/opus-completion/out/`;
4. runs `_selectionprobe` (Mouseion's `selection.Project`,
   `NewEligibility(nil, nil)`, `AllowsBookDeckCandidate` and
   `AnalyzableStatistics`) and requires its output to match the Python model
   byte for byte.

The Go input and output go to a temporary directory under
`~/.cache/scratch/greek-oga-anabasis1-import-proof/opus-completion/`, which is
then deleted. Exit status is 1 on any mismatch; `--no-go` always exits 1.
Only `--write` regenerates, and it writes nothing but this pass's summary and
`private/opus-completion/out/`. Earlier outputs are never rewritten:
`mapping_completion_v2.py` still reproduces exactly.

**New private inputs.** All are hash-pinned in the registry. They are narrow,
static and public, and no analyzer, model or provider was run. Rights are in
[NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md).

| Input | Pin | Use |
| --- | --- | --- |
| Morpheus stem sources `vbs.simp.ml`, `vbs.cmp.ml`, `vbs.cmp.lsj`, `nom01`, `nom03`, `nom04`, `nom05`, `lemlist` | `PerseusDL/morpheus@b1b33c56`; each hash equals the existing manifest | Key, stem-class and compound-derivation witnesses |
| Morpheus `doc/morpheus.html`, `stemlib/Greek/xsl/lemmaRegistry.xsl`, `stemlib/Greek/makefile` | same commit | Stem-file format; LSJ-keyed lemma registry |
| Gorman `Xen_Anab_book_1.1-5.xml`, `Xen_Anab_book_1.6-9.xml`, README, licence | `vgorman1/Greek-Dependency-Trees@d823e96d` (blobs `e3b2a6e3`, `6ef273f3`) | Training-label lineage witness only |
| UD Ancient Greek-Perseus train/dev/test, README, LICENSE | `331ddef9`; byte-identical to the #2050 pins | AGDT-to-UPOS conversion evidence |
| UD docs `_grc/pos/*`, `_u-pos/{AUX_,VERB,PROPN,PART,ADV,NUM,DET}`, `_u-dep/cop`, `_u-overview/simple-syntax` | `UniversalDependencies/docs@24add7d0` | Category and copula definitions |

A single Perseus Hopper request for one LSJ entry returned HTTP 405. It was not
retried or worked around, and no LSJ file was acquired.

**Checks on 2026-10-15.** All of these passed:

- the read-only reproduction, including Go parity;
- all 127 companion tests;
- `python3 -I mapping_completion_v2.py`, which still compares exactly;
- `git diff --check`;
- `make lint` (golangci-lint 2.14.0: 0 issues; ruff and mypy clean);
- `make coverage-new` (NLP: 90 passed, 3 skipped; Go unit and integration).
  The research Python sits outside the coverage reports, so it reports "No
  lines with coverage information".

### B. Lexical interpretation (`oga-greta-lemma/1`)

**Evidence chain.** Each link below comes from a primary record:

1. **Training data.** The producer paper (arXiv 2410.12055v1, §3 and
   Appendix A) trains GreTa on AGDT, the Gorman Trees and the Pedalion Trees,
   all "natively annotated using the AGDT annotation scheme". Appendix A lists
   `tlg0032.tlg006` (*Anabasis*) among the training, validation and test texts.
   The paper's §3.1 normalization changes apostrophes, `_CO`/`_AP` suffixes,
   split conjunctions and ellipsis. It does not mention lemma digits.
2. **Where the digits come from.** In AGDT2 guidelines §2 (the morphological
   layer), Arethusa's morphology options "are the output of the tagger Morpheus". §2.5 says that
   "the Morpheus tagger outputs lemmas followed by a number" and that the number
   should be deleted. So any digit that survives in the lemma field is a
   Morpheus number. Gorman's README says the trees were annotated in Arethusa.
3. **Same strings on the same tokens.** Gorman's Book 1 trees carry the
   identical labels: `λέγω3` on all 49 aligned OGA `λέγω3` tokens, and also
   `ἤ1`, `θεῖος1`, `ξένος1`, `πάρειμι1`, `πρόειμι1`, `σύνειμι2`, `ἄπειμι2`,
   `ἄρωμα1` and `ἔπειμι2`. These labels are the training data's own strings,
   not an OGA-specific code.
4. **What a Morpheus key is.** In `morpheus.html`, `:le:` gives "the lemma or
   headword" of a stem entry. `lemmaRegistry.xsl` takes `entryFree/@key` from
   LSJ as the lemma ID. The makefile builds `lsj.vbs` and `lsj.nom` from LSJ
   lemmata. The keys therefore belong to the Perseus LSJ key lineage. They were
   not checked one by one against LSJ (see the 405 above).
5. **What each digit means.** Each numbered key is its own Morpheus lexical
   entry, and the stem records state how entries differ (table below).

**Rules.** The canonical key always uses the accepted `ancient-greek/1` profile
(NFC, then lowercase, then final σ, then NFC). The display headword keeps final
`ς`.

| Rule | Condition | Result | Printed lexical nodes |
| --- | --- | --- | ---: |
| `I-PLAIN` | No trailing digits | The headword is the raw lemma, which the release lowercases (`query/README.md`). No discriminator. Never a wildcard for numbered forms. | 9,163 |
| `I-NUM-MORPHEUS-KEY` | Headword plus digits equals a non-loose Morpheus key, with every diacritic kept | Headword plus discriminator `{scheme: morpheus, value: n}`; probe key `λέγω#morpheus:3` | 141 |
| `I-COMPOUND-RECORD` | No exact key; exactly one single-prefix Morpheus compound record whose prefix + base, without the hyphen, equals the label | Decodes to that record's key | 1 (`t_302`) |
| `I-UNRESOLVED-NUMBERED` | Anything else with digits | Unresolved, which blocks publication | 0 |

**The 21 labels.** "Declared" means the distinction the Morpheus record states.
"Gorman" is the training tree's label on the same aligned token.

| Label | Occ. | Morpheus key (witness) | Declared | Gorman, same token | Form check |
| --- | ---: | --- | --- | --- | --- |
| `λέγω3` | 49 | `le/gw3` (vbs.simp.ml:12240) | own `:le:` entry (`le/gw1`, `le/gw2` distinct) | λέγω3 49 | — |
| `ἤ1` | 36 | `h)/1` (nom03:3438) | `wd … conj`; `ἤ2` is `exclam` | ἤ1 21, ἤ 8, ἢ 7 | native c 31, d 5 |
| `πάρειμι1` | 9 | `pa/reimi1` (vbs.cmp.ml:4562) | `para/-ei)mi/`: παρά + εἰμί | πάρειμι 7, πάρειμι1 2 | 9 consistent |
| `εὐθύς2` | 7 | `eu)qu/s2` (nom03:1005) | adverb; `εὐθύς1` adjective | εὐθύς 7 | consistent |
| `πρόσειμι1` | 6 | `pro/seimi1` (vbs.cmp.ml:5639, 5643) | derived from **both** εἶμι and εἰμί | προσέρχομαι 5, πρόσειμι 1 | scheme-ambiguous |
| `δέω2` | 5 | `de/w2` (vbs.simp.ml:5091) | own entry (`de/w1` distinct stems) | δέω 5 | — |
| `ἔπειμι2` | 5 | `e)/peimi2` (vbs.cmp.ml:3089) | ἐπί + εἶμι | ἔπειμι2 3, ἐπέρχομαι 2 | 5 consistent |
| `ξένος1` | 4 | `ce/nos1` (nom04:10739) | noun; `ξένος2` adjective | ξένος1 3, ξένος 1 | consistent |
| `πρόειμι2` | 3 | `pro/eimi2` (vbs.cmp.ml:5221) | πρό + εἰμί | προέρχομαι 2, πρόειμι1 1 | **3 inconsistent** |
| `ἄπειμι1` | 3 | `a)/peimi1` (vbs.cmp.ml:912) | ἀπό + εἰμί | ἀπέρχομαι 3 | **3 inconsistent** |
| `ἄπειμι2` | 3 | `a)/peimi2` (vbs.cmp.ml:908) | ἀπό + εἶμι | ἄπειμι2 2, ἀπέρχομαι 1 | 3 consistent |
| `περίειμι1` | 2 | `peri/eimi1` (vbs.cmp.ml:4820) | περί + εἰμί | περίειμι 2 | 2 consistent |
| `χράω2` | 2 | `xra/w2` (vbs.simp.ml:22963) | own entry | χράω 2 | — |
| `θεῖος1` | 1 | `qei=os1` (nom03:4889) | adjective; `θεῖος2` noun | θεῖος1 1 | consistent |
| `πάρειμι2` | 1 | `pa/reimi2` (vbs.cmp.ml:4559) | παρά + εἶμι | παρέρχομαι 1 | consistent |
| `πιστός2` | 1 | `pisto/s2` (nom05:4877) | adjective, same stem as `pisto/s1` | πιστός 1 | — |
| `πρόειμι1` | 1 | `pro/eimi1` (vbs.cmp.ml:5218) | πρό + εἶμι | πρόειμι1 1 | consistent |
| `σύνειμι2` | 1 | `su/neimi2` (vbs.cmp.ml:6338) | σύν + εἶμι | σύνειμι2 1 | consistent |
| `ἄρωμα1` | 1 | `a)/rwma1` (nom01:11021) | noun, same stem as `a)/rwma2` | ἄρωμα1 1 | — |
| `ἔπειμι1` | 1 | `e)/peimi1` (vbs.cmp.ml:3092) | ἐπί + εἰμί | ἐπέρχομαι 1 | **inconsistent** |
| `σύνλέγω2` | 1 | → `sulle/gw` (vbs.cmp.ml:6459) | record `su/n-le/gw2 sulle/gw`: συλλέγω, no digit | συλλέγω 1 | decoded |

"Form check" compares the stem of each surface form with the base its key
declares. The reviewer table in `COMPOUND_SURFACE_BASE` assigns the stems:
εἶμι shows ἰ-/ᾐ-/ῄ-, and εἰμί shows ἦν/ὤν/ὄντ-/εἶναι. Of the 35 compound
occurrences, 22 are consistent, 7 are inconsistent and 6 are ambiguous in the
scheme itself. The seven inconsistent ones (`t_1590`, `t_3153`, `t_3251`,
`t_3800`, `t_5625`, `t_5642`, `t_5708`) are **upstream prediction errors**,
and the adapter keeps them unchanged. Native POS agrees with the declared
stem class on 101 of 106 simplex occurrences. The other five are `ἤ1` tagged
`d`, which is variance in the native tag.

**Limits of the identity layer:**

- `πρόσειμι1` is a valid, distinct key. The scheme does not say which base it
  stands for.
- The `I-COMPOUND-RECORD` rule rests on one producer record. Rejecting it would
  leave `t_302` unresolved (S4 in §G).
- Seven predicted citation forms break Greek orthographic rules: `ἐνεἰμί` ×3,
  `σύλλέγω`, `σύνεἶδον`, `κατάκαθέζομαι`, and `ἑπτὰ` with a grave accent.
  Like `σύνλέγω2`, they look like prefix + base notation, but no producer
  record decodes them. They are **kept as predicted and never repaired**.
  Their headwords fail the card-headword check (`ἐνεἰμί` is omitted). S3
  treats them as unresolved instead.
- The plain and numbered forms of nine headwords stay separate identities, for
  example `λέγω` 10 and `λέγω#morpheus:3` 49. The same holds for `πάρειμι`,
  `δέω`, `χράω`, `πιστός`, `ξένος`, `ἔπειμι`, `πρόειμι` and `ἄπειμι` 1/2.
  Merging them would need equivalence evidence, which is absent.

### C. POS interpretation (`oga-agdt-upos/1`)

**Primary evidence.** UD Ancient Greek-Perseus is the AGDT-to-UD conversion
(its README: "UPOS: converted from manual", converted by G. Celano). Its
202,989 tokens are paired with native XPOS, giving an observed letter mapping:

| Letter | UPOS |
| --- | --- |
| a | ADJ 21,626 |
| d | ADV 19,008 |
| i | INTJ 424 |
| l | DET 11,831 |
| n | NOUN 41,252 |
| p | PRON 12,633 |
| r | ADP 10,649 |
| u | PUNCT 24,963 |
| g | PART (absent from OGA) |
| m | NUM (absent from OGA) |
| c | CCONJ 6,695 / SCONJ 2,299 |
| v | VERB 35,836 / AUX 1,875 |

The code fails closed if any single-valued letter ever becomes multi-valued.

| Rule | Basis | Nodes |
| --- | --- | ---: |
| `P-LETTER-{a,d,i,l,n,p,r,u}` | Single-valued conversion | 7,740 |
| `P-C-PAIR` | (c, lemma) pair attested with a single UPOS; otherwise CCONJ\|SCONJ, determinate non-vocabulary | 756 (all attested) |
| `P-V-EIMI-PNOM` | εἰμί (exact diacritics) with a PNOM child, directly or through COORD/APOS → AUX | 124 |
| `P-V-EIMI-OTHER` | Other εἰμί → AUX | 54 (IDs in the summary) |
| `P-V-VERB` | Every other verb, including εἶμι and all εἰμί compounds | 1,788 |
| Unknown tag; `AuxV` on another verb | Fail closed as unresolved | 0 |

Why the context-dependent cases come out as they do:

- **Nominals and adjectives with an ADV relation stay NOUN/ADJ.** AGDT2 §2.6
  tags a word as adverb only once it has attained adverb status. UD Perseus
  v2.11 moved "adverbially used nominals from advmod to obl" and kept them
  NOUN. The conversion is single-valued even for those relations. So syntax is
  not converted into lexical ADV. This affects 411 n nodes with ADV and 113 a
  nodes with ADV, AuxY or AuxZ.
- **Copula.** UD cop says "verbal copulas are tagged AUX", and that existential
  *be* may be a copula when it is the same verb as in equational clauses. UD
  simple-syntax analyses one-verb languages as copular across equation,
  attribution, location, possession and benefaction. AGDT2 §3.7 defines PNOM as
  the complement of a copulative verb, and §4.1.9.4 labels the periphrastic
  participle PNOM. The UD grc AUX page covers periphrasis. UD Perseus has εἰμί
  `cop` AUX 1,863 times and VERB twice. All 11 of its tokens with the misspelt
  lemma `είμί` are VERB, which shows that the converter keys on the lemma.
  - That makes the 124 PNOM cases well supported.
  - The other 54 include locative and possessive copulas, plus pure
    existentials where UD makes the verb the head and leaves its UPOS
    unspecified. AUX there follows the conversion. VERB is S1.
- **Proper names stay NOUN.** The conversion has no PROPN, and capitalization
  and Morpheus starred keys are not used.
- **Particles follow the native tag.** AGDT2 §2.6 says sentence-adverb
  particles "should be tagged as adverbs". §2.8 treats inferential, causal and
  some adversative conjunctions, such as μέντοι, as adverbs, and UD Perseus pairs `d` δέ (2,322),
  καί (1,828), μέν, γάρ, ἄν and others with ADV. The UD grc pages define
  DET/NUM/PART more narrowly (they give δή and αὖ as PART). AGDT2 §2.9 has its
  own numeral POS, which the conversion maps to NUM, but OGA never predicts it,
  so cardinals arrive as `a` and become ADJ. That divergence is measured in S2,
  not adopted.

The conversion is single-valued, so the letter rule also covers 452 printed
nodes whose (letter, lemma) pair never occurs in UD Perseus: n 272, v 74, a 69,
d 29, p 3, u 5.

### D. Denominators, sentences, selection

| Count | Value |
| --- | ---: |
| Native / unprinted (`e_*`) / printed | 10,572 / 110 / 10,462 |
| Punctuation | 1,157 |
| Non-vocabulary lexical | 3,288: DET 1,047; PRON 721; ADP 579; CCONJ 541; SCONJ 215; AUX 178; INTJ 7 |
| **Eligible occurrences** (analyzable denominator) | **6,017** |
| Unresolved printed nodes | **0** |

**Sentences (`oga-reading-terminal/1`).**

- Segmentation runs over the exact 59,353-code-point reading text. A sentence
  ends at `.` (372) or `;` (9; Perseus's ASCII question mark), plus any
  following closers. Ano teleia `·` (226) is medial.
- Any `!`, `?`, U+037E or U+0387 fails the run, and none occurs. The Book ends
  on a terminal, and no parenthesis contains a terminal.
- That gives **381 complete sentences**. All 607 native groups nest whole
  inside them (groups per sentence: 1–7), and none crosses a boundary.
- All nine `;` boundaries were inspected. `ἔφη Ὀρόντας .` (1.6.7, 3 units) is a
  complete short assent.
- 85 sentences contain empty nodes, which are never printed.

**Lengths.** Units are whitespace-delimited after an outer trim, and code points
are counted on the same trimmed text.

| Measure | Units | Code points |
| --- | --- | --- |
| Range | 3–109 | 13–625 |
| Median | 24 | 138 |
| p90 | 49 | 275 |

- 10 sentences have at most 5 units. No minimum applies.
- 36 sentences exceed 50 units and 4 exceed 400 code points; none exceeds only
  the code-point cap. 345 sentences are within both caps.
- The PAULA text adds a space next to punctuation that the TEI does not have
  (1,151 before punctuation, 5 after an opening parenthesis), so punctuation
  counts as units. Without punctuation units, only 21 sentences exceed 50.
- PAULA also splits 47 words, for example `οὐ δὲ`, `οὔ τε`, `κ ἂν` and
  `τ οὔμπαλιν`; 33 sentences contain such a split.
- The text is reported as bound, never respaced.

**Selection.** One analyzed Book; Known, Reserved, corrections, exclusions and
external frequencies are all empty. With a single Book the across-Books total
equals the in-Book count, so 3/15 reduces to at least three occurrences. The
probe supplies the total as `currentReadingCandidateFrequency` does.

| Measure | Value |
| --- | --- |
| Identities | **1,428**: NOUN 489, VERB 521, ADJ 279, ADV 139 |
| Selected | **437**: NOUN 146, VERB 153, ADJ 79, ADV 59 |
| Selected identities with a discriminator | 11 |
| Selected occurrences | 4,796 of 6,017 (79.7%) |
| Known-token coverage | 0.0%; all 1,428 Unknown; the snapshot would freeze all 437 |

The real Go probe printed `candidates=1428 selected=437
analyzable_tokens=6017 distinct_identities=1428 sentences=381`. Its TSV is
byte-identical to the model. The most frequent identities are δέ|ADV 326,
καί|ADV 236, κῦρος|NOUN 170, οὐ|ADV 103, μέν|ADV 100 and βασιλεύς|NOUN 73.

### E. Deterministic sentence inputs (`grc-earliest-capped/1`)

For each selected identity, the rule takes the earliest occurrence whose
sentence:

- is complete;
- has at most 50 units and 400 code points;
- contains the verified target span (its reading slice equals the surface);
- has a headword that passes the orthographic check.

Ties go by span start, then node ID. Nothing is shortened, and the frozen
selection does not change.

- **432 eligible, 5 omitted.**
  - Over a cap in every occurrence: οἴχομαι (3), πατήρ (4), πρότερος|ADJ (3)
    and `ἄπειμι#morpheus:1` (3, all three of them mispredictions).
  - `ἐνεἰμί` (3): malformed headword, and also over the unit cap.
- 56 inputs use a later occurrence. The earlier rejections were 75 over the
  unit cap and 19 over the code-point cap.
- The chosen sentences run 4–50 units, median 27.5, across 178 distinct
  sentences.
- Flags on chosen inputs:
  - 36 sentences contain a word-internal split;
  - 9 contain editorial positions;
  - 40 targets are capitalized nouns;
  - 1 sentence has at most 5 units.

**Examples.** Each example is the first case that satisfies its stated rule. All
are private in `examples.json`, and the summary only records whether each
exists:

- ordinary complete sentence;
- short sentence chosen (`t_2855`, 4 units);
- cap rejection followed by a later choice;
- repeated target;
- lexical discriminator (`t_507`);
- compound-record decoding (`t_302`);
- εἰμί AUX without PNOM (`t_139`);
- lowercased name as NOUN;
- particle as ADV;
- word-internal split;
- elision (`t_128`, not chosen);
- editorial material;
- empty node in the sentence;
- optional morphology absent;
- malformed citation form (`t_437`);
- omission.

**NFD source text is absent**: the reading text is entirely NFC.

### F. Practical quality

These are measured, not certified. Each is either an upstream prediction or a
representation fact that the adapter preserves.

| Finding | Kind | Effect |
| --- | --- | --- |
| Discourse particles and conjunction uses tagged `d` become ADV (AGDT2 §2.6; conversion) | Mapping, faithful | 24 selected ADV identities (1,041 occurrences) have lemmas that UD Perseus elsewhere tags g or c: δέ, καί, μέν, τε, γάρ, ἄν, δή, οὖν, μή, ἀλλά, ὡς, ὅτι, ὥστε… |
| Names are NOUN, and their headwords are lowercase (`query/README.md:32`) | Conversion and producer convention | 40 selected NOUN identities (453 occurrences) are always capitalized in the Book |
| Cardinals and adjectival pronouns tagged `a` become ADJ | Conversion | Applying the UD grc DET/NUM/PART definitions (S2) selects 17 fewer identities |
| Wrong εἰμί/εἶμι homograph in compounds | Upstream prediction | 7 occurrences; `ἄπειμι#1` is selected and then omitted |
| Malformed predicted citation forms | Upstream prediction | 7 occurrences in 5 identities; `ἐνεἰμί` is selected and then omitted |
| Numbered and plain forms of one headword split | Training-data convention | 9 headwords, e.g. λέγω 10 + λέγω#3 49 |
| PAULA spacing and word splits | Representation | Units are inflated; card sentences differ from the printed word division |
| Punctuation and relation artifacts | Producer data | `(` carries lemma U+0314 (×5); relation `AuxΖ` with a Greek Zeta (×4). None affects vocabulary |
| Training overlap | Lineage | Gorman's Book 1 is in the training texts. OGA matches Gorman's human lemma on 8,636 of 8,926 aligned lexical nodes and its native POS letter on 8,839, so these predictions are not held-out |

### G. Sensitivity (not adopted)

| Scenario | Eligible occ. | Identities | Selected | Unresolved |
| --- | ---: | ---: | ---: | ---: |
| Adopted | 6,017 | 1,428 | 437 | 0 |
| S1: εἰμί without PNOM as VERB | 6,071 | 1,429 | 438 | 0 |
| S2: UD grc DET/NUM/PART definitions | 5,723 | 1,403 | 420 | 0 |
| S3: malformed citation forms unresolved | 6,010 | 1,423 | 436 | 7 (publication blocked) |
| S4: compound-record decoding rejected | 6,016 | 1,428 | 436 | 1 (publication blocked) |

### H. Verdicts

| Layer | Verdict |
| --- | --- |
| Binding / reading text | Verified baseline, unchanged |
| Canonical identity | **Established under `oga-greta-lemma/1`.** Qualifications: the S3/S4 policy choices; `πρόσειμι1` derivation ambiguity; LSJ keys not checked one by one |
| Canonical POS | **Established under `oga-agdt-upos/1`**, with S1 and S2 as the open policy alternatives |
| Sentences | Complete; native groups corroborate the boundaries |
| Selection and Known comparison | Measured, with real Go parity |
| Card inputs | 432 / 437 |
| Vocabulary-ready accounting | Complete, conditional on #2030 accepting S1–S4 as adopted, and on practical usefulness |
| Practical quality | Concerns measured in §F; no source or MVP acceptance |
| Unperformed | Gloss, translation, dictionary coverage, Anki/client rendering, human example review |

For #2030 to decide:

1. Is the producer's `d`→ADV for particles acceptable? It makes δέ, καί, μέν
   and similar words vocabulary.
2. Names as lowercase NOUN headwords.
3. Malformed predicted forms: keep them as predicted (adopted) or treat them as
   unresolved (S3).
4. The `σύνλέγω2` compound-record decoding (S4).
5. Pure-existential εἰμί as AUX (S1).
6. PAULA spacing in card sentences.

None of these needs a new survey.

## Prior mapping-completion review revision (historical, superseded above)

**Neither canonical layer is established, so the pass stopped before the
sentence, selection, cap, and omission checks.** The retained binding stays
verified. This is a bounded negative result: no Book 1 vocabulary is ready, and
none was published.

This revision supersedes the first proposal (commit `eb2e9f70`). That proposal
used accent-erased keys, called candidate matches "resolved", treated POS
candidate evaluator output as 938 "blocked" nodes, and had a silent unknown-tag
default. Its files are kept as historical evidence: `mapping_completion.py`,
`test_mapping_completion.py`, and `mapping-completion-summary.json`. Do not run
the historical `run` command for reproduction, because it overwrites its tracked
summary and private ledgers.

Reproduce with `python3 -I mapping_completion_v2.py` from this directory. The
default run recomputes everything in memory, checks retained inputs against
pinned hashes, and compares the committed `mapping-completion-v2-summary.json`
and the private v2 ledgers. It exits 1 on any mismatch and writes nothing.
`--write` regenerates the summary and the private v2 ledgers under
`private/mapping-completion-v2/`. Tests: `python3 -m unittest discover -p 'test_*.py'`.

### What is verified

- Native nodes: 10,572 (`private/import/native-nodes.json`, sha256 `2a5aca90…`),
  matching `import-ledger.json` on 10,572 native, 110 empty, and 10,462 printed
  nodes. Printed AGDT POS counts also match the ledger.
- CoNLL-U member `tlg0032.tlg006.perseus-grc2.tok01_sentence-seg01_annotated_lemma.conllu`,
  sha256 `0f9cbba8…`, matching `archive-members.json`. All 10,462 printed nodes
  join it and match on surface, lemma, and native POS. The 110 empty nodes are
  counted separately (all 110 have a row). The file has 65,022 rows, and 54,450
  rows have ids that are not native nodes. Their scope was not checked in this
  pass. Nothing here claims they are Book 1.
- Morpheus index (`b1b33c56…`, index sha256 `57f81c86…`, manifest `bb672f88…`):
  188,173 records. Undecodable records are counted by class and never dropped
  silently: 362 undecodable records (`le-block` 208, lemlist 154) have no head and
  never match. 129 decodable records are loose and never approve.
- Producer documents (`producer-docs.json`) are checked by SHA-256. The UD
  Ancient Greek-Perseus README is pinned to commit `331ddef9…`. The UD UPOS
  index's original URL was not recorded, so it is listed as unknown. The index
  lists UPOS names and short labels only; it is not evidence for any category
  mapping.

### Identity layer: not established (0 canonical identities adopted)

Exact matching uses NFC, lower case, and final sigma folded to sigma. Every
diacritic is kept. Accent-erased matches are listed as near witnesses only and
never approve.

| Decision (per label) | Labels | Occurrences | Meaning |
| --- | ---: | ---: | --- |
| `candidate-match-under-unvalidated-hypothesis` | 19 | 135 | One exact key with this headword and digit. The digit correspondence between OGA and Morpheus is unvalidated. |
| `source-key-collision` | 1 (πρόσειμι1) | 6 | The same exact key from two producer derivations (`vbs.cmp.ml:5639` and `:5643`). This is shared output, not proof of distinct homographs. |
| `headword-absent-in-pinned-source` | 1 (σύνλέγω2) | 1 | No producer key or derivation with this headword. The spelled prefix-compound derivation `su/n-le/gw2` yields `sulle/gw` with no digit (`vbs.cmp.ml:6459`). |

- Total: 21 labels and 142 numbered occurrences. The 135 candidate occurrences
  are a hypothesis, not identity. The seven occurrences in the collision and
  absent decisions are one subset diagnostic, not the count of unresolved items.
- The earlier figure of 7 blocked occurrences is a subset of the 142, and the
  earlier "135 resolved" wording is withdrawn.
- No loose-decoded record approved any label. The earlier `ἦ1`→`ἤ1` and
  `εὒθύς2`→`εὐθύς2` matches are regression fixtures (`test_mapping_completion_v2.py`)
  and no longer resolve.
- Not established: that OGA digits mean Morpheus digits. No retained producer
  document establishes that correspondence, so every candidate remains a hypothesis.

### Canonical POS layer: not established (0 UPOS mappings adopted)

The values below are candidate proposals from `propose()`. They are exploratory
diagnostics, not verified mappings and not blockers. Candidate sets are checked
against the 17 UD UPOS tags.

- 10,462 printed nodes: 8,816 single-candidate proposals and 1,646 multi-candidate
  proposals. Unsupported native tags: 0 (the verdict would become `unsupported`
  if any appeared; unknown tags are never upper-cased).
- Native tags in Book 1 (AGDT letters, matching the import ledger): `a` 928, `c` 756,
  `d` 1,491, `i` 7, `l` 1,047, `n` 1,810, `p` 721, `r` 579, `u` 1,157, `v` 1,966.

Main proposal rules (counts of printed nodes):

- `u`→PUNCT 1,157; `i`→INTJ 7; `l`→DET 1,047; `p`→PRON or DET 721 (multi).
- `c`: COORD→CCONJ 473; AuxC→SCONJ 277; AuxY→CCONJ 5; ADV function (πρίν) 1 (multi).
- `r`: AuxP→ADP 577; other relation 2 (ADP or SCONJ, multi).
- `d`: ADV, ATR, OBJ, OCOMP, PNOM, AuxY, AuxZ→ADV 1,476; COORD, AuxC, or AuxP
  function 11 (multi, ADV plus another tag); unrecognised relation 4 (multi).
- `a`: ADJ 815; ADV, AuxY, or AuxZ relation 113 (multi, ADJ or ADV).
- `n`: ADV relation 411 (multi, NOUN or ADV); starred-only Morpheus key 283 (PROPN);
  unstarred-only 733 (NOUN); mixed or absent 383 (multi, NOUN or PROPN).
- `v`: copula εἰμί with a PNOM dependent in the same sentence 115 (AUX); other
  verbs 1,851 (VERB, including 63 εἰμί nodes without a PNOM dependent).

Limits of the POS candidates:

- **Syntactic function is not lexical status.** A noun or adjective with an ADV
  relation yields a multi-candidate proposal. AGDT §2.6 says attained adverb status
  is annotated as an adverb. It does not say every nominal or adjectival head with
  an adverbial relation changes POS, and this pass does not decide that.
- **Proper names.** Starred Morpheus keys are an exact-key proposal of PROPN only.
  Whether starred-only lemmas match UD PROPN convention, and whether the OGA
  lineage is the same, is unestablished. The 283 PROPN proposals are not
  verified exclusions or verified PROPNs.
- **Copula.** The AUX proposal follows the UD Perseus changelog. The consulted
  README alone is not a validated complete conversion, so the rule is a proposal.
- **Particles** (`d` AuxY and AuxZ) propose ADV. That is inside the current
  `AllowedPOS` default (NOUN, VERB, ADJ, ADV). This pass does not use `AllowedPOS`
  as an adoption criterion, and it does not claim ADR 0005 forces a new product
  decision. Any practical choice about function words is a #2030 quality
  question, not a blocker here.
- **Native POS counts** are preserved from the retained AGDT tags. They are not
  eligibility denominators.

### Verdicts and boundaries

- Canonical identity: **not established**. The digit correspondence is an
  unvalidated hypothesis, and no discriminator interpretation is adopted.
- Canonical POS: **not established**. Proposals are unadopted. No unsupported tag
  is present in Book 1, so no layer failed closed on an unknown tag.
- Sentence, selection, cap, and omission: **not run**. Both canonical layers are
  not established, so no tested subset is claimed as a ready Book.
- The historical proposal's counts and claims (7 of 142 blocked, 938 blocked,
  4,865 eligible, 278 PROPN exclusions, 178 AUX, 1,788 VERB) are withdrawn.
  The v2 counts above replace them.

### Not run

- No Greek provider, model, dictionary, or training-corpus call was made.
- No archive redownload, and no edits to `private/archive*`, `private/inputs`,
  `private/text`, `private/import`, or accepted documents.
- Repository checks are reported separately in the handoff.

## Prior stop after explicit archive authorization (historical, 2026-10-14)

**Acquisition and complete Book 1 text binding succeeded; vocabulary-ready
publication stopped at required producer lexical decoding.** This is a bounded
negative full-workflow result, not a successful import into learner vocabulary,
source/MVP acceptance, or evidence that OGA is linguistically unusable.

Justin [explicitly authorized the official archive](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178890938),
superseding only the original no-archive acquisition limit. The same research
task/branch continued. No fallback corpus/edition, accounts/challenge bypass,
analyzer/model execution, provider calls, learner-state changes, cards, production
changes, PR or merge occurred. The previous gate evidence remains immutable at
`4104c8be238fe0d4b5500f8e18522e80dd516467`; `gate.py`, `evidence.json`, `ledger.json`
and the historical sections below still reproduce **that initial assessment**.
The current verdict is [import-ledger.json](ancient-greek-oga-anabasis1-import-proof/import-ledger.json).

### Exact acquired components and rights

The one ordinary official request completed: **8,629,032,478 bytes**, published
MD5 **`d4117ae52f1cc6319c6ba20af0c1cf3b`**, locally computed SHA-256
**`fffc246e9833f7ef265f0daffd1f4cc679489cb6de1fd9b652474a6313ecf6c3`**.
The source is [Zenodo record 14206061](https://zenodo.org/records/14206061),
`opera_graeca_adnotata_v0.2.0.zip`. A documentary subagent mistakenly unlinked
the partial file. Its original open inode was retained and restored locally
after the original request completed; size and checksums were independently
verified again. There was **no second download, network range extraction,
alternate source or bypass**. This incident is not an upstream access failure.

Only nested local ZIP directories and allowlisted Anabasis members were read;
no corpus-wide decompression or analysis occurred. See
[archive-members.json](ancient-greek-oga-anabasis1-import-proof/archive-members.json)
for every exact container/member, size, CRC and SHA-256, and
[unit-inputs.json](ancient-greek-oga-anabasis1-import-proof/unit-inputs.json)
for acquisition provenance. Important containers/members:

- `original_Greek_files.zip`: the original
  `canonical-greekLit-0.0.11376425141/data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`
  (within `original_Greek_files/`) is **byte-identical** to the upstream pinned
  work XML, SHA-256 `865cf2c761e55a0f603a2ddbb58a08e94934f963724552b268a1493e4b9ae2ec`.
- `workspace/merge.zip`: `merge/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`.
- `workspace/oga.zip`: `oga/oga_v0.2.0_2/tlg0032.tlg006.perseus-grc2/`
  carries primary PAULA text/ranges/features; `oga/laula/tlg0032.tlg006.perseus-grc2/`
  carries original-text references and native sentence groups.
- `workspace/conllu.zip`: the matching work's native-format file is retained,
  not mistaken for canonical UD merely because its container says CoNLL-U.

The archive's own README **99–107** explicitly grants CC BY-SA 4.0. Its README
**12–40** documents the PAULA, LAULA, original and normalized representations.
The original XML header identifies **Xenophon, Anabasis, E. C. Marchant,
Opera Omnia volume 3, Clarendon Press 1904/reprint 1961**, Perseus/Tufts and
preparation contributors, without a component-specific rights exception.
The upstream repository's explicit default BY-SA grant remains evidence.
[NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md) now separately
assesses these exact components: private inspection and bounded attributed
diagnostics proceed; full text/annotations/maps stay Git-ignored locally.
Exports/bundling, provider use and semantic/artifact acceptance are not approved.

### Full Book 1 binding and explicit reading projection

The proof measures **Book 1 only**, retaining the indivisible work unchanged:

| Measurement | Verified result |
| --- | --- |
| Extent | **1.1.1–1.10.19**, 10 chapters / 205 sections |
| Native nodes | **10,572**, required lemma/POS/CTS targets and text ranges resolved |
| Printed nodes | **10,462**, `t_1`–`t_10462` |
| Unprinted nodes | **110**, `e_1`–`e_110`; 95 literal `[0]`, 15 literal `[1]` |
| PAULA Book 1 source span | Unicode code points **[0, 59793)** |
| Marker-free reading | **59,353** code points; UTF-8 SHA-256 `0f18cae963899218fda6c62726d6a0b0429763f390597c1781a93d80c2ddd907` |
| Exact non-whitespace correspondence | **48,892 characters**, every section verified against both original and merged TEI |

Rules are versioned **`oga-anabasis1-native-binding/1`**. PAULA annotation-carried
text is the declared reading-text authority (ADR 0093); it is not casual joining
of lemma/token strings. Every 1-based native range is verified as a half-open
Unicode-code-point span. All adjacent node spans have exactly one source space;
there are no overlapping ranges, unexplained gaps or omitted printed characters.
Printed referents agree exactly with the separate LAULA token attributes.

Remove **only** verified `e_*` source spans and their following single separator:
each also has native POS `-` and a zero-length, empty LAULA referent. No regex
deletes lookalike printed content. All printed characters, polytonic marks and
remaining PAULA spacing are unchanged, including spaces around punctuation.
Every reading character maps back to its exact PAULA source coordinate; empty
nodes remain retained evidence with **no invented reading span**.

The original and merged Book 1 section string-values are exact, not merely NFC
equivalent. TEI comparison omits explicit `del` text, chooses `corr` inside
`choice`, retains `add` and standalone `sic`, and permits **only whitespace
reflow**. Significant accents, case, apostrophes and punctuation are not folded.
Every printed character maps monotonically to its original TEI section
string-value coordinate, including split forms; this is not fuzzy alignment or
an inferred lexical span. Suppressed editorial material remains in the originals
and the diagnostic element spans; it is not silently made a learner exclusion.
These rules describe this exact release's correspondence, not universal Greek
tokenization or an accepted production adapter.

Optional LAULA pointers are separately qualified: **486 nodes in 19 sections**
do not reproduce the claimed referent when their literal element path is
resolved and the element string-value is whitespace-folded. For example,
`t_876` at 1.2.5 restarts at position 1 but targets the section rather than its
later paragraph; `t_3328` at 1.3.20 resets after editorial markup. These are
representation diagnostics, **not failures of the independently verified PAULA
binding** or a claim that every possible LAULA implementation behaves identically.
Also, **47 empty nodes** carry a citation different from their preceding printed
node. Their raw metadata is retained, not used to reorder text or invent words.
The ledger lists exact IDs/sections for both qualifications.

### Required lexical stop, not a vocabulary result

**142 printed occurrences have 21 distinct raw lemma strings ending in digits**,
including 49 `λέγω3` predictions, nine `πάρειμι1`, one `πάρειμι2`, three each of
`ἄπειμι1` and `ἄπειμι2`. The ledger lists **all affected native IDs, citations,
raw lemma values and native POS**; they are not hidden by defaults or exclusions.

The release's `query/README.md:32` documents lowercased NFC lemmas, and its
`query/morphology.md` documents native POS meanings. The pinned OGA README
[66–78][oga] attributes automatic parser/lemmatizer outputs to AGDT-trained
models, linking the [producer paper](https://arxiv.org/abs/2410.12055).
That is provenance, **not a verified decoding table or lexical namespace for
these exact predicted numeric strings**. The investigated documentary evidence
has not established their source-declared discriminator meanings, namespace,
or complete citation-headword decoding. This is a bounded verification gap,
not a claim no upstream explanation can exist or that numeric predictions are
necessarily errors. An unpinned mapping draft was not promoted into authority.

The main session completed the draft's documentary gaps, retaining exact
originals/hashes in [producer-docs.json](ancient-greek-oga-anabasis1-import-proof/producer-docs.json):

- [Producer paper v1 §3.1](https://arxiv.org/html/2410.12055v1#S3.SS1)
  describes heterogeneous training conventions and normalization, including
  numbered ellipsis placeholders; §4.4 describes character-sequence lemma
  prediction. It does **not** define these lexical numeric endings as verified
  dictionary IDs. Aggregate model scores do not certify Book 1 predictions.
- [Lemmatizer README at `267dbf045dfca1129115aef05ba9ac6ebc49f4db`](https://git.informatik.uni-leipzig.de/celano/lemmatizer_for_oga/-/blob/267dbf045dfca1129115aef05ba9ac6ebc49f4db/README.md)
  identifies **GreTa**, its 91.41 score and inference interface, but supplies
  no numbered-lemma citation/discriminator decoding. Only the README was
  retrieved, not code, models or training data; no commands in it were run.
- [AGDT2 guidelines §2.5 at `0ebc9289bc6c8eb1069fac3433955f83f442762f`](https://github.com/PerseusDL/treebank_data/blob/0ebc9289bc6c8eb1069fac3433955f83f442762f/AGDT2/guidelines/Greek_guidelines.md#vrb)
  say Morpheus numbers **should be deleted in the annotated lemma field**.
  This is genuine annotation-policy evidence, not a dictionary namespace or
  proof that a learned OGA prediction of either contrasting numbered form is
  a valid source-declared identity. Mechanically importing this deletion rule
  into canonical identity would erase the distinction ADR 0094 requires us
  to preserve, without verifying what the differing predictions signify.

An initial arXiv `v2` HTML request returned 404; the existing `v1` above was
read normally. No challenge, alternate corpus or archive transport was involved.

Do not strip the digits, silently merge the contrasting forms, manufacture an
`OGA`/`Morpheus` homograph equivalence scheme, or publish numbered raw predictions
as verified citation headwords. This blocks required identities under ADR 0094.
**No canonical mapping was adopted for the other nodes either**: native `n/v/d`
are not automatically universal UD NOUN/VERB/ADV, and no capitalization-based
PROPN or blanket εἰμί→AUX exclusion was imported. Native POS counts in the ledger
are **native**, not canonical vocabulary/eligibility denominators.

Per the task's stop condition, downstream sentence completeness/practical input
judgments, Go selection/statistics, independent model/Known comparison, 3/15
selection, cap distribution, deterministic inputs and omissions are **unperformed,
not zeros**. Native sentence groups and optional annotation files are retained
but not promoted into optional-capability success to bypass this required stop.
Gloss/translation, dictionaries, Anki/client artifacts and their semantics remain
unassessed. No learner state or supported-language implementation changed.

### Comparison and decision returned

OGA now has a verified, maintainably pinned complete Book 1 **text-binding
result**, beyond its preserved Stage 1 example. Its marker-free authority and
original-text transformation are explicit; optional link metadata needs care.
Required lexical decoding remains unresolved, so there is no comparable accepted
selection or dry-input yield. This research does not select or reject OGA for
the live [source decision](https://github.com/justin-hayes/mouseion/issues/2030).
Any continuation must first establish the exact producer decoding/mapping
evidence, not repeat the archive download, thin Book 1 or switch editions.

The retained GLAUx proof remains provisional: 1,294 proposed identities, 373
selected and 372 dry inputs are **not accepted vocabulary or a ready deck**.
Retain its 63 lexical words in editorial deletions, presentation/headword/POS
and sentence-boundary qualifications, and its unsupported blanket εἰμί→AUX
rationale affecting 180 occurrences (cited UD evidence: 1,864 AUX and two VERB).
No count difference establishes superiority or a prediction error by itself.
The exact OGA original/merged release has a `sic/corr` choice at **1.3.20**;
GLAUx's retained `text_error_propagated` example instead targets **1.3.14**.
That particular row is **not a verified witness** to this 1.3.20 correction.
The general revision qualification remains; neither earlier branch is modified
and no GLAUx remediation becomes a prerequisite.

### Reproduce the current bounded result

Python **3.13 / Unicode 15.1.0**, standard library only. With the retained official
archive in Git-ignored `private/archive/`, one offline command rechecks the whole
archive size/MD5/SHA-256, extracts only allowlisted work members, checks member
and retained producer-document hashes, validates the Book 1 binding/projection, compares the current
ledger **byte-for-byte**, reconstructs private coordinate artifacts and runs all
focused tests:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 doc/research/ancient-greek-oga-anabasis1-import-proof/import_gate.py --from-archive
```

Without `--from-archive`, it verifies retained selected-member hashes and reruns
the same binding/ledger/tests, without rereading 8.63 GB. `--write` explicitly
regenerates the ledger, never overwrites differing private projections, and is
not used for verification. The official archive request is not retried by any
reproducer; independent acquisition needs explicit access/scope authorization.
The original one-shot acquirer and extraction helper remain inspectable code.
The supplemental producer originals must also be present under
`private/producer-docs/` at the hashes in `producer-docs.json`; they are small
documentary inputs, not part of the corpus archive. No supplementary document
or archive retrieval occurs implicitly in this offline command.

**61 tests pass**: the 17 historical metadata tests, 10 acquisition tests, eight
invented-ZIP tests and 26 invented-Unicode/XML binding/identity-stop tests. These
exercise wrong ranges/bases/references, missing nodes/interior sections/chapters,
duplicate targets, Unicode NFD/non-BMP coordinates, typed versus printed marker
lookalikes, edition/character disagreement, lexical omission, entity declarations,
ZIP bounds/path safety and unsupported numbered identity defaults. Invented
fixtures are not claimed as source observations or sentence-quality tests.

Full originals, the complete native-node manifest, reading text and coordinate
maps stay private; public hashes/element spans/blocker IDs allow inspection of
the bounded claims. Producer model/version/configuration specific to each
released layer is not reconstructed or executed; undocumented details stay
unknown. The older documentary-only command below remains a reproduction of
the **initial** acquisition assessment, not the present verdict.

Validation on **2026-10-14**: archive-inclusive and retained-member-only commands
reproduce byte-identical current diagnostics; every selected member's original
size/SHA-256 was independently checked again, and public section/native/blocker
totals reconcile. `git diff --check` passes. Reattempted `make lint` is blocked
by missing **golangci-lint 2.14.0**; `make coverage-new` stops at the missing
**`.venv`/pytest** preflight. Neither whole-project check is claimed to pass.

## Historical initial gate result

**Result: no complete-unit import was attempted.** The examined official download
inventory supplies one whole-corpus archive, not an individual Anabasis package.
That route is explicitly prohibited by
[Prove a complete OGA Anabasis Book 1 import into Mouseion vocabulary workflows](https://github.com/justin-hayes/mouseion/issues/2052).
No permitted individual-work package was established, so the task stops at Gate 1.
This is a bounded negative acquisition result, **not evidence that OGA lacks
Anabasis or is linguistically unusable**, and not source/MVP acceptance.

Companion [registry](ancient-greek-oga-anabasis1-import-proof/inputs.json),
[documentary evidence](ancient-greek-oga-anabasis1-import-proof/evidence.json),
[verdict ledger](ancient-greek-oga-anabasis1-import-proof/ledger.json),
[operation-specific notice](ancient-greek-oga-anabasis1-import-proof/NOTICE.md), and
[reproducer/tests](ancient-greek-oga-anabasis1-import-proof/gate.py).

## Exact availability and acquisition assessment

Assessment date: **2026-10-14**. OGA documentation is pinned to
**`a69d917dbe117a307829977dd556e22f05ce0715`**, also the observed `main` HEAD.
This pins documentation and the example, **not a full-unit release payload**.

1. [OGA README, lines 11–41][oga] names the source releases, links the corpus
   download to Zenodo, and separately advertises hosted ANNIS queries and a
   desktop corpus. Its [example README][example] directs users to Zenodo too.
   The [complete, non-truncated Git tree][tree] has no Anabasis payload path;
   its annotation example is `ggm0001.ggm001.1st1K-grc1`, not Xenophon. No
   example was substituted for Book 1.
2. [Zenodo record 14206061][zenodo] lists exactly one file:

   | Field | Published value |
   | --- | --- |
   | Release | OGA `0.2.0`, DOI `10.5281/zenodo.14206061`, 2024-11-24 |
   | File | `opera_graeca_adnotata_v0.2.0.zip` |
   | File UUID | `96c1ae43-3193-4386-94ec-4f62263b1cae` |
   | Size | **8,629,032,478 bytes** (8.63 GB decimal) |
   | Published checksum | `md5:d4117ae52f1cc6319c6ba20af0c1cf3b` |
   | Scope | Whole-corpus archive; **not acquired** |

   These are inventory declarations, not an inspected archive-member manifest.
   No archive SHA-256, member path, unit size or complete Book 1 coverage is
   invented. The archive content endpoint is deliberately absent from the
   reproducer's request allowlist. No HEAD, GET, range/stream extraction, mirror
   or alternative transport request to that file was made.
3. [OGA's bibliographic list, lines 5562–5570][bibliography] has one target
   record: `tlg0032.tlg006.perseus-grc2`, Xenophon, *Anabasis*, Marchant,
   *Xenophontis Opera omnia Volume III Expeditio Cyri*. This establishes an
   advertised **work/edition lead**, not Book 1 endpoints, interior coverage,
   original text correspondence or annotation availability in an inspected unit.
4. The original-text lead is canonical-greekLit release `0.0.11376425141`.
   Its [tag reference][tagref] identifies annotated **tag object**
   `90ac67b935af3b46d6f910c1e173ffd99c657d78`; [peeling that object][tag] gives
   **commit** `476fb6133498e5710de7eec0b492791da86be4b4` (not the tag object).
   The [work-directory listing][listing] advertises
   `data/tlg0032/tlg006/tlg0032.tlg006.perseus-grc2.xml`, **886,347 bytes**,
   Git blob `a220f4b8accd1448a0189a595493a78aa09c707b`.
   The [CTS metadata][cts] agrees on the edition identifier and bibliography.
   **The Greek work file was not fetched.** Its internal structure, full extent,
   SHA-256 and relationship to OGA remain unverified. An individual original
   text without the accompanying OGA annotation does not satisfy this task.
5. The [official website][site] sends downloads to the same Zenodo record.
   Its [ANNIS page][annis] documents querying/visualization, not an advertised,
   revision-pinned, source-preserving individual Anabasis package. No hosted
   corpus query/export, endpoint probing or reconstruction by paginating matches
   was performed. This does **not** establish that ANNIS can never export such
   data, or that every query is a prohibited survey. A suitable supported unit
   export remains **unestablished**, not silently treated as archive access.

**Gate decision:** no demonstrated acquisition route satisfies the existing
individual-unit/work-package boundary. The observed archive route is out of
scope regardless of download feasibility. No package was acquired while deciding
its permission, and no Gate 2 work followed the stop. The bounded inventory is
not a global claim that no other file or future upstream export exists.

## Component rights and the actual decision needed

OGA [README lines 162–170][oga] explicitly declares **CC BY-SA 4.0** and refers to
the original-text repositories. The declaration expressly accompanies a README
that identifies the corpus/release. Zenodo's `other-at` shorthand is recorded
without treating it as a competing prohibition, a missing license or a veto.
Likewise, the upstream [README][upreadme] explicitly licenses repository contents
BY-SA 4.0 unless otherwise indicated, while retaining Tufts's general copyright
and varying-component notices. Neither notice automatically cancels that grant
or makes contacting anyone a prerequisite.

Those declarations support this documentary assessment; they do not establish
which exact uninspected text/header/annotation components a full-unit package
would contain or their verified derivation. **Exact package/component clearance
is unperformed, not a finding that private computation is prohibited.** No
alternative license was guessed, and no legal or Greek-classification question
is delegated to Justin. Local inspection, supported acquisition, public source
redistribution, exports/bundling and external-provider operations have separate
dispositions in [NOTICE.md](ancient-greek-oga-anabasis1-import-proof/NOTICE.md),
following [Choose the licensing and distribution boundary for classical data](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026).

The concrete scope decision returned to Justin is whether to **leave this OGA
proof deferred under the current no-archive boundary**. Continuing is not
authorized on present evidence. It would require an identified permitted unit
package (with its component review), or a separately explicit change of scope.
This task does **not** offer an archive retry as an executable next step, create
an access/survey prerequisite chain, contact anyone, or choose another corpus,
edition or Reading unit. The live source/usefulness decision remains
[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030).

## Unperformed layers and comparison

| Layer | This OGA task |
| --- | --- |
| Acquisition | **Stopped**: no demonstrated in-scope individual package |
| Exact full unit / edition | Bibliographic lead only; completeness and exact edition unverified |
| Binding / clean reading-text projection | Not performed |
| Required lemma / UPOS / discriminators / headwords | Not performed |
| Complete sentences / verified targets | Not performed |
| Native morphology / syntax / optional capabilities | Not assessed on Book 1 |
| Real Go selection / independent model / Known comparison | Not run |
| Counts / 3–15 selection / selected occurrence coverage | **Unmeasured, not zero** |
| Deterministic sentence inputs / cap distribution / omissions | **Unmeasured, not zero** |
| Practical examples / prediction quality | No Book 1 examples acquired or judged |
| Dictionary / Gloss / translation / Anki / client rendering | Not performed |

Consequently there are no Book 1 node, selection, sentence, card-input or omission
manifests masquerading as successful empty results. The ledger uses `null` for
those measurements. No Greek service/analyzer/model, provider call, dictionary
acquisition, card construction or learner-state change occurred.

The retained [OGA official-example format probe][stage1] and the retained
[GLAUx complete-unit proof][glaux] are unchanged. This task does not redo their
tests or promote their conclusions beyond the independent parent verification:

| Comparison axis | OGA evidence | GLAUx evidence |
| --- | --- | --- |
| Acquisition / maintenance | Documentary pin and linked sample work; no demonstrated complete-unit package under this task's limits | Individually addressable pinned annotation and text files; real full-unit reproduction exists |
| Coherent complete text/analysis | Stage 1 shows linked-format feasibility only; no Book 1 binding | Parent checked all 10,645 native-node correspondences, 10 chapters/205 sections and full source-span coverage |
| Required repair / practical quality | Full-unit requirements and prediction quality unassessed, not presumed better or worse | 63 unannotated lexical words in editorial deletions; editorial-sign presentation, headword/POS quality, a boundary case and the annotated 2019 text's 1.3.14 typo remain qualifications |
| Vocabulary / dry inputs | Unmeasured | Proposed interpretation: 1,294 identities, 373 selected, 372 deterministic inputs; **not accepted vocabulary or a ready deck** |
| Rights | Explicit declarations preserved; unacquired exact components unassessed | Exact-file declarations and prior operation-specific assessment exist, not blanket source/export approval |

In particular, GLAUx's blanket εἰμί→AUX rule affects 180 occurrences. The cited
UD evidence has **1,864 AUX and two VERB** εἰμί occurrences; a supported
rationale/refinement is still needed. Neither all 180 exclusions nor all 180
being errors follows mechanically. No such rule is copied into OGA.
Edition-aware output comparison is impossible without OGA's exact acquired text.
Different future counts would not by themselves establish errors or superiority;
neither GLAUx nor human annotation is assumed always correct. Parent
[comparison/qualifications](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178553143)
remain controlling. No candidate or whole-Anabasis readiness is accepted here.

## Reproduce and independently inspect the stop

Python **3.13 / Unicode 15.1.0**, standard library only. One command, repo root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 doc/research/ancient-greek-oga-anabasis1-import-proof/gate.py --fetch
```

This fetches **only 13 allowlisted documentation/inventory inputs**, validates
their hashes, derives the evidence/ledger, compares both **byte-for-byte**, and
runs **17 focused tests**. It never follows response URLs, XML references or
redirects, never retries automatically, and cannot fetch archive/text/annotation
payloads through its allowlist. `--fetch` means documentary retrieval, **not
Gate 2 acquisition**. Without that flag it is offline. `--cache PATH` selects
retained inputs; `--write` explicitly regenerates the two diagnostic files.

The registry records original response byte sizes/SHA-256. API JSON also has a
canonical-JSON SHA-256 over **every parsed field**, because GitHub returns
equivalent JSON with different whitespace/content negotiation. Evidence hashes
declare that distinction. Raw documentary originals are unchanged in Git-ignored
`ancient-greek-oga-anabasis1-import-proof/private/inputs/`; no full-unit source
or normalized reading text is there. The full bibliographic inventory is not
republished: only the target record is projected publicly. Published checksums
of unacquired payloads are clearly distinguished from locally verified hashes.

API/website responses are dated snapshots, not immutable upstream objects.
Changed metadata must fail verification; it must not silently refresh this
assessment. Independent reviewers can inspect the cited inventories and the
public target-specific evidence even if raw response snapshots later change.
The Git tree and raw repository files use the declared revisions. No global
inventory size is used as Book 1 coverage evidence.

Validation on 2026-10-14: retained-input and fresh-documentary retrieval both
reproduce exact diagnostics and pass all 17 tests. Initial raw-byte-only API
verification rejected equivalent JSON formatting; canonical whole-JSON checking
was added and tests cover both serialization equivalence and changed values.
Mutation/failure tests additionally reject wrong/incomplete tree revisions, new
unreviewed target packages, missing/duplicate/wrong work or edition references,
changed release inventories, tag/commit confusion, missing declarations,
non-allowlisted payload URLs, corrupt inputs and redirects. They are **metadata
gate tests**, not span/mapping/sentence validation or claims of corpus correctness.

`git diff --check` passes. Whole-project `make lint` stops because
`golangci-lint` 2.14.0 is missing; `make coverage-new` stops because the project
`.venv`/pytest environment is absent. No full-tree lint or application coverage
success is claimed for this standalone research artifact. No production code,
schema, dependencies, ADR, glossary, earlier research branch, PR or merge changed.

[oga]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/README.md
[example]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/annotation_example/README.md
[tree]: https://api.github.com/repos/OperaGraecaAdnotata/OGA/git/trees/a69d917dbe117a307829977dd556e22f05ce0715?recursive=1
[bibliography]: https://github.com/OperaGraecaAdnotata/OGA/blob/a69d917dbe117a307829977dd556e22f05ce0715/urn_cts/texts/urn_cts_plus_date_label.xml#L5562-L5570
[zenodo]: https://zenodo.org/api/records/14206061
[site]: https://varro.informatik.uni-leipzig.de/oga/en/index.html
[annis]: https://varro.informatik.uni-leipzig.de/oga/en/annis.html
[tagref]: https://api.github.com/repos/PerseusDL/canonical-greekLit/git/refs/tags/0.0.11376425141
[tag]: https://api.github.com/repos/PerseusDL/canonical-greekLit/git/tags/90ac67b935af3b46d6f910c1e173ffd99c657d78
[listing]: https://api.github.com/repos/PerseusDL/canonical-greekLit/contents/data/tlg0032/tlg006?ref=0.0.11376425141
[cts]: https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/data/tlg0032/tlg006/__cts__.xml
[upreadme]: https://github.com/PerseusDL/canonical-greekLit/blob/476fb6133498e5710de7eec0b492791da86be4b4/README.md
[stage1]: https://github.com/justin-hayes/mouseion/blob/f0034fea98458491ce24413c8a24717c41b3cfd8/doc/research/ancient-greek-oga-format-probe.md
[glaux]: https://github.com/justin-hayes/mouseion/blob/957c99f25de9ea16e139055e6a87bbcda1996cc9/doc/research/ancient-greek-glaux-anabasis1-import-proof.md
