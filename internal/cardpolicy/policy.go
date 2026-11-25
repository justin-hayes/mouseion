// Package cardpolicy holds the one card policy each study language has
// (ADR 0133). A policy replaces every "is this language Ancient Greek" test:
// callers read the policy for the language instead of naming a language.
package cardpolicy

import "github.com/justin-hayes/mouseion/internal/canonicalization"

// Boundary is the rule that decides whether a sentence is complete.
type Boundary int

const (
	// BoundaryCapitalisedStart requires a sentence ending and a capitalised
	// start, as modern editions have.
	BoundaryCapitalisedStart Boundary = iota
	// BoundaryTerminalMark requires only a terminal mark, for imported sources
	// whose editions do not capitalise sentence starts.
	BoundaryTerminalMark
)

// Lookup is the rule that matches a lemma to dictionary entries.
type Lookup int

const (
	// LookupStored reads the stored-normalised dictionary entries.
	LookupStored Lookup = iota
	// LookupExactlyOne matches an identity only when exactly one candidate
	// entry exists.
	LookupExactlyOne
	// LookupEveryEntry matches a lemma to every entry under its key in the
	// language's own dictionary, with no part-of-speech filter and no choice
	// between homographs (Lewis & Short for Latin).
	LookupEveryEntry
)

// TranslationSource is where a language's passage-context translations come
// from.
type TranslationSource int

const (
	// TranslationNone means the language has no imported reference translation.
	TranslationNone TranslationSource = iota
	// TranslationPerseus is a pinned Perseus perseus-engN translation file,
	// fetched lazily at preparation and stored once per installation, keyed by
	// pin and Citation.
	TranslationPerseus
)

// EntrySource is the dictionary a source-evidence card freezes its entry from.
type EntrySource int

const (
	// EntryNone freezes no dictionary entry.
	EntryNone EntrySource = iota
	// EntryLSJ freezes the matched Liddell-Scott-Jones entry.
	EntryLSJ
	// EntryLewisShort freezes every Lewis & Short entry under the lemma's key,
	// in entry id order, with no cap and no reordering (ADR 0133).
	EntryLewisShort
)

// CitationFilter is the rule that decides which citations the full dictionary
// entry keeps.
type CitationFilter int

const (
	// CitationsNone keeps every citation the index kept: Greek's filter is
	// applied when the LSJ index is derived, so the card filters nothing more.
	CitationsNone CitationFilter = iota
	// CitationsWorkAuthor keeps only citations of the card's own Work's author
	// and drops one with no usable author reference (ADR 0133).
	CitationsWorkAuthor
)

// Imported-source notices name each source and its licence; a deck carries the
// notice of its language's policy. They live here, beside the policy that picks
// them, so the policy names its notice as a value.
const (
	// GreekSourceNotice names OGA v0.2.0 as packaged by oga_src, derived from
	// Perseus/canonical-greekLit, all CC BY-SA 4.0.
	GreekSourceNotice = "Ancient Greek text and annotation: OGA v0.2.0 (CC BY-SA 4.0), packaged as oga_src (Zenodo DOI 10.5281/zenodo.15199546) and derived from Perseus/canonical-greekLit (CC BY-SA 4.0). Commentary and derived data remain share-alike under the same licence."
	// LatinSourceNotice names the Opera Latina Adnotata v0.2.0 release (LatinPipe
	// output, Zenodo record 15183688) and the Perseus canonical-latinLit text it
	// annotates, both CC BY-SA 4.0. The translation's own pin rides in its
	// passage notice.
	LatinSourceNotice = "Latin text and annotation: Opera Latina Adnotata (OLA) v0.2.0, LatinPipe output (CC BY-SA 4.0, Zenodo record 15183688), derived from Perseus/canonical-latinLit (CC BY-SA 4.0; the text is the revision OLA v0.2.0 annotates, recorded in the release). Commentary and derived data remain share-alike under the same licence."
)

// Policy is the card behaviour of one study language. A language either uses
// source-evidence cards or takes the modern-language provider route, never
// both.
type Policy struct {
	// SourceEvidence marks a language whose cards are built from imported
	// sources (passage context and the dictionary entry), so a deck can be
	// prepared with no provider.
	SourceEvidence bool
	// EvidenceAssistedPass reports whether the evidence-assisted provider pass
	// may run on top of the source-evidence card.
	EvidenceAssistedPass bool
	// SourceNotice is the imported-source notice a deck of this language
	// carries, or empty when the language has none.
	SourceNotice string
	// Translations is where the passage-context translations come from.
	Translations TranslationSource
	// Entries is the dictionary a source-evidence card freezes its entry from.
	Entries EntrySource
	// Citations is the citation filter of the full dictionary entry.
	Citations CitationFilter
	// NLPAnalysed reports whether the NLP service analyses the language, so the
	// analysis-models notice applies.
	NLPAnalysed bool
	// SentenceBoundary is the sentence-completeness rule.
	SentenceBoundary Boundary
	// DictionaryLookup is the dictionary lookup rule.
	DictionaryLookup Lookup
}

var (
	sourceEvidenceGreek = Policy{
		SourceEvidence:       true,
		EvidenceAssistedPass: true,
		SourceNotice:         GreekSourceNotice,
		Translations:         TranslationPerseus,
		Entries:              EntryLSJ,
		Citations:            CitationsNone,
		SentenceBoundary:     BoundaryTerminalMark,
		DictionaryLookup:     LookupExactlyOne,
	}
	// sourceEvidenceLatin has no evidence-assisted pass: the provider payload
	// has no recorded assessment for Latin (ADR 0133), so a Latin deck is the
	// default card with no provider request. Its dictionary is Lewis & Short, and
	// its full entry keeps only the Work author's citations.
	sourceEvidenceLatin = Policy{
		SourceEvidence:   true,
		SourceNotice:     LatinSourceNotice,
		Entries:          EntryLewisShort,
		Citations:        CitationsWorkAuthor,
		Translations:     TranslationPerseus,
		SentenceBoundary: BoundaryTerminalMark,
		DictionaryLookup: LookupEveryEntry,
	}
	modern = Policy{
		NLPAnalysed:      true,
		SentenceBoundary: BoundaryCapitalisedStart,
		DictionaryLookup: LookupStored,
	}
)

// For returns the card policy of a language tag. Every language without its own
// policy takes the modern-language provider route.
func For(language string) Policy {
	switch canonicalization.NormalizeLanguage(language) {
	case "grc":
		return sourceEvidenceGreek
	case "la":
		return sourceEvidenceLatin
	}
	return modern
}
