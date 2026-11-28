package ancientgreek

import (
	"context"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// Import codes are the classical package's stable preparation codes, kept under
// their Greek import names.
const (
	CodeReadFailed           = classical.CodeReadFailed
	CodeOversize             = classical.CodeOversize
	CodePinMismatch          = classical.CodePinMismatch
	CodeMalformed            = classical.CodeMalformed
	CodeUnknownPOS           = classical.CodeUnknownPOS
	CodeUnknownInterpret     = classical.CodeUnknownInterpret
	CodeSentenceUnassigned   = classical.CodeSentenceUnassigned
	CodeSentenceAmbiguous    = classical.CodeSentenceAmbiguous
	CodeEmptyCitationRange   = classical.CodeEmptyCitationRange
	CodeOpeningMissing       = classical.CodeOpeningMissing
	CodeUnknownSourceFormat  = classical.CodeUnknownSourceFormat
	CodeSourceLayerUnknown   = classical.CodeSourceLayerUnknown
	CodeSourceDownloadFailed = classical.CodeSourceDownloadFailed
	CodeAdapterRefused       = classical.CodeAdapterRefused
)

// ImportError is a refused import: the neutral classical preparation error.
type ImportError = classical.PreparationError

// ImportAnalyzerName identifies an imported classical analysis run.
const ImportAnalyzerName = "mouseion-greek-import"

// SourceMediaType is the media type of the pinned oga_src CoNLL-U file. It is
// deliberately not an EPUB type: the import path never treats the source as an
// EPUB.
const SourceMediaType = domain.ImportedSourceMediaType

// Interpretation slot versions recorded on an imported run. They are compiled
// reviewed data; a new value produces a new analysis run.
const (
	PosMappingVersion = "oga-agdt-upos/5"
	// InterpretationVersionNativeAGDT is the OGA interpretation version that
	// stores each token's AGDT relation and head and the unprinted syntax nodes
	// (ADR 0122). It supersedes PosMappingVersion as the compiled manifest's
	// default interpretation version.
	InterpretationVersionNativeAGDT = "oga-agdt-syntax/1"
	FeatsMappingVersion             = "oga-agdt-feats/2"
	NormalizationVersion            = "ancient-greek/1"
	ProperNameRuleVersion           = "oga-anabasis-proper-name/1"
	// SearchKeyRuleVersion names the stored surface-form search key derivation.
	// It is part of the run's version so a run written before the key existed is
	// superseded by a re-import that writes keys.
	SearchKeyRuleVersion = "oga-form-search-key/1"
	// SentenceCitationRuleVersion names the stored per-sentence citation
	// derivation. It is part of the run's version so a run imported before the
	// column existed is superseded by a re-import that writes the cited
	// sections a representative sentence touches (ADR 0101).
	SentenceCitationRuleVersion = "oga-sentence-citations/1"
	ImportContractVersion       = "greek-import/1"
)

// SyntaxState records whether an analysis run's tokens carry UD syntax, native
// AGDT syntax that is never read as UD, or no syntax at all.
type SyntaxState string

const (
	// SyntaxStateUD is an ordinary analysis run's state.
	SyntaxStateUD SyntaxState = "ud"
	// SyntaxStateNativeAGDT marks stored native AGDT syntax.
	SyntaxStateNativeAGDT SyntaxState = "native_agdt"
	// SyntaxStateAbsent records that the run stores no syntax. Greek consumers
	// show "Syntax unavailable" for a native_agdt or absent run.
	SyntaxStateAbsent SyntaxState = "absent"
)

// FeatureValueCount is one unmapped morphology value, counted by feature. Both
// fields are ASCII.
type FeatureValueCount struct {
	Feature string
	Value   string
	Count   int
}

// ImportReport is the outcome of one import. It names counts and codes only,
// never any source text.
type ImportReport struct {
	BookID                        string
	RunID                         string
	Sentences                     int
	Tokens                        int
	EllipsisTokensSkipped         int
	Crossings                     []SentenceCrossing
	ProperNamesDerived            int
	NumberedLemmaTypes            int
	BareLemmasWithNumberedSibling int
	UnmappedFeatureValues         []FeatureValueCount
	Reimported                    bool
}

// ImportWriteRequest is the source-neutral candidate the writer persists. It
// carries no dialect-specific detail.
type ImportWriteRequest struct {
	OwnerID         string
	BookID          string
	Definition      domain.ReadingUnitDefinition
	SourceBytes     []byte
	FullText        string
	UnitID          string
	UnitTitle       string
	Result          analyzer.Result
	SyntaxState     SyntaxState
	AnalyzerName    string
	AnalyzerVersion string
	ConfigIdentity  string
	// RiverJobID is the River job publishing this import, or zero when none
	// is. It is recorded with the carried-over decisions in the publishing
	// transaction so /jobs can name the job that published the run.
	RiverJobID int64
}

// ImportWriteResult reports the run the writer used.
type ImportWriteResult struct {
	RunID      string
	Reimported bool
}

// ImportWriter persists and publishes one imported analysis. It is the only
// part of the seam that touches the installation.
type ImportWriter interface {
	WriteImportedAnalysis(context.Context, ImportWriteRequest) (ImportWriteResult, error)
}

// InterpretationVersion is the versioned interpretation identity recorded on an
// imported run: every mapping slot that shaped the tokens.
func InterpretationVersion(definition domain.ReadingUnitDefinition) string {
	if definition.Language == classical.LanguageLatin {
		return latinInterpretationVersion(definition)
	}
	return strings.Join([]string{
		definition.InterpretationVersion,
		FeatsMappingVersion,
		NormalizationVersion,
		ProperNameRuleVersion,
		SearchKeyRuleVersion,
		SentenceCitationRuleVersion,
	}, "+")
}

// ImportConfigIdentity is the run's configuration identity: the reviewed
// definition and the syntax decision. It distinguishes runs that share an
// interpretation version.
func ImportConfigIdentity(definition domain.ReadingUnitDefinition, state SyntaxState) string {
	return strings.Join([]string{
		ImportContractVersion,
		definition.Key,
		string(state),
	}, ":")
}
