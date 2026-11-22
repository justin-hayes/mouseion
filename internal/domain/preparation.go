package domain

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

// sharedPreparationStage maps the codes both kinds define to their stage. It
// reports ok=false for a kind-specific code, so each kind keeps its own
// fallback and never reinterprets the other kind's codes.
func sharedPreparationStage(code BookPreparationCode) (BookPreparationStage, bool) {
	switch code {
	case BookPreparationCodeNone:
		return BookPreparationStageNone, true
	case BookPreparationCodeSourceDownloadFailed:
		return BookPreparationStageFetch, true
	case BookPreparationCodeSourceOversize, BookPreparationCodeSourceFormatUnknown:
		return BookPreparationStageSource, true
	default:
		return BookPreparationStageNone, false
	}
}

// ClassicalPreparationStageForCode returns the stage that owns a stable
// Classical preparation failure code. An unrecognised code is an import-stage
// failure: the source was acquired, but its analysis was not published.
func ClassicalPreparationStageForCode(code BookPreparationCode) BookPreparationStage {
	if stage, ok := sharedPreparationStage(code); ok {
		return stage
	}
	switch code {
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

// EPUBAcquisitionStageForCode returns the stage that owns a stable EPUB
// acquisition failure code. An unrecognised code is a fetch-stage failure: the
// acquisition stopped before the source could be imported.
func EPUBAcquisitionStageForCode(code BookPreparationCode) BookPreparationStage {
	if stage, ok := sharedPreparationStage(code); ok {
		return stage
	}
	switch code {
	case BookPreparationCodeCatalogEntryMissing:
		return BookPreparationStageEntry
	case BookPreparationCodeEPUBImportFailed:
		return BookPreparationStageImport
	default:
		return BookPreparationStageFetch
	}
}
