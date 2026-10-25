# ADR 0110: Bound Mouseion by philological reading competence

Status: **Accepted — direction** · Date: 2026-10-25 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Specified in
[#2290](https://github.com/justin-hayes/mouseion/issues/2290). Records the product direction
and boundary. It changes no shipped behavior and amends no earlier ADR.
[ADR 0099](0099-ancient-greek-imported-analyses.md)'s `absent` Greek syntax
stays in force as a current limit.

## Context

Mouseion describes itself as an environment for "advanced foreign-language
reading". That statement names no limit on which languages belong. It also
gives no test for which classical-text capabilities are worth building.

That gap showed during the Ancient Greek MVP (#2098). Within a week the work
went through:

- four source candidates
- a licensing rule reversed between #2089 and #2128
- an additive design of about 6,700 lines, with ADRs 0093–0098, written and
  withdrawn (#2062, #2097)

The withdrawn design modelled independently acquired text and annotation
sources, exact bindings between them, and owner adoption of upstream evidence.
That is research-infrastructure work the learner never needed. What shipped is
much smaller: one pinned text, imported into the existing pipeline, with cards
and Concordance.

## Decision

### Boundary

> **Mouseion builds the polyglot reading ability that classical philology
> requires: Ancient Greek and Latin, and German, French, Italian and Modern
> Greek. It treats each language's literature as worth reading for its own
> sake. It does not provide tools for constructing a scholarly edition, and it
> is not a reader.**

Philology sets the limits of the language set. It is not the only reason to
read a Book. A German novel is as legitimate a reading as a German commentary
on Thucydides. The interest is comparative: each language's literature is
respected on its own terms, and the connections between these languages are a
reason to study them together.

### Languages

- **Classical:** Ancient Greek (`grc`) and Latin. Latin is named as direction
  and is not scheduled.
- **Languages of scholarship and literature:** German, French, Italian and
  Modern Greek, all already supported.
- **English** is not a study language. It stays the language of glosses,
  translations and passage context.

A language outside this set needs a new ADR.

### Surfaces

Mouseion has no in-app reader. The learner reads elsewhere. Classical-text
capabilities enhance the existing surfaces where they add value:

| Surface | Capabilities in scope |
| --- | --- |
| **Sentence study** | Morphology, syntax (dependency relations, clause structure) and the construction an occurrence belongs to, with links to the relevant reference-grammar concept (indirect statement, genitive absolute, *cum* clause, *Konjunktiv I*, *passé simple*) |
| **Concordance** | Parallels for a lemma or construction, first within the author's works and then across the study language |
| **Anki cards** | Contextual Gloss, the dictionary entry (LSJ for Ancient Greek), principal parts and irregular forms; translations only as the labelled, provider-proposed fields cards already carry |
| **Vocabulary and Reading** | The existing frequency, Known/Unknown, coverage, current-reading snapshot and prepared-deck machinery, unchanged |

Modern-language Books need lighter morphology. They keep the same vocabulary
and reading workflow.

**Syntax for Ancient Greek and Latin is in scope and wanted.** "Syntax
unavailable" is a current limit, not the intended end state. Choosing a syntax
source and its licence is a later decision. This ADR does not reopen #2089 or
#2128.

### Cross-language connections

Connections between study languages are an admitted direction:

- Latin vocabulary inherited or borrowed into French and Italian
- the continuity from Ancient to Modern Greek
- Greek and Latin influence on German

No mechanism is chosen. Data sources, and whether a connection appears as
etymology on a card or as a cross-language Concordance query, are decided when
a feature is specified.

### Texts

- **Classical Books** stay operator-curated. Each has one chosen Edition per
  Work, divided by its conventional citations (book, chapter, section), with
  its provenance kept. Mouseion never compares Editions.
- **Upstream data.** The Open Greek and Latin project, Perseus and treebanks
  are upstream data that Mouseion imports through bounded adapters. Mouseion
  does not reproduce them.
- **Modern-language Books** keep coming from the learner's own OPDS catalog,
  unchanged.

### Non-goals

- an in-app reader
- English as a study language
- manuscript or witness management, diplomatic transcription, paleography
- critical-apparatus creation, variant collation, stemmatics
- competing Editions or edition comparison
- TEI authoring, arbitrary annotation layers, editorial workflows, scholarly
  publication
- a general-purpose CTS/CITE datastore

## Consequences

- **A test for proposed features.** A feature passes if it improves reading
  ability in a study language on an existing surface. Edition-construction or
  research-infrastructure features are out of scope without a new ADR.
- **The withdrawn source model stays withdrawn.** The withdrawn multi-source
  model (ADRs 0093–0098) is not a deferred target. Its glossary terms have been
  removed. Work, Edition and Reading unit remain.
- **The language set is closed.** Adding a language beyond the six named here
  is a product-boundary change.
- **Existing behavior is unchanged.** `product.md` describes present state and
  only gains the boundary statement.

## Alternatives considered

- **Philology as the sole purpose.** Modern languages would be justified only
  as languages of scholarship. Rejected: it demotes literature in German,
  French, Italian and Modern Greek to a side effect, and the learner's
  interest is comparative.
- **General advanced foreign-language reading.** This is the previous
  framing. Rejected: it sets no limit on languages and gives no test for
  classical-text scope, which is the confusion the MVP showed.
- **An interactive reader as the centre.** Rejected: Concordance, Sentence
  study and Anki cards are the primary focus. A reader would duplicate the
  learner's own reading environment.
