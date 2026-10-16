package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

// ImportedSourceMediaType is the media type of a pinned imported classical
// annotation (a CoNLL-U file). It is deliberately not an EPUB type: the import
// path never treats the source as an EPUB, but its current content admits a
// Reading like an EPUB analysis does.
const ImportedSourceMediaType = "application/x-conllu"

// Work is the author and title of a classical text, independent of any edition
// or learner. Operator registration stores it as shared reference data.
type Work struct {
	ID            string
	DefinitionKey string
	Title         string
	Author        string
	CreatedAt     time.Time
}

// Edition is one reviewed edition of a Work. The label names the editor and
// year the learner sees, for example "Marchant 1904".
type Edition struct {
	ID            string
	WorkID        string
	DefinitionKey string
	Label         string
	CreatedAt     time.Time
}

// ReadingUnit is one citation-range slice of an Edition, with the pinned source
// file and interpretation version an import uses. It carries no learner state.
type ReadingUnit struct {
	ID                    string
	EditionID             string
	DefinitionKey         string
	CitationStart         string
	CitationEnd           string
	SourceFileName        string
	SourceFileSize        int64
	SourceFileSHA256      string
	InterpretationVersion string
	CreatedAt             time.Time
}

// BookReadingUnit is the immutable association of one Book to the Reading unit
// it was registered for, with the edition label My Books shows.
type BookReadingUnit struct {
	OwnerID       string
	BookID        string
	ReadingUnitID string
	CitationStart string
	CitationEnd   string
	EditionLabel  string
}

// ReadingUnitDefinition is compiled, reviewed data for one operator-registered
// text. Adding a text means adding a value, not code.
type ReadingUnitDefinition struct {
	// Key is the stable definition identifier; it is also the Reading unit's
	// key, so repeated registrations upsert the same row.
	Key string
	// Language is the canonical language tag of the text, for example "grc".
	Language string
	// WorkTitle and WorkAuthor are the Work's title and author.
	WorkTitle  string
	WorkAuthor string
	// EditionLabel names the editor and year, for example "Marchant 1904".
	EditionLabel string
	// CitationStart and CitationEnd are the inclusive citation range of the
	// Reading unit, for example "1.1.1" and "1.10.19".
	CitationStart string
	CitationEnd   string
	// SourceFileName, SourceFileSize and SourceFileSHA256 pin the exact file a
	// later import reads. SHA256 is 64 lowercase hex characters.
	SourceFileName   string
	SourceFileSize   int64
	SourceFileSHA256 string
	// InterpretationVersion names the versioned interpretation recipe. A new
	// version produces a new analysis run on import.
	InterpretationVersion string
}

// Validate reports whether a compiled definition is complete and canonical.
func (d ReadingUnitDefinition) Validate() error {
	switch {
	case strings.TrimSpace(d.Key) == "":
		return errors.New("domain: reading unit definition key is required")
	case strings.TrimSpace(d.Language) != d.Language || d.Language == "":
		return errors.New("domain: reading unit definition language is required")
	case d.Language != canonicalization.NormalizeLanguage(d.Language):
		return errors.New("domain: reading unit definition language must be canonical")
	case strings.TrimSpace(d.WorkTitle) == "":
		return errors.New("domain: reading unit definition work title is required")
	case strings.TrimSpace(d.WorkAuthor) == "":
		return errors.New("domain: reading unit definition work author is required")
	case strings.TrimSpace(d.EditionLabel) == "":
		return errors.New("domain: reading unit definition edition label is required")
	case strings.TrimSpace(d.CitationStart) == "":
		return errors.New("domain: reading unit definition citation start is required")
	case strings.TrimSpace(d.CitationEnd) == "":
		return errors.New("domain: reading unit definition citation end is required")
	case strings.TrimSpace(d.SourceFileName) == "":
		return errors.New("domain: reading unit definition source file name is required")
	case d.SourceFileSize <= 0:
		return errors.New("domain: reading unit definition source file size must be positive")
	case !isLowerHexSHA256(d.SourceFileSHA256):
		return errors.New("domain: reading unit definition source file SHA-256 must be 64 lowercase hex characters")
	case strings.TrimSpace(d.InterpretationVersion) == "":
		return errors.New("domain: reading unit definition interpretation version is required")
	}
	return nil
}

// isLowerHexSHA256 reports whether value is exactly 64 lowercase hexadecimal
// characters, the stored form of a SHA-256 digest.
func isLowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// ReadingUnitBookRegistration is the outcome of registering a definition for
// one owner: the Book's identifier and whether this run created it.
type ReadingUnitBookRegistration struct {
	BookID  string
	Created bool
}

// LegacyLanguageRows counts rows carrying an existing language identity that a
// registration refuses to mix with its own. Surface names the table, and
// Count is the row count for one owner on that surface.
type LegacyLanguageRows struct {
	OwnerID string
	Surface string
	Count   int64
}
