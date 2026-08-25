package epub

import "github.com/justin-hayes/mouseion/internal/domain"

// ExtractedUnitsSchemaVersion identifies the serialization and semantic
// contract for an ExtractedUnits document. It is independent of EPUB versions.
const ExtractedUnitsSchemaVersion = domain.ExtractedUnitsSchemaVersion

// ErrExtractedUnitsUnavailable identifies a legacy source that has no usable
// extracted-unit envelope. Re-extraction, rather than an empty analysis scope,
// is the compatible interpretation.
var ErrExtractedUnitsUnavailable = domain.ErrExtractedUnitsUnavailable

const (
	UnitTitleHeading    = domain.UnitTitleHeading
	UnitTitleManifestID = domain.UnitTitleManifestID
)

// ExtractedUnits is the versioned, deterministic EPUB unit envelope. Extract
// will populate it in the implementation phase; zero-valued legacy data means
// that extracted units are unavailable and must not be interpreted as empty.
type ExtractedUnits = domain.ExtractedUnits

// ExtractedUnit describes one readable XHTML spine resource and its exact span
// in the compatibility FullText value. Offsets are half-open Unicode code-point
// offsets, not byte offsets.
type ExtractedUnit = domain.ExtractedUnit

// UnitID returns the v1 stable identity for an emitted spine item. The manifest
// ID is deliberately verbatim; spineIndex makes repeated itemrefs distinct.
func UnitID(spineIndex uint64, manifestID string) string {
	return domain.EPUBUnitID(spineIndex, manifestID)
}

// ValidateOffsets checks the non-negotiable relationship between ordered units
// and the existing FullText representation. Other EPUB validation remains the
// extractor's responsibility.
