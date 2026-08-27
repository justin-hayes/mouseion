// Package analyzer defines the project-owned boundary for batch linguistic
// analysis of source documents.
package analyzer

import (
	"context"
	"time"
)

// Analyzer performs one coarse-grained analysis operation for an entire
// source document. Implementations may call an external service, but must not
// expose backend-specific or transport-specific types through this interface.
type Analyzer interface {
	Analyze(context.Context, AnalyzeRequest) (Result, error)
}

// AnalyzeRequest describes a complete source document to analyze.
type AnalyzeRequest struct {
	Language string
	Document SourceDocument
}

// SourceDocument identifies a document and contains all text analyzed in one
// batch. ID is stable within the resulting corpus.
type SourceDocument struct {
	ID               string
	SourceIdentifier string
	Title            string
	Text             string
}

// Result is a normalized, transport-independent corpus produced by an
// Analyzer. Tokens carry the complete vocabulary identity required downstream:
// Language (on Result), CanonicalLemma, and UPOS.
type Result struct {
	SchemaVersion        string
	Language             string
	Sentences            []Sentence
	SourceDocuments      []SourceDocumentMetadata
	Analysis             AnalysisProvenance
	NormalizationProfile NormalizationProfile
}

// Sentence is a source-ordered sentence and its source-ordered tokens.
type Sentence struct {
	Text     string
	Tokens   []Token
	Location SourceLocation
}

// Token is one analyzed token. Surface removes surrounding Unicode punctuation
// and symbols while sentence text and offsets preserve the source; RawLemma
// preserves backend output. CanonicalLemma is derived using
// Result.NormalizationProfile. UPOS contains a coarse Universal Dependencies
// part-of-speech tag.
type Token struct {
	Surface        string
	RawLemma       string
	CanonicalLemma string
	UPOS           string
	Morphology     map[string]string
	NamedEntity    *string
	Location       SourceLocation
}

// SourceDocumentMetadata describes a source referenced by SourceLocation.
type SourceDocumentMetadata struct {
	ID               string
	SourceIdentifier string
	Title            string
}

// AnalysisProvenance records which backend produced a result.
type AnalysisProvenance struct {
	RunID           string
	AnalyzedAt      time.Time
	AnalyzerName    string
	AnalyzerVersion string
}

// NormalizationProfile identifies the deterministic canonicalization rules
// used to derive Token.CanonicalLemma from Token.RawLemma.
type NormalizationProfile struct {
	Name    string
	Version string
}

// SourceLocation is a reproducible half-open span [StartOffset, EndOffset) in
// Unicode code points within a source document's extracted text.
type SourceLocation struct {
	SourceDocumentID string
	Chapter          string
	Section          string
	StartOffset      uint64
	EndOffset        uint64
}
