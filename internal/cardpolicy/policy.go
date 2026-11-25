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
	// ImportedSourceNotice reports whether the imported-source notices apply.
	ImportedSourceNotice bool
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
		ImportedSourceNotice: true,
		SentenceBoundary:     BoundaryTerminalMark,
		DictionaryLookup:     LookupExactlyOne,
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
	if canonicalization.NormalizeLanguage(language) == "grc" {
		return sourceEvidenceGreek
	}
	return modern
}
