# Mouseion

Mouseion is a self-hosted reading environment for learning foreign languages.
A learner connects their own OPDS catalog, syncs books into a personal
library, analyzes each Book according to its declared EPUB structure, and prepares
Anki recognition decks from unknown vocabulary.

The current disposition and current-reading contract is recorded in
[ADR 0078](doc/adr/0078-book-dispositions-and-current-reading.md).
[ADR 0072](doc/adr/0072-goal-owned-vocabulary-and-journey-forecast.md)
records the historical Goal and ordered-Journey model.
[ADR 0079](doc/adr/0079-contextual-glosses-require-llm.md) records the
implemented contextual Gloss vocabulary below.
The Book-scoped Vocabulary Browse inventory and Reading-owned Book vocabulary
definitions below are accepted targets under [ADR 0085](doc/adr/0085-reading-owned-book-vocabulary.md);
the shipped application has not completed the Reading-owned selection cutover.

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
Unknown lemmas appearing at least N times in an analyzed Book; one source of
Book vocabulary candidates. N is a selection parameter with a default of
three, and selection makes no coverage claim. Current Known vocabulary and
Reserved vocabulary in the same study language are excluded; generated
provenance is not an exclusion state.
_Avoid_: frequent words, deck coverage.

**Book vocabulary candidates**:
Unknown vocabulary eligible for a Book's prepared deck and current-reading
vocabulary snapshot: either recurring in that Book or appearing twice there
and frequently across the learner's currently analyzed Books in the same
study language, including that Book. Generated provenance does not exclude it.
_Avoid_: recurring vocabulary (only one route into this pool), corpus words.

**Reserved vocabulary**:
The frozen Book-vocabulary-candidate snapshot held by a current reading in its
study language, neither counted as Known nor available for selection while active.
Ending, switching, or finishing releases the reservation.
_Avoid_: active-campaign vocabulary, Goal vocabulary, known vocabulary.

**Unknown vocabulary**:
An eligible analyzed vocabulary identity that is neither Known nor Reserved in
its study language. A word can be Unknown even if it has appeared in a prepared
Book deck; generation does not establish learner knowledge.
_Avoid_: difficult words, never generated vocabulary.

**Current-reading vocabulary snapshot**:
The immutable Book-vocabulary-candidate identity set frozen from the Book's exact
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
An Anki recognition deck built asynchronously from a current reading's frozen
Book vocabulary candidates and exact completed analysis of that Book. The ready
deck is downloaded and studied in the learner's own Anki; preparation does not
mark vocabulary Known. Reading completion owns the vocabulary transition; a
current reading with an empty snapshot requires no deck.
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

**English target alignment**:
The optional correspondence between the tested vocabulary in a representative
sentence and one or more spans of its complete English translation. A confident
correspondence can be discontinuous; an uncertain one leaves the translation
unemphasised.
_Avoid_: English bolding (the presentation of an alignment, not the alignment).

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
A learner-and-Book-scoped triage relationship: Inbox or To Read. It
follows the Book across language corrections and is independent of analysis,
deck state, and reading history; the current Book remains To Read underneath.
_Avoid_: book status, reading state, workflow state.

**Hidden (Book visibility)**:
An owner-and-Book choice stored apart from disposition (`book_visibility`; a
missing row is a visible Book at revision 0). Hide/Unhide request a desired
value against the expected visibility revision. It only removes the Book from
default My Books and the default Reading chooser; it never changes disposition,
Current reading, history, Known, analysis, or corpus participation. Recovery is
My Books → Show hidden books → Unhide.
_Avoid_: archived, deleted, set aside.

**Book workflow bucket**:
The one visible My Books placement of a Book, derived in precedence order from
Current reading, To Read, Read when history exists, then Inbox.
Current reading is included in the To Read tab while retaining its distinct
Currently reading label.
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
The disposition of a Book with no explicit future-reading intent. It is the default for a newly discovered Book, and it does not mean the Book was never triaged.
Metadata changes or catalog reappearance do not recreate Inbox work. A Book
marked previously read, and a Book whose reading was finished, leaves the
visible Inbox for Read while Inbox remains its underlying disposition.
_Avoid_: unprocessed queue, notification.

**To Read**:
The disposition expressing that a Book is a possible future reading, with
acquisition and analysis intent but no current-reading commitment.
_Avoid_: Journey member, backlog, queue.

**Set Aside** (retired):
A former disposition for a Book removed from ordinary consideration. ADR 0086
retired it as a value and action: requests are rejected, not translated into
Hidden or End. Legacy rows map to Inbox in the pending maintenance cutover.
_Avoid_: current disposition, archived, rejected, abandoned.

**Current reading**:
The analyzed To Read Book the learner has committed to reading in a study
language, at most one per language. Its frozen snapshot remains distinct from
the Book's later analysis evidence.
_Avoid_: Primary Goal, reading status, current project.

**End current reading**:
Clears the Book's current-reading role and reservations without a completion or
Known acceptance. The Book stays To Read, keeping its visibility, snapshots,
history, and artifacts. It is not a pause: nothing is paused, and inactivity
never ends a reading.
_Avoid_: pause, stop, set aside.

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

**Effective vocabulary identity**:
The language, canonical lemma, and part-of-speech identity attributed to an
analyzed occurrence in learner-facing vocabulary after any confirmed
correction. It can differ from the preserved analyzer assignment; an excluded
occurrence contributes none.
_Avoid_: edited analyzer lemma, global corrected lemma.

**Vocabulary Browse inventory**:
The collection of effective vocabulary identities evidenced by the Book in
the active study language's Current reading, including single occurrences and
identities that are Known, Reserved, or previously Generated for that Book. It
is distinct from both the filtered Browse results and the recurring-vocabulary
pool used for Book prepared decks and current-reading snapshots.
_Avoid_: prepared-deck candidates, known-vocabulary list.

**Browse selection**:
An owner- and study-language-scoped unnamed selection of effective vocabulary
identities made in Vocabulary Browse before creating a Custom deck. It persists
across changes of Current reading and can contain identities not evidenced in
the current Book; it is not itself a saved deck or preparation artifact.
_Avoid_: custom deck, prepared deck, temporary filter result.

**Lemma review flag**:
A non-authoritative indication that an analyzed occurrence's lemma may assign
the wrong vocabulary identity, offered for learner review before it affects a
current-reading snapshot or prepared deck. Missing dictionary evidence alone
does not prove an error.
_Avoid_: invalid lemma, unconfirmed lemma.

**Lemma suggestion**:
An unapproved alternative lemma for an analyzed occurrence, supported by
dictionary, model, or other evidence. It changes no vocabulary identity until
the learner confirms it; review remains possible without such suggestions.
_Avoid_: automatic correction, LLM correction.

**Lemma correction**:
A learner-confirmed alternative canonical lemma for one occurrence in a Book's
exact analysis, governing derived reading and deck vocabulary while preserving
analyzer evidence. It need not have a dictionary entry, can be made without a
flag, and is never applied globally or across analyses automatically.
_Avoid_: spelling correction, dictionary correction, global lemma override.

**Occurrence exclusion**:
A learner-confirmed choice to omit one analyzed occurrence from vocabulary
selection while retaining it in the Book's source and analysis evidence. It
does not mark its lemma Known, remove it from analyzable-token coverage, or
exclude other occurrences of that lemma.
_Avoid_: delete token, ignore lemma, Known vocabulary.

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
The local, read-only collection of Wiktionary-derived entries used as meaning
evidence and dictionary-form evidence for cards. It is not learner or Book state.
_Avoid_: dictionary database, lexicon service.

**Sense**:
A distinct meaning of a lemma within a dictionary entry. It may have a
source-provided gloss and other identifying context. A sense is evidence for a
card's meaning, not necessarily the wording shown on the card.
_Avoid_: definition (the full native-language explanation, a separate deferred
field), translation.

**Gloss**:
The short English cue for what the target means in its representative sentence,
rendered on the card back. It expresses one contextual meaning, not a list of
different senses; one to three comma-separated words or close synonyms are
preferred when they convey that meaning faithfully. It is distinct from the
English translation of the whole sentence.
_Avoid_: definition, translation (the contextual whole-sentence field).

**Dictionary gloss**:
A source-provided English explanation of a dictionary meaning, used as evidence
for a card's contextual Gloss rather than necessarily displayed verbatim.
_Avoid_: local gloss, definition.

**Meaning evidence**:
Source-attributed Wiktionary senses relevant to a target in its representative
sentence. Meaning evidence informs the contextual Gloss without dictating its
wording; the sentence itself may support a context-only gloss when no sense fits.
_Avoid_: card Gloss, undifferentiated definitions.

**Fallback gloss**:
A historical label for a card's English meaning cue supplied when dictionary
evidence was absent or unsuitable for the representative sentence. It named the
fallback path, not a separate kind of learner-facing meaning.
_Avoid_: definition, Gloss (when referring to the historical fallback path).
