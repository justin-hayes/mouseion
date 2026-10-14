# Lysias I lexical evidence for the vocabulary-blocked occurrences (#2049)

Status: research pass, not a readiness claim. Nothing here is adopted. The 235 T1A2 vocabulary-blocked occurrences all remain `vocab_blocked`, with unchanged eligibility, UPOS, headword and discriminator. Each proposal below needs a parent decision.

Companion folder: [`ancient-greek-lysias1-lexical-completion/`](ancient-greek-lysias1-lexical-completion/README.md). Rights: [`NOTICE.md`](ancient-greek-lysias1-lexical-completion/NOTICE.md).

Baseline: immutable reconciliation `7ac01256` (#2048, route T1A2). The prior mapping `50f4ad6c` and binding `dc3e5ed7` are inputs, not accepted policy.

## Scope and non-claims

- Covered: the 235 T1A2 nodes with `readiness = vocab_blocked`. Each is checked against the named primary sources, and each gets a before and after record with proposal, basis and open points.
- Not covered: the binding (#2048), the punctuation and segmentation work (separate agent), the #2047 measurement, OGA, Anabasis and Steadman material, any witness or corpus survey, any language model or provider call, card generation, and any manual annotation.
- Not claimed: overall readiness. Lexical resolutions here are not full readiness; the binding and segmentation questions stay with the parent.
- Labels: **exact fact** (a check run against pinned text, with file and line); **proposed interpretation** (a candidate UPOS or headword with its stated basis); **unsupported case** (no pinned source decides it, or a sub-point is unresolved); **unperformed check** (a named check not run).

## Inputs

- 20 pinned files, hashed in [`inputs/sha256sums.txt`](ancient-greek-lysias1-lexical-completion/inputs/sha256sums.txt) and registered in [`inputs/pinned-inputs.tsv`](ancient-greek-lysias1-lexical-completion/inputs/pinned-inputs.tsv): the AGDT 2 Greek guidelines, Smyth §2769, 11 UD docs (8 grc POS, 3 general POS and dependency), and seven Morpheus stem files. All 20 verify against their sha256 values (`inputs_verified = 20`).
- Upstream commits for the UD, Morpheus, AGDT guideline and Smyth files were not recorded in this pass. They are identified by sha256 only. This is a limitation of the registry, not a verified commit.
- An earlier working note said 24 files. The hash list has 20 entries, and 20 is the verified count.
- The sources are retained outside the repository for parent review. They are not committed.

## Totals (exact)

From the reconciliation manifest at `7ac01256`, reproduced by the script:

| Item | Count |
|---|---|
| Native nodes | 2,834 |
| Word / punctuation / artificial | 2,428 / 374 / 32 |
| T1A2 readiness: vocab_ready / vocab_blocked | 1,160 / 235 |
| T1A2 vocab_blocked by blocker | BI-ELIG-PARTICLE 222, BI-ELIG-NAME 8, BI-ELIG-EIMI 2, BI-HEADWORD-CONDITIONAL 2, BI-HEADWORD-UNRESOLVED 1 |
| Overlap with BT-S011-ORDER | 2: `s011w026`, `s011w029` (within the 222 particles) |
| Vocab-blocked rule split | R03P 72, R03C 70, R03A 46, R03N 32, R03S 2, R01C 4, R01A 2, R01M 2, R05X 2, R06 3 |

Every blocker union equals the 235 vocab-blocked set. The script asserts this and fails otherwise.

## Exact facts

**Particles (222 occurrences).**

- Smyth §2769, PARTICLES list (main text, 61 Greek tokens, 46 distinct): of the 222 particle occurrences, 140 exact, 17 accent variant (τε 8, γε 1, καὶ 8), 65 absent. The figures match the earlier re-fetch. The sidebar index is not used.
- AGDT 4.1.12.1 conjunction-particle list (13 items: ἀλλά, ἀτάρ, δέ, εἴτε, ἤ, ἠδέ, καί, καίτοι, οὐδέ, οὔτε, μηδέ, μήτε, τε): 70 occurrences exact (the R03C set), 8 accent variant (καὶ), 144 absent.
- UD grc CCONJ names καί, τε and ἀλλά at token level. UD grc PART names καί, and excludes "(coordinating and subordinating) conjunctions and adverbs such as καί 'even'". The UD grc SCONJ match for τε is a substring of ὅτε and is not counted.
- AGDT text (guideline §4.1.12.1, quoted with the source's own wording): "Particle are here defined as those adverbs, typically sentence adverbs, which in AG are usually in pospositive position." It also says conjunctions "have to be always annotated as conjunctions", and "Some of these particles (sic) can of course be adverbs".
- Smyth §2769 note (footnote [*]): "Under the head of particles are included sentence adverbs (1094) and conjunctions."
- The UD general PART doc lists negation among particle categories ("Particles may encode grammatical categories such as negation"). The UD grc docs do not name οὐ or μή.
- A UD-to-Smyth mismatch: the UD grc PART link text cites Smyth "631–671", while its link targets are Perseus 2769 and 3003. Recorded as an exact fact; not resolved.

**Names (8 occurrences; six lemmas).** Morpheus lookups by exact key, with line numbers (`name_checks.tsv`):

- Ἑλλάς (1): `lemlist:304` only. No `nom.proper` entry.
- Θεσμοφόρια (1): `lemlist:14894` and `entitylist.txt:9043` (`person`, `festival`). No `nom.proper` entry.
- Ἀθηναῖος (2): `lemlist:2860` exact. `nom.proper:19808` is a near match only: the entry lacks the breathing mark, so the exact key fails.
- Ἄρειας (2): `lemlist:1254` only.
- πᾶγος (1): `lemlist:39275` only.
- πάγος (1): `lemlist:39152` (common noun). `nom.proper:15918` is a capitalised near match with no name type.

**Headwords (3 occurrences).** `headword_checks.tsv`:

- ἀντί-βολέω (2): H1 fails (`:le:bole/w`, `lsj.vbs:7750`, has no `pb:a)nti/`). H2 fails (no `a)nti/-bole/w` row). H3 has exactly one candidate: `vbs.simp.ml:1964`, `:le:a)ntibole/w`, with `pb:a)nti/` at line 1966. This matches the mapping's existing conditional H3 status.
- σύν-λέγω (1): H1 fails. H2 finds `vbs.cmp.ml:6459` `su/n-le/gw2`, homograph 2, but the node's v1.6 homograph is 1, so H2 does not match. No H3 candidate.

**εἰμί (2 occurrences).** `eimi_checks.tsv`:

- εἴη (`s002w022`): `v3spoa---`, optative present active, relation OBJ.
- ἐσομένων (`s119w006`): `v-pfpmng-`, future middle participle, relation ATR.
- Neither is perfect middle, so the UD grc AUX rule does not apply. No relation is `cop`.

**Rules.** The AGDT §2.2 text: an adjective used as a noun with no separate lemma "is morphologically categorized as an adjective". The UD grc NOUN doc cites Ἀθηναῖος as a substantivised adjective. The UD grc PROPN doc: a proper noun refers to "a (more or less) unique object, such as a person or a country". The UD grc AUX doc: auxiliaries "are restricted to forms of the verb εἰμί in the perfect middle system … and perfect and pluperfect periphrastic forms".

## Proposed interpretations (not adopted)

Each proposal is an option for the parent. Counts are occurrences of the 235. The per-occurrence records are in `occurrence_patches.tsv`.

| ID | Rule and lemma | n | Proposed UPOS | Basis | Open points |
|---|---|---|---|---|---|
| P01 | R03P, lemmas other than καὶ/μὴ | 62 | ADV | AGDT 4.1.12.1: particles are "adverbs, typically sentence adverbs"; lemma outside the AGDT conjunction list | Smyth absent 26 (ἄν 21, ἔπειτα 2, εἶτα 1, ὅμως 1, σφόδρα 1); Smyth exact 35 (μέν 34, νῦν 1); accent variant 1 (γε) |
| P03 | R03A γάρ, οὖν, τοίνυν | 46 | ADV | Same basis as P01 | Smyth absent 5 (τοίνυν) |
| P05 | R03N negators οὐ, μή | 32 | PART | UD general PART doc names negation | UD grc docs silent on negation (all 32) |
| P06 | R05X εἰμί without a predicate nominative | 2 | VERB | UD grc AUX doc: AUX restricted to perfect-middle and periphrastic forms | none |
| P07 | R01A Ἀθηναῖος (plural forms) | 2 | ADJ | AGDT 2.2 plus UD grc NOUN doc | nom.proper near match only; LSJ separate-entry check unperformed |
| P08 | R01C Ἑλλάς | 1 | PROPN | UD grc PROPN doc: unique object, "such as … a country" | no nom.proper entry |
| P10 | R06 ἀντί-βολέω | 2 | none (headword unchanged) | H3 candidate `vbs.simp.ml:1964` | H3 stem-and-class condition not decided; unperformed |

## Unsupported cases (no proposal)

| ID | Rule | n | Reason |
|---|---|---|---|
| P02 | R03C conjunction-particle list | 70 | The pinned text says conjunctions are annotated as conjunctions, and that some particles "can of course be adverbs". No pinned rule decides the AuxY and AuxZ cases. Needs a relation-level rule decision by the parent. |
| P04 | R03P grave-accent lemmas καὶ (8), μὴ (2) | 10 | Accent variants of καί and μή. Reported, not folded. Whether to fold them is a parent policy decision. |
| P11 | R06 σύν-λέγω | 1 | H2 homograph mismatch and no H3 candidate. The headword remains unresolved. |
| none | R01C Ἄρειας (2), Θεσμοφόρια (1) | 3 | No `nom.proper` entry for Ἄρειας. Θεσμοφόρια is a festival; the UD PROPN text "such as a person or a country" does not settle it. |
| none | R01M πάγος, πᾶγος | 2 | The name-component status depends on the adjacent R01C word, and no primary check decides it. |
| none | R03S ὡς | 2 | No pinned rule for this lemma with AuxY or AuxZ. |

## Unperformed checks

- LSJ separate-entry check for Ἀθηναῖος (P07).
- The H3 stem-and-class condition for ἀντιβολέω (P10).
- The syntactic-join test for R01M name components, beyond the mapping's rule.
- Not run, and not named in the brief: a witness check, any language-model or provider check, and a Perseus lemma-count cross-check.

## Per-occurrence before and after

Each row of `manifest/occurrence_patches.tsv` gives: the document, both versions, sentence and word ids, node key, document order, the raw and NFC form and lemma code points, postag, relation and head; the before fields (rule, UPOS candidates and status, eligibility, headword and status, discriminator, blockers); the proposal ID, proposed UPOS, basis, unsupported and unperformed points, and labels; the evidence checks; and the after fields. The after fields are identical to the before fields, because nothing is adopted. `eligibility_changes = 0`, and `status_after` is `vocab_blocked (unchanged)` for all 235.

## Open decisions for the parent

1. Adopt P01 and P03 (ADV, 108 occurrences) on the AGDT definition, or require further evidence?
2. Decide the R03C policy (70 occurrences): conjunction or adverb for AuxY and AuxZ conjunction-list lemmas. No pinned source settles it.
3. R03N negation (32): PART (UD general) or ADV|PART (current candidates)?
4. Accent-variant policy: fold καὶ and μὴ to καί and μή, or keep them separate? The current answer is no folding.
5. H3 ἀντιβολέω: does the stem-and-class condition hold? If so, the conditional status can remain.
6. Ἀθηναῖος (P07, ADJ): confirm the substantivised-adjective basis, after the LSJ check.
7. Ἑλλάς (P08, PROPN) and Θεσμοφόρια (none): PROPN or not?

## Corrections to earlier working notes

- The 24-file count was wrong. The hash list has 20 files.
- The "157" figure in earlier notes is an occurrence count (140 exact plus 17 variant), not a list count.
- A first draft mapped every R03C occurrence to ADV. That default is withdrawn: the pinned text does not support it.
- A first draft Smyth parser read the sidebar index and produced 11 tokens. The corrected parser reads the main-text PARTICLES list (to the next all-caps heading): 61 Greek tokens, 46 distinct.

## Reproduction

```sh
R=doc/research
python3 -I $R/ancient-greek-lysias1-lexical-completion/scripts/lexical_completion.py \
  $R/ancient-greek-lysias1-reconciliation/manifest <sources-dir> $R/ancient-greek-lysias1-lexical-completion/manifest
LEX_MANIFEST=$R/ancient-greek-lysias1-reconciliation/manifest LEX_SOURCES=<sources-dir> \
  python3 -I $R/ancient-greek-lysias1-lexical-completion/scripts/test_lexical_completion.py -v
```

Result of the last run: 16 tests, OK. The source-backed tests include byte-identical regeneration.

## Rights

Escaping code points does not change rights obligations. The retained qualifications from #2035 and #2048 still apply. See the folder's `NOTICE.md`.
