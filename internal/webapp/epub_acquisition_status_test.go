package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func acquisitionFailureBook(status domain.EPUBAcquisitionStatus) domain.MyBook {
	return domain.MyBook{
		Book:            domain.Book{ID: "book-1", Title: "Der lange Weg", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		Disposition:     domain.BookDispositionToRead,
		EPUBAcquisition: &status,
	}
}

func TestEPUBAcquisitionFailurePresentation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   domain.EPUBAcquisitionStatus
		evidence string
		recovery string
	}{
		{
			name:     "entry missing",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.EPUBAcquisitionFailureEntry, Code: domain.EPUBAcquisitionCodeCatalogEntryMissing},
			evidence: epubAcquisitionEntryMissingLabel,
			recovery: epubAcquisitionRetryLabel,
		},
		{
			name:     "download failed",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.EPUBAcquisitionFailureFetch, Code: domain.EPUBAcquisitionCodeSourceDownloadFailed},
			evidence: epubAcquisitionDownloadFailedLabel,
			recovery: epubAcquisitionRetryLabel,
		},
		{
			name:     "oversize source",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.EPUBAcquisitionFailureSource, Code: domain.EPUBAcquisitionCodeSourceOversize},
			evidence: epubAcquisitionOversizeLabel,
			recovery: "",
		},
		{
			name:     "unknown source format",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.EPUBAcquisitionFailureSource, Code: domain.EPUBAcquisitionCodeSourceFormatUnknown},
			evidence: epubAcquisitionFormatLabel,
			recovery: "",
		},
		{
			name:     "import failed",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.EPUBAcquisitionFailureImport, Code: domain.EPUBAcquisitionCodeEPUBImportFailed},
			evidence: epubAcquisitionImportFailedLabel,
			recovery: epubAcquisitionRetryLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book := acquisitionFailureBook(tt.status)
			assert.Equal(t, tt.evidence, myBookMarginEvidence(book))
			assert.Equal(t, tt.recovery, myBookEvidenceRecovery(book))
			if tt.recovery != "" {
				assert.Equal(t, "/reading/books/book-1/reanalyze", myBookEvidenceRecoveryURL(book))
			}
		})
	}
}

func TestEPUBAcquisitionFailureDoesNotOverrideAnAnalysisInEffect(t *testing.T) {
	t.Parallel()
	book := acquisitionFailureBook(domain.EPUBAcquisitionStatus{
		Stage: domain.EPUBAcquisitionFailureFetch, Code: domain.EPUBAcquisitionCodeSourceDownloadFailed,
	})
	book.Acquired = &domain.SourceMaterialSummary{
		Source:  domain.SourceMaterial{MediaType: "application/epub+zip"},
		Signals: domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunCompleted},
	}
	assert.NotEqual(t, epubAcquisitionDownloadFailedLabel, myBookMarginEvidence(book))
	assert.Empty(t, myBookEvidenceRecovery(book))
}

func TestEPUBAcquisitionFailureDoesNotPresentOutsideToRead(t *testing.T) {
	t.Parallel()
	book := acquisitionFailureBook(domain.EPUBAcquisitionStatus{
		Stage: domain.EPUBAcquisitionFailureFetch, Code: domain.EPUBAcquisitionCodeSourceDownloadFailed,
	})
	book.Disposition = domain.BookDispositionInbox
	assert.Empty(t, myBookMarginEvidence(book))
	assert.Empty(t, myBookEvidenceRecovery(book))

	book.CompletionCount = 1
	assert.Empty(t, myBookMarginEvidence(book))
	assert.Empty(t, myBookEvidenceRecovery(book))
}
