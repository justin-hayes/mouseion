# Mouseion

Mouseion is a self-hosted reading environment for learning foreign languages.
A learner connects their own OPDS catalogue, syncs books into a personal
library, analyzes confirmed scopes, and prepares Anki recognition decks from
unknown vocabulary.

## Language

**Study language**:
A language the learner studies. The set is inferred from the languages present
in the learner's library (books with a chosen language), never configured by
the learner; it defines the languages for which the learner can import known
vocabulary.
_Avoid_: configured language, preferred language, library language (as a stored
selection).

**Catalogue sync scope**:
The set of non-English languages that a connected catalogue offers and the NLP
service reports ready. The sync walks these feeds regardless of learner
configuration and tags each book with the walked language.
_Avoid_: study-language scope, synced languages.

**Known vocabulary**:
Words the learner already knows in a language, owned per learner and language,
populated only by explicit lemma import or campaign graduation; card generation
never marks vocabulary as known.
_Avoid_: known words, learned vocabulary.

**Recurring vocabulary**:
Unknown lemmas appearing at least N times in an analyzed book; the pool a
prepared deck selects. N is a selection parameter with a default of three, and
selection makes no coverage claim. Known vocabulary, generated vocabulary, and
active-campaign vocabulary are excluded before the pool is formed.
_Avoid_: frequent words, deck coverage.

**Book language**:
A book's chosen language tag, or its absence recorded as an unknown-language
state. A chosen tag is stored in one canonical base form — lowercased, with `_`
as `-` and region subtags collapsed, so `de_DE` and `de-de` are the same as `de`.
Chosen book languages are the raw material from which a learner's study
languages are derived.
_Avoid_: detected language, inferred language (nothing is ever inferred from
content).

**Catalogue entry**:
A book as offered by a learner-owned catalogue, identified by the catalogue
connection plus that connection's stable entry identifier. A Book may carry one
catalogue-entry alias per connection; two connections may each offer the same
entry identifier without being the same book.
_Avoid_: source identifier on its own, OPDS entry (as a standalone identity).

**Book**:
A learner's bibliographic identity for a work, owned per learner and stable
across acquisition, analysis, and content revisions. A Book is addressed by its
owner-scoped Book ID; its current acquired source and analysis evidence are
resolved underneath that identity. Books are catalogue-derived: they enter the
library only through a connected catalogue, never by manual entry. A Book is
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
A fluid, provisional order of learner-selected Books they currently imagine
reading. Membership is reversible and expresses reading intent; adding a Book to
the Journey automatically acquires and analyzes it (ensure-once) so it can be
weighed against other members. It is a candidate pool, not a commitment.
_Avoid_: learning queue, backlog, curriculum, plan, roadmap.

**Primary Goal**:
The one Book the learner currently intends to finish, when one exists. It is a
promotion of a Reading Journey member: choosable only for a member with a
successfully completed current analysis, and it clears if that member leaves the
Journey. The Goal carries commitment; Reading Journey membership does not.
_Avoid_: active campaign, target destination, current project.
