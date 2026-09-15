# Mouseion

Mouseion is a self-hosted reading environment for learning foreign languages.
A learner connects their own OPDS catalog, syncs books into a personal
library, analyzes Books according to their declared EPUB structure, and prepares
Anki recognition decks from unknown vocabulary.

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
Reading Journey, Vocabulary) present, and never defines which languages are
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
populated only by explicit lemma import or the justified graduation of a Book's
snapshotted vocabulary; card generation never marks vocabulary as known.
_Avoid_: known words, learned vocabulary.

**Recurring vocabulary**:
Unknown lemmas appearing at least N times in an analyzed book; the pool a
prepared deck selects. N is a selection parameter with a default of three, and
selection makes no coverage claim. Known vocabulary, generated vocabulary, and
vocabulary reserved by the currently-studied book are excluded before the pool
is formed.
_Avoid_: frequent words, deck coverage.

**Reserved vocabulary**:
The snapshotted vocabulary of the book currently being studied, held aside so it
is neither counted as known nor re-selected into another book's deck until the
study is resolved (graduated to known or released on abandonment). It is the
exclusion set of the current study; only one book's vocabulary is reserved at a
time per owner.
_Avoid_: active-campaign vocabulary, known vocabulary.

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
itself never starts vocabulary study and never marks vocabulary known.
_Avoid_: study plan, in-app review deck.

**Book-anchored vocabulary study**:
The vocabulary facet of a Book: a Book's vocabulary is reserved while its
prepared deck is being studied, and graduates into known vocabulary on
confirmed deck review. It is one of the Book's two independent facts (the other
is its reading state on the Primary Goal / Journey). There is no separate
campaign object; the Book is the unit of the learner loop. One Book's
vocabulary is studied at a time per owner.
_Avoid_: learning campaign, active campaign, plan (as a separate object).

**Vocabulary study**:
The learner's activity of working through a Book's prepared deck in their own
Anki, recorded by Mouseion as that Book's vocabulary-study state. Confirming
that the deck was reviewed is the justified study confirmation; it completes
the study and graduates the Book's snapshotted vocabulary. Reading progress is
an independent fact from study progress.
_Avoid_: mastery, finishing the book, reading completion.

**Graduation**:
The consequential transition that promotes a Book's snapshotted,
provenance-linked vocabulary identities into the learner's known vocabulary,
justified only by confirmed deck review. Graduation is never implied by
generation, assignment, reading, or Goal choice.
_Avoid_: completion, mastering, promotion (reserved for Primary Goal).

**Analysis evidence**:
The classification of a Book's current acquired source against its analysis
standing, owned per Book and derived from the raw analysis signals rather than
stored by hand. It is the single state both My Books and the Reading Journey
present: not acquired, unavailable (content present but no current revision),
stale (a prior analysis no longer matches the current content), analyzed, or
acquired-but-unassessed (current content present, analysis not yet complete).
_Avoid_: book status, analysis state (the raw signal the classification reads,
not the classification itself), deck readiness.

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
languages are derived. A book without a chosen language belongs to no language
partition: it participates in nothing until a re-sync admits it into one.
_Avoid_: detected language, inferred language (nothing is ever inferred from
content).

**Book evidence state**:
The domain-derived classification of a Book's current acquired evidence:
`not_acquired`, `unavailable`, `acquired_unassessed`, `analyzed`, or `stale`.
It is derived from raw acquisition and current-analysis signals rather than
persisted as learner state. Goal eligibility is a separate domain derivation
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

**Reading intent**:
A learner's voluntary act of selecting a Book as a candidate they may read and
study next. Intent is expressed by adding the Book to the Reading Journey, and
is a promise that analysis evidence should exist so the learner can weigh the
Book against other candidates. Analysis is an automatic, ensure-once consequence
of expressed intent, not a separate chore.
_Avoid_: interest, wanting to read, commitment (which is Primary Goal).

**Reading Journey**:
One per study language: a fluid, provisional order of that language's Books the
learner currently imagines reading. Membership is reversible and expresses
reading intent in that language; adding a Book to the Journey automatically
acquires and analyzes it (ensure-once) so it can be weighed against other
members of the same language. It is a candidate pool, not a commitment. A Book
with a chosen language joins its language's Journey.
_Avoid_: learning queue, backlog, curriculum, plan, roadmap.

**Journey entry**:
The learner-facing view of one Book as a step in its language's Reading Journey,
combining that Journey member with its current completed analysis evidence and
the actions available for the entry. It exists only for a Book that belongs to
the Journey and has a current completed analysis. A Journey entry is not a
separate identity or membership object: the Book remains the My Books
bibliographic identity, and the Reading Journey remains the provisional
candidate pool.
_Avoid_: Book detail, analysis page, journey item.

**Primary Goal**:
One per study language: the Book in that language's Reading Journey the learner
currently intends to finish, when one exists. It is a promotion of a member of
that language's Journey: choosable only for a member with a successfully
completed current analysis, and it clears if that member leaves the Journey.
How many Goals across languages are active at once is the learner's own
discipline, not an enforced invariant. The Goal carries commitment; Reading
Journey membership does not.
_Avoid_: active campaign, target destination, current project.

**Concordance**:
A listing of a word's (or lemma's) occurrences with their surrounding context,
at the scope of a Book or of a study language's analyzed library. Context
covers both the linear text around each occurrence and the occurrence's
persisted syntactic role. A future
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
