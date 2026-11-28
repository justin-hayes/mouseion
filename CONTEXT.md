# Mouseion

Mouseion is a self-hosted reading environment for the languages of classical
philology: Ancient Greek and Latin, and German, French, Italian and Modern Greek
([ADR 0110](doc/adr/0110-philological-reading-competence-boundary.md)).
A learner finds texts in one Catalog, which holds the curated Classical catalog
and the learner's own OPDS catalogs, adds the ones they choose to a personal
library, analyzes each Book according to its declared EPUB structure, and studies
the vocabulary of the Books they read in Study.

This glossary is the project's ubiquitous language: each term below names one
concept and is used with that meaning in code, interface text, and
documentation. Each entry gives the definition and, under _Avoid_, the
synonyms that are deliberately not used.

**Status notes.** The current disposition and current-reading contract is recorded in
[ADR 0078](doc/adr/0078-book-dispositions-and-current-reading.md), as amended
by ADR 0136 below.
[ADR 0072](doc/adr/0072-goal-owned-vocabulary-and-journey-forecast.md)
records the historical Goal and ordered-Journey model.
[ADR 0079](doc/adr/0079-contextual-glosses-require-llm.md) records the
implemented contextual Gloss vocabulary below;
[ADR 0112](doc/adr/0112-gloss-is-lemma-sense-in-citation-form.md) records the
citation-form Gloss definition, whose implementation is pending.
[ADR 0117](doc/adr/0117-one-catalog-my-books-holds-chosen-books.md) (accepted)
defines **Catalog**, **Catalog index**, **Catalog entry**, **Book**, **Book
disposition**, **Book workflow bucket**, **My Books** and **Previously read**
below, and retires **Inbox** and **Hidden**. My Books now holds only chosen
Books; sync maintains each connection's local Catalog index instead of creating
Books.
ADRs [0122](doc/adr/0122-ancient-greek-native-agdt-syntax.md)–[0126](doc/adr/0126-analysis-updates-carry-decisions-onto-identical-tokens.md)
(accepted, implementation pending) define the Construction terms, **Unprinted
syntax node**, **Grammar reference** and **Concept note** below, the
`native_agdt` reading of **Imported analysis**, and the carry-over of decisions
in an **Analysis update**.
The Study terms (**Study**, **Study day**, **Today** and the terms after them)
are defined by [ADR 0134](doc/adr/0134-study-in-mouseion-vocabulary-review.md),
[ADR 0135](doc/adr/0135-study-reports.md) and
[ADR 0137](doc/adr/0137-idioms-are-study-items.md), as amended by ADR 0136.
[ADR 0136](doc/adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md)
made Study the only vocabulary workflow at one cutover. It defines **Corpus**,
**Already known**, **Target coverage**, **Read next** and **Book detail** below.
The cutover retired **Known vocabulary**, **Prepared deck** and every term that
served them, marked retired below the way **Inbox** is: **Recurring
vocabulary**, **Book vocabulary candidates**, **Reserved vocabulary**,
**Unknown vocabulary**, **Current-reading vocabulary snapshot**, **Generated
vocabulary**, every **Omission** kind, **Deck specification**, **Card
presentation**, **Presentation version**, **Render-input version**, **Deck
revision**, **Vocabulary transition** and **Graduation**. It also retired
**Lemma review**, **Lemma review flag** and **Lemma suggestion**, and **Reading**
as a page. Reading completion records history only.
Superseded decisions ([ADR 0036](doc/adr/0036-primary-goal-justified-graduation.md),
[0085](doc/adr/0085-reading-owned-book-vocabulary.md),
[0086](doc/adr/0086-reading-working-desk-hidden-visibility-and-concordance.md),
[0090](doc/adr/0090-library-total-book-vocabulary-selection.md),
[0100](doc/adr/0100-retryable-provider-omissions-preparation.md),
[0114](doc/adr/0114-idiom-cards-from-dictionary-evidence.md)) remain as
history.

## Language

**Study language**:
A language the learner studies. The set is inferred from the languages present
in the learner's library (books with a chosen language), never configured by
the learner.
_Avoid_: configured language, preferred language, library language (as a stored
selection defining the set).

**Active study language**:
The study language of the per-language page in view (My Books, Study,
Vocabulary, Concordance, Sentence study), named by that page's address. It is
context, not configuration: it never defines which languages are studied, and
two pages open at once may each have their own. The Catalog, a Book's own pages
and pages without a language have none.
_Avoid_: current language, mode, active profile, stored language.

**Last study language**:
The study language of the per-language page the learner most recently viewed,
remembered only to choose where an address without a language leads. It points
into the derived study-language set: when the set is unambiguous it defaults
deterministically, and if it leaves the set it resets. Its absence is never a
learner selection.
_Avoid_: active study language (the page's own), preferred language, default
language (as configuration).

**Catalog sync scope**:
The set of offerings eligible for a connection's Catalog index. In the OPDS
flow, this is the non-English language feeds whose NLP service reports ready,
independent of learner configuration.
_Avoid_: study-language scope, synced languages.

**Known vocabulary** (retired):
Retired by ADR 0136: knowledge is Study progress, and the one claim of knowledge
is **Mature**. The Known vocabulary table was replaced by Already-known Study
progress, with first reviews spread over several weeks, and Study, coverage and
every vocabulary surface no longer consult it.
_Avoid_: known words, learned vocabulary, Mature (a different, verified fact).

**Recurring vocabulary** (retired):
Retired by ADR 0136 with Book vocabulary candidates: no Book has a vocabulary
pool selected for it. Study orders new words from the Corpus instead.
_Avoid_: frequent words, deck coverage.

**Book vocabulary candidates** (retired):
Retired by ADR 0136 with the Current-reading vocabulary snapshot: nothing selects
a Book's vocabulary for a deck. A candidate for Study is any effective vocabulary
identity in the Corpus that is not in study.
_Avoid_: recurring vocabulary, corpus words.

**Personal analyzed-Book occurrences**:
The learner-and-study-language-scoped total of an effective vocabulary identity's
eligible occurrences across currently analyzed Books, after confirmed occurrence
decisions. Distinct editions and overlapping Reading units contribute separately;
the total is not a count of unique textual encounters.
_Avoid_: global frequency, unique corpus occurrences, external corpus frequency.

**External frequency evidence**:
Source-attributed vocabulary occurrence counts or ranks from a corpus independent
of the learner's analyzed Books. It is not personal analyzed-Book occurrences or
evidence of learner knowledge.
_Avoid_: personal corpus counts, universal Greek frequency.

**Reserved vocabulary** (retired):
Retired by ADR 0136 with the Current-reading vocabulary snapshot it was frozen
from. Starting, switching, ending or finishing a Book reserves and releases
nothing.
_Avoid_: active-campaign vocabulary, Goal vocabulary, known vocabulary.

**Unknown vocabulary** (retired):
Retired by ADR 0136: with no Known or Reserved vocabulary, a word is either in
study or not yet introduced, and Mature once its interval is long enough. The
Vocabulary page and a Book's detail page show those Study states.
_Avoid_: difficult words, never generated vocabulary.

**Current-reading vocabulary snapshot** (retired):
Retired by ADR 0136: Read next freezes nothing, and a Book's later analysis is
the only evidence of its words. Reading history keeps the Book and the time of a
completion, not a vocabulary set.
_Avoid_: deck contents, generated vocabulary, study snapshot.

**Generated vocabulary** (retired):
Retired by ADR 0136 with the Prepared deck: no deck is generated, so no
identity carries generation provenance.
_Avoid_: assigned knowledge, known vocabulary, reserved vocabulary.

**Omission** (retired):
Retired by ADR 0136 with the Prepared deck, together with every kind it had
(sentence-quality, meaning, lemma-mismatch and provider omissions). Study has
no equivalent: a learner who meets a wrong word reports it.
_Avoid_: failure, dropped card, excluded word (an Occurrence exclusion is an
analysis-time decision).

**Sentence-quality omission** (retired):
A kind of Omission, retired with it by ADR 0136.
_Avoid_: quality failure, invalid sentence.

**Meaning omission** (retired):
A kind of Omission, retired with it by ADR 0136.
_Avoid_: unresolved gloss, unknown word.

**Lemma-mismatch omission** (retired):
A kind of Omission, retired with it by ADR 0136. A learner meets a wrong word in
Study and reports it as a Wrong word.
_Avoid_: wrong lemma, ambiguous omission.

**Idiom**:
A multi-word lexeme with its own dictionary entry, such as Italian *in gamba*
("capable") or *macchina da scrivere* ("typewriter"), whose meaning belongs to
the whole expression rather than to any one of its words. A proper name
(*La Calamita*, *Palazzo Madama*) is never an idiom. It is a Study item in its
own right, matched across the Corpus and confirmed by the provider sentence by
sentence, and it does not change coverage. It is not a
Construction, which is a syntactic pattern rather than a lexeme.
_Avoid_: multi-word expression (fine in prose), collocation, phrase,
construction.

**Idiom candidate**:
A match of an idiom's words in one sentence of the Corpus, found from dictionary
evidence. It is unconfirmed until the provider glosses it as an idiom in that
sentence.
_Avoid_: detected idiom, idiom occurrence.

**Idiom card** (retired):
Retired by ADR 0136 with the Prepared deck. An Idiom is a Study item
(ADR 0137), taught from its own Study material.
_Avoid_: phrase card, lemma card (for an idiom).

**Idiom omission** (retired):
A kind of Omission, retired with it by ADR 0136.
_Avoid_: lemma-mismatch omission, meaning omission.

**Idiom-card bound** (retired):
Retired by ADR 0136 with the Prepared deck. Idioms rank with words in one Study
stream and have no per-deck bound.
_Avoid_: idiom cap, phrase limit.

**Idiom overflow** (retired):
Retired by ADR 0136 with the Prepared deck and the idiom-card bound it reported.
_Avoid_: omitted idiom (for overflow), idiom omission.

**Provider omission** (retired):
A kind of Omission, retired with it by ADR 0136.
_Avoid_: failed card, provider failure, partial deck.

**Separable particle**:
The prefix component of a separable verb that detaches from the finite form in
some clauses (for example `auf` in `ich stehe auf`), identified by its
dependency relation (`compound:prt`) to the verb. A particle is a component of
the verb's full lemma, never its own vocabulary item.
_Avoid_: prefix (ambiguous with derivational prefix), verb particle.

**Separable verb**:
A German verb whose particle separates from the finite form in some clause
positions and rejoins in others (`aufstehen` → `ich stehe auf`, `aufgestanden`).
Its vocabulary identity is the full lemma, not the analyzer's base lemma.
_Avoid_: particle verb, prefix verb.

**Prepared deck** (retired):
Retired by ADR 0136: there is no Anki export, and vocabulary is studied only in
Study. The preparation, status and download pages and their jobs are removed.
_Avoid_: study plan, in-app review deck.

**Deck specification** (retired):
Retired by ADR 0136 with the Prepared deck.
_Avoid_: manifest, deck content.

**Card presentation** (retired):
Retired by ADR 0136 with the Prepared deck. Study's card is presented by the
Study surface itself.
_Avoid_: rendering, formatting.

**English target alignment**:
The optional correspondence between the tested vocabulary in a representative
sentence and one or more spans of its complete English translation. A confident
correspondence can be discontinuous; an uncertain one leaves the translation
unemphasised.
_Avoid_: English bolding (the presentation of an alignment, not the alignment).

**Passage context**:
The labelled English of the cited section or sections that contain a
representative sentence, from a source translation. It is explicitly not a
translation of the sentence and carries no English target alignment.
_Avoid_: sentence translation, wider translation, study context.

**Source-evidence card**:
A Study card built from imported sources (the study context, its passage context
and the language's dictionary entry) with a contextual Gloss that cites the
dictionary (ADR 0138). Each study language has one card policy: it either uses
source-evidence cards or takes the modern-language provider route, never both.
_Avoid_: classical card, offline card, Greek card.

**Evidence-assisted provider pass**:
The preparation layer for a language that uses source-evidence cards, which
sends the study context, its passage context and its meaning evidence to the
provider. Its contextual Gloss, which must cite at least one dictionary sense,
and its sentence translation are provider-proposed and labelled as such. A
failed or abstaining answer leaves the word without usable Study material; there
is no providerless fallback.
_Avoid_: LLM card, fallback, provider path.

**Presentation version** (retired):
Retired by ADR 0136 with the Prepared deck.
_Avoid_: template version, model version.

**Render-input version** (retired):
Retired by ADR 0136 with the Prepared deck.
_Avoid_: schema version, manifest version.

**Deck revision** (retired):
Retired by ADR 0136 with the Prepared deck.
_Avoid_: version, regeneration.

**Reading completion**:
An append-only, Book-anchored fact recorded when the learner finishes a current
reading. It ends the current-reading role and clears To Read, and has no
vocabulary effect: finishing records history only (ADR 0136).
_Avoid_: Journey completion, deck completion, mastery.

**Previously read**:
A one-time assertion that the learner read a Book outside its current-reading
workflow. It records assertion time, not a historical reading date, and implies
no acquisition, analysis or vocabulary effect. Recording it from the Catalog
adds the Book as Read.
_Avoid_: reading completion, known vocabulary.

**Vocabulary transition** (retired):
Retired by ADR 0136: finishing a Book changes no vocabulary. See Reading
completion.
_Avoid_: mastery, deck review, vocabulary study.

**Graduation** (retired):
Retired by ADR 0136 with Known vocabulary. A word becomes Mature through graded
reviews, or at once through Already known, never because a Book was finished.
_Avoid_: completion, mastering, promotion.

**Analysis evidence** (book evidence state):
The classification of a Book's current acquired source against its analysis
standing, owned per Book and derived from raw acquisition and analysis signals
rather than stored by hand or persisted as learner state. It is the single
state every surface presents: not acquired, unavailable (content present but no
current revision), stale (a prior analysis no longer matches the current
content or its current extraction), analyzed, or acquired-but-unassessed
(current content present, analysis not yet complete). It reflects the Book's
published analysis: a newer analysis run that is still in progress, failed, or
was cancelled does not change it while the published analysis still matches
the current content.
_Avoid_: book status, analysis state (the raw signal the classification reads,
not the classification itself), evidence status.

**Source structure update**:
The learner-confirmed re-extraction of a Book's unchanged acquired content
under a newer extraction contract, offered only when that contract would
record different text, structural roles or a different structure convention. It adds a new extraction of the
same content and makes the Book's analysis evidence stale until reanalysis; it
never rewrites earlier extractions, analyses or Lemma corrections, which keep the
evidence they froze.
_Avoid_: re-import, reacquisition, migration, backfill.

**Analysis pipeline identity**:
The analyzer-side identity that produced an analysis: the analyzer's version and
selection configuration together with the language's normalization profile and
version reported by the running analysis service. A model or dependency change
alone does not change it; only a change that can alter a Book's evidence does.
_Avoid_: model version, pipeline generation, analyzer version (one part of it).

**Analysis update**:
The learner-confirmed reanalysis of a Book's unchanged content and extraction
under a newer Analysis pipeline identity, offered when the identity that
produced the Book's published analysis differs from the current one. For a
classical Book, the offer compares the prepared definition's source pin and
interpretation version with the current definition; a curatorial or
translation-pin change alone gives no offer. It is not stale evidence: the
published analysis stays in effect until the update publishes, and the update
never rewrites earlier analyses or Lemma corrections. Lemma corrections and Occurrence exclusions on unchanged
words carry over; the earlier analysis keeps its own.
_Avoid_: stale analysis, outdated evidence, re-import, migration.

**Vocabulary coverage band**:
A neutral label of a trustworthy analyzed To Read Book by its Mature
coverage, the share of its analyzable tokens whose effective vocabulary identity
is Mature in Study: at least 99%, at least 97% but below 99%, at least
95% but below 97%, or below 95%. Coverage counts word tokens by effective vocabulary identity, so
confirmed Lemma corrections apply, excluded occurrences remain in the
analyzable-token total, and an Idiom changes nothing. A lighter second figure
also counts words in study. It reads the Book's Browse
count projection, the same per-Book counts My Books uses, and shows Updating
instead of a band while that projection is not ready. It is evidence, not a
reading recommendation.
_Avoid_: accessibility, readiness, difficulty band.

**No vocabulary comparison**:
The chooser band for a successfully analyzed Book with no analyzable-token
denominator. It means neither zero coverage nor failed evidence.
_Avoid_: 0% coverage, Needs attention.

**Main text**:
The contiguous run of a Book's readable units that the EPUB structure declares to
be the body of the work: from a declared body-matter start to a declared
back-matter start, or to the end when none is declared. Absent a declared
body-matter start, the Book has no identified main text and its whole snapshot is
treated as main text. Recognised Distributor text is never part of it.
_Avoid_: body text, main matter (the retired classifier category).

**Distributor text**:
Text a distributor wraps around the work, such as a header, a licence, and
pages after the licence, recognised only from that distributor's published
packaging convention and never inferred from what the text says.
_Avoid_: boilerplate, front matter, ancillary text (which the EPUB declares).

**Ancillary text**:
The readable units of a Book that the EPUB structure declares to lie outside the
main text — front matter, and back matter such as a bibliography, index, or
glossary.
_Avoid_: boilerplate, main_matter.

**Source line**:
Text of a readable unit bounded by authorial line breaks or line-group breaks.
It is a fact of the source, not a line as wrapped on a screen, and it does not
assert that the text is verse.
_Avoid_: verse line (asserts a genre), display line.

**Line group**:
A run of source lines bounded by group breaks: a source block boundary or a
blank line. It may be a stanza, part of a speech, or a prose paragraph; the
source markup does not say which.
_Avoid_: stanza (not established by markup), paragraph (as a generic term).

**Structural role**:
A declared, typed function of source text: a **speaker label** or a **stage
direction**. It is recognized only from an accepted source markup convention
and is never inferred from the text itself; unrecognized markup has no role.
_Avoid_: speaker detection, drama mode.

**Sentence**:
The analyzer's linguistic segmentation of a readable unit's text. A sentence
may span several source lines and may share a source line with other
sentences, but it never crosses a line group. It is not itself a source line or
a line group.
_Avoid_: line, verse.

**Study context**:
The complete source sentence a learner studies for one occurrence, exactly as
it appears in the readable unit, including any authorial line breaks. It is
always one analyzer sentence within one line group; a source line, a line
group, or a larger passage is never the study context, and it is never
truncated, paraphrased, modernized, or padded with adjacent text. The same
complete sentence supplies the representative sentence, the contextual Gloss,
the translation, and the English target alignment.
_Avoid_: study excerpt, verse line, passage, snippet.

**Speaker attribution**:
The speaker label a study context is attributed to, derived from the nearest
preceding speaker label in the same readable unit and reset at a heading. It is
display metadata shown beside the study context: it is not part of the context's
source text, is never counted as vocabulary, translated, emphasised, or sent to
a provider, and is absent rather than guessed when no label precedes.
_Avoid_: speaker detection, dialogue tag, character id.

**Book language**:
A book's chosen language tag, or its absence recorded as an unknown-language
state. A chosen tag is stored in one canonical base form — lowercased, with `_`
as `-` and region subtags collapsed, so `de_DE` and `de-de` are the same as `de`.
Chosen book languages are the raw material from which a learner's study
languages are derived. A Book without a chosen language belongs to no study
language partition; a Catalog entry without a language is never offered.
_Avoid_: detected language, inferred language (nothing is ever inferred from
content).

**Catalog**:
The one surface where a learner finds and adds texts from every source: the
Classical catalog and each OPDS catalog connection. It shows whether an entry is
in My Books, never its disposition, analysis or preparation. Sources are shown
side by side, each presented alike, and never merged. It is not a per-language
surface: its language filter may offer languages the learner does not study.
_Avoid_: Catalogs (the retired plural label), library, Browse (the vocabulary
view).

**Catalog index**:
A connection's local, synced mirror of the Catalog entries its OPDS catalog
offers within the Catalog sync scope. It follows upstream, including removals,
and is not learner state.
_Avoid_: My Books, cache, feed.

**Catalog entry**:
An offering from a learner-owned catalog, identified by the catalog
connection plus that connection's stable entry identifier. It is not a Book:
adding it creates the learner's Book, or restores the one it had. A Book may
carry one catalog-entry alias per connection in the current OPDS flow; two
connections may each offer the same entry identifier without identifying the
same offering.
_Avoid_: source identifier on its own, OPDS entry (as a standalone identity),
Book.

**Catalog file**:
The EPUB exactly as a Catalog entry's acquisition link serves it. Mouseion
streams it to a temporary file and derives the Retained EPUB from it; it is not
stored. Its digest is kept in processing history (ADR 0109).
_Avoid_: original EPUB, retained bytes.

**Retained EPUB**:
The EPUB Mouseion stores for a content revision: the Catalog file without the
image, font, audio and video resources Mouseion never reads, or the Catalog file
itself when it has none. Extraction, structure checks and the content digest
all use it (ADR 0109).
_Avoid_: slim EPUB, stripped EPUB, original bytes.

**Work**:
The intellectual composition that groups its particular textual versions,
independent of a learner's Books or the sources offering it. Titles and authors
are descriptive metadata, not proof that two descriptions identify the same Work.
_Avoid_: Book (the learner-owned identity), Edition (a particular version).

**Edition**:
A particular textual version of a Work, distinct from the repository or producer
that supplies its text or annotations. Different Editions remain distinct even
when their text overlaps or their identifiers appear to agree.
_Avoid_: Work, annotation release.

**Reading unit**:
The defined extent of one Edition that a Book represents. For a Work divided
into Constituent books it is the whole Work in that Edition; a learner never
holds a Book for one constituent book. Its extent is stable across source
revisions and is not defined by whichever annotation fragments are available.
A sentence belongs to the Reading unit that holds its first word, so each
sentence of an Edition belongs to exactly one of its Reading units.
_Avoid_: readable unit (an extracted structural unit), annotation coverage.

**Opening**:
The first sentence of a Reading unit that is not a speaker label, exactly as
its Edition prints it and as the learner will read it. A Work's Opening is that
of its first Reading unit. It is derived from the source pin and the citation
range, so it never raises the Definition revision by itself. It identifies a
text by its words and says nothing about difficulty.
_Avoid_: incipit, first line, excerpt, preview.

**Constituent book**:
A Work's conventional top-level division that its citation scheme names a book,
such as Anabasis 1–7 or Iliad 1–24. It is not a chapter, a page, or a line, even
when one of those is the first citation level. It is a division inside a
learner's Book, located by citation, and never a Book or a Reading unit of its
own: a learner owns, studies the whole Work, not one of its books.
_Avoid_: Book (the learner-owned identity), volume, chapter.

**Citation**:
A position in a Work's own reference system, written as dotted components without
leading zeros (Anabasis `1.2.3`, Livy `1.pr.1`, `331a`). A component is a number
with an optional letter suffix or a named section from a reviewed list (`pr`,
`ep`); a name outside the list is refused. A preface section orders before the
numbers and an epilogue section after them, and a suffixed number follows its
bare number (`331` before `331a`). It runs from the Work's top-level divisions
down to the finest level its chosen Edition numbers. A page, a print line or a
heading is never a citation.
_Avoid_: line number, page reference.

**Reading-unit definition**:
A reviewed, keyed description of one Reading unit: its Work, Edition, citation
range, source pin, optional translation pin and interpretation version. It ships
with Mouseion, keeps its key permanently, and takes a new revision whenever a
content-bearing part changes.
_Avoid_: manifest entry, shelf item, catalog entry (an OPDS offering).

**Classical catalog**:
The installation-wide collection of Reading-unit definitions a learner can add
to My Books. They are curated, not learner-authored; adding one gives the
learner at most one Book for its Reading unit, which keeps that link when the
definition takes a new revision.
_Avoid_: shelf (a Book workflow bucket synonym), Browse (the vocabulary view),
library.

**Definition revision**:
A monotonic number on a Reading-unit definition that increases when a
content-bearing part changes: its source pin, translation pin, or interpretation
version. Curatorial corrections do not increase it, and a changed citation range
is a new Reading unit rather than a revision.
_Avoid_: version (the interpretation version is a separate fact), update.

**Withdrawn definition**:
A Reading-unit definition removed from the Classical catalog, kept as a
tombstone so existing Books never dangle. It is hidden from browsing and cannot
be added or prepared; a Book already prepared from it stays readable.
_Avoid_: deleted definition, removed Book.

**Book preparation**:
The durable work that makes a To Read Book readable. It has two kinds, chosen
by where the Book came from: Classical preparation and EPUB preparation. Every
Book preparation is described by the same lifecycle state (queued, running,
completed, failed or cancelled), the stage it has reached, and, when it fails, a
stable code.
_Avoid_: job (the queue's unit, not the learner's), import, processing.

**EPUB preparation**:
The Book preparation of a Book added from a Catalog entry: it acquires the
entry's Catalog file, then analyzes it. EPUB acquisition is its first stage, not
a separate operation.
_Avoid_: EPUB analysis (only its second half), acquisition (when the whole
preparation is meant).

**Classical preparation**:
The Book preparation of a classical Book, a durable, serial import that makes
its Reading unit readable: it acquires and verifies the pinned work file, cuts the unit, publishes an
ordinary analysis, and records the definition it used. It is triggered by To
Read, runs on `classical_preparation`, and never calls the NLP service.
_Avoid_: analysis job (when the classical path is meant), EPUB analysis.

**Book preparation failure**:
The per-Book evidence of why a Book preparation last failed before any analysis
of the Book began, classified by the stage that failed and a stable code. It has
two kinds, a Classical preparation failure and an EPUB acquisition failure, and
both use the same stages and codes wherever the meaning matches. A failure
during analysis is evidence of the analysis itself, not a Book preparation
failure.
_Avoid_: job error, download error.

**Classical preparation failure**:
The Book preparation failure of a classical Book: the per-Book evidence of one
settled preparation attempt, classified by the stage that failed: fetch, import,
a refused source, the catalog precondition, or a withdrawn definition. It is recorded per Book, never per pinned file, so one
Book's failure does not fail its siblings.
_Avoid_: pin failure, download cache failure.

**EPUB acquisition failure**:
The Book preparation failure of a Book added from a Catalog entry: the per-Book
evidence of the latest failed attempt to fetch and import its Catalog file,
classified by the stage that failed: the entry, fetch, a refused source, or
import. It is recorded per Book and cleared when an
acquisition succeeds.
_Avoid_: download error, acquisition flash.

**Annotation occurrence**:
A text-attested linguistic occurrence distinguished within one exact analysis,
even when it shares a source span with another occurrence. An unprinted syntax
node or multiword-token container is not an additional vocabulary occurrence.
_Avoid_: source span (its textual anchor), native token ID (source-scoped evidence).

**Book**:
A learner's bibliographic identity for a defined Reading unit, owned per learner
and stable across acquisition, analysis, and content revisions. A Book is
addressed by its owner-scoped Book ID; its current acquired source and analysis
evidence are resolved underneath that identity. A Book comes into being only
when the learner adds a Catalog entry or Reading unit, or records it as
Previously read; a learner never creates one by manual entry, and listing or
syncing an entry never creates one. Removing a Book from My Books ends its
membership but not the Book, and adding the same entry again restores it.
Analysis is Book-level evidence, not membership state.
_Avoid_: source material (the acquired evidence, not the identity), acquired
book.

**Book cover**:
The catalog-supplied image representing a Catalog entry and any Book added from
it, retained with that entry and allowed to change as its metadata changes. It
supports the title and author but is never the sole learner-facing identity.
_Avoid_: cover art, thumbnail (a presentation size, not the Book metadata).

**Title plate**:
The image Mouseion composes from a text's own title and author where no Book
cover exists, such as for every Classical catalog Reading unit. It is
presentation, not Book metadata: it is never retained, never supplied by a
catalog, and never called a cover.
_Avoid_: generated cover, placeholder cover, cover (when no catalog supplied one).

**Book disposition**:
Whether a learner's Book is To Read. Adding a Book or reading it again makes it
To Read; finishing it or marking it Previously read clears it. It follows the Book across language corrections and is independent of analysis
and reading history; the current Book remains To Read underneath.
_Avoid_: book status, reading state, workflow state.

**Hidden (Book visibility)** (retired):
Retired by ADR 0117; removing the Book from My Books replaces it. The code no
longer reads or writes the former owner-and-Book visibility choice: no surface
hides a Book, Hide/Unhide and Show hidden books are gone, and their routes are
unsupported. The `book_visibility` rows are retained for a separate, reviewed
data cleanup, and a Book with a stored row is an ordinary Book.
_Avoid_: archived, deleted, set aside.

**Book workflow bucket**:
The one visible My Books placement of a Book, derived in precedence order from
Current reading, To Read, then Read. Every Book in My Books has one. The Current
reading is pinned at the top of My Books, and is also listed under To Read,
where it keeps its distinct Currently reading label.
_Avoid_: Book disposition, shelf.

**My Books**:
The learner's chosen Books, on one page per study language: the Current reading
pinned at the top with its progress toward Target coverage, then To Read, then
Read. It owns Read next, Finish, End, To Read, Read again, Mark as previously
read and Remove, but not finding texts (Catalog). A Book that needs action
shows a status line, and a filter shows only those Books. To Read is in title
order unless the learner chooses the optional **most readable** sort by Mature
coverage, which the address keeps.
_Avoid_: library queue, reading plan.

**Reading** (retired as a page):
Retired by ADR 0136: My Books holds what the Reading page held. The old Reading
addresses redirect to My Books, or to the Book detail page when they name a
Book, and the shell no longer lists Reading. "Current reading" stays as the role
a Book has. A classical Book is a whole Work in one Edition, so it is read,
finished and chosen as one unit; its Constituent books are locations within it,
never entries of their own. My Books keeps no learner-authored order.
_Avoid_: Reading Journey, Primary Goal, reading queue.

**Inbox** (retired):
Retired by ADR 0117: a Book does not exist before the learner chooses it, so
nothing is untriaged. Unread Inbox Books left My Books through the backfill and
their entries stay in the Catalog. The stored `inbox` disposition value now
means only "not To Read"; it has no visible bucket. A Book marked previously
read, and a Book whose reading was finished, appears in Read.
_Avoid_: unprocessed queue, notification.

**To Read**:
The disposition expressing that a Book is a possible future reading, with
acquisition and analysis intent but no current-reading commitment.
_Avoid_: Journey member, backlog, queue.

**Set Aside** (retired):
A former disposition for a Book removed from ordinary consideration. ADR 0086
retired it as a value and action: requests are rejected, not translated into
Hidden or End. The one production legacy row was mapped to Inbox by hand.
_Avoid_: current disposition, archived, rejected, abandoned.

**Current reading**:
The analyzed To Read Book the learner has committed to reading in a study
language, at most one per language. It puts its words first among new words in
Study until it reaches Target coverage. A Book is eligible to become the Current
reading only when it is To Read, has a chosen Book language, and its Analysis
evidence is analyzed; an ineligible Book has one reason: not To Read, no chosen
language, missing current content, analysis in progress, failed or cancelled
analysis, stale analysis, or no completed analysis.
_Avoid_: Primary Goal, reading status, current project.

**End current reading**:
Clears the Book's current-reading role without a completion. The Book stays To
Read, keeping its history. It needs no confirmation. It is not a pause: nothing is paused, and inactivity
never ends a reading.
_Avoid_: pause, stop, set aside.

**Read next**:
The one action that makes an analysed To Read Book with a Book language the
Current reading. It starts a reading or, when another Book is current, switches
to this one and leaves the former Book in To Read. It takes no confirmation and
freezes nothing. It is refused, with its reason, when the Book is not To Read, not
analysed or has no Book language.
_Avoid_: Start, Switch (the retired two-step actions), commit.

**Book detail**:
The page for one Book in My Books, in the Book's own study language, at
`/books/{id}`. It hosts the Book's analysis actions (retry, Analysis update and
Source structure update), shows its Book language, and lists the Book's words
as Mature, in study or not yet introduced, read-only, each linking to its
Concordance. It edits no vocabulary: fixes start only from a Study report.
_Avoid_: working desk (the retired Reading surface), Reading page.

**Reading history**:
Append-only, Book-anchored reading completions and previously-read assertions.
History remains even when a Book returns to To Read for rereading.
_Avoid_: Read disposition, completed list, Journey history.

**Study**:
The learner's in-Mouseion review of vocabulary from their reading, on one card
surface: Today's due reviews, then New-word batches, then Fluency. It is one
continuous stream per study language, drawn from the Corpus, and it is the only
way the learner studies vocabulary. The Current reading and the To Read Books
only set which new words come first: starting, switching, ending or finishing a
Book changes nothing else in Study. It is separate from Sentence study, which
studies one sentence's grammar.
_Avoid_: review deck, practice, SRS, vocabulary study, Explore (a retired idea
for browsing upcoming words).

**Corpus**:
The analysed Books in My Books in one study language. Study draws Study
material and Fluency contexts from all of it, Read Books included, and
Concordance searches it. Adding a Book from the Catalog is how a learner
includes it; only its To Read Books set the priority of new words.
_Avoid_: library, analysed library, collection.

**Target coverage**:
The coverage, counting Mature and in-study words, up to which the Current
reading's words come first among new words. It is one fixed value for every
language, 98%. Once the Book reaches it, frequency across the To Read Books
decides, until the Book falls below it again.
_Avoid_: readiness, goal, threshold (alone).

**Already known**:
An answer the learner can give when Study introduces a word. The word enters
study already Mature, and from then on is an ordinary word: a lapse ends its
maturity. It is an answer, not a status, and nothing else marks a word as
known.
_Avoid_: Known (a retired fact), skip, ignore, mark as known.

**Study day**:
The learner's day for Study. It begins at a fixed early-morning hour rather
than at midnight, so late-night reviews count towards the day just ending.
_Avoid_: calendar day, session.

**Today**:
The graded part of Study with a daily finish line: it is finished once the
reviews due at the start of the Study day are answered. New words are offered
in New-word batches afterwards and are never part of the finish line. Once
finished, Today stays finished until the next Study day.
_Avoid_: session, queue, daily deck.

**New-word batch**:
A fixed-size group of new words that Today offers after its reviews and the
learner may take or decline. Taking one introduces its words. A daily cap
limits how many are offered, and fewer, or none, are offered when the
forecast review load is high.
_Avoid_: daily new allowance, new-card limit, quota.

**Study item**:
What Study introduces and reviews: a word (a vocabulary identity) or an Idiom,
which is identified by its study language and dictionary headword. Each Study
item has its own Study progress and Learned meaning, and takes one place in a
New-word batch. Neither kind needs the other: an Idiom is introduced whether or
not its member words are in study. "Word" means a vocabulary identity only, so
coverage, which counts word tokens, is unchanged by Idioms.
_Avoid_: learning item, card, entry, word (for an Idiom).

**In study**:
A word Today has introduced, which therefore has Study progress. A word stays
in study whichever Book is the Current reading, and whichever Books are in My
Books: removing a Book never costs Study progress.
_Avoid_: learning, active card, enrolled, learning item.

**Study progress**:
A learner's graded review record for one vocabulary identity in one study
language. It belongs to neither a Book nor a sentence, so a new sentence or a
change of Current reading does not reset it. It is the only record of what the
learner knows. Only Today's reviews, and an Already-known answer when a word is
introduced, change it.
_Avoid_: mastery, card state, Known.

**Mature**:
A word in study whose current review interval is at least 21 days. A lapse
that shortens the interval below that ends maturity until the word earns it
again. It is the one claim of knowledge: coverage counts Mature words.
_Avoid_: known, mastered, graduated.

**Study material**:
What Today needs in order to present a word: the one complete study context
that introduces it and carries every one of its reviews, together with that
context's Gloss and translation. It is prepared one word at a time, shortly
before Today needs it, from current evidence with the learner's Lemma
corrections and Occurrence exclusions applied.
_Avoid_: card, deck.

**Learned meaning**:
The Gloss a word showed when Today introduced it. Fluency contexts must agree
with it. In a language that uses source-evidence cards, the Gloss comes from
the Evidence-assisted provider pass and must cite at least one sense of the
language's dictionary; it records the senses it cites. If a confirmed correction shows that the introducing context was not
this word, the next review re-introduces the word with valid Study material and
re-anchors its learned meaning, keeping its Study progress.
_Avoid_: sense (a dictionary entry's meaning), pedagogical sense, definition.

**Study report**:
A learner's report, made in Today, that a word's Study material is wrong. It
is one of two kinds:

- **Wrong word**: the occurrence in the study context belongs to a different
  vocabulary identity, such as `kam … umhin` taken as `kommen` rather than
  `umhinkommen`. The word is held: it gets no reviews, is not part of Today's
  finish line, and does not fall overdue, and it keeps its Study progress. The
  report ends when the learner confirms a Lemma correction or Occurrence
  exclusion on that occurrence, when the learner dismisses it, or when an
  Analysis update gives that occurrence a different effective vocabulary
  identity. It opens a remediation screen that previews and confirms the
  correction, and lists the other occurrences in the Corpus with the same
  surface and source lemma, none ticked, for the learner to correct too. It
  changes no vocabulary identity itself.
- **Unsuitable context**: the identity is right, but the study context does not
  teach the word's own meaning, for example because it is part of an Idiom
  (`immer` in *immer wieder*) or because its Gloss or translation is wrong. The
  word is not held. That sentence is never used again as this word's Study
  material or Fluency context, and the word is re-introduced from another
  context.

_Avoid_: flag, suspend, bury, bug report.

**Fluency**:
The ungraded, open-ended part of Study after Today's reviews and new words,
which cycles mature words through Fluency contexts. Nothing in it changes Study
progress; the learner's "didn't recognise it" responses are kept as evidence.
_Avoid_: Explore, free review, cram, extra reviews, Fluency Development.

**Fluency context**:
A further study context of a mature word, beyond its Study material, in which
the word carries its learned meaning, preferably in a different form. It has
its own Gloss check and translation.
_Avoid_: example sentence, variant card, context variant, alternative sentence.

**Concordance**:
A listing of a word's (or lemma's) occurrences with their surrounding context,
at the scope of a Book or of a study language's Corpus. Context
covers both the linear text around each occurrence and the occurrence's
syntactic role from dependency parsing. It lists every occurrence, including
ones that are not vocabulary, such as proper nouns and separable particles. A
lemma query applies confirmed Lemma corrections, and excluded occurrences are
hidden from every word query. A Construction query lists the parallels of one
Construction, reached from Sentence study; it ignores corrections and
exclusions unless it is narrowed to an anchor lemma.
_Avoid_: KWIC (a rendering style, not the feature), occurrence list.

**Sentence study**:
The learner's study of one complete study context, with each word's analyzed
morphology and, where the analysis has syntax, its dependency relation and head.
It is where construction-level help belongs: where the analysis supports them,
it names the sentence's Constructions, each with its Concept note.
_Avoid_: sentence detail, parse view.

**Dependency relation**:
The syntactic function a token (the dependent) fulfils relative to its head
in a sentence's dependency parse, for example `nsubj` (subject), `obj`, or
`advcl` in Universal Dependencies, or `SBJ`, `OBJ`, `ADV` in an AGDT
(`native_agdt`) analysis; the analysis's syntax state says which scheme it
uses. One of the dimensions of a sentence's surrounding context; what makes
grammar-aware concordance queries and syntactic sentence scoring possible.
_Avoid_: parse tag, syntax label.

**Head** (governor):
The token a dependent attaches to in a dependency relation. The root of a
sentence is its own head. Headedness is part of an occurrence's persisted
context, so an occurrence can be located under its governor or its dependents.
_Avoid_: parent (implementation flavoured), controller.

**Dependent**:
A token whose dependency relation attaches it to a head. A queried lemma's
dependents in a specific relation are what a grammar query returns.
_Avoid_: child, argument.

**Unprinted syntax node**:
A node of a sentence's syntax tree with no printed word, such as an AGDT
ellipsis node standing for an understood verb. It keeps its relation, head and
tree position, and a printed word may depend on it, but it has no form, lemma,
morphology or vocabulary identity and is never an Annotation occurrence.
Learner-facing text calls it "an unprinted word" and never guesses what it is.
_Avoid_: ellipsis token, empty token, elided word (for the stored node).

**Construction**:
A syntactic pattern of a study language, such as the genitive absolute or a
result clause, that Mouseion identifies from an analysis's syntax and names in
Sentence study. It is defined per language over the analysis's syntax scheme
and has a stable construction id. It is not an Idiom: an Idiom is a lexical
multiword unit with its own dictionary entry, while a Construction is a
grammatical shape any words can fill.
_Avoid_: grammar point, pattern (alone), idiom, phrase.

**Construction occurrence**:
One Construction found in one exact analysis: its Construction anchor and its
Construction extent, with an optional Construction subtype. Occurrences may
nest and overlap; one anchor may carry several different Constructions, never
the same one twice. It is a derived reading of the analysis's frozen parse, not
evidence: it can change when the Construction detector version changes, and no
card or count depends on it.
_Avoid_: construction match, detected construction, hit (outside measurement).

**Construction anchor**:
The one Annotation occurrence that signals a Construction occurrence, such as
the conjunction of a conditional clause or the participle of a genitive
absolute. It is always a printed word, never an Unprinted syntax node.
Concordance centres a parallel on it, and a lemma filter applies to it.
_Avoid_: head (a dependency term), trigger, keyword.

**Construction extent**:
The set of printed words that belong to a Construction occurrence: the
anchor's subtree, read through the syntax scheme's adapter. It may be
discontinuous, so it is a set of words, not a span. Unprinted syntax nodes
inside it are not members.
_Avoid_: span, range, scope.

**Construction subtype**:
An attribute of one Construction that distinguishes its forms, such as natural
or actual result, or the case of an articular infinitive's article. A subtype
is not a separate Construction. One that cannot be determined is omitted, and
the Construction is still shown.
_Avoid_: construction type (for the attribute), variant.

**Subtype label**:
A Construction subtype that has cleared the shipping bar on its own and is
therefore shown to the learner, for example "natural result". An occurrence
whose subtype is not a Subtype label shows the Construction's name with no
subtype.
_Avoid_: hedged label, tentative subtype.

**Construction detector version**:
One integer per syntax scheme that identifies how Mouseion reads Constructions
from a parse: the scheme adapter's normalisation, the detectors, their
reviewed verb lists and the shipping gate that decides which Constructions and
Subtype labels are on. Changing any of them bumps it and rebuilds every
analysis's Construction occurrences without learner confirmation; until an
analysis is rebuilt it shows no Constructions. Where Mouseion is not confident
it abstains silently, with no hedged labels and no "none found" statement.
_Avoid_: detector model version, construction analysis version.

**Grammar reference**:
The printed reference grammar a study language's Constructions cite, with a
stable § numbering Mouseion can link to: Smyth 1920 for Ancient Greek, Allen &
Greenough for Latin. Mouseion shows the citation ("Smyth 1920 §2070") and
links to it, and never reproduces its wording.
_Avoid_: Smyth (for the general concept), grammar source.

**Concept note**:
Mouseion's own reviewed description of one Construction in one language: a
learner-facing name, a one- or two-sentence definition and one Grammar
reference citation. It is presentation looked up when the page renders, not
analysis evidence, so editing it changes no analysis.
_Avoid_: grammar entry, Smyth note, definition (alone).

**Full lemma** (compound lexeme):
The reattached dictionary lemma of a separable verb — `aufstehen` rather than
the analyzer's base `stehen` — formed by joining the separable particle to the
verb's lemma. For German, the full lemma is the vocabulary identity; the
analyzer's base form is preserved as the raw lemma.
_Avoid_: compound lemma (ambiguous with lexical compounding), prefixed lemma.

**Lemma**:
The citation form an analyzer assigns to a token (for example `stehen`), the
raw input from which a language-specific normalization profile derives the
canonical lemma. Not to be confused with the full lemma of a separable verb.
_Avoid_: root (morphology), base form (ambiguous).

**Effective vocabulary identity**:
The language, canonical lemma, part-of-speech and lexical discriminator
attributed to an analyzed occurrence in learner-facing vocabulary after any
confirmed correction. It can differ from the preserved analyzer assignment; an
excluded occurrence contributes none.
_Avoid_: edited analyzer lemma, global corrected lemma.

**Lexical discriminator**:
A scheme-qualified string that keeps two vocabulary identities with the same
language, canonical lemma and part of speech apart, such as `morpheus:2`. It is
empty for vocabulary with no source discriminator. A learner correction never
sets or clears it.
_Avoid_: homograph id, sense id.

**Citation headword**:
The learner-facing citation form of a vocabulary identity: for an imported
classical text, the source lemma with any trailing homograph number removed
(for example δέω for δέω2). Learner-facing views show the headword, while the
stored canonical lemma is the language profile's key.
_Avoid_: canonical lemma, dictionary key.

**Bare lemma with a numbered sibling**:
An imported classical vocabulary identity whose source lemma carries no
homograph number while its work numbers another form of the same lemma. It stays
a separate identity, and learner-facing views label it "the source gives no
homograph number" so the learner knows why it is listed apart from the numbered
form.
_Avoid_: unnumbered homograph, null discriminator.

**Imported analysis**:
Reading a pinned pre-annotated source file through one bounded, checked adapter
and writing it as an ordinary analysis run, without the NLP service. The run
records its versioned interpretation and its syntax state: `ud`, `native_agdt`
or `absent`. The syntax state names the scheme of every relation and head in the
run. Consumers show "Syntax unavailable" for an `absent` run. A `native_agdt`
run stores the source's AGDT relations and heads, with Unprinted syntax nodes;
generic UD consumers treat it as having no UD syntax, and its relations reach
the learner only through Constructions and the plain-English Word relations in
Sentence study.
_Avoid_: NLP analysis, corpus sync.

**Provider payload assessment**:
The reviewed, recorded disposition for sending one language's exact
external-provider payload (the Evidence-assisted provider pass, the LSJ sense
summary, or the Idiom card) to the configured provider for personal use. An
assessment names the payload kind it covers. It is compiled reviewed data,
pinned to a provider, prompt contract and transmitted field set. A payload whose
assessment is unrecorded, or whose configured provider, prompt or fields no
longer match the record, fails closed before any request is transmitted. An
assessed payload with no recorded assessment for its kind and language also
fails closed; a payload that is not assessed does not consult the gate.
_Avoid_: consent flag, provider allowlist.

**Vocabulary Browse inventory**:
The collection of effective vocabulary identities evidenced by one analysed
Book, including single occurrences, each shown as Mature, in study or not yet
introduced. It is the read-only word list of the Book detail page, and is
distinct from the filtered Browse results.
_Avoid_: prepared-deck candidates, known-vocabulary list.

**Browse selection** (retired):
An owner- and study-language-scoped unnamed selection of effective vocabulary
identities that Vocabulary Browse once kept before creating a Custom deck. ADR
0085 retired it as a value and action: Browse shows no selection controls, and
the selection is neither read nor written. Stored rows may remain until a schema
cleanup is approved under ADR 0038 and ADR 0070.
_Avoid_: custom deck, prepared deck, temporary filter result.

**Lemma review** (retired):
Retired by ADR 0136: the Book-scoped queue is gone, and so is the lemma-risk
assessment. A wrong vocabulary identity is fixed only from a Wrong-word Study
report, on its remediation screen, with the existing preview-and-confirm step
for a **Lemma correction** or **Occurrence exclusion**.
_Avoid_: lemma fix, lemma audit, dictionary check.

**Lemma review flag** (retired):
Retired by ADR 0136: no occurrence is flagged in advance; the learner finds a
wrong word in Study and reports it.
_Avoid_: invalid lemma, unconfirmed lemma.

**Lemma suggestion** (retired):
Retired by ADR 0136 with Lemma review flags: nothing proposes a lemma in advance,
and the learner decides on the remediation screen.
_Avoid_: automatic correction, LLM correction.

**Lemma correction**:
A learner-confirmed alternative canonical lemma for one occurrence in a Book's
exact analysis, governing its effective vocabulary identity (and so Study and
coverage) while preserving analyzer evidence. It need not have a dictionary entry, can be made without a
flag, and is never applied globally. The only automatic step across analyses
is an Analysis update copying it onto the same word (same source span, surface
and source lemma) in the new analysis.
_Avoid_: spelling correction, dictionary correction, global lemma override.

**Occurrence exclusion**:
A learner-confirmed choice to omit one analyzed occurrence from vocabulary
selection while retaining it in the Book's source and analysis evidence. It
does not remove it from analyzable-token coverage or exclude other occurrences of
that lemma.
_Avoid_: delete token, ignore lemma.

**Collocation**:
Words that co-occur with a queried lemma within a limited span (typically the
same sentence), counted across an analyzed Book or study language. A
concordance enhancement considered for a later milestone, not part of the
persistence foundation.
_Avoid_: co-occurrence (fine in prose), n-gram.

**Knock-out criterion**:
A sentence-quality rule whose failure rejects a candidate sentence outright,
regardless of its gradual score — for example an incomplete boundary, or no
finite verb and subject. Distinct from the gradual criteria that only rank.
_Avoid_: hard rule, gate reason.

**Representative sentence**:
The sentence chosen for a lemma's recognition card: among accepted candidate
sentences, one whose target is not part of an idiom candidate leads when the
language's idiom-card switch is on; otherwise the highest-scoring accepted
candidate sentence, with source order breaking ties. It is always the learner's
complete source sentence — the occurrence's study context — never truncated or
paraphrased.
_Avoid_: example sentence (ambiguous with the historical
`example_sentences` store), best sentence.

**Sentence quality**:
The deterministic, explainable score a candidate sentence receives as a
potential representative, computed at export time from its tokens and dependency
structure (GDEX-informed). It has two parts: the knock-out gate (accept or
reject) and the gradual score (ranking among accepted candidates).
_Avoid_: sentence score (fine in prose), readability.

**Dictionary index**:
The local, read-only collection of dictionary entries (Wiktionary-derived,
LSJ-derived for Ancient Greek, and Lewis & Short-derived for Latin) used as
meaning evidence and dictionary-form evidence for cards. It is not learner or Book state.
_Avoid_: dictionary database, lexicon service.

**Sense**:
A distinct meaning of a lemma within a dictionary entry. It may have a
source-provided gloss and other identifying context. A sense is evidence for a
card's meaning, not necessarily the wording shown on the card.
_Avoid_: definition (the full native-language explanation, a separate deferred
field), translation.

**Gloss**:
The short English equivalent of the target lemma, in the sense its
representative sentence uses, rendered on the card back. It is in citation
form: verbs as "to …", nouns singular without an article, adjectives in their
base form. It is the common equivalent of the lemma unless the sentence clearly
uses a different or narrower sense. It is never a definition-style paraphrase
and never carries the sentence's inflection, number or arguments. It expresses
one meaning, not a list of senses; one to three words or close comma-separated
synonyms are preferred, with a longer phrase only where a short cue would
mislead. It is distinct from the English translation of the whole sentence.
_Avoid_: definition, translation (the contextual whole-sentence field).

**Dictionary gloss**:
A source-provided English explanation of a dictionary meaning, used as evidence
for a card's contextual Gloss rather than necessarily displayed verbatim.
_Avoid_: local gloss, definition.

**Meaning evidence**:
Source-attributed dictionary senses relevant to a target in its representative
sentence: Wiktionary for modern languages, LSJ for Ancient Greek, Lewis & Short for
Latin. Meaning evidence informs the contextual Gloss without dictating its wording; the
sentence itself may support a context-only gloss when no sense fits.
_Avoid_: card Gloss, undifferentiated definitions.

**Dictionary entry display**:
The target's dictionary entries, every one the lookup returns, in source order,
each with its whole sense hierarchy, shown on a card when no contextual Gloss is
supported. It abstains from choosing an entry or a sense rather than ranking or
truncating.
_Avoid_: definition, first sense, Gloss.

**Sense summary**:
A provider-written, context-free condensation of one dictionary entry, each part
citing the entry's senses. It is shown above the entry it condenses, never
instead of it, and never chooses a sense for the representative sentence.
_Avoid_: Gloss, contextual meaning, definition.

**Fallback gloss**:
A historical label for a card's English meaning cue supplied when dictionary
evidence was absent or unsuitable for the representative sentence. It named the
fallback path, not a separate kind of learner-facing meaning.
_Avoid_: definition, Gloss (when referring to the historical fallback path).

**Analysis resource**:
A third-party layer from which the learner receives only derived linguistic
labels about a text: sentence boundaries, lemmas, parts of speech, morphology or
syntax. Models, embeddings, character models, treebanks and lemma tables are
typical. Classification is per layer, so one artifact can hold both an Analysis
resource and Source content.
_Avoid_: tool, data, asset, model (for the general category).

**Source content**:
A third-party layer whose own expression reaches the learner, either shown in
Mouseion: texts, translations, dictionary
definitions.
_Avoid_: data, asset, corpus (for the general category).
