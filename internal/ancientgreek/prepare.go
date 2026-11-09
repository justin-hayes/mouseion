package ancientgreek

import (
	"errors"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ParsedWork is one whole pinned source file parsed by an adapter. It is
// opaque so a caller can cache it and reuse the parse for another Reading unit
// of the same file. It carries source text and is never logged.
type ParsedWork struct {
	doc *sourceDocument
}

// PreparedUnit is one Reading unit cut from a parsed work: only the unit's
// text and the source-neutral analysis candidate, never the whole source.
type PreparedUnit struct {
	// UnitID is the classical Reading-unit identity the corpus refers to.
	UnitID string
	// UnitKey is the permanent Reading-unit key.
	UnitKey string
	// FullText is the unit's text and nothing else.
	FullText string
	// SourceBytes is the unit's text as bytes, the source material's content.
	SourceBytes []byte
	// Result is the source-neutral interpretation of the unit.
	Result analyzer.Result
	// SyntaxState records that an imported run stores no UD syntax.
	SyntaxState SyntaxState
	// AnalyzerName, AnalyzerVersion and ConfigIdentity name the run's identity.
	AnalyzerName    string
	AnalyzerVersion string
	ConfigIdentity  string
	// Report carries counts and codes only.
	Report ImportReport
}

// ParseOgaSource parses one whole oga_src CoNLL-U file. The result can be cut
// repeatedly for different Reading units of the same work file.
func ParseOgaSource(data []byte) (*ParsedWork, error) {
	doc, err := parseOgaSrc(data)
	if err != nil {
		return nil, err
	}
	return &ParsedWork{doc: doc}, nil
}

// CutUnit cuts one Reading unit from the parsed work, validates it and returns
// only its text and analysis candidate. It never returns the whole work file.
func (p *ParsedWork) CutUnit(definition domain.ReadingUnitDefinition) (PreparedUnit, error) {
	if p == nil || p.doc == nil {
		return PreparedUnit{}, errors.New("ancientgreek: parsed work is required")
	}
	if err := definition.Validate(); err != nil {
		return PreparedUnit{}, err
	}
	unitID := domain.ClassicalUnitID(definition.Key)
	reading, err := interpret(p.doc, definition, unitID)
	if err != nil {
		return PreparedUnit{}, err
	}
	if err := reading.validate(); err != nil {
		return PreparedUnit{}, err
	}
	return PreparedUnit{
		UnitID:          unitID,
		UnitKey:         definition.Key,
		FullText:        reading.FullText,
		SourceBytes:     []byte(reading.FullText),
		Result:          reading.Result,
		SyntaxState:     SyntaxStateAbsent,
		AnalyzerName:    ImportAnalyzerName,
		AnalyzerVersion: InterpretationVersion(definition),
		ConfigIdentity:  ImportConfigIdentity(definition),
		Report:          reading.Report,
	}, nil
}
