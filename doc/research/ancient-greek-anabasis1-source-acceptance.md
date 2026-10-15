# Practical acceptance review: OGA *Anabasis* Book 1 as the first Ancient Greek slice (#2030)

Status: **reviewer recommendation, 2026-10-15.** This is not source acceptance.
Justin decides on [#2030](https://github.com/justin-hayes/mouseion/issues/2030),
after the parent's independent review. Scope:
[authorizing comment](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6187239562).
Nothing was fetched or downloaded. No repair, mapping, selection, stopword,
Gloss, provider, deck or production change was made.

## 1. Recommendation

**Accept** Opera Graeca Adnotata v0.2.0, Xenophon *Anabasis* Book 1 (Marchant
1904, as pinned in §2), as the **first planned MVP slice**. Disclose two things
with it:

- **automatic annotation provenance**;
- **the explicit, versioned research-repair layer**: 7 citation repairs and 6
  εἰμί POS repairs.

Acceptance would mean exactly this:

1. **A source choice that ends the source-fidelity loop.** This edition, release
   and unit become the bounded candidate that #2031 plans around. No further
   annotation repair, source comparison or acquisition research is needed to
   make that choice.
2. **Not readiness.** Shipping, cards and any Mouseion-hosted data still pass
   the separate implementation, presentation and permission gates in §6.
   Failing one of those later gates blocks that stage; it does not retroactively
   mark the #2052 proof as blocked. A gate that proves concretely infeasible, or
   a newly discovered fatal source or rights issue, may legitimately reopen the
   source choice. Ordinary gate work should not restart open-ended source
   research.
3. **Not a gold standard.** The predictions remain attributed automatic output.
   This limited inspection found specific, named identity errors (§4.4, §4.5).
   Disclosure does not make them correct. Nothing is certified or silently
   fixed.

The reasons, from the actual outputs:

- **Content vocabulary in the sample is plausible.** All 12 content words in a
  deterministic sample have a plausible lemma and a useful sentence input. Twelve
  items are a spot check, not an error rate.
- **The known identity problems fall into named families.** These are
  lexical-prediction errors (wrong Morpheus homograph numbers; wrong name
  lemmas) and duplicated presentation (one lexeme split across two selected
  identities). Each case is listed by node ID. This was not an exhaustive audit
  of the 434 inputs, so no global error rate or upper bound is claimed.
- **Particles, name headwords and PAULA spacing are real costs, but bounded
  ones.** The span binding remains exact. Spacing affects the reading text,
  sentence caps and which sentences are eligible (§4.6).
- **The main known operational cost is acquisition.** The demonstrated route is
  the official 8.6 GB corpus archive. The retained investigation did not
  establish a supported per-work package or a complete machine-readable
  catalogue. That is a #2031/#2035 engineering and redistribution-review gate,
  not evidence that the slice is unusable.

No new stopword strategy is needed (§4.3). Justin's stopword idea stays a
recorded fallback. This review does not use it as permission to change
`AllowedPOS`, selection or coverage.

## 2. Exact source identity, and what is (not) being asked

| Item | Pin |
| --- | --- |
| Annotation release | Celano, *Opera Graeca Adnotata* v0.2.0, [doi:10.5281/zenodo.14206061](https://doi.org/10.5281/zenodo.14206061). The archive `opera_graeca_adnotata_v0.2.0.zip` is 8,629,032,478 bytes, MD5 `d4117ae52f1cc6319c6ba20af0c1cf3b`, SHA-256 `fffc246e…ecf6c3`. Members are listed in [`archive-members.json`](ancient-greek-oga-anabasis1-import-proof/archive-members.json). |
| Text and edition | canonical-greekLit `0.0.11376425141`, `tlg0032.tlg006.perseus-grc2.xml` (886,347 bytes, SHA-256 `865cf2c7…e2ec`). Byte-identical to the upstream file at `476fb613`. Marchant, *Opera Omnia* vol. 3, Oxford 1904. |
| Unit | Book 1, 1.1.1–1.10.19: 205 sections; 10,572 native nodes (10,462 printed and 110 empty); 48,892 non-whitespace characters with exact TEI correspondence; 381 complete sentences. |
| Reading-text authority | The PAULA annotation-carried text (ADR 0093), with explicit TEI correspondence: `del` omitted, `corr` chosen, `add` and standalone `sic` retained. |
| Interpretation | `oga-greta-lemma/2` + `oga-agdt-upos/2` + εἰμί rule v3 + [`opus-repair-records.json`](ancient-greek-oga-anabasis1-import-proof/opus-repair-records.json) (`oga-anabasis1-opus-repair/1`). Normalization profile `ancient-greek/1`, unchanged. |
| Proof commits | Repair proof `230decab`, review fix `edea7210`, parent verification `c29412d5` (all on this branch). |

**Asked:** whether this exact edition, release, unit and interpretation should
be the first planned slice.

**Not asked, and not settled here:**

- Books 2–7 or the whole work;
- Gloss, translation or dictionary coverage;
- Anki or client rendering;
- provider, export, bundling or redistribution permission;
- architecture, schema or adapter design;
- any change to the repair layer.

The eventual goal is still the complete *Anabasis*. Book 1 proves nothing
about the quality or completeness of Books 2–7.

## 3. Method

[`acceptance_review.py`](ancient-greek-anabasis1-source-acceptance/acceptance_review.py)
(`oga-anabasis1-acceptance-review/1`) reads only retained outputs:

- the repaired node ledger, card inputs and selection model;
- the v2 sentence extents;
- the TEI coordinate map.

It verifies each input's pinned SHA-256 before use. It also reads the GLAUx
selection model through `git show` from immutable commit `957c99f2` and checks
that file's hash too.

Before using its own choice function for any sensitivity measure, it replays all
438 recorded sentence choices and fails on any disagreement. It also fails unless
every sentence's PAULA unit count equals its printed-node count.

Each sample category below is chosen by a stated rule, not by inspection. The
content sample, for example, takes every 27.08th entry of the 325 plain,
lowercase-attested NOUN/VERB/ADJ selected identities, sorted by key.

Outputs:

- **Public:** [`review-summary.json`](ancient-greek-anabasis1-source-acceptance/review-summary.json),
  SHA-256 `11350dec26b58e9d849f9327c0f3e91069263e45ad6470d5db71835d7d09dc82`.
  It holds counts, node IDs, citations and short lemma/surface labels.
- **Private:**
  `ancient-greek-oga-anabasis1-import-proof/private/acceptance-review/examples.json`,
  SHA-256 `62062067785ae6ea76aae4bda63ace7456d44469fbc896d406f459966b29de11`.
  It holds the full text of every sentence referenced below, the target for each
  of the 438 identities, and all 47 word-split pairs. Like the other private
  outputs, it is Git-ignored.

The reviewer read every sampled sentence in Greek. The judgments below are
practical judgments about learning, not scholarly certification. No provider
translation was used.

## 4. Findings from representative outputs

Each finding separates four things: the **observed defect**, the **possible
learning cost**, what remains **unresolved**, and the **reviewer's judgment**.

### 4.1 Ordinary content vocabulary and sentence lengths

The content sample has 12 identities. Each is listed as identity, target node,
section:

| Identity | Target | Section |
| --- | --- | --- |
| αἰσθάνομαι | `t_1834` | 1.2.21 |
| διαβαίνω | `t_914` | 1.2.6 |
| θάλασσα (target θάλατταν) | `t_311` | 1.1.7 |
| λόφος | `t_10208` | 1.10.13 |
| πέντε | `t_1066` | 1.2.8 |
| πλοῖον | `t_911` | 1.2.5 |
| στρατηγός | `t_51` | 1.1.2 |
| τρεῖς | `t_509` | 1.1.10 |
| χρύσεος (target χρυσαῖ) | `t_1226` | 1.2.10 |
| ἀποθνήσκω (target ἀποθνῄσκοντος) | `t_9623` | 1.9.31 |
| ἐμβάλλω | `t_1053` | 1.2.8 |
| ἡδύς (target ἥδιον) | `t_3986` | 1.4.9 |

- **Observed:** for all 12, the headword is a plausible citation form of the
  target, and the sentence is a complete, coherent source sentence. Attic -ττ-
  and iota-subscript forms keep their exact surfaces against standard headwords.
  ἥδιον is used adverbially but identified as ADJ, which is producer-compatible.
- **Judgment:** these are plausible lemmas with useful sentence inputs, and none
  needs repair. They are dry inputs only: no card was built, and Gloss,
  dictionary coverage and client rendering are untested. Twelve items do not
  give a general error rate.

The chosen sentences are 4–50 units long, median 27.5, across 179 sentences:

- shortest: sentence 114 (4 units);
- median: sentence 245 (26 units, 22 in TEI spacing);
- longest: sentence 268 (50 units, 45 in TEI spacing).

All three are complete and readable. The 50-unit sentence is long for a card
but is within the accepted #2034 cap.

### 4.2 Repair-affected items

- **ἔνειμι** is newly selected (4 occurrences). Its input is `t_5775`, 1.6.3, a
  13-unit sentence beginning ἐνῆν δὲ ἐν τῇ ἐπιστολῇ. The other three
  occurrences are over the cap. **Judgment:** a plausible lemma and a useful
  sentence input, which the repair made possible. The #2052 positive evidence
  for the repair stands. Dictionary coverage of the headword was not checked,
  and no card was built.
- **Changed εἰμί readings.** Five reviewed nodes are now AUX and non-vocabulary:
  `t_2739`, `t_3351`, `t_7640`, `t_10265` and `t_10412`. `t_5989` (1.6.6, ἔστιν
  ὅ τι) is now VERB. The εἰμί|VERB input moved to `t_3631`, 1.4.4. The target is
  the ἦν in **καὶ παρελθεῖν οὐκ ἦν βίᾳ**, under ledger rule
  `P-V-EIMI-IMPERSONAL-INFINITIVE`: the impersonal "it was not possible [to pass
  by force]". The same 43-unit sentence also contains copular and locative
  ἦν/ἦσαν, but those are other nodes, not the target. **Judgment:** a sensible
  impersonal example. Card value depends on the Gloss, which is unassessed. The
  two open readings (`t_5651`, `t_9711`) cannot change the selected identities
  or the inputs.
- **The formed citation συνεῖδον** (`t_5145`, 1.5.9) is an eligible vocabulary
  identity: 1 occurrence, 1 of the 1,426 identities.
  - It is not in this Book's 438-identity frozen selection and has no sentence
    input, because it occurs once.
  - It still appears in the Browse vocabulary inventory and in the reading
    source.
  - It could be selected under other conditions, for example with multi-Book
    across-Books counts or different Known/Reserved state.
  - Its medium-uncertainty disclosure still applies; being unselected does not
    hide it.
- **Selected citation repairs:** συλλέγω, ἑπτά and ἔνειμι. All are common
  content words with plausible sentence inputs, pending Gloss, dictionary and
  client checks.

### 4.3 Particles and conjunction uses as ADV

| | Count |
| --- | --- |
| Selected ADV identities | 59, with 1,395 occurrences |
| …whose key also occurs as CCONJ/SCONJ/PART/DET/PRON in this Book | 13 (δέ, καί, τε, ἄν, ὡς, μή, ὥστε, ἀλλά, εἶτα, ἤ#1, ὅς, ὅτι, ὥσπερ), 765 occurrences (15.9% of selected occurrences) |
| The broader v2 comparison against UD Perseus | 24 identities, 1,041 occurrences |

Earliest inputs:

- δέ: `t_17`, 1.1.1, ἐπεὶ δὲ;
- καί: `t_50`, 1.1.2, καὶ … δὲ, "also";
- μέν: `t_9`;
- γάρ: `t_254`;
- ἄν: `t_516`.

All are in ordinary complete sentences.

- **Slot impact: none.** Selection is a frequency threshold, not a quota:
  `readingFrequencyEligible`, `internal/selection/eligibility.go:52`, with no
  card cap. Particle identities therefore displace no content word. They add
  about 13–24 cards (3–5.5% of 438).
- **Learning value:** these are the most frequent words a reader of the text
  must know anyway. Once Known, they carry a large share of coverage.
- **Possible cost:** a short contextual Gloss for δέ/μέν can be weak, or the
  Gloss can abstain. Under #2034 an abstention omits that card, which is
  existing, accepted behaviour.
- **Not unique to OGA.** GLAUx also selects οὐ and μή as ADV.
- **Judgment:** acceptable. **No stopword strategy is needed.**

Five selected ADV identities have keys that also occur as ADJ elsewhere in the
Book: καλός, κακός, ὅς, ταχύς, ὕστερος. For καλός/κακός/ταχύς/ὕστερος the
adverb sits under the adjective headword, the same producer convention GLAUx
has. ὅς instead covers the adverbial relatives οὗ and ᾗ. The headword cost is
small.

### 4.4 Names and lowercase headwords

40 selected NOUN identities never appear in lowercase. They cover 453
occurrences, and all 40 have inputs. The top five are κῦρος 170, ἕλλην 54,
κλέαρχος 34, τισσαφέρνης 20 and μένων 10. Headwords are lowercase by producer
convention, while sentences keep their capitals. Two selected ADJ identities
are also always capitalized: λακεδαιμόνιος and ἑλληνικός.

**Observed upstream defects within these names:**

| Name | What happens | Nodes |
| --- | --- | --- |
| Μένων | Split: oblique forms → μένων (10); nominative → **μένω** NOUN (5), the citation form of the verb "remain" | `t_939`, 1.2.6 |
| Ὀρόντας | Split into ὀρόντης (10) and ὀρόντας (3) | `t_5653`, `t_5810` |
| Κιλικία | Split into κιλικία (6) and κιλίκια (3) | `t_1841`, `t_1710` |
| Ξενίας | → **ξενία** ("hospitality") (6) | `t_82`, 1.1.2 |

These are two different kinds of problem:

- **Lexical-prediction errors.** `ξενία` NOUN is a name lemmatized as the common
  noun for "hospitality", so the name shares that noun's identity. `μένω` NOUN is
  not the name's lemma at all, and its headword is the verb "remain".
- **Duplicated presentation.** Ὀρόντας and Κιλικία are each one name spread
  across two selected identities. Μένων is split as well, and one half of its
  split is the wrong `μένω` lemma above.

**Learning cost.** Duplicated pairs affect both cards in each pair. Wrong
identities, once Known, also affect future counts and what is recorded as known.
For example, a Known `ξενία` NOUN would cover a later real occurrence of
"hospitality" in another Book.

**Judgment.** Acceptable, with disclosure, for a bounded first slice. The view
that names are lower-value cards is the reviewer's judgment, not a fact about
every name. Mouseion's occurrence-scoped learner corrections (ADR 0081) are the
established route for an owner who wants to fix these; whether they apply to
Greek is not assessed here.
- Capitalized display headwords would need a presentation rule with its own
  evidence. Not adopted.

### 4.5 Wrong homographs and plain/numbered splits

Of the seven recorded wrong-homograph predictions, six reach selected
identities. Every occurrence of those two identities is a misprediction:

- **πρόειμι#morpheus:2** (`t_1590`, `t_5625`, `t_5642`). The key declares the
  εἰμί base; the forms are from εἶμι. **It has a card: `t_1590`**, 1.2.17,
  θᾶττον προϊόντων.
- **ἄπειμι#morpheus:1** (`t_3153`, `t_3251`, `t_3800`). It is frozen in the
  snapshot but has no card, because all three of its sentences are over the
  cap.

**Learning cost:**

- A contextual Gloss of προϊόντων ("advancing") would be right. Only the hidden
  homograph number is wrong. That matters if a dictionary is keyed by Morpheus
  number, which is a dictionary-stage gate (§6).
- On Finish, both identities would become Known under the wrong number. A
  Known πρόειμι#2 would also match any later Book's genuine εἰμί-based πρόειμι#2.
  ADR 0097 freezing stops later evidence from rewriting that fact. It does not
  stop a wrong prediction from becoming Known.

**Plain/numbered splits.** Four headwords have **two selected identities**:

| Headword | Identities |
| --- | --- |
| λέγω | 10, all εἶπεν, + #3 49 |
| δέω | 8 + #2 5; both include middle forms such as δεῖται / ἐδεῖτο |
| πάρειμι | 4 + #1 9; both εἰμί-based |
| ἄπειμι | #1 3 + #2 3 |

In the λέγω, δέω and πάρειμι pairs, both identities are selected and both have
sentence inputs. Each pair therefore shows the learner two cards under one
headword. This is duplicated presentation, not proof that either label is a
wrong prediction. The ἄπειμι pair is different: its #1 half is the wrong-number
error above. The splits are preserved, not merged, as required. Because
identities are compared on the exact key, marking `λέγω` Known does not cover
`λέγω#morpheus:3`.

**Judgment:** a real cost. One chosen card target carries a wrong homograph
number, and three headwords appear as duplicated cards. Do not merge or relabel
silently. A later owner correction, or a reviewed versioned identity rule, could
address this as new evidence under ADR 0097, without rewriting frozen state.

### 4.6 Sentence-input omissions and PAULA spacing

PAULA puts a space before every punctuation mark and splits 47 words. Measured
against the exact TEI coordinate map, there are 1,203 joins: places where PAULA
has a space that the TEI does not.

**Reading harm, observed:**

- 33 sentences contain a split. 17 of the 179 chosen sentences do, and those 17
  serve **36 of 434 inputs (8.3%)**.
- The pairs are `οὔ τε`/`οὔ τʼ` (18), `οὐ δὲ`/`οὐ δʼ` (19), `κ ἂν` (2),
  `μή τε`/`μη δὲ` (3), and `τ αὐτὰ`, `τ οὔμπαλιν`, `τ ἆλλα`.
- Examples:
  - sentence 89 (1.2.26), inputs φημί, χείρ and ἐθέλω: `οὔ τε … οὔ τε`;
  - sentence 110 (1.3.11): `οὐ δʼ ἀμελεῖν`;
  - sentence 125 (1.3.20): `κ ἂν μὲν ᾖ` for κἂν.
- No selected target is itself a split fragment. Still, 37 of the 103 οὐ
  occurrences are the first halves of οὐδέ or οὔτε. GLAUx's οὐ absorbs these
  words in the same way.

**Unit inflation, measured.** These counts are a sensitivity check, not a rule
change:

| | PAULA spacing | TEI spacing |
| --- | --- | --- |
| Sentences over 50 units | 36 | 21 |
| Selected identities with a sentence input | 434 | 437 |
| Of the 58 inputs that use a later occurrence, those that could use the first | — | 26 |

Only οἴχομαι stays omitted under TEI spacing, which is also GLAUx's only
omission. With TEI spacing:

- **πατήρ** (`t_8708`) and **πρότερος** ADJ (`t_7139`) would gain inputs;
- `ἄπειμι#1` would also gain one (`t_3153`), but that identity is a
  misprediction.

**Judgment:**

- The source span binding stays exact. The cost is still real: a learner sees
  `κ ἂν`, and three identities lose sentence inputs.
- This is not just display styling. Removing whitespace changes the analysed
  reading text, its coordinates and which sentences fit under the caps.
- The retained TEI map supports this sensitivity measure. It is not a validated
  or adopted new projection.
- Any future TEI-spaced derivation would need:
  - a versioned text mapping, the kind of "explicit versioned transformation
    provenance" #2034 permits;
  - a full rerun of binding, coverage, target-span, sentence and cap checks.

  It must not assume the 437 inputs above are ready. Whether to do this is a
  #2031 adapter stage decision. It is not implemented or adopted here.
- **Judgment:** an acceptable, bounded source-quality cost for a planning
  choice. It is not guaranteed to be a trivial cosmetic fix, and it does not need
  fixing before the slice is accepted.

### 4.7 Editorial material

16 sentences contain editorial elements:

- 8 `del`, omitted from the reading;
- 1 `choice`/`corr`/`sic` at 1.3.20, where the reading is ἢν;
- 2 `add`, retained unmarked;
- 7 standalone `sic`, retained unmarked.

Six chosen sentences contain editorial elements: 56, 125, 137, 279, 336 and
357.

- **Observed:**
  - Secluded words are simply absent. Sentence 56 at 1.2.12 reads cleanly.
  - Supplements and cruces appear without brackets or †. For example, sentence
    279 at 1.8.4 has a `sic` region and serves κέρας.
  - At 1.3.20, OGA annotates the corrected ἢν (`t_3328`) as ἐάν, SCONJ,
    non-vocabulary.
- **Judgment:**
  - Better for a learner than showing deleted words without brackets.
  - Unmarked cruces and supplements are a minor presentation qualification.
    Six chosen sentence inputs are affected.
  - No vocabulary-identity decision about seclusion is needed for OGA, because
    the deleted text is not in the annotation-carried reading.

## 5. Bounded comparison with GLAUx (#2050, `957c99f2`)

Different text revisions, units, projections and mappings mean that
**438 versus 373 is not an accuracy ranking.** GLAUx is not worse for selecting
fewer, and OGA is not better for selecting more.

**Overlap of selected identities.** The unit compared is the pair (normalized
lemma key, UPOS). OGA's Morpheus discriminators are suppressed for this
comparison only, which turns OGA's 438 selected identities into 434 pairs:

| | (key, UPOS) pairs |
| --- | --- |
| Shared | 347 |
| OGA only | 87 |
| GLAUx only | 26 |

A shared pair is a string match. It does not show that the two sources'
identities are equivalent, and it is not a canonical merge. Both annotations are
automatic and unreviewed, so these are differences in quantity and scheme, not
quality. Most of the difference is scheme mapping rather than lexical
disagreement.

- **OGA only (87).**
  - **ADJ (36):** numerals and quantifiers that GLAUx maps to NUM or DET/PRON,
    which are non-vocabulary there (δύο, τρεῖς, οὗτος, πᾶς, ἄλλος, τις…).
  - **ADV (24):** particles that GLAUx maps to PART (δέ, μέν, γάρ, δή, οὖν…).
  - **VERB (12)** include εἰμί|VERB, which is always AUX in GLAUx, and
    suppletive or homograph-numbered lemmas.
  - **NOUN (15)** include the name splits and mislemmas from §4.4.
- **GLAUx only (26).** Examples: ξενίας (a correct name); προέρχομαι,
  προσέρχομαι and ἐπέρχομαι (GLAUx lemmatizes εἶμι-compound forms such as
  προϊών under the suppletive -έρχομαι verb); κρατύς and ἀποθνῄσκω.

| Dimension | OGA v0.2.0 Book 1 | GLAUx `b077d8f6` Book 1 |
| --- | --- | --- |
| Text revision | canonical-greekLit `0.0.11376425141`; 1.3.20 reads the corrected ἢν, annotated ἐάν/SCONJ (`t_3328`) | Perseus `bd7b1c7a` (2019); 1.3.20 reads uncorrected ἦν, annotated εἰμί/AUX (node `101440265`) |
| Binding | Complete, exact TEI correspondence, parent-verified | Complete, 0 exceptions, parent-reproduced |
| Secluded text | Absent from the annotated reading; no decision needed | 63 lexical words present but unannotated; the seclusion classification is still open |
| Reading spacing | PAULA: spaced punctuation, 47 splits (§4.6) | TEI spacing; 40 suffix and 7 crasis components bound to source substrings |
| εἰμί | Context rule v3 plus 6 reviewed repairs; AUX 158 / VERB 20 | Blanket AUX for 180, an unsupported qualification per the [parent review](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6178553143) |
| Lexical homographs | Morpheus numbers preserved (141 occurrences), under a reviewed research mapping with medium-uncertainty limits, not gold. Costs: 4 double-selected headwords and 2 identities whose every occurrence carries the wrong number | The source declares no lexical discriminators; it uses a different identity scheme. Its λέγω, δέω and πάρειμι are single identities. Nothing shows whether any particular merge is right or wrong |
| Names | Lowercase headwords; 4 name problems found (§4.4) | Capitalized headwords; Μένων, Ὀρόντης, Κιλικία and Ξενίας are single identities |
| Card inputs | 434/438 (437 under TEI spacing) | 372/373 |
| Remaining work before vocabulary-ready | None, under the disclosed repair layer | Seclusion decision, and an εἰμί rule still to be supported |
| Acquisition | Demonstrated route: the official 8.6 GB corpus archive. The retained investigation did not establish a supported per-work package or a complete machine-readable catalogue | Small per-file pinned GitHub raw files; `metadata.txt` catalogue |
| Rights evidence | OGA BY-SA 4.0 (archive README); text BY-SA 4.0 | GLAUx BY-SA 4.0; text revision under a BY-SA 3.0 US README |
| Maintenance | 13 node-bound repairs; fail closed on any component change | No repair layer, but its open rules would still need deciding |

**What the comparison means.**

- **Where GLAUx is ahead:** operations, and name/lemma tidiness.
- **Where OGA is ahead:** the current corrected text, no seclusion decision,
  homograph discriminators preserved (with the costs and limits above), and a
  completed, parent-verified εἰμί and identity pass.
- **What switching would cost:** choosing GLAUx now would reopen its own
  mapping questions, the εἰμί rule and seclusion.
- **Why this review accepts OGA anyway:** it found no OGA practical defect
  severe enough to justify that switch.

GLAUx remains a retained, unaccepted alternative. Nothing was overlaid,
combined or adopted from it.

## 6. Operation-specific rights and stage gates

| Gate | Status | Blocks the source choice? | Blocks what |
| --- | --- | --- | --- |
| Private non-commercial inspection; bounded attributed diagnostics | Assessed ([`NOTICE.md`](ancient-greek-oga-anabasis1-import-proof/NOTICE.md)): OGA and text BY-SA 4.0; Morpheus witnesses CC BY-SA 3.0 US; UD Perseus witnesses CC BY-NC-SA 2.5; kept separate, no combined licence | No | — |
| Acquisition route (#2028/ADR 0096, #2035) | **Unassessed engineering and permission.** Either each installation downloads the 8.6 GB archive and extracts one work, or Mouseion redistributes a reviewed exact-component package. BY-SA is eligible after exact-component review under #2035. The retained investigation found no supported per-work manifest, so discovery may need a pinned one. Choosing between these is an engineering and operation-rights decision; no new acquisition research is authorized now. | No | Supported To Read acquisition |
| Runtime use of the repair layer and mappings | **Unassessed.** The adapter needs only the 13 repaired values and the decoding rules, not the NC witnesses. Whether shipping the repaired values or Morpheus-key validation data carries obligations has not been reviewed. | No | Shipping the adapter or app data |
| Reading presentation (spacing, editorial signs, name headwords) | Evidence is retained (TEI map, editorial spans); the projection is undecided | No | Card and reading presentation quality |
| Gloss and translation (ADR 0079, #2034) | **Unassessed**; particle Gloss abstention is possible | No | Ready cards |
| Dictionary coverage | **Unassessed**; optional under #2034. Any dictionary keyed by Morpheus number must not import the wrong sense for πρόειμι#2 / ἄπειμι#1 | No | Dictionary evidence on cards |
| Greek display and Anki polytonic rendering | **Unassessed** | No | Deck readiness |
| Provider use, exports, bundling (#2035) | **Not approved.** Sending a BY-SA sentence to the configured provider, deck exports with notices, and bundling each need their own review | No | Provider calls, exports, distribution |
| Books 2–7 | **Unproven.** The work file is present in the archive, but each Book needs its own import proof and possibly its own repairs | No (Book 1 slice) | Whole-work Reading units |

Each gate can block its own stage. If one turns out to be concretely
infeasible, it may legitimately reopen the source choice.

ADR 0097, lines 55–57, protects frozen learner facts: frozen snapshots, Known
facts and corrections keep their exact original evidence. A later repair-layer
version or OGA release must therefore be adopted as new evidence, and it cannot
rewrite them. That immutability does **not** stop a wrong prediction from
becoming Known, or from colliding with another Book's identity later. The 2
wrong-number identities are accepted here as disclosed known errors, not as
harmless ones.

## 7. Why the remaining issues are acceptable, not blocking

**Known identity issues, by family.** The audit was limited, so these counts are
not an error rate and not an upper bound.

- **Lexical-prediction errors:**
  - 2 selected identities carry a wrong homograph number throughout:
    πρόειμι#2 and ἄπειμι#1.
  - 1 chosen card target carries a wrong number (`t_1590`).
  - 2 names have wrong lemmas: `ξενία` NOUN and `μένω` NOUN.
- **Duplicated presentation:**
  - 3 headwords (λέγω, δέω, πάρειμι), each with two selected identities and two
    inputs;
  - 3 names, each split across two identities (Μένων, Ὀρόντας, Κιλικία). The
    Μένων split overlaps the wrong `μένω` lemma above.

Wrong identities that become Known affect future counts and what is recorded as
known.

**Unit inflation from PAULA spacing costs 3 sentence inputs** in this
sensitivity measure, one of them a misprediction. Recovering them would need a
versioned derivation and a full rerun (§4.6), not a display tweak.

**Particle cards cost review time but take no slots.** They teach words a reader
must know.

**Judgment:** this limited inspection found no material systemic defect for the
requested use, which is a first bounded Reading/vocabulary slice with honest
stage gates. The known errors above are real and disclosed, and the 12-item
content sample is not a general error rate. Requiring every such error to be
removed first would bring back the 100%-fidelity prerequisite that the
[scope reset](https://github.com/justin-hayes/mouseion/issues/2030#issuecomment-6177588105)
rejected. It allows automatic annotation quality with disclosure, but disclosure
does not make the data error-free.

**What would have justified deferral:**

- a systemic content-lemma problem;
- wrong sentence spans;
- a rights obstacle to self-hosted direct use;
- an acquisition route that is concretely infeasible.

This inspection demonstrated none of these. The known acquisition route is
costly; its feasibility is a stage gate.

## 8. One next step, if Justin accepts

Resume **#2031** with this slice as its named fixture. Its first act is the
human-reviewed reconciliation it already owns: the superseded
candidate-specific human-reviewed MVP wording in ADRs 0094–0096. The §6 rows
then become its stage gates.

This review opens no research ticket, repair, acquisition or other chain.

## 9. Limits

- **Book 1 only.** Books 2–7 are not inspected.
- **The sample is a judgment, not a certification.** It covers 12 content items
  plus the targeted categories. The remaining inputs were not read individually,
  so no error rate or upper bound is claimed.
- **The ADV classification is a heuristic.** The within-Book function-UPOS
  comparison is a heuristic, not gold; the v2 UD comparison gives 24 identities.
- **TEI-spacing figures are a sensitivity measure.** They are not adopted.
- **Rights statements are documentary, not legal advice.**
- **No Gorman file was read for this review.**

## 10. Reproduce

From `doc/research/ancient-greek-anabasis1-source-acceptance/`:

```sh
python3 -I acceptance_review.py          # verifies pins, replays 438 choices, compares both outputs; exit 1 on mismatch
python3 -m unittest test_acceptance_review   # 9 tests, invented fixtures
```

`--write` regenerates only `review-summary.json` and the private
`examples.json`; a guard and a test refuse any other path. The underlying proof
still reproduces unchanged:

```sh
cd ../ancient-greek-oga-anabasis1-import-proof && GOTMPDIR=$HOME/.cache/gotmp python3 -I opus_repair.py
```

Expected output: `gates_failing: []`, `reproduction_problems: []`, 438 selected,
434 card inputs.

**Checks.**

- **The reviewer ran:** the read-only acceptance review, its 9 tests, the
  read-only `opus_repair.py` reproduction, and ruff check/format on the new
  files.
- **The parent independently ran** (review of `c911cf3a`): the 9 review tests;
  the 164 proof companion tests; the read-only acceptance and proof
  reproductions, including Go parity; `make lint`; and `make coverage-new`. All
  passed. NLP reported 90 passed and 3 skipped; research Python is outside
  production coverage.
- **The parent also independently:**
  - read all 12 content sentences and their targets, plus 10 targeted cases;
  - recomputed every measurement this report cites: the spacing joins, splits
    and TEI sensitivity; the ADV, function-UPOS and name counts; and the GLAUx
    overlap.

  All matched.

Attribution: OGA v0.2.0 (Celano 2024, CC BY-SA 4.0); Xenophon, ed. Marchant,
Perseus Digital Library canonical-greekLit (CC BY-SA 4.0); GLAUx (Keersmaekers,
CC BY-SA 4.0) selection labels from `957c99f2`. The short labels and counts here
are bounded research diagnostics under those terms (see `NOTICE.md` on both
proof branches). Nothing is relicensed.
