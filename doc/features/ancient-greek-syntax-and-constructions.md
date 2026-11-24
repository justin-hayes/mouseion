# Ancient Greek syntax and constructions

Status: **Accepted specification — implementation pending** · Date: 2026-11-24 ·
Author: Justin + OpenCode

This spec records the route charted by
[Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597)
and written up in
[Task: write the spec, ADRs and CONTEXT.md docs PR](https://github.com/justin-hayes/mouseion/issues/2611).
It owns learner behavior, the construction inventory and the test plan. The
decisions sit in five ADRs:

- [ADR 0122: Ancient Greek syntax is stored as native AGDT behind a construction interface](../adr/0122-ancient-greek-native-agdt-syntax.md)
- [ADR 0123: Constructions are a derived, versioned reading of a frozen parse](../adr/0123-constructions-are-a-derived-reading.md)
- [ADR 0124: Grammar references and Mouseion-authored concept notes](../adr/0124-grammar-references-and-concept-notes.md)
- [ADR 0125: Construction parallels in Concordance](../adr/0125-construction-parallels-in-concordance.md)
- [ADR 0126: Analysis updates carry decisions onto identical tokens](../adr/0126-analysis-updates-carry-decisions-onto-identical-tokens.md)

It implements the direction of
[ADR 0110](../adr/0110-philological-reading-competence-boundary.md): classical
syntax in Sentence study, with parallels in Concordance, first within the
author's works and then across the study language. Terms follow
[`CONTEXT.md`](../../CONTEXT.md). The implementation parent issue and its
children are created from this spec by
[Task: create implementation issues and run the cold-read test](https://github.com/justin-hayes/mouseion/issues/2612).

## Problem Statement

A learner reading Plato or Xenophon in Mouseion sees each word's lemma and
morphology, but not the syntax that makes a Greek sentence hard. Reading breaks
down at a genitive absolute, an articular infinitive or an infinitive of
indirect statement, and Mouseion says nothing about any of them. Sentence study
shows "Syntax unavailable" for every Greek sentence, so the learner cannot see
which words belong together, which word signals the pattern, or what the
pattern is called. Nor can they find other examples of a pattern they have just
met, in the same Book, the same author or other authors.

The source files Mouseion already imports carry this syntax, but Mouseion
discards it.

## Solution

When a learner studies a sentence from a Classical Greek Book, Sentence study
names the constructions the sentence contains, drawn from six well-defined
ones. For each construction it shows:

- which words belong to it and which word signals it;
- whether it contains or is signalled by the word being studied;
- a short definition in Mouseion's own words, with a "Smyth 1920 §N" citation
  linked to the grammar;
- a **Parallels** link.

Where Mouseion is not confident, it says nothing at all. A closed **Word
relations** disclosure gives every word's relation and the word it depends on,
in plain English, including when that word is unprinted.

**Parallels** opens Concordance with other occurrences of the same construction.
They come in three labelled tiers: this Book, then the author's other Books,
then other authors. The learner can narrow them to the signalling word's lemma
or to a labelled subtype, and go back to Sentence study with the construction
focused.

Existing Greek Books gain all this through the learner-confirmed Analysis
update they already know. The confirmation says what the update adds, and the
learner's lemma corrections and occurrence exclusions carry over wherever the
word is unchanged. Later improvements to construction detection reach every
Book without another confirmation, while each Reading's frozen evidence stays
untouched.

## User Stories

**Seeing constructions in Sentence study**

1. As a Greek learner, I want Sentence study to name the constructions in the sentence I am studying, so that I can recognise the syntax that makes it hard.
2. As a Greek learner, I want each construction shown as the whole sentence with its words at full weight and the rest muted, so that I can see exactly which words belong to it.
3. As a Greek learner, I want the word that signals a construction underlined and named ("Signalled by ὥστε"), so that I learn which word to look for next time.
4. As a Greek learner, I want to be told when the word I came to study signals a construction or sits inside one, so that I connect the vocabulary item to its syntax.
5. As a Greek learner, I want a construction whose words are not next to each other shown without extra notation, so that a discontinuous Greek clause still reads naturally.
6. As a Greek learner, I want a construction inside another to say "Inside construction N", so that I understand nesting without brackets cluttering the text.
7. As a Greek learner, I want constructions listed in the order they start in the sentence, with an outer one before an inner one that starts at the same word, so that the list follows my reading.
8. As a Greek learner, I want a subtype such as "natural result" shown only when Mouseion is reliable about it, so that I am never taught a wrong label.
9. As a Greek learner, I want a construction whose subtype is unreliable still shown, without the subtype, so that I don't lose a correct construction because a detail is uncertain.
10. As a Greek learner, I want each construction to carry a one- or two-sentence definition in plain English, so that I can understand it without leaving the page.
11. As a Greek learner, I want each definition to cite "Smyth 1920 §N" as text, so that I can find the section in any copy of Smyth even if a link breaks.
12. As a Greek learner, I want the citation to link to the 1920 Smyth on Perseus, so that I can read the full treatment in one click.
13. As a Greek learner, I want the grammar link to open externally without passing my private address, so that my tailnet URLs never leave my network.
14. As a Greek learner, I want no Constructions region at all when Mouseion has nothing confident to say, so that silence never pretends to be an answer.
15. As a Greek learner, I want a construction that depends on an unprinted word to be left out rather than guessed, so that what I see is reliable.

**Word relations**

16. As a Greek learner, I want a closed "Word relations" disclosure listing each word, its relation in plain English and the word it depends on, so that I can check how the sentence hangs together when I want to.
17. As a Greek learner, I want relation names such as "subject", "object" and "adverbial (how, when, why…)" instead of AGDT codes, so that I don't need to learn a treebank scheme.
18. As a Greek learner, I want the top of the sentence marked "— (top of the sentence)", so that I can see where the structure starts.
19. As a Greek learner, I want a word that depends on an unprinted word to say "‹an unprinted word›, which is the object of ὅτι (word 6)", so that I understand the ellipsis without Mouseion inventing a word.
20. As a Greek learner, I want a single note explaining unprinted words, shown only when one occurs, so that the table stays uncluttered.
21. As a Greek learner, I want punctuation left out of the relations table, so that it lists only words.
22. As a Greek learner studying a Book still on the old analysis, I want "Syntax unavailable" as before and no constructions, so that the page never mixes analyses.

**Accessibility**

23. As a screen-reader user, I want each construction to list its words in order, with "…" between separated parts, so that I get the same information as the visual extract.
24. As a screen-reader user, I want the complete text alternative to say which constructions contain each word and which one it signals, so that I can study word by word.
25. As a learner with low colour perception, I want constructions marked by weight, muting and underline, so that colour is never the only cue.
26. As a learner on a phone, I want long Greek sentences and extracts to fit without horizontal page scrolling, so that I can study on any screen.
27. As a learner without JavaScript, I want every construction, link and disclosure to work, so that the page stays server-rendered.

**Concordance parallels**

28. As a Greek learner, I want a **Parallels** link on each construction, so that I can see more examples of the pattern I just met.
29. As a Greek learner, I want parallels in this Book first, so that I see the pattern in the text I am reading.
30. As a Greek learner, I want the author's other Books next, with the same Work's other parts first, so that I see the author's own habits.
31. As a Greek learner, I want other authors' examples last, labelled as such, so that I can compare styles.
32. As a Greek learner, I want examples inside each tier in textual order, so that the list is predictable and not a ranking I can't judge.
33. As a Greek learner, I want the example I came from left out of the list, with a link back to it, so that I don't see it twice.
34. As a Greek learner, I want to narrow parallels to those signalled by the same lemma ("only with λέγω"), so that I can study one word's habits.
35. As a Greek learner, I want parallels for a labelled subtype to open filtered to that subtype, with a way to widen to the whole construction, so that I can choose how broad to look.
36. As a Greek learner, I want each parallel centred on its signalling word, with the expanded row showing the same extract as Sentence study, so that the marking means the same everywhere.
37. As a Greek learner, I want **Study** on a parallel to open Sentence study with that construction focused and marked "· from Concordance", so that I can study it in full.
38. As a Greek learner, I want Books that can't be searched for constructions named in one quiet line ("Not searched for constructions: *Apology* — Analysis update available"), so that I know an absence is not zero.
39. As a Greek learner, I want "Current evidence changed" with Refresh if the results change while I page, so that I never get a page that mixes old and new results.
40. As a learner of another language, I want construction URLs to give the usual safe restart, so that nothing breaks outside Greek.
41. As a Greek learner, I want my occurrence exclusions to hide a parallel only when I narrow by lemma, so that a vocabulary decision about one word doesn't remove a syntactic example.

**Analysis update and carry-over**

42. As a Greek learner with existing Books, I want the familiar "A newer analysis is available" notice, so that I adopt syntax the same way I adopt other updates.
43. As a Greek learner, I want the confirmation to say what the update adds ("Adds sentence constructions in Sentence study"), so that I know why to accept it.
44. As a Greek learner who has corrected lemmas, I want my corrections and exclusions to carry over wherever the word is unchanged, so that I don't pay for constructions with my own work.
45. As a Greek learner, I want the confirmation to state how many corrections and exclusions I have and that they carry over where the word is unchanged, so that I know what will happen.
46. As a Greek learner updating a whole Work, I want one Work-level statement of the totals, so that I can confirm once.
47. As a Greek learner, I want `/jobs` to report "N of M decisions carried over" after the update, so that I can see the outcome.
48. As a Greek learner, I want prepared decks and earlier Readings to keep the evidence they froze, so that my study history never shifts under me.
49. As a learner of a modern language, I want my corrections to carry over in an ordinary Analysis update too, so that NLP improvements don't cost me my review work.
50. As a learner, I want a Source structure update to keep its current rules, so that a re-extraction that may move words never misapplies my decisions.

**Improving over time**

51. As a Greek learner, I want detection improvements to reach all my Books without another confirmation, so that I benefit from fixes automatically.
52. As a Greek learner, I want a Book that is being re-read for constructions to show none rather than a mixture, so that I never see two versions at once.
53. As a Greek learner, I want my lemma corrections to have no effect on which constructions are found, so that a vocabulary decision never changes the syntax shown.
54. As a Greek learner, I want definitions and relation names improved without any update, so that wording fixes are free.

**Maintainer and operator**

55. As the maintainer, I want every construction shipped only when its measured precision clears the bar, so that the learner-facing promise of precision is kept.
56. As the maintainer, I want a detector change that changes output without a version bump to fail CI, so that rebuilds are never skipped.
57. As the maintainer, I want a release check that recomputes precision against a committed, ID-only judgment file, so that regressions are caught before release without committing source annotations.
58. As the maintainer, I want to review every concept note, verb list and relation name before it ships, so that Mouseion's wording is accurate and its own.
59. As the maintainer, I want a test that fails when a construction can be emitted without a reviewed note and citation, so that no unexplained label reaches a learner.
60. As the deployer, I want the new interpretation version to need no new licence row or download, so that upgrading is routine.
61. As the deployer, I want the change to leave modern-language analyses, prepared decks and deck notices unchanged, so that the upgrade is low-risk.
62. As a future Latin implementer, I want detectors written against a source-neutral interface with one adapter per scheme, so that Latin's UD syntax can be added without a redesign.

## Implementation Decisions

### Syntax source and storage ([ADR 0122](../adr/0122-ancient-greek-native-agdt-syntax.md))

- **Native AGDT, not UD.** No public converter fits OGA, conversion merges five
  distinctions the detectors need, and Stanza `grc` is non-commercial and would
  discard the gold-trained trees.
- **A new OGA interpretation version** for all 22 Classical Reading-unit
  definitions. It bumps `interpretationVersion` and `revision` and is adopted
  through the existing Classical Analysis update. Its runs record
  `syntax_state = 'native_agdt'`, and the interpretation version stays the run's
  analyzer version.
- **The run's syntax state is the scheme marker** for every stored relation and
  head. There is no per-token scheme. The proto and domain comments that call
  the token relation UD are corrected to say it is read according to the run's
  syntax state.
- **Stored verbatim.** Relations and heads are stored as the source has them,
  with plain encoding defects corrected at import (a Greek-capital-Zeta `AuxZ`
  becomes Latin `AuxZ`). Comma roots, ambiguous COORDs and stray labels (`AuxV`,
  `_`) are normalised at read time by the AGDT adapter.
- **Unprinted syntax nodes.** Ellipsis nodes are stored with their relation,
  head and stable source id within the sentence, and with no form, lemma,
  morphology, vocabulary identity or Annotation occurrence. A printed token's
  head may be an unprinted node.
- **Printed spans are unchanged.** Ordinals and offsets come from the existing
  sentence reconstruction, which skips unprinted nodes. The new version must not
  change that reconstruction or the sentence-inclusion rules. The carry-over
  depends on it.
- **Morphology remap.** The version also maps VerbForm (Fin/Part/Inf), Aspect
  (Perf for perfect and future perfect, Imp for imperfect), the perfect tense
  and common gender, following UD Perseus r2.19. Whether non-finite forms carry
  Mood is settled on the implementation issue.
- **UD consumers stay off for Greek.** Sentence study's relation display and
  Concordance's syntax reads keep treating `native_agdt` as no UD syntax. Every
  other reader of the token relation must produce the same Greek output as
  before. Today every Greek token carries `dep`. Selection eligibility and card
  bolding key on `compound:prt`, and GDEX is German-only. The lexical
  sense-ordering context adds the raw relation as a context word, so it must be
  gated or shown unused for Greek.

### Construction layer ([ADR 0123](../adr/0123-constructions-are-a-derived-reading.md))

- **Modules:**
  - an **AGDT adapter** that presents the stored parse through a small
    source-neutral construction interface (nodes, heads, normalised relation
    categories, morphology, source lemma);
  - a **detector module** with one detector per construction, a pure function
    over that interface;
  - a **shipping gate** and **authored data** (verb lists, concept notes,
    relation names), all compiled data keyed by stable ids;
  - a **builder** that writes occurrences for a run;
  - a **rebuild worker** on `classical_preparation`.
- **Deep-module boundary.** The detector module's interface is one call: given
  a sentence read through an adapter, return its construction occurrences
  (construction id, optional subtype id, anchor, extent). Normalisation, the
  gate and the verb lists sit behind it.
- **The parse is evidence; constructions are a derived reading.** Occurrences
  are stored per run in a derived table, with a per-run marker holding the
  **construction detector version** and readiness. This follows the Browse-count
  readiness and builder-version pattern.
- **One AGDT construction detector version** covers normalisation, the six
  detectors, the verb lists and the shipping gate. It is bumped by hand.
- **Built at publish.** A new `native_agdt` run's occurrences are built inside
  the preparation transaction, before it publishes. Runs in any other syntax
  state get none.
- **Rebuild without confirmation.** A version bump rebuilds every `native_agdt`
  run, including pinned ones. Each run is swapped in one transaction (delete and
  insert under the marker). Only the current version's rows are kept, and a run
  whose marker is behind shows no constructions.
- **Source inputs only.** Detectors read source lemmas and morphology. Lemma
  corrections and Occurrence exclusions never affect occurrences.
- **Rows hold ids only.** Names, definitions and citations are looked up at
  render. A subtype is stored only when the shipping gate labels it. Word
  relations are read through the adapter at render, and are neither stored nor
  versioned.

### Schema (ADR 0038 brief; exact DDL in the migration review)

- **Printed tokens (existing):** the relation column holds the AGDT relation. A
  token headed by an unprinted node references that node, for example through a
  nullable reference beside the head ordinal. Its head ordinal is never read as
  UD.
- **Unprinted syntax nodes (new):** owner, run, sentence, stable source id,
  relation, and a head that is a printed token, another unprinted node or the
  root.
- **Construction occurrences (new, derived):** owner, run, sentence, language
  (denormalised for Concordance), construction id, optional subtype id, anchor
  token ordinal and extent token-ordinal set. Indexed on (owner, language,
  construction id, subtype).
- **Per-run construction marker (new):** owner, run, detector version and
  readiness.
- All additive, with no backfill. Existing `absent` runs have no rows.

### Occurrence model

- An occurrence is one construction in one exact analysis.
- Its **anchor** is exactly one printed token.
- Its **extent** is the set of printed tokens in the anchor's subtree, read
  through the adapter. It may be discontinuous, and unprinted nodes are not
  members.
- Occurrences may nest and overlap. One anchor may carry several different
  constructions, never the same one twice.
- A subtype is an attribute, not a separate construction. An undeterminable
  subtype is omitted.

### First-slice inventory (Greek-specific)

"Through COORD" means looking up or down through coordination groups. Lemmas
are the run's source lemmas.

| Id | Construction | Smyth 1920 | Anchor | Definition | Exclusions and accepted limits |
|---|---|---|---|---|---|
| `conditional` | Conditional clause (protasis) | §§2280–2368 | the conjunction | AuxC εἰ, ἐάν, εἴπερ or ἄν (OGA's lemma for contracted ἐάν), heading through COORD an ADV verb or ADV unprinted node | Not after, or headed by, ὡς / ὥσπερ. Concessive εἰ καί / καὶ εἰ (§§2369–2382) counts. The apodosis is outside the extent. No "whether" exclusion. |
| `articular_infinitive` | Articular infinitive | §§2025–2038 | the infinitive | a printed infinitive with a neuter singular article as a direct dependent, including under a preposition | The article may be misattached across a long subject. |
| `result` | Result clause | §§2249–2278 | ὥστε | AuxC ὥστε heading through COORD an ADV verb or ADV unprinted node | With a participle: no hit. Independent "and so" ὥστε (§2275) is not yet separated. |
| `purpose` | Purpose clause | §§2193–2206 | ἵνα | AuxC ἵνα (incl. ἵνα μή) heading through COORD a printed ADV subjunctive or optative | ὅπως / ὡς final clauses deferred. Effort and fearing clauses are not purpose. |
| `genitive_absolute` | Genitive absolute | §§2070–2075 | the participle | a printed genitive ADV participle with a printed genitive SBJ, directly or through COORD | Not when its head (through COORD) is a verb of perceiving (§§2110–2112). Subjectless (§2072) not detected. Unprinted participle: no hit. |
| `indirect_statement` | Indirect statement | §§2016–2024; §§2577–2588 | the infinitive, or the conjunction | **infinitive:** a printed OBJ/SBJ infinitive whose head (through COORD) is in the infinitive list, with every SBJ accusative or none (§1973); ὑπισχνέομαι and ὄμνυμι only with a future infinitive. **ὅτι/ὡς:** AuxC ὅτι/ὡς heading through COORD a printed OBJ/SBJ indicative, subjunctive or optative whose head is in the ὅτι/ὡς list | The governing verb is not a member. δοκέω is excluded from the infinitive list. Verbs of will and impersonal δεῖ/χρή are outside. ὅτι before a direct quotation (§2590 a) counts. Cross-sentence speech and the personal passive (§1982) are accepted losses. |

Plain "accusative + infinitive" (§§1972–1981) is not a construction.

**Subtypes:**

| Construction | Subtype id | Meaning | Smyth 1920 | Labelled? |
|---|---|---|---|---|
| Result | `natural` | ὥστε + infinitive | §2258 | yes, "natural result" |
| Result | `actual` | ὥστε + finite mood | §2257 | no |
| Indirect statement | `infinitive` | infinitive form | §2016 | yes, "infinitive" |
| Indirect statement | `hoti_hos` | ὅτι/ὡς clause | §2577 | no |
| Articular infinitive | `genitive` / `dative` | article case | §2032 / §2033 | yes |
| Articular infinitive | `nominative` / `accusative` | article case | §2031 / §2034 | no |

A result clause whose verb is unprinted has no subtype.

**Authored lexical data** (compiled, reviewed by Justin on the implementation
issue, part of the detector version):

- **Infinitive list:** φημί, φάσκω, λέγω, εἶπον, ἐρῶ, ἀγγέλλω, ἀπαγγέλλω,
  ἐξαγγέλλω, ἀποκρίνω, ὁμολογέω, ἀρνέομαι, ἀπαρνέομαι, ὑπισχνέομαι*, ὄμνυμι*,
  νομίζω, οἴομαι, ἡγέομαι, ὑπολαμβάνω, ἐλπίζω, ὑποπτεύω, πιστεύω, διηγέομαι,
  ἐννοέω (* future infinitive only).
- **ὅτι/ὡς list:** the infinitive list plus δοκέω, οἶδα, ἐπίσταμαι, μανθάνω,
  ὁράω, ἀκούω, μιμνῄσκω/μιμνήσκω, ἀναμιμνῄσκω/ἀναμιμνήσκω and πείθω.
- **Perceiving:** ἀκούω, ἀκροάομαι, αἰσθάνομαι, πυνθάνομαι.

### Abstention and the shipping gate

- Precision-first and silent: no hedged labels, no "none found" statement, and
  no abstained state.
- Per construction only, with no per-occurrence confidence layer.
- **Detectors fail closed.** A required input that is missing, or reached only
  through an unprinted node where the definition asks for a printed token, gives
  no hit. The conditional and result definitions explicitly admit an ADV
  unprinted node as the clause.
- **Bar.** A construction ships at precision ≥ 0.90 on at least 120 random
  corpus hits, with a 95% Wilson lower bound ≥ 0.84. A Subtype label meets the
  same bar on at least 60 hits. Recall is reported, not gated.
- **First slice.** All six constructions ship. Labels: natural result,
  infinitive indirect statement, and genitive and dative articular infinitive.
  Conditional ships only after the first release-check run confirms it without
  the "whether" exclusion.

### Concept notes and grammar reference ([ADR 0124](../adr/0124-grammar-references-and-concept-notes.md))

- **One grammar-reference record per language:** an id, a citation label, an
  edition and a URL template filled from an integer §.
- **Greek:**
  `https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={§}`.
  Documented swaps: Hopper's `doc=Smyth+grammar+{§}` form and Alpheios anchors.
  There is no runtime fallback and no link checking.
- **Citation text.** "Smyth 1920 §N" or "§§N–M" is always shown as text. A range
  links to its first §, and the link carries `rel="noopener noreferrer"`. A
  construction without a citation does not ship.
- **Concept notes.** A note has a name, a one- or two-sentence definition in
  Mouseion's own wording and one citation, with no example. Notes are compiled
  data keyed by construction id, each recording its review ticket, and are
  looked up at render.
- **Relation names** are compiled labels in Mouseion's wording, owned by the
  adapter:

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

- **Latin shape** (nothing compiled): Allen & Greenough at DCC, cited "A&G §N".

### Sentence study page

From
[Prototype: constructions and syntax in Sentence study](https://github.com/justin-hayes/mouseion/issues/2609#issuecomment-7127948787),
variant C.

- **Order:** sentence (unmarked) → **Constructions** (`h3`, ordered list) →
  interlinear morphology line → Notation key → **Word relations** (closed native
  disclosure) → Complete text alternative.
- **Each entry:**
  - the name, plus the Subtype label in parentheses;
  - the extract (see [the design pattern](../design/components.md#accepted-construction-extract-target));
  - "Signalled by *anchor*." plus "The identified target signals this
    construction.", "Contains the identified target." or nothing;
  - the definition and the linked citation;
  - **Parallels**, with an accessible name that includes the construction and
    subtype.
- **Ordering:** textual order of each extent's first token, outer before inner
  at the same token.
- **Silence:** the region is absent when there is nothing to show, including
  for `absent` and rebuilding runs.
- **Focus from Concordance:** order unchanged, a thicker accent edge, visible
  "· from Concordance", scrolled into view.
- **Word relations table:** columns #, Word, Relation, Depends on (with "(word
  n)"). The target is bold, with a hidden "(identified target)". There are no
  punctuation rows, and the root shows "— (top of the sentence)".
- **Unprinted heads:** a word whose head is unprinted reads "‹an unprinted
  word›, which is the {relation} of {first printed ancestor} (word n)". One
  note explains this, shown only when one occurs.
- **Accessibility:** the extract is `aria-hidden`, a hidden "Words: …" line
  replaces it, and the text alternative names each word's constructions.

### Concordance parallels

Recorded in [ADR 0125](../adr/0125-construction-parallels-in-concordance.md).

- **API contract.** The existing lookup route gains `match=construction`, with
  the construction id as the term and optional `subtype` and anchor-lemma
  parameters. It reuses captured priority, the revision guard, 25-row global
  paging and KWIC rendering. Only languages with a construction scheme (`grc`)
  are offered it; others get the safe-restart notice. The search box never
  recognises construction names.
- **Tiers:**
  - **This Book** (captured like the priority Book);
  - **This author**, same Work first, then the author's other Works by title;
  - **Other authors**, by author, Work title and Reading unit order.

  The author is the curated Work author string, matched exactly. A Book with no
  Work has no author tier.
- **Order inside a tier:** Work, Reading unit, sentence, anchor. The origin
  occurrence is omitted.
- **Filters.** The lemma filter applies to the anchor's effective lemma only.
  The subtype filter covers shipped labels only. A construction-only query
  ignores corrections and exclusions. With a lemma filter, the anchor is
  corrected and an excluded anchor is hidden.
- **Unsearched Books** are named in one quiet line; rebuilding runs stay silent.
- **Revision guard.** The evidence revision includes the detector version and
  each eligible run's marker.
- **Query shape:** construction rows → eligible current analyses with a current
  marker → tier join through Reading unit → Edition → Work → rank and page.
  Eligibility matches the existing three Concordance eligibility statements.
- **Rows:** centred on the anchor. The expanded row reuses the extract. **Study**
  opens Sentence study with that construction focused.

### Analysis update and carry-over ([ADR 0126](../adr/0126-analysis-updates-carry-decisions-onto-identical-tokens.md))

- **What is copied.** At publish, inside the publishing transaction, each Lemma
  correction and Occurrence exclusion on the run being replaced is copied onto
  the new run's token with the same source document, offset span, surface and
  source lemma. UPOS and morphology are not compared.
- **Absorbed corrections.** A correction whose target lemma now equals the
  source lemma is not copied and counts as carried over.
- **Scope.** This covers every Analysis update (NLP and Classical), not Source
  structure updates. The old run keeps its own decisions.
- **Change note.** An optional per-interpretation-version change note is held
  next to the Reading-unit definition data and looked up at render.
- **Copy:**
  - The notice is unchanged.
  - The per-Book confirmation reads: change note; "The published analysis stays
    in effect until the update publishes."; "Your N confirmed lemma corrections
    and M occurrence exclusions carry over wherever the word is unchanged in the
    new analysis."; "Prepared decks and earlier Readings keep the evidence they
    froze."
  - The Work-level line reads: "Together, the Books to be updated have X
    confirmed lemma corrections and Y occurrence exclusions, which carry over
    wherever the word is unchanged in their new analyses."
  - `/jobs` reads "N of M decisions carried over".

### Register and notices

- There is no new register row or `layers.json` layer, because the existing OGA
  annotations row already names syntax. When syntax ships, the use column gains
  one sentence: relations, heads and the constructions derived from them are
  shown in Sentence study and Concordance.
- The deck notice is unchanged, and cards carry no constructions.
- Showing any Smyth or A&G wording, or shipping a host-derived § → URL map,
  would need a row.

## Testing Decisions

**What makes a good test here.** Tests assert learner-visible behavior and stored
outcomes through the public boundary. That means the rendered page, the
Concordance results, the update confirmation, the `/jobs` line and the
published run's evidence. They do not assert adapter internals or the order of
queries. Fixtures are short and synthetic, and use the real AGDT label
inventory: PRED, SBJ, OBJ, ATR, ADV, AuxC, AuxP and so on, never `DET` or
`PUNCT`. No OGA text or annotation is committed.

**Seams (confirmed with Justin, 2026-11-24).**

1. **Primary seam: the Classical preparation HTTP integration harness**
   (Postgres via testcontainers, real routes and in-process River workers, no
   NLP). A synthetic OGA fixture with AGDT syntax and ellipsis goes through
   Add, then To Read, then the `classical_preparation` worker, which publishes.
   Tests then read Sentence study and Concordance with `match=construction`
   over HTTP. The same seam covers:
   - adopting the new interpretation version through the Work and Book Analysis
     update, including carry-over and the `/jobs` line;
   - the detector-version rebuild, by running the rebuild worker and then
     reading the pages;
   - the unchanged Greek output of every other reader of the token relation.

   Prior art:
   - `TestClassicalCatalogCompleteLoop` and
     `TestClassicalPreparationFetchesOnceAndPublishesEachUnit` (preparation and
     publish);
   - `TestClassicalWorkAnalysisUpdatePublishesAndRefreshesPreparedDefinition`
     and `TestAnalysisUpdateReplacesOutdatedAnalysisThroughOneConfirmedRun`
     (updates);
   - `TestBrowseCountsRebuildWorkerRebuildsCurrentBookDurably` (rebuild under a
     readiness marker);
   - `TestConcordanceCapturesPriorityAndKeepsStableOccurrencePagesOverHTTP`
     (tiers, paging and the revision guard);
   - `TestAncientGreekLemmaReviewCorrectsAndExcludesHomographs` (decisions on
     Greek tokens);
   - `TestAncientGreekBookOneDeckPreparationAndExport` (decks unchanged).
2. **Supporting seam: the detector module** (pure Go, no database). Each
   definition, exclusion, subtype and fail-closed case gets a small synthetic
   tree read through the AGDT adapter. A **golden-output test** over a synthetic
   corpus fails when emitted occurrences change without a detector-version bump.
   A unit test asserts that every emittable construction id has a reviewed
   concept note with a citation. The repo has no golden-file pattern yet; the
   prior art is the committed byte-for-byte golden data in the enrichment
   request-codec test.
3. **Supporting seam: browser smoke** with Playwright against the fixture server
   (in-memory fixtures, no Postgres). The fixtures gain a Greek sentence with
   nested and discontinuous constructions and an unprinted head. Smoke covers:
   - extract marking that is not colour-only;
   - the focused state;
   - disclosure operation and the no-JS path;
   - 375px and 1200px widths in both themes.

   Prior art: the existing smoke spec "Concordance lookup is server-rendered and
   submits without JavaScript", and the responsive-theme spec.

**Behavior to cover, by seam:**

| Behavior | Seam |
|---|---|
| New run records `native_agdt`, verbatim relations (Zeta `AuxZ` repaired) and unprinted nodes with heads | primary |
| Printed spans, surfaces and source lemmas equal the previous version's | primary |
| VerbForm, Aspect, perfect and common gender mapped | primary (published evidence) |
| UD consumers off; other relation readers unchanged for Greek | primary |
| Each definition's hits, exclusions, subtypes and fail-closed cases | detector |
| Only shipped constructions and labels emitted; output changes only with a version bump | detector (golden) |
| Every emittable id has a reviewed note and citation | detector |
| Rows built before publish; `absent` runs get none | primary |
| Version bump rebuilds every `native_agdt` run in one swap; nothing shows while behind | primary |
| Corrections and exclusions never change construction rows | primary |
| Sentence study order, entry text, citation link attributes, Parallels names, silence | primary |
| Word relations names, root, unprinted-head wording and note | primary |
| Extract marking, focus state, accessibility, widths, themes, no-JS | browser smoke |
| Tiers, textual order, origin omitted, filters, corrections and exclusions, 25-row paging | primary |
| Unsearched Books named; rebuilding silent; revision guard on version or readiness change | primary |
| Non-`grc` construction URL gives the safe restart | primary |
| Update confirmation and Work copy with and without a change note | primary |
| Carry-over: matched copied, absorbed counted, unmatched dropped, old run unchanged, `/jobs` line; NLP updates too; Source structure updates unchanged | primary |
| Decks, deck notices and frozen Readings' parses unchanged | primary (existing deck loop) |

**Release check (not a CI test).** A `make` target recomputes per-construction
and per-subtype precision from a committed, ID-only reviewed judgment file
against the pinned OGA files, which are fetched and never committed. Every
detector-version bump runs it and puts its output in the PR. A bump fails if a
shipped unit drops below the bar or a reviewed item flips without a new
judgment. A bump changing more than 10% of a construction's hits needs a fresh
sample of 120.

## Out of Scope

- Latin (direction only). Every choice above leaves room for a UD adapter and a
  Latin grammar reference, and nothing Latin is built.
- Modern-language constructions (*Konjunktiv I*, *passé simple*).
- Anki card fields carrying constructions or syntax, and any deck or deck-notice
  change.
- Cross-language parallels.
- Learner corrections of syntax or constructions (plausibly next, not never).
- Construction-based deck selection or difficulty scoring.
- Greek syntax on the Stanza `grc` / graded-reader path (paused).
- A construction picker in Concordance, or ranking parallels by quality.
- An author identity beyond the curated Work author string.
- Showing raw AGDT or UD labels, or any Smyth or A&G wording, to learners.
- Runtime or CI link checking of grammar references.
- The deferred construction tail below.

## Further Notes

### Deferred tail (ranked)

Ranges were checked against Smyth 1920.

1. Conditional type (simple, general, future more or less vivid, contrary to
   fact), which needs the apodosis mood and ἄν.
2. Indirect statement with a participle (§§2106–2115).
3. Effort clause, ὅπως + future indicative (§§2209–2219).
4. Clause of fearing, μή / μὴ οὐ (§§2221–2233).
5. Temporal clauses: ἐπεί, ὅτε, πρίν, ἕως (§§2383–2461).
6. Attributive or substantive participle with the article (§§2049–2053).
7. Potential optative and ἄν constructions (§§1824–1826).
8. Accusative absolute (§§2076–2078).
9. Conditional relative clauses (§§2560–2573).

Added by
[Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608#issuecomment-7106323004),
unranked:

- a test for substantive "whether" εἰ (§2671);
- ὅτι/ὡς + OBJ under any verb;
- cross-sentence indirect statement;
- personal δοκέω;
- ὅπως and ὡς final clauses;
- a rule separating independent ὥστε (§2275), which would let actual result be
  labelled;
- the nominative and accusative articular-infinitive labels.

### Smyth 1920 verification

On 2026-11-24 every § was checked against the Perseus Hopper 1920 text: the
opening of each range's first and last §, plus the § after it. Ranges cited from
memory were corrected:

| Cited as | Finding | Now cited |
|---|---|---|
| Articular infinitive §§2025–2034 | §§2035–2038 still treat it; §2039 starts the participle | §§2025–2038 |
| "After a preposition" §2032 | §2032 is the genitive; prepositions are treated within each case | §§2031–2034 |
| Natural result §2251 | §2251 introduces both forms; the infinitive of result is §2258 | §2258 |
| Perception verbs §§2110–2115 | §§2110–2112 are perception; §2113 is verbs of finding | §§2110–2112 |
| ὅτι recitativum §2590 | confirmed in §2590 a | §2590 a |

All other ranges cited here are confirmed.

### Measured precision

From the re-measure: 95% Wilson interval, 120 random hits each.

| Construction | Precision | Recall |
|---|---|---|
| Purpose (ἵνα only) | 1.00 (0.97–1.00) | 0.95 |
| Result | 0.97 (0.92–0.99) | 0.91 |
| Indirect statement | 0.94 (0.88–0.97) | 0.60 |
| Conditional | 0.93 (0.86–0.96), with the since-dropped exclusion | 0.92 |
| Articular infinitive | 0.92 (0.85–0.95) | 0.98 |
| Genitive absolute | 0.91 (0.84–0.95); every false hit is a parser error | 0.74 |

Subtypes:

- **Clear the bar:** natural result 1.00, dative articular infinitive 0.98.
- **Straddle it:** genitive articular infinitive 0.95, infinitive indirect
  statement 0.95, ὅτι-ὡς 0.92, actual result 0.90.
- **Fail:** nominative articular infinitive 0.85, accusative 0.82.

The blind spot-check agreed on 35 of 36 items.

**Limits.** Gold comparisons exist only for training texts (*Apology*, *Crito*,
*Anabasis*). *Phaedo*, *Symposium*, *Meno* and *Republic* are measured only
through the reviewed samples. Recall figures are optimistic, and AGDT
inter-annotator agreement is 0.806.

### Evidence

The research branches are throwaway and never merged; they commit IDs and
aggregates only.

| Ticket | Evidence |
|---|---|
| [Research: AGDT syntax profile](https://github.com/justin-hayes/mouseion/issues/2598#issuecomment-7098250031) | [`research/grc-oga-agdt-syntax-profile`](https://github.com/justin-hayes/mouseion/blob/7dfd50d55781399585334e9dbd8c8e8c6c0ff17d/doc/research/grc-oga-agdt-syntax-profile.md) |
| [Research: AGDT-to-UD conversion cost](https://github.com/justin-hayes/mouseion/issues/2599#issuecomment-7098298122) | [`research/grc-agdt-ud-conversion`](https://github.com/justin-hayes/mouseion/blob/0e458333932b4a059120f29127ee6cd0ec8aca5b/doc/research/grc-agdt-ud-conversion.md) |
| [Research: OGA parser accuracy and prior art](https://github.com/justin-hayes/mouseion/issues/2600#issuecomment-7098290790) | [`research/grc-parser-accuracy-construction-prior-art`](https://github.com/justin-hayes/mouseion/blob/6d0284e903fdb37cc8e9f2f582dfa46ad18026c6/doc/research/grc-parser-accuracy-construction-prior-art.md) |
| [Research: grammar-reference link targets](https://github.com/justin-hayes/mouseion/issues/2601#issuecomment-7098264270) | [`research/grc-grammar-reference-targets`](https://github.com/justin-hayes/mouseion/blob/cf2e07459642570b4b97aedc45406d839d4e3c7b/doc/research/grc-grammar-reference-targets.md) |
| [Research: ADR 0111 and OGA-derived syntax](https://github.com/justin-hayes/mouseion/issues/2602#issuecomment-7098154703) | [`research/grc-syntax-licence-posture`](https://github.com/justin-hayes/mouseion/blob/bf1b301fe8a84d2010d5e109dd61ac8e0f091607/doc/research/grc-syntax-licence-posture.md) |
| [Task: measure construction detection](https://github.com/justin-hayes/mouseion/issues/2607#issuecomment-7100600509) | [`research/grc-construction-detection-measurement`](https://github.com/justin-hayes/mouseion/blob/15ae542a/doc/research/grc-construction-detection-measurement.md) |
| [Task: re-measure the amended definitions](https://github.com/justin-hayes/mouseion/issues/2615#issuecomment-7102169398) | [same branch, `db6ba186`](https://github.com/justin-hayes/mouseion/blob/db6ba186/doc/research/grc-construction-detection-remeasure.md) |
| [Prototype: constructions in Sentence study](https://github.com/justin-hayes/mouseion/issues/2609#issuecomment-7127948787) | [`research/grc-sentence-study-constructions-prototype`](https://github.com/justin-hayes/mouseion/tree/1c3bd586/doc/research/grc-sentence-study-constructions-prototype) |

The grillings that settled each decision are linked from the ADRs above and
from the map's Decisions so far.
