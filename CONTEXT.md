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
resolved underneath that identity.
_Avoid_: source material (the acquired evidence, not the identity), acquired
book.