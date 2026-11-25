# Ancient Greek syntax and constructions

Status: **Accepted; implemented** · Date: 2026-11-24 ·
Author: Justin + OpenCode

This spec records the route charted by
[Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597)
and written up in
[Task: write the spec, ADRs and CONTEXT.md docs PR](https://github.com/justin-hayes/mouseion/issues/2611).
It owns learner behavior, the construction inventory and the acceptance matrix.
The decisions sit in five ADRs:

- [ADR 0122: Ancient Greek syntax is stored as native AGDT behind a construction interface](../adr/0122-ancient-greek-native-agdt-syntax.md)
- [ADR 0123: Constructions are a derived, versioned reading of a frozen parse](../adr/0123-constructions-are-a-derived-reading.md)
- [ADR 0124: Grammar references and Mouseion-authored concept notes](../adr/0124-grammar-references-and-concept-notes.md)
- [ADR 0125: Construction parallels in Concordance](../adr/0125-construction-parallels-in-concordance.md)
- [ADR 0126: Analysis updates carry decisions onto identical tokens](../adr/0126-analysis-updates-carry-decisions-onto-identical-tokens.md)

[ADR 0110](../adr/0110-philological-reading-competence-boundary.md) is the
direction this implements: classical syntax in Sentence study, with parallels in
Concordance, first within the author's works and then across the study
language.

## Problem

A learner reading Plato or Xenophon in Mouseion sees each word's lemma and
morphology, but not the syntax that makes a Greek sentence hard. A genitive
absolute, an articular infinitive or an infinitive of indirect statement is
where reading breaks down. Today Mouseion shows nothing about them for Greek:

- The pinned OGA files carry AGDT dependency syntax, but the import discards it
  and records `syntax_state = 'absent'` ([ADR 0099](../adr/0099-ancient-greek-imported-analyses.md)).
- Sentence study shows "Syntax unavailable" for every Greek sentence.
- Concordance has no way to find other examples of a construction.

## Learner outcome

In Sentence study, a learner sees which of six well-defined Greek constructions
the sentence contains, which words belong to each, which word signals it, a
short Mouseion-written definition with a Smyth 1920 citation and link, and a
way to see parallels. Where Mouseion is not confident, it says nothing. A
closed **Word relations** disclosure gives each word's relation and head in
plain English.

From a construction, **Parallels** opens Concordance with other occurrences of
the same construction: in this Book first, then in the author's other Books,
then in other authors.

## Scope

**In scope (first slice):**

- Ancient Greek Books from the Classical catalog, imported from the pinned OGA
  v0.2.0 files (22 Reading units over 7 Works: Plato's *Apology*, *Crito*,
  *Meno*, *Phaedo*, *Republic* and *Symposium*, and Xenophon's *Anabasis*).
- A new OGA interpretation version that stores AGDT relations and heads,
  unprinted syntax nodes, and the VerbForm/Aspect morphology remap. It is adopted
  through a learner-confirmed Analysis update.
- Six constructions, detected over the stored parse, stored as derived rows and
  shown in Sentence study.
- Construction parallels in Concordance, sequenced after Sentence study.
- Carry-over of Lemma corrections and Occurrence exclusions onto identical
  tokens in every Analysis update.

**Non-goals (out of scope for this effort):**

- Latin. The design leaves room for it (a UD adapter, a per-language grammar
  reference), and every Greek-specific choice below is marked as such. Nothing
  Latin is built.
- Modern-language constructions (*Konjunktiv I*, *passé simple*).
- Anki card fields carrying constructions or syntax. Prepared decks are
  unchanged and the deck notice is unchanged.
- Cross-language parallels.
- Learner corrections of syntax or constructions (plausibly next, not never).
- Construction-based deck selection or difficulty scoring.
- Greek syntax from Stanza `grc`, which Mouseion does not use (see
  [ADR 0122](../adr/0122-ancient-greek-native-agdt-syntax.md)).
- Showing raw AGDT or UD relation labels to learners.
- Any Smyth or Allen & Greenough wording in the app.

## Terms

The glossary terms are defined in [`CONTEXT.md`](../../CONTEXT.md):
**Construction**, **Construction occurrence**, **Construction anchor**,
**Construction extent**, **Construction subtype**, **Subtype label**,
**Construction detector version**, **Unprinted syntax node**, **Grammar
reference** and **Concept note**. A Construction is a syntactic pattern; it is
not an **Idiom**, which is a lexical multiword unit with a dictionary entry.

## Syntax source and storage

Decided in [Grilling: choose the Ancient Greek syntax source and representation](https://github.com/justin-hayes/mouseion/issues/2603#issuecomment-7098557008);
recorded in [ADR 0122](../adr/0122-ancient-greek-native-agdt-syntax.md).

- **Native AGDT, not UD.** No public converter fits OGA (no `_CO`/`_AP`
  suffixes, ellipsis markup dropped), conversion merges five distinctions the
  detectors need, and the guessed conjunct set is exact for only 82.6% of
  coordinations. Stanza `grc` would discard gold-trained trees and is
  non-commercial on every package.
- **Run state.** A run from the new interpretation version records
  `syntax_state = 'native_agdt'`. The state names the scheme of every stored
  relation and head in the run; there is no per-token scheme.
- **Stored verbatim.** Relations and heads are stored as the source has them.
  Only plain encoding defects are corrected at import (the Greek-capital-Zeta
  `AuxZ` on 4 *Anabasis* tokens becomes the Latin-script `AuxZ`). Comma roots,
  ambiguous COORDs and stray labels (`AuxV`, `_`) are normalised at read time by
  the AGDT adapter.
- **Unprinted syntax nodes.** AGDT ellipsis nodes (`[0]`, POS `-`, 2.6% of
  nodes, heading 7% of printed tokens) are stored as unprinted syntax nodes:
  relation, head and tree position, keyed by their stable source id within the
  sentence, with no printed form, lemma, vocabulary identity or Annotation
  occurrence. A printed token's head may be an unprinted syntax node.
- **Spans unchanged.** Sentence and token ordinals and offsets still come from
  `reconstruct` (`internal/ancientgreek/ogasrc.go`), which skips unprinted nodes.
  The new interpretation version must not change `reconstruct` or the
  sentence-inclusion rules, so every printed token keeps its source span. ADR
  0126's carry-over depends on this.
- **Morphology remap.** The new version also maps what `oga-agdt-upos/5`
  drops: VerbForm (Fin/Part/Inf), Aspect (Perf for perfect and future perfect,
  Imp for imperfect), the perfect tense and common gender, following UD Perseus
  r2.19. Whether non-finite forms carry Mood is settled on the implementation
  issue. Participle and infinitive stay distinguishable as today.
- **UD consumers stay off.** The gates that treat `native_agdt` as "no UD
  syntax" (`sentenceStudySyntaxUnavailable` in `internal/webapp/sentence_study.go`
  and the Concordance syntax-state read in `internal/persistence/concordance.go`)
  stay. AGDT relations reach learners only through the AGDT adapter. Today
  Greek tokens all carry `dep`; after this change they carry AGDT labels, so
  every other reader of `dependency` must be shown unaffected for `grc`:
  vocabulary eligibility and card bolding key on `compound:prt`, GDEX is
  German-only, and the lexical sense-ordering context
  (`internal/enrichment/lexical.go`) adds the raw relation as a context word,
  which the implementation must gate or show unused for Greek.
- **Source-neutral construction interface.** Detectors read a small interface
  (nodes, heads, relations normalised to construction-relevant categories,
  morphology, source lemma), and each scheme has one adapter. The AGDT adapter
  is first. A UD adapter for Latin can be added later without changing the
  detectors' interface.
- **Licence.** Relations, heads, unprinted nodes and constructions are
  Analysis-resource output under
  [ADR 0111](../adr/0111-third-party-licence-posture.md). The deployer register
  row and `layers.json` already name syntax for `oga-src-v0.2.0-annotations`;
  the new interpretation version changes only `interpretationVersion` and
  `revision` on the 22 manifest definitions. Test fixtures stay synthetic.
  See [Register and notices](#register-and-notices).

## Construction occurrence model

Decided in [Grilling: first-slice construction inventory and definitions](https://github.com/justin-hayes/mouseion/issues/2604#issuecomment-7098688802).

- A **Construction occurrence** is one construction found in one exact analysis.
- Its **anchor** is exactly one Annotation occurrence (a printed token), never
  an unprinted syntax node.
- Its **extent** is the set of printed tokens in the anchor's subtree, read
  through the AGDT adapter. The extent may be discontinuous (13.4% of arcs are
  non-projective), so it is a set, not a span. Unprinted nodes inside the
  subtree stay in the tree but are not members.
- Occurrences may nest and overlap. One anchor may carry several different
  constructions, never the same construction twice.
- A **Construction subtype** is an attribute of one construction. It does not
  count toward the cap of six. A subtype that cannot be determined is omitted
  and the construction still shows.

## First-slice inventory

Six constructions ship. All six are **Greek-specific** definitions over AGDT.
The detectors read the run's **source** lemma and morphology only; Lemma
corrections and Occurrence exclusions never affect them.

"Through COORD" means the detector looks up or down through coordination
groups. The construction ids are stable data keys; the learner-facing names
come from the concept notes.

| Id | Construction | Smyth 1920 | Anchor | Definition (AGDT, via the adapter) | Exclusions and accepted limits |
|---|---|---|---|---|---|
| `conditional` | Conditional clause (protasis only) | §§2280–2368 | the conjunction | AuxC with source lemma εἰ, ἐάν, εἴπερ, or ἄν (OGA's lemma for contracted ἐάν and ἐάνπερ), heading (through COORD) an ADV child that is a verb or an ADV unprinted node | Not when the previous printed word or the conjunction's head is ὡς or ὥσπερ ("as if"). Concessive εἰ καί / καὶ εἰ (§§2369–2382) counts. The apodosis is not in the extent. No "whether" exclusion (see below). |
| `articular_infinitive` | Articular infinitive | §§2025–2038 | the infinitive | a printed infinitive with a neuter singular article as a direct dependent, including when a preposition governs the phrase | An article governing a clause or coordinated infinitives attaches to one; an article far from the infinitive across a long subject may be misattached. |
| `result` | Result clause | §§2249–2278 | ὥστε | AuxC ὥστε heading (through COORD) an ADV child that is a verb or an ADV unprinted node | ὥστε + participle: no hit. Independent "and so" ὥστε (§2275) is not separated yet, which is why the actual-result subtype is unlabelled. |
| `purpose` | Purpose clause | §§2193–2206 | ἵνα | AuxC ἵνα (including ἵνα μή) heading (through COORD) a printed ADV subjunctive or optative | ὅπως and ὡς final clauses are deferred (ὅπως 6/14, ὡς 2/10 in measurement). Effort and fearing clauses are not purpose. |
| `genitive_absolute` | Genitive absolute | §§2070–2075 | the participle | a printed genitive participle labelled ADV with a printed genitive SBJ, directly or through COORD | Not when the participle's head (through COORD) is a verb of perceiving: ἀκούω, ἀκροάομαι, αἰσθάνομαι, πυνθάνομαι (supplementary participle, §§2110–2112). Subjectless genitive absolute (§2072) is not detected. An unprinted participle (ὄντος): no hit. |
| `indirect_statement` | Indirect statement | §§2016–2024 (infinitive); §§2577–2588 (ὅτι/ὡς) | the infinitive, or the conjunction | **infinitive**: a printed infinitive labelled OBJ or SBJ whose head (through COORD) is in the infinitive verb list, with every SBJ accusative or no SBJ (§1973); ὑπισχνέομαι and ὄμνυμι only with a future infinitive. **ὅτι/ὡς**: AuxC ὅτι or ὡς heading (through COORD) a printed OBJ or SBJ indicative, subjunctive or optative, whose head (through COORD) is in the ὅτι/ὡς verb list | The governing verb is not a member. δοκέω is excluded (personal "seem" can't be separated from "think"). Verbs of will (κελεύω, βούλομαι) and impersonal δεῖ, χρή fall outside by design. ὅτι introducing a direct quotation (§2590 a) counts as the clause. Cross-sentence reported speech and personal-passive nominative subjects (§1982) are accepted recall losses. Recall is 0.60. |

Plain "accusative + infinitive" (§§1972–1981) is **not** a construction: it is
too general for learners, and its raw rule scored 0.73.

### Subtypes

| Construction | Subtype id | Meaning | Smyth 1920 | Labelled in slice 1? |
|---|---|---|---|---|
| Result | `natural` | ὥστε + infinitive | §2258 | **yes**, "natural result" |
| Result | `actual` | ὥστε + any finite mood | §2257 | no (0.90, lower bound 0.80) |
| Indirect statement | `infinitive` | infinitive form | §2016 | **yes**, "infinitive" |
| Indirect statement | `hoti_hos` | ὅτι/ὡς + finite verb | §2577 | no (0.92, lower bound 0.82) |
| Articular infinitive | `genitive` | case of the article | §2032 | **yes**, "genitive" |
| Articular infinitive | `dative` | case of the article | §2033 | **yes**, "dative" |
| Articular infinitive | `nominative` | case of the article | §2031 | no (0.85) |
| Articular infinitive | `accusative` | case of the article | §2034 | no (0.82) |

Conditional, purpose and genitive absolute have no subtypes. A result clause
whose verb is an unprinted node has no subtype. An unlabelled subtype is not
hidden: its occurrences show the construction name with no subtype label.

### Authored lexical data

The verb lists, like the concept notes and the relation names, are
**Mouseion-authored compiled reviewed data**, keyed by construction id, reviewed
by Justin on the implementation issue, and part of the construction detector
version. The measured lists (by source lemma) are:

- **Infinitive indirect statement:** φημί, φάσκω, λέγω, εἶπον, ἐρῶ, ἀγγέλλω,
  ἀπαγγέλλω, ἐξαγγέλλω, ἀποκρίνω, ὁμολογέω, ἀρνέομαι, ἀπαρνέομαι, ὑπισχνέομαι*,
  ὄμνυμι*, νομίζω, οἴομαι, ἡγέομαι, ὑπολαμβάνω, ἐλπίζω, ὑποπτεύω, πιστεύω,
  διηγέομαι, ἐννοέω (* future infinitive only).
- **ὅτι/ὡς indirect statement:** the infinitive list plus δοκέω, and οἶδα,
  ἐπίσταμαι, μανθάνω, ὁράω, ἀκούω, μιμνῄσκω / μιμνήσκω, ἀναμιμνῄσκω /
  ἀναμιμνήσκω and πείθω. (δοκέω is excluded only from the infinitive list.)
- **Perceiving (genitive-absolute exclusion):** ἀκούω, ἀκροάομαι, αἰσθάνομαι,
  πυνθάνομαι.

### Deferred tail (ranked)

Reviewed at the next construction decision, in this order. Ranges were checked
against Smyth 1920.

1. Conditional type (simple, general, future more/less vivid, contrary to fact),
   which needs the apodosis mood and ἄν.
2. Indirect statement with a participle (§§2106–2115): ADV/OBJ confusion, and it
   collides with the genitive absolute.
3. Effort clause, ὅπως + future indicative (§§2209–2219).
4. Clause of fearing, μή / μὴ οὐ (§§2221–2233).
5. Temporal clauses: ἐπεί, ὅτε, πρίν, ἕως (§§2383–2461).
6. Attributive or substantive participle with the article (§§2049–2053).
7. Potential optative and ἄν constructions (§§1824–1826).
8. Accusative absolute (§§2076–2078).
9. Conditional relative clauses (§§2560–2573).

Added by [Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608#issuecomment-7106323004)
(unranked within the tail):

- a clause-position or relation test for substantive "whether" εἰ (§2671);
- ὅτι/ὡς + OBJ clause under any verb, which needs its own precision run;
- indirect statement carried across sentences;
- personal δοκέω;
- ὅπως and ὡς final clauses;
- a rule separating independent "and so" ὥστε (§2275), which could label actual
  result;
- the nominative and accusative articular-infinitive labels.

### Smyth 1920 verification

Every § above was checked on 2026-11-24 against the Perseus Hopper 1920 text
(`Perseus:text:1999.04.0007`), by reading the opening of each range's first and
last § and the § after it. Ranges cited from memory (marked † in the inventory
grilling) were corrected as follows:

| Cited as | Finding | Now cited |
|---|---|---|
| Articular infinitive §§2025–2034 | §§2035–2038 (apposition, exclamation, adjuncts, verbs of hindering) still treat the articular infinitive; §2039 starts the participle | §§2025–2038 |
| Articular infinitive "after a preposition, §2032" | §2032 is the genitive articular infinitive; prepositions are treated within each case (e.g. §2034 b) | case subtypes §§2031–2034 |
| Natural result §2251 | §2251 introduces both forms; the infinitive of result is §2258 (and §2260) | §2258 |
| Perception-verb exclusion §§2110–2115 | §§2110–2112 are verbs of perception (§2111 names ὁρῶ, αἰσθάνομαι, ἀκούω, πυνθάνομαι); §2113 is verbs of finding | §§2110–2112 |
| ὅτι recitativum §2590 | confirmed, in §2590 a (direct quotation introduced by ὅτι) | §2590 a |

Confirmed unchanged: §§2280–2368, §§2369–2382, §2671, §§2249–2278, §2257,
§2275, §§2193–2206, §§2070–2075, §2072, §§2016–2024, §§2577–2588, §1973,
§§1972–1981, §1982, §§2106–2115, §§2209–2219, §§2221–2233, §§2383–2461,
§§2049–2053, §§1824–1826, §§2076–2078 and §§2560–2573.

## Abstention

Decided in [Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608#issuecomment-7106323004).

- **Precision-first and silent.** No hedged labels, no "no construction found"
  statement, and no learner-visible "abstained" state.
- **Per construction only.** A construction or subtype is shipped or not. There
  is no per-occurrence confidence layer.
- **Detectors fail closed.** Each definition above lists the inputs it needs.
  When a required input is missing, or is reached only through an unprinted
  node where the definition asks for a printed token, the detector produces no
  hit. The conditional and result definitions explicitly admit an ADV unprinted
  node as the clause, because the node's own relation is their input.
- **Shipping bar.** A construction ships when its precision on at least 120
  random corpus hits is ≥ 0.90 (point estimate) with a 95% Wilson lower bound
  ≥ 0.84. A subtype label is gated by the same bar on at least 60 hits. Recall
  is reported, not gated.
- **Conditional gate.** Conditional shipped only once the first release-check
  run confirmed it clears the bar without the dropped "whether" exclusion
  (measured 0.93 with it; about 0.91 estimated without). The
  [release check](https://github.com/justin-hayes/mouseion/issues/2622#issuecomment-7135682811)
  measured 0.912 (lower bound 0.866), and conditional ships.

## Persistence and versioning

Decided in [Grilling: persistence and versioning of syntax and constructions](https://github.com/justin-hayes/mouseion/issues/2606#issuecomment-7098921699);
recorded in [ADR 0123](../adr/0123-constructions-are-a-derived-reading.md).

- **The parse is frozen evidence; constructions are a derived reading.** A
  run's tokens, lemmas, morphology, relations, heads and unprinted nodes are
  never rewritten. Construction occurrences are a reading of that parse, like
  Browse counts and concept notes. No card, Known fact, count or completion
  depends on them.
- **Construction detector version.** One integer per syntax scheme (AGDT now;
  UD for Latin later) covers the adapter's normalisation, the six detectors,
  the authored verb lists and the shipping gate. It is bumped by hand and
  enforced by a golden-output test.
- **Rebuild without confirmation.** A detector-version change rebuilds every
  `native_agdt` run's occurrences, including runs that active Readings pin. Only
  the current version's rows are kept. A frozen Reading's parse never changes;
  what Mouseion identifies in it may improve.
- **Show nothing while rebuilding.** Until a run's readiness marker reaches the
  current version, Sentence study and Concordance show no constructions for
  it, with no stale fallback and no "updating" notice. Each run is swapped in
  one transaction. Rebuild jobs run on `classical_preparation`.
- **Built at publish.** A new `native_agdt` run's occurrences are built inside
  the preparation transaction, before it publishes.
- **Old runs.** Runs with `syntax_state = 'absent'` get no rows and show no
  constructions, which is silent abstention. They are not rebuilt.
- **Word relations are query-time.** The per-word relation and head are read
  through the AGDT adapter at render; they are not stored or versioned.

## Data contract

An ADR 0038 brief, not SQL. The exact DDL is settled in the implementation
issue's migration, under CODEOWNER review.

**Analysis run (existing).** The new interpretation version is the run's
`analyzer_version` (already the classical run identity in
`internal/ancientgreek/prepare.go`) and `syntax_state = 'native_agdt'`. No
change to `analysis_runs`.

**Printed tokens (existing `corpus_tokens`).** `dependency` holds the stored
AGDT relation; it is read according to the run's syntax state. The proto
(`proto/mouseion/v1/normalized_corpus.proto`) and domain comments that call it
UD are corrected. A token whose head is printed records that token's ordinal.
A token whose head is an unprinted node references that node; how (for
example, a nullable reference beside `head`) is settled in the migration.
Ordinals, offsets, surfaces and lemmas follow `reconstruct` unchanged.

**Unprinted syntax nodes (new).** One row per elided node: owner, run,
sentence, the node's stable source id within the sentence, its relation, and
its head (a printed token or another unprinted node, or the root). No form,
lemma, morphology or vocabulary identity.

**Construction occurrences (new, derived).** One row per occurrence:

| Field | Meaning |
|---|---|
| owner, run, sentence | the exact analysis and sentence |
| language | denormalised from the run, for the Concordance index |
| construction id | a stable data key, e.g. `genitive_absolute` |
| subtype id | optional; present only when the shipping gate labels it |
| anchor | the anchor token's ordinal |
| extent | the set of member token ordinals |

No text, name, definition, citation or detector version is stored per row.

**Per-run marker (new).** Owner, run, the construction detector version its
rows were built under, and readiness. Construction rows are read only when the
marker is current.

**Index.** (owner, language, construction id, subtype) on construction
occurrences, for Concordance. Work and author are reached through Book →
Reading unit → Edition → Work.

**Compiled data (not in the database).** Concept notes, verb lists, relation
names, the shipping gate and the grammar-reference records are Go literals
keyed by stable ids. The per-interpretation-version change note is held next to
the Reading-unit definition data.

## Surfaces

### Sentence study

Decided by [Prototype: constructions and syntax in Sentence study](https://github.com/justin-hayes/mouseion/issues/2609#issuecomment-7127948787),
variant C (code on the throwaway branch
[`research/grc-sentence-study-constructions-prototype`](https://github.com/justin-hayes/mouseion/tree/1c3bd586/doc/research/grc-sentence-study-constructions-prototype)).
The pattern is in [`doc/design/components.md`](../design/components.md#accepted-construction-extract-target).

**Order.** Sentence (unmarked) → **Constructions** (`h3`, ordered list) →
interlinear morphology line → Notation key → **Word relations** (closed
`<details>`) → Complete text alternative.

**Each construction entry:**

1. The name from the concept note, plus the subtype label in parentheses when
   it is a shipped Subtype label.
2. An **extract**: the whole sentence reprinted, with the extent at full weight,
   the rest muted at regular weight, and the anchor underlined. A gap made only
   of punctuation counts as part of the extent. A discontinuous extent needs no
   extra notation. An inner construction says "Inside construction N".
3. "Signalled by *anchor*." followed by one of: "The identified target signals
   this construction." (target = anchor), "Contains the identified target."
   (target in extent), or nothing.
4. The concept note's definition, then "Smyth 1920 §§N–M", linked to Perseus
   (opens externally, `rel="noopener noreferrer"`).
5. **Parallels**, carrying the subtype when it is labelled. Its accessible name
   includes the construction and subtype ("Parallels for Result clause, natural
   result").

**Ordering.** Textual order of each extent's first token; an outer
construction comes before an inner one that starts at the same token.

**Silence.** With no shipped construction in the sentence, or a run that is
`absent` or rebuilding, the whole Constructions region is absent: no heading and
no message.

**Focused construction.** Arriving from Concordance's **Study**, the order is
unchanged; the focused entry gets a thicker accent edge and the visible text
"· from Concordance", and is scrolled into view.

**Word relations.** A table with columns #, Word, Relation, Depends on (the
head's word plus "(word n)"). The target word is bold, with a visually hidden
"(identified target)". Punctuation rows are omitted. The root shows "— (top of
the sentence)". Plain-English relation names (draft wording, reviewed with the
concept notes):

| AGDT | Shown as |
|---|---|
| PRED | main verb |
| SBJ | subject |
| OBJ | object |
| ATR | describes a noun |
| ADV | adverbial (how, when, why…) |
| PNOM | predicate noun or adjective |
| OCOMP | object complement |
| ATV, AtvV | predicative complement |
| APOS | in apposition |
| COORD | joins coordinated words |
| AuxC | subordinating conjunction |
| AuxP | preposition |
| AuxY | sentence particle |
| AuxZ | emphasising or negating particle |
| AuxV | auxiliary |
| ExD | stands outside the sentence structure |

**Unprinted heads.** A word whose head is unprinted shows "‹an unprinted
word›, which is the object of ὅτι (word 6)": the display walks up through
unprinted nodes to the first printed word and names the unprinted node's own
relation. A note under the table explains it once, only when one occurs.
Unprinted nodes never get rows and never show a lemma or a guessed identity.

**Accessibility.** The extract is `aria-hidden`. Each entry carries a visually
hidden "Words: …" line listing the extent in textual order, with "…" between
discontinuous parts. The Complete text alternative names, for each word, the
constructions that contain it and which one it signals. Marking is by weight,
muted tone and underline, never colour alone.

**Other runs.** Sentence study for an `absent` Greek run keeps "Syntax
unavailable" for Word relations and shows no Constructions. UD runs (modern
languages) are unchanged.

### Concordance parallels

Decided in [Grilling: Concordance construction parallels — query, scope and ranking](https://github.com/justin-hayes/mouseion/issues/2610#issuecomment-7106742314);
recorded in [ADR 0125](../adr/0125-construction-parallels-in-concordance.md).
Sequenced after Sentence study.

- **A third match kind.** `match=construction`, with the construction id as the
  term, an optional subtype and an optional anchor-lemma filter. It reuses the
  lookup URL, captured-priority slot, revision guard, 25-row global paging and
  KWIC rendering. The search box never recognises a construction name. The
  match kind is offered only for a language whose analyses have a construction
  scheme (`grc`); any other language gets the existing safe-restart notice.
- **Entry point.** Only Sentence study's **Parallels**. There is no construction
  picker on the Concordance page.
- **Three labelled tiers in one sequence:** **This Book** (the origin Book,
  captured like today's priority Book); **This author** (the same Work's other
  Reading units first, then the author's other Works by title); **Other
  authors** (by author, Work title, Reading unit order). The author is the
  reviewed catalog's Work author string, matched exactly. A Book with no Work
  has no author tier and goes from "This Book" to "Other Books". This tiering is
  Classical- and Greek-specific.
- **Textual order inside a tier.** Work, Reading unit, sentence, anchor token.
  No quality or "best example" ranking.
- **The origin occurrence is omitted.** The heading links back to the origin
  sentence.
- **Filters.** The lemma filter applies to the anchor's effective lemma only.
  The subtype filter is offered only for shipped Subtype labels. Parallels
  opens construction-only, and the results offer "only with {anchor lemma}". A
  labelled subtype opens filtered to it and offers "all {construction}".
- **Corrections and exclusions.** A construction-only query ignores both. With a
  lemma filter, the anchor uses its corrected effective lemma, and an excluded
  anchor is hidden.
- **Books that can't contribute are named** in one quiet line, for example "Not
  searched for constructions: *Apology* — Analysis update available". This
  covers `absent` runs and any Stanza-path `grc` Book. A rebuilding run stays
  silent.
- **Revision guard.** The evidence revision also covers the construction
  detector version and each eligible run's readiness marker, so a rebuild
  mid-paging returns *Current evidence changed* with Refresh.
- **Rows.** Each KWIC row is centred on the anchor. The expanded row reuses the
  Sentence study extract as-is, one construction per row. **Study** opens
  Sentence study with that construction focused.
- **Eligibility** matches the existing three statements of Concordance
  eligibility (the view, the listing, `ConcordanceLemmaEvidenced`).

### Analysis update

Decided in [Grilling: Analysis update copy and carry-over for the syntax interpretation version](https://github.com/justin-hayes/mouseion/issues/2614#issuecomment-7099181118);
recorded in [ADR 0126](../adr/0126-analysis-updates-carry-decisions-onto-identical-tokens.md).

The new interpretation version reaches existing Greek Books through ADR 0116's
Classical Analysis update. No new mechanism.

- **Notice unchanged:** "A newer analysis is available."
- **Change note.** Each Classical interpretation version may carry an optional
  Mouseion-authored change note, held as data next to the definition and looked
  up at render. This version's note says, in substance, "Adds sentence
  constructions in Sentence study." A version with no note shows no line.
- **Per-Book confirmation:** change note (if any); "The published analysis stays
  in effect until the update publishes."; "Your N confirmed lemma corrections
  and M occurrence exclusions carry over wherever the word is unchanged in the
  new analysis."; "Prepared decks and earlier Readings keep the evidence they
  froze." Singular and zero forms follow the existing ones.
- **Work level:** "Together, the Books to be updated have X confirmed lemma
  corrections and Y occurrence exclusions, which carry over wherever the word is
  unchanged in their new analyses."
- **Carry-over** at publish, by exact token identity (same source document,
  offset span, surface and source lemma). See ADR 0126.
- **`/jobs`** reports "N of M decisions carried over" on the update's result
  line. No notice on the Book.

## Grammar reference and concept notes

Decided in [Grilling: grammar link target and Mouseion-authored concept notes](https://github.com/justin-hayes/mouseion/issues/2605#issuecomment-7098653869);
recorded in [ADR 0124](../adr/0124-grammar-references-and-concept-notes.md).

- **Greek grammar reference:** Smyth, *A Greek Grammar for Colleges* (1920), on
  Perseus Hopper:
  `https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={§}`.
  Documented swaps, if Hopper changes: the on-host
  `text?doc=Smyth+grammar+{§}&fromdoc=Perseus:text:1999.04.0007` form, or
  Alpheios `#s{§}` anchors (which need a § → file map). There is no runtime
  fallback; one data row is edited.
- **Citation** "Smyth 1920 §N" or "Smyth 1920 §§N–M" is always shown as text; a
  range links to its first §. A construction without a citation does not ship.
  Every § comes from the 1920 numbering, never Messing 1956.
- **Concept note:** a learner-facing name, one or two plain-English sentences
  of Mouseion's own wording describing the concept (not the detection rule),
  and one citation. No example sentence: the learner's sentence is the example.
  Drafted by the agent on the implementation issue, approved there by Justin,
  and recording its review ticket. A unit test asserts every construction id
  the detector can emit has a reviewed note with a citation.
- **Notes are not evidence.** They are looked up at render, so editing one needs
  no Analysis update and no rebuild.
- **No link checking** at runtime or in CI (Perseus answers bots with 403 or a
  cookie redirect, and the app is tailnet-only).
- **Latin (shape only, nothing compiled):** Allen & Greenough at DCC,
  `https://dcc.dickinson.edu/grammar/latin/category-search?field_gl_section_number_value={§}`,
  cited "A&G §N", Perseus as the documented swap. Concept notes are per
  language: the ablative absolute is its own note.

## Register and notices

- **No new register row and no new `layers.json` layer.** The
  `oga-src-v0.2.0-annotations` row and layer already name syntax. The new
  interpretation version bumps `interpretationVersion` and `revision` on the 22
  manifest definitions.
- When syntax ships, the implementation issue adds one sentence to that
  register row's use column: relations, heads and constructions derived from
  them are shown in Sentence study and Concordance.
- The grammar link needs no row. A row would become necessary if Mouseion showed
  any Smyth or A&G wording (headings included), or shipped a § → URL map derived
  from a host's files.
- The deck notice and the deck's "Output only (`Lemma`, `POS`)" column are
  unchanged: cards carry no constructions.
- The research branches commit only IDs, verdicts and judgments. The release
  check's reviewed judgment file follows the same rule.

## Release check

Part of the construction detector version
([ADR 0123](../adr/0123-constructions-are-a-derived-reading.md)).

- **Reviewed judgment file** (committed, IDs only): work, sentence id, OGA
  anchor id, construction, subtype and judgment for every reviewed item from
  the two measurements. Justin's judgment is final where he gave one; otherwise
  the reference or agent judgment stands. No Greek text, relations or heads.
- **A `make` target**, `make construction-release-check` (the command
  `cmd/constructioncheck`; `CONSTRUCTION_CHECK_CACHE` overrides the cache
  directory, default `~/.cache/mouseion/oga-src`), recomputes per-construction and per-subtype precision
  against the pinned OGA files, fetched and never committed. Every
  detector-version bump runs it, and the bump's PR includes its output. It is
  not a default CI step. A bump fails when a shipped construction or subtype
  falls below the bar, or a reviewed item flips (true → missing, false → hit)
  without a new judgment.
- **Fresh sample.** A bump that changes more than 10% of a construction's corpus
  hits needs a new random sample of 120 hits for that construction.
- **Synthetic golden test (CI)** runs on every change and fails when emitted
  occurrences change without a version bump. Fixtures stay synthetic, and use
  the real AGDT label inventory (the current `DET`/`PUNCT` fixtures do not
  match any pinned file).

## Acceptance matrix

| # | Behavior | Verified by |
|---|---|---|
| A1 | A new Greek run records `native_agdt`, the new interpretation version, verbatim relations (Zeta `AuxZ` repaired) and unprinted nodes with heads | unit + integration (synthetic fixture) |
| A2 | Every printed token's span, surface and source lemma equals the previous interpretation version's for the same file | integration over synthetic fixtures; `make` check over the pinned files |
| A3 | VerbForm, Aspect, the perfect and common gender are mapped | unit |
| A4 | UD consumers (Sentence study relation display, Concordance syntax reads) treat `native_agdt` as no UD syntax; every other `dependency` reader (selection eligibility, card bolding, GDEX, lexical sense context) produces the same Greek output as before | unit + characterization |
| A5 | Each detector emits the inventory's hits and none of its exclusions; no hit on missing or unprinted required inputs | unit (synthetic trees per definition and exclusion) |
| A6 | Only shipped constructions and Subtype labels are emitted | golden test |
| A7 | Emitted occurrences change only with a detector-version bump | golden test (CI) |
| A8 | Every emittable construction id has a reviewed concept note with a citation | unit |
| A9 | Construction rows are built in the preparation transaction before publish; `absent` runs get none | integration |
| A10 | A version bump rebuilds every `native_agdt` run, including pinned runs, swapping each run in one transaction; nothing shows while the marker is behind | integration |
| A11 | Lemma corrections and exclusions never change construction rows | integration |
| A12 | Sentence study order, extract marking, anchor/target text, citation link attributes, Parallels names, focus state and silent absence | templ unit + browser smoke (fixture server) |
| A13 | Word relations: plain names, root, unprinted-head wording and note, no punctuation rows | templ unit |
| A14 | Accessibility: `aria-hidden` extract, "Words: …" line, text alternative, not colour-only; 375px/1200px, both themes, no-JS | browser smoke + manual check |
| A15 | `match=construction` tiers, textual order, origin omitted, filters, corrections and exclusions as specified, 25-row paging | integration |
| A16 | Books with no construction evidence are named; rebuilding runs are silent | integration + templ unit |
| A17 | Revision guard changes on detector version or readiness change | integration |
| A18 | Non-`grc` construction URLs get the safe-restart notice | handler unit |
| A19 | Analysis update confirmation and Work-level copy, change note present and absent | templ unit |
| A20 | Carry-over: matched decisions copied at publish, absorbed corrections counted, unmatched ones not copied, old run unchanged, `/jobs` "N of M" line; applies to NLP updates too, not to Source structure updates | integration |
| A21 | Release check: every shipped unit meets the bar on the reviewed file; conditional confirmed without the "whether" exclusion before it ships | `make` target output in the PR |
| A22 | Prepared decks, deck notices and frozen Readings' parses are unchanged | existing characterization tests |

## Evidence

Every figure was measured on the pinned OGA v0.2.0 files. The research branches
are throwaway and never merged; they commit IDs and aggregates only.

| Ticket | Finding | Evidence |
|---|---|---|
| [Research: AGDT syntax profile](https://github.com/justin-hayes/mouseion/issues/2598#issuecomment-7098250031) | Clean trees across 7 Works (251k tokens); no `_CO`/`_AP`; ellipsis 2.6% of nodes, heading 7% of printed tokens; dropping it leaves 18.6% of sentences multi-root | [`research/grc-oga-agdt-syntax-profile`](https://github.com/justin-hayes/mouseion/blob/7dfd50d55781399585334e9dbd8c8e8c6c0ff17d/doc/research/grc-oga-agdt-syntax-profile.md) |
| [Research: AGDT-to-UD conversion cost](https://github.com/justin-hayes/mouseion/issues/2599#issuecomment-7098298122) | No usable converter; 5 distinctions merged; 82.6% exact conjunct sets; Latin arrives as both UD and AGDT | [`research/grc-agdt-ud-conversion`](https://github.com/justin-hayes/mouseion/blob/0e458333932b4a059120f29127ee6cd0ec8aca5b/doc/research/grc-agdt-ud-conversion.md) |
| [Research: OGA parser accuracy and prior art](https://github.com/justin-hayes/mouseion/issues/2600#issuecomment-7098290790) | LAS 0.771 / UAS 0.826 published; coordination weakest; detectors need lemma, mood and relation | [`research/grc-parser-accuracy-construction-prior-art`](https://github.com/justin-hayes/mouseion/blob/6d0284e903fdb37cc8e9f2f582dfa46ad18026c6/doc/research/grc-parser-accuracy-construction-prior-art.md) |
| [Research: grammar-reference link targets](https://github.com/justin-hayes/mouseion/issues/2601#issuecomment-7098264270) | Perseus Hopper is the 1920 edition and resolves §§1–3048; no register row for links | [`research/grc-grammar-reference-targets`](https://github.com/justin-hayes/mouseion/blob/cf2e07459642570b4b97aedc45406d839d4e3c7b/doc/research/grc-grammar-reference-targets.md) |
| [Research: ADR 0111 and OGA-derived syntax](https://github.com/justin-hayes/mouseion/issues/2602#issuecomment-7098154703) | Analysis-resource output; permitted with ADR 0111's conditions; no new row | [`research/grc-syntax-licence-posture`](https://github.com/justin-hayes/mouseion/blob/bf1b301fe8a84d2010d5e109dd61ac8e0f091607/doc/research/grc-syntax-licence-posture.md) |
| [Task: measure construction detection](https://github.com/justin-hayes/mouseion/issues/2607#issuecomment-7100600509) | 148 items reviewed live by Justin (144/148 agreement); purpose and δοκέω amendments | [`research/grc-construction-detection-measurement`](https://github.com/justin-hayes/mouseion/blob/15ae542a/doc/research/grc-construction-detection-measurement.md) |
| [Task: re-measure the amended definitions](https://github.com/justin-hayes/mouseion/issues/2615#issuecomment-7102169398) | 120 random hits per construction; blind spot-check 35/36; "whether" exclusion backfires | [same branch, `db6ba186`](https://github.com/justin-hayes/mouseion/blob/db6ba186/doc/research/grc-construction-detection-remeasure.md) |
| [Prototype: constructions in Sentence study](https://github.com/justin-hayes/mouseion/issues/2609#issuecomment-7127948787) | Variant C extract; page order; relation names | [`research/grc-sentence-study-constructions-prototype`](https://github.com/justin-hayes/mouseion/tree/1c3bd586/doc/research/grc-sentence-study-constructions-prototype) |

**Measured precision** (re-measure; 95% Wilson interval; 120 random hits each):

| Construction | Precision | Recall |
|---|---|---|
| Purpose (ἵνα only) | 1.00 (0.97–1.00) | 0.95 |
| Result | 0.97 (0.92–0.99) | 0.91 |
| Indirect statement | 0.94 (0.88–0.97) | 0.60 |
| Conditional | 0.93 (0.86–0.96), with the since-dropped exclusion | 0.92 |
| Articular infinitive | 0.92 (0.85–0.95) | 0.98 |
| Genitive absolute | 0.91 (0.84–0.95); every false hit a parser error | 0.74 |

**Release check** ([#2622](https://github.com/justin-hayes/mouseion/issues/2622#issuecomment-7135682811)):
conditional without the "whether" exclusion measured 0.912 (95% Wilson lower
bound 0.866) over the seven pinned OGA files. It clears the bar and ships.

**Limits of the evidence.** The parser's only gold comparisons are on its
training texts (*Apology*, *Crito*, *Anabasis*), so agreement figures overstate
accuracy there; *Phaedo*, *Symposium*, *Meno* and *Republic* are measured only
through the reviewed samples. Recall figures are optimistic. AGDT
inter-annotator agreement is itself 0.806.
