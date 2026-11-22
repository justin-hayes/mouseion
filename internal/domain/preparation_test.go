package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBookPreparationStageValuesAreStable(t *testing.T) {
	t.Parallel()
	for stage, want := range map[BookPreparationStage]string{
		BookPreparationStageNone:      "",
		BookPreparationStageEntry:     "entry",
		BookPreparationStageCatalog:   "catalog",
		BookPreparationStageWithdrawn: "withdrawn",
		BookPreparationStageFetch:     "fetch",
		BookPreparationStageSource:    "source",
		BookPreparationStageImport:    "import",
		BookPreparationStageAnalysis:  "analysis",
		BookPreparationStageFinalize:  "finalize",
	} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, string(stage))
		})
	}
}

func TestBookPreparationCodeValuesAreStable(t *testing.T) {
	t.Parallel()
	for code, want := range map[BookPreparationCode]string{
		BookPreparationCodeNone:                         "",
		BookPreparationCodeCatalogEntryMissing:          "catalog_entry_missing",
		BookPreparationCodeSourceDownloadFailed:         "source_download_failed",
		BookPreparationCodeSourcePinMismatch:            "source_pin_mismatch",
		BookPreparationCodeSourceOversize:               "source_oversize",
		BookPreparationCodeSourceLayerUnknown:           "source_layer_unknown",
		BookPreparationCodeSourceFormatUnknown:          "source_format_unknown",
		BookPreparationCodeSourceAdapterRefused:         "source_adapter_refused",
		BookPreparationCodeEPUBImportFailed:             "epub_import_failed",
		BookPreparationCodeClassicalWithdrawn:           "classical_definition_withdrawn",
		BookPreparationCodeClassicalCatalogLegacyRows:   "classical_catalog_legacy_rows",
		BookPreparationCodeClassicalFailed:              "classical_preparation_failed",
		BookPreparationCodeAnalysisFailed:               "analysis_failed",
		BookPreparationCodeDependencyParsingUnavailable: "dependency_parsing_unavailable",
		BookPreparationCodeAnalysisInterrupted:          "analysis_interrupted",
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, string(code))
		})
	}
}

func TestBookPreparationStageIsEPUBAcquisitionStage(t *testing.T) {
	t.Parallel()
	for stage, want := range map[BookPreparationStage]bool{
		BookPreparationStageEntry:       true,
		BookPreparationStageFetch:       true,
		BookPreparationStageSource:      true,
		BookPreparationStageImport:      true,
		BookPreparationStageNone:        false,
		BookPreparationStageCatalog:     false,
		BookPreparationStageWithdrawn:   false,
		BookPreparationStageAnalysis:    false,
		BookPreparationStageFinalize:    false,
		BookPreparationStage("unknown"): false,
	} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, stage.IsEPUBAcquisitionStage())
		})
	}
}

func TestClassicalPreparationStageForCodeTreatsEPUBCodesAsImportFailures(t *testing.T) {
	t.Parallel()
	for _, code := range []BookPreparationCode{
		BookPreparationCodeCatalogEntryMissing,
		BookPreparationCodeEPUBImportFailed,
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, BookPreparationStageImport, ClassicalPreparationStageForCode(code))
		})
	}
}
