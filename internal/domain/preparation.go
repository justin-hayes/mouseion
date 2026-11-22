package domain

import "time"

// BookPreparationStage names one step of Book preparation. Classical
// preparation, EPUB acquisition and analysis share this vocabulary, so
// presentation and recovery read one set of stage names. The first six values
// describe a failure before an analysis run exists; Analysis and Finalize name
// the run and the work after it publishes.
type BookPreparationStage string

const (
	BookPreparationStageNone      BookPreparationStage = ""
	BookPreparationStageEntry     BookPreparationStage = "entry"
	BookPreparationStageCatalog   BookPreparationStage = "catalog"
	BookPreparationStageWithdrawn BookPreparationStage = "withdrawn"
	BookPreparationStageFetch     BookPreparationStage = "fetch"
	BookPreparationStageSource    BookPreparationStage = "source"
	BookPreparationStageImport    BookPreparationStage = "import"
	BookPreparationStageAnalysis  BookPreparationStage = "analysis"
	BookPreparationStageFinalize  BookPreparationStage = "finalize"
)

// BookPreparationKind names one kind of durable Book preparation shown in the
// one /jobs table. Analysis is listed beside the two preparation kinds because
// it is the second half of an EPUB preparation and the recorded outcome of a
// Classical preparation.
type BookPreparationKind string

const (
	BookPreparationKindClassical       BookPreparationKind = "classical_preparation"
	BookPreparationKindEPUBAcquisition BookPreparationKind = "epub_acquisition"
	BookPreparationKindAnalysis        BookPreparationKind = "analysis"
)

// BookPreparationCode is a stable Book preparation failure code. Classical
// preparation, EPUB acquisition and analysis record codes from this one
// vocabulary; the values are durable and never translated.
type BookPreparationCode string

const (
	BookPreparationCodeNone                       BookPreparationCode = ""
	BookPreparationCodeCatalogEntryMissing        BookPreparationCode = "catalog_entry_missing"
	BookPreparationCodeSourceDownloadFailed       BookPreparationCode = "source_download_failed"
	BookPreparationCodeSourcePinMismatch          BookPreparationCode = "source_pin_mismatch"
	BookPreparationCodeSourceOversize             BookPreparationCode = "source_oversize"
	BookPreparationCodeSourceLayerUnknown         BookPreparationCode = "source_layer_unknown"
	BookPreparationCodeSourceFormatUnknown        BookPreparationCode = "source_format_unknown"
	BookPreparationCodeSourceAdapterRefused       BookPreparationCode = "source_adapter_refused"
	BookPreparationCodeEPUBImportFailed           BookPreparationCode = "epub_import_failed"
	BookPreparationCodeClassicalWithdrawn         BookPreparationCode = "classical_definition_withdrawn"
	BookPreparationCodeClassicalCatalogLegacyRows BookPreparationCode = "classical_catalog_legacy_rows"
	BookPreparationCodeClassicalFailed            BookPreparationCode = "classical_preparation_failed"
	// Analysis failure codes stay on the analysis run and its attempt; they
	// are not written to the Book preparation failure record.
	BookPreparationCodeAnalysisFailed               BookPreparationCode = "analysis_failed"
	BookPreparationCodeDependencyParsingUnavailable BookPreparationCode = "dependency_parsing_unavailable"
	BookPreparationCodeAnalysisInterrupted          BookPreparationCode = "analysis_interrupted"
)

// IsEPUBAcquisitionStage reports whether an EPUB acquisition failure can record
// this stage. The other values belong to Classical preparation or to the
// analysis run that follows acquisition.
func (s BookPreparationStage) IsEPUBAcquisitionStage() bool {
	switch s {
	case BookPreparationStageEntry, BookPreparationStageFetch,
		BookPreparationStageSource, BookPreparationStageImport:
		return true
	default:
		return false
	}
}

// ClassicalPreparationStageForCode returns the stage that owns a stable
// Classical preparation failure code. An unrecognised code is an import-stage
// failure: the source was acquired, but its analysis was not published.
func ClassicalPreparationStageForCode(code BookPreparationCode) BookPreparationStage {
	switch code {
	case BookPreparationCodeNone:
		return BookPreparationStageNone
	case BookPreparationCodeSourceDownloadFailed:
		return BookPreparationStageFetch
	case BookPreparationCodeSourceOversize, BookPreparationCodeSourceFormatUnknown:
		return BookPreparationStageSource
	case BookPreparationCodeClassicalWithdrawn:
		return BookPreparationStageWithdrawn
	case BookPreparationCodeClassicalCatalogLegacyRows:
		return BookPreparationStageCatalog
	case BookPreparationCodeSourcePinMismatch, BookPreparationCodeSourceLayerUnknown,
		BookPreparationCodeSourceAdapterRefused:
		return BookPreparationStageSource
	case BookPreparationCodeClassicalFailed:
		return BookPreparationStageImport
	default:
		return BookPreparationStageImport
	}
}

// BookPreparationJob is one row of the /jobs Book preparations table: the one
// shape Classical preparation, EPUB acquisition and analysis jobs share. An
// EPUB preparation contributes two rows, its acquisition job and its analysis
// job. For an analysis row JobID is the analysis detail page's id and
// DisplayNumber distinguishes the run; the other kinds carry their River job
// id as row identity only, since they have no detail page. BookID and BookTitle
// identify the Book the row links to.
type BookPreparationJob struct {
	Kind          BookPreparationKind
	JobID         int64
	DisplayNumber int64
	BookID        string
	BookTitle     string
	// BookInMyBooks reports whether the Book is a current My Books member, so
	// its owner-scoped My Books link resolves. A removed Book keeps its
	// history row but renders without a link.
	BookInMyBooks bool
	Lifecycle     JobLifecycleState
	Stage         BookPreparationStage
	Code          BookPreparationCode
	// Progress is the run's percent where the kind reports one. Analysis is
	// the only kind that does, so a nil Progress renders no bar.
	Progress  *int
	CreatedAt time.Time
}
