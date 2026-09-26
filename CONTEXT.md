# Mouseion

Mouseion is a self-hosted reading environment for learning foreign languages.
A learner connects their own OPDS catalog, syncs books into a personal
library, analyzes each Book according to its declared EPUB structure, and prepares
Anki recognition decks from unknown vocabulary.

The current disposition and current-reading contract is recorded in
[ADR 0078](doc/adr/0078-book-dispositions-and-current-reading.md).
[ADR 0072](doc/adr/0072-goal-owned-vocabulary-and-journey-forecast.md)
records the historical Goal and ordered-Journey model.

## Language

**Study language**:
A language the learner studies. The set is inferred from the languages present
in the learner's library (books with a chosen language), never configured by
the learner; it defines the languages for which the learner can import known
vocabulary.
_Avoid_: configured language, preferred language, library language (as a stored
selection defining the set).

**Active study language**:
The one language the learner is currently working in. It is a stored selection
pointing into the derived study-language set — context, not configuration: it
chooses which study language the per-language surfaces (My Books browse,
Reading, Vocabulary) present, and never defines which languages are
studied. When the set is unambiguous the selection defaults deterministically;
if the selection leaves the set, it resets. There is no "no language" choice:
absence of an active study language is never a learner selection, only a
transient result of defaulting (an ambiguous set with no history) or reset (the
selection left the set with no remaining candidate).
_Avoid_: current language, mode, active profile.

**Catalog sync scope**:
The set of non-English languages that a connected catalog offers and the NLP
service reports ready. The sync walks these feeds regardless of learner
configuration and tags each book with the walked language.
_Avoid_: study-language scope, synced languages.

**Known vocabulary**:
Words the learner already knows in a language, owned per learner and language,
populated by explicit lemma import or acceptance of a completed current reading's
frozen vocabulary snapshot. This is modeled learner knowledge, not a claim of
verified mastery; card generation never marks vocabulary as known.
_Avoid_: known words, learned vocabulary.

**Recurring vocabulary**:
Unknown lemmas appearing at least N times in an analyzed book; the pool a
prepared deck or current-reading snapshot selects. N is a selection parameter
with a default of three, and selection makes no coverage claim. Current Known
vocabulary and Reserved vocabulary in the same study language are excluded;
generated provenance is not an exclusion state.
_Avoid_: frequent words, deck coverage.

**Reserved vocabulary**:
The frozen recurring-vocabulary snapshot held by a current reading in its study
language, neither counted as Known nor available for selection while active.
Stopping, switching, setting aside, or finishing releases the reservation.
_Avoid_: active-campaign vocabulary, Goal vocabulary, known vocabulary.

**Current-reading vocabulary snapshot**:
The immutable recurring-vocabulary identity set frozen from the Book's exact
source and current analysis when reading starts. Later evidence and deck changes
cannot mutate it; rereading freezes a new snapshot.
_Avoid_: deck contents, generated vocabulary, study snapshot.

**Generated vocabulary**:
Immutable provenance that an identity was included in a prepared deck, including
its Book/deck origin. Generation is neither Known nor Reserved and does not by
itself exclude an identity from a later current reading.
_Avoid_: assigned knowledge, known vocabulary, reserved vocabulary.

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

**Prepared deck**:
An Anki recognition deck built asynchronously from the recurring vocabulary of
one exact completed analysis of a Book and that analysis's EPUB snapshot. The
ready deck is downloaded and studied in the learner's own Anki; preparation
does not mark vocabulary Known. Reading completion owns the vocabulary
transition; a current reading with an empty snapshot requires no deck.
_Avoid_: study plan, in-app review deck.

**Deck specification**:
The frozen, presentation-independent data of a prepared deck: the selected
vocabulary, its representative sentence and target, and the syntax, morphology,
and dictionary forms needed to present it, together with the identity of any
exact translation. A deck is a presentation of one specification; changing how
a deck is presented does not change its specification.
_Avoid_: manifest (the storage of it), deck content.

**Card presentation**:
The rules that turn a deck specification into learner-facing Anki notes: which
components of the target are emphasised, the headword line, the note's field
order, and the Anki model, template, and styling.
_Avoid_: rendering, formatting.

**Presentation version**:
Identifies the card-presentation rules a deck was built with, so a deck that
presents an older version can be recognised and rebuilt.
_Avoid_: template version, model version.

**Render-input version**:
Identifies the set of inputs a deck specification froze, so presentation can
tell whether a specification carries everything it needs or must be prepared
again.
_Avoid_: schema version, manifest version.

**Deck revision**:
A built artifact of one deck specification at one presentation version. A newer
presentation version produces a newer revision of the same specification
without re-selecting, re-analyzing, or re-translating.
_Avoid_: version (ambiguous with presentation version), regeneration.

**Reading completion**:
An append-only, Book-anchored fact recorded when the learner finishes a current
reading. It accepts eligible identities from that reading's frozen snapshot into
modeled Known vocabulary and ends the current-reading role.
_Avoid_: Journey completion, deck completion, mastery.

**Previously read**:
A one-time assertion that the learner read a Book outside its current-reading
workflow. It records assertion time, not a historical reading date, and implies
no analysis or vocabulary transition.
_Avoid_: reading completion, known vocabulary.

**Vocabulary transition**:
The set-based addition of a current reading's frozen snapshot to modeled Known
vocabulary when the learner finishes reading. It is independent of deck
artifact readiness and does not claim per-card mastery.
_Avoid_: mastery, deck review, vocabulary study.

**Graduation**:
The historical name for the consequential transition that accepts a frozen
snapshot into modeled Known vocabulary when reading is finished. It is never
implied by generation, deck readiness, or merely starting a Book.
_Avoid_: completion, mastering, promotion.

**Analysis evidence**:
The classification of a Book's current acquired source against its analysis
standing, owned per Book and derived from the raw analysis signals rather than
stored by hand. It is the single state both My Books and Reading
present: not acquired, unavailable (content present but no current revision),
stale (a prior analysis no longer matches the current content), analyzed, or
acquired-but-unassessed (current content present, analysis not yet complete).
_Avoid_: book status, analysis state (the raw signal the classification reads,
not the classification itself), deck readiness.

**Vocabulary coverage band**:
A neutral grouping of trustworthy analyzed To Read Books by current
analyzable-token coverage: at least 99%, at least 97% but below 99%, at least
95% but below 97%, or below 95%. It is evidence, not a reading recommendation.
_Avoid_: accessibility, readiness, difficulty band.

**No vocabulary comparison**:
The chooser group for a successfully analyzed Book with no analyzable-token
denominator. It means neither zero coverage nor failed evidence.
_Avoid_: 0% coverage, Needs attention.

**Main text**:
The contiguous run of a Book's readable units that the EPUB structure declares to
be the body of the work: from a declared body-matter start to a declared
back-matter start, or to the end when none is declared. Absent a declared
body-matter start, the Book has no identified main text and its whole snapshot is
treated as main text.
_Avoid_: body text, main matter (the retired classifier category).

**Ancillary text**:
The readable units of a Book that the EPUB structure declares to lie outside the
main text — front matter, and back matter such as a bibliography, index, or
glossary.
_Avoid_: boilerplate, main_matter.

**Book language**:
A book's chosen language tag, or its absence recorded as an unknown-language
state. A chosen tag is stored in one canonical base form — lowercased, with `_`
as `-` and region subtags collapsed, so `de_DE` and `de-de` are the same as `de`.
Chosen book languages are the raw material from which a learner's study
languages are derived. A Book without a chosen language belongs to no study
language partition and remains visible in Needs language.
_Avoid_: detected language, inferred language (nothing is ever inferred from
content).

**Book evidence state**:
The domain-derived classification of a Book's current acquired evidence:
`not_acquired`, `unavailable`, `acquired_unassessed`, `analyzed`, or `stale`.
It is derived from raw acquisition and current-analysis signals rather than
persisted as learner state. Current-reading eligibility is a separate derivation
with reason codes for missing current content, analysis in progress, failed or
cancelled analysis, stale analysis, no completed analysis, and eligibility.
_Avoid_: evidence status as a persisted source of truth.

**Catalog entry**:
A book as offered by a learner-owned catalog, identified by the catalog
connection plus that connection's stable entry identifier. A Book may carry one
catalog-entry alias per connection; two connections may each offer the same
entry identifier without being the same book.
_Avoid_: source identifier on its own, OPDS entry (as a standalone identity).

**Book**:
A learner's bibliographic identity for a work, owned per learner and stable
across acquisition, analysis, and content revisions. A Book is addressed by its
owner-scoped Book ID; its current acquired source and analysis evidence are
resolved underneath that identity. Books are catalog-derived: they enter the
library only through a connected catalog, never by manual entry. A Book is
always in My Books once discovered; analysis is Book-level evidence, not
membership state.
_Avoid_: source material (the acquired evidence, not the identity), acquired
book.

**Book cover**:
The catalog-supplied image representing a Book, retained with its originating
Catalog entry and allowed to change as that metadata changes. It supports the
Book's title and author but is never its sole learner-facing identity.
_Avoid_: cover art, thumbnail (a presentation size, not the Book metadata).

**Book disposition**:
A learner-and-Book-scoped triage relationship: Inbox, To Read, or Set Aside. It
follows the Book across language corrections and is independent of analysis,
deck state, and reading history; the current Book remains To Read underneath.
_Avoid_: book status, reading state, workflow state.

**Book workflow bucket**:
The one visible My Books placement of a Book, derived in precedence order from
Current reading, To Read, Inbox, Read when history exists, then Set Aside.
_Avoid_: Book disposition, shelf.

**My Books**:
The learner's Book collection, browsed by study language, including untriaged
Inbox Books and Books without a usable language. It owns disposition choices,
not the decision to start reading.
_Avoid_: library queue, reading plan.

**Reading**:
The active-language workflow for the current Book or, between Books, a neutral
chooser of trustworthy To Read candidates. It maintains no learner-authored
order or automatic next Book.
_Avoid_: Reading Journey, Primary Goal, reading queue.

**Inbox**:
The disposition of a newly discovered Book not yet triaged by the learner.
Metadata changes or catalog reappearance do not recreate Inbox work.
_Avoid_: unprocessed queue, notification.

**To Read**:
The disposition expressing that a Book is a possible future reading, with
acquisition and analysis intent but no current-reading commitment.
_Avoid_: Journey member, backlog, queue.

**Set Aside**:
The disposition of a Book removed from ordinary consideration without deletion
or a claim that it was read.
_Avoid_: archived, rejected, abandoned.

**Current reading**:
The analyzed To Read Book the learner has committed to reading in a study
language, at most one per language. Its frozen snapshot remains distinct from
the Book's later analysis evidence.
_Avoid_: Primary Goal, reading status, current project.

**Reading history**:
Append-only, Book-anchored reading completions and previously-read assertions.
History remains even when a Book returns to To Read for rereading.
_Avoid_: Read disposition, completed list, Journey history.

**Concordance**:
A listing of a word's (or lemma's) occurrences with their surrounding context,
at the scope of a Book or of a study language's analyzed library. Context
covers both the linear text around each occurrence and the occurrence's
syntactic role from dependency parsing. A future
learner-facing surface; its persistence foundation is the per-analysis
normalized corpus.
_Avoid_: KWIC (a rendering style, not the feature), occurrence list.

**Dependency relation**:
The syntactic function a token (the dependent) fulfils relative to its head
in a sentence's dependency parse, for example `nsubj` (subject), `obj`, or
`advcl`. One of the dimensions of a sentence's surrounding context; what makes
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
The sentence chosen for a lemma's recognition card: the highest-scoring accepted
candidate sentence, with source order breaking ties. It is always the learner's
complete source sentence, never truncated or paraphrased.
_Avoid_: example sentence (ambiguous with the historical
`example_sentences` store), best sentence.

**Sentence quality**:
The deterministic, explainable score a candidate sentence receives as a
potential representative, computed at export time from its tokens and dependency
structure (GDEX-informed). It has two parts: the knock-out gate (accept or
reject) and the gradual score (ranking among accepted candidates).
_Avoid_: sentence score (fine in prose), readability.

**Dictionary index**:
The build-time-derived, read-only lexical dataset, per language, that the
dictionary enrichment provider reads in-process in Go. It is a static SQLite
artifact extracted from Wiktextract/Kaikki raw JSONL (English glosses, forms,
IPA), regenerated from the weekly dumps by a build step; it is not a database of
application state, not a service, and not part of the NLP analysis service.
_Avoid_: dictionary database, lexicon service.

**Sense**:
A distinct meaning of a lemma within a dictionary entry. A Wiktionary-derived
entry carries an ordered list of senses, each with a concise English gloss and
optional tags, topics, and examples. A recognition card renders a compact set of
a lemma's senses ordered by context — deterministic by default, optionally
reselected by the external LLM when the learner consents.
_Avoid_: definition (the full native-language explanation, a separate deferred
field), translation.

**Gloss**:
The concise English sense explanation of a lemma rendered on the card back. A
gloss is dictionary-sourced by default; when the dictionary has no gloss for the
lemma, or none of its senses fit the representative sentence, it may be supplied
by the consent-gated external LLM. It is distinct from the contextual
`EnglishSentence` translation, which renders the representative sentence in
English.
_Avoid_: definition, translation (the contextual whole-sentence field).

**Dictionary gloss**:
A gloss authored by the dictionary index — local, consent-free, deterministic
enrichment; the default source of the card's meaning block.
_Avoid_: local gloss, definition.

**Fallback gloss**:
A gloss authored by the external LLM when the dictionary supplies no gloss or
none of its senses fit the representative sentence. Consent-gated; distinct from
the contextual sentence translation.
_Avoid_: LLM gloss (ambiguous with the discarded provider gloss field),
definition.
