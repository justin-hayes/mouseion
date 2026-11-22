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
		BookPreparationCodeNone:                       "",
		BookPreparationCodeCatalogEntryMissing:        "catalog_entry_missing",
		BookPreparationCodeSourceDownloadFailed:       "source_download_failed",
		BookPreparationCodeSourcePinMismatch:          "source_pin_mismatch",
		BookPreparationCodeSourceOversize:             "source_oversize",
		BookPreparationCodeSourceLayerUnknown:         "source_layer_unknown",
		BookPreparationCodeSourceFormatUnknown:        "source_format_unknown",
		BookPreparationCodeSourceAdapterRefused:       "source_adapter_refused",
		BookPreparationCodeEPUBImportFailed:           "epub_import_failed",
		BookPreparationCodeClassicalWithdrawn:         "classical_definition_withdrawn",
		BookPreparationCodeClassicalCatalogLegacyRows: "classical_catalog_legacy_rows",
		BookPreparationCodeClassicalFailed:            "classical_preparation_failed",
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, string(code))
		})
	}
}

func TestBookPreparationStageForCodeSharedValues(t *testing.T) {
	t.Parallel()
	for code, want := range map[BookPreparationCode]BookPreparationStage{
		BookPreparationCodeNone:                 BookPreparationStageNone,
		BookPreparationCodeSourceDownloadFailed: BookPreparationStageFetch,
		BookPreparationCodeSourceOversize:       BookPreparationStageSource,
		BookPreparationCodeSourceFormatUnknown:  BookPreparationStageSource,
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, want, ClassicalPreparationStageForCode(code))
			assert.Equal(t, want, EPUBAcquisitionStageForCode(code))
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

func TestBookPreparationStageForCodeKeepsEachKindsFallback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		code      BookPreparationCode
		classical BookPreparationStage
		epub      BookPreparationStage
	}{
		{
			name: "unrecognised", code: "an_unrecognised_failure_code",
			classical: BookPreparationStageImport, epub: BookPreparationStageFetch,
		},
		{
			name: "EPUB entry code is unrecognised by Classical", code: BookPreparationCodeCatalogEntryMissing,
			classical: BookPreparationStageImport, epub: BookPreparationStageEntry,
		},
		{
			name: "EPUB import code is unrecognised by Classical", code: BookPreparationCodeEPUBImportFailed,
			classical: BookPreparationStageImport, epub: BookPreparationStageImport,
		},
		{
			name: "Classical withdrawn code is unrecognised by EPUB", code: BookPreparationCodeClassicalWithdrawn,
			classical: BookPreparationStageWithdrawn, epub: BookPreparationStageFetch,
		},
		{
			name: "Classical catalog code is unrecognised by EPUB", code: BookPreparationCodeClassicalCatalogLegacyRows,
			classical: BookPreparationStageCatalog, epub: BookPreparationStageFetch,
		},
		{
			name: "Classical source code is unrecognised by EPUB", code: BookPreparationCodeSourcePinMismatch,
			classical: BookPreparationStageSource, epub: BookPreparationStageFetch,
		},
		{
			name: "Classical failure code is unrecognised by EPUB", code: BookPreparationCodeClassicalFailed,
			classical: BookPreparationStageImport, epub: BookPreparationStageFetch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.classical, ClassicalPreparationStageForCode(tt.code))
			assert.Equal(t, tt.epub, EPUBAcquisitionStageForCode(tt.code))
		})
	}
}
