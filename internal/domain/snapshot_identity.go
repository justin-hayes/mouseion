package domain

// SnapshotIdentity is one study-language vocabulary identity: a canonical
// lemma with its analyzed part of speech and its scheme-qualified lexical
// discriminator. The discriminator is empty for modern-language vocabulary and
// distinguishes source homographs such as morpheus:2. Headword is the imported
// citation form shown to learners, empty for modern-language vocabulary.
//
// The Current reading no longer freezes a snapshot of these (ADR 0136 §4); the
// type remains as the identity store-contract seeds declare for a Book.
type SnapshotIdentity struct {
	Language, CanonicalLemma, UPOS, LexicalDiscriminator, Headword string
}
