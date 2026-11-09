package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClassicalPreparation struct {
	handle analysis.ClassicalPreparationHandle
	err    error
}

func (f fakeClassicalPreparation) SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error) {
	return f.handle, f.err
}

func classicalStatusBook(status domain.ClassicalPreparationStatus) domain.MyBook {
	return domain.MyBook{
		Book:                 domain.Book{ID: "greek", Title: "Anabasis", LanguageState: domain.LanguageChosen, LanguageTag: "grc"},
		Disposition:          domain.BookDispositionToRead,
		ReadingUnit:          &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: status.CurrentRevision, Withdrawn: status.Withdrawn},
		ClassicalPreparation: &status,
	}
}

func TestClassicalPreparationLearnerStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     domain.ClassicalPreparationStatus
		label      string
		recovery   string
		margin     string
		action     bool
		inProgress bool
	}{
		{name: "queued", status: domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationQueued, CurrentRevision: 1}, label: classicalAnalysisIncompleteLabel, margin: "Analysis incomplete.", inProgress: true},
		{name: "running", status: domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationRunning, CurrentRevision: 1}, label: classicalAnalysisIncompleteLabel, margin: "Analysis incomplete.", inProgress: true},
		{name: "fetch", status: domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureFetch, CurrentRevision: 1}, label: classicalAssessmentUnavailable, recovery: classicalRetryAcquisitionLabel, margin: "Assessment unavailable.", action: true},
		{name: "import", status: domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureImport, CurrentRevision: 1}, label: classicalAnalysisFailedLabel, recovery: classicalRetryAnalysisLabel, margin: "Analysis failed.", action: true},
		{name: "source", status: domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureSource, CurrentRevision: 1}, label: classicalSourceMismatchLabel, margin: "Source doesn't match its reviewed definition."},
		{name: "withdrawn", status: domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureWithdrawn, CurrentRevision: 1, Withdrawn: true}, label: classicalWithdrawnLabel, margin: "No longer in the Classical catalog."},
		{name: "cleared", status: domain.ClassicalPreparationStatus{Cleared: true, FailureRevision: 1, CurrentRevision: 2}, label: classicalAnalysisNotStarted, recovery: classicalRetryPreparationLabel, margin: "Analysis not started.", action: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book := classicalStatusBook(tt.status)
			view, ok := classicalPreparationFor(book)
			require.True(t, ok)
			assert.Equal(t, tt.label, view.Label)
			assert.Equal(t, tt.recovery, view.Recovery)
			assert.Equal(t, tt.action, view.Actionable)
			assert.Equal(t, tt.inProgress, view.InProgress)
			assert.Equal(t, tt.margin, myBookMarginEvidence(book))
			assert.Equal(t, tt.recovery, myBookEvidenceRecovery(book))
			if view.Actionable {
				assert.Equal(t, "/reading/books/greek/reanalyze", myBookEvidenceRecoveryURL(book))
			}
		})
	}
}

func TestClassicalPreparationRecoveryFromInboxMovesToToRead(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureFetch, CurrentRevision: 1})
	book.Disposition = domain.BookDispositionInbox
	assert.Equal(t, classicalRetryAcquisitionLabel+" by moving to To Read", myBookEvidenceRecovery(book))
	assert.Equal(t, "/library/books/greek/to-read", myBookEvidenceRecoveryURL(book))
}

func TestClassicalPreparationIgnoresPublishedAnalysis(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureSource, CurrentRevision: 1})
	book.Acquired = &domain.SourceMaterialSummary{
		Source:        domain.SourceMaterial{ID: "source", MediaType: domain.ImportedSourceMediaType, ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
		Signals:       testAnalyzed,
		AnalysisRunID: "run",
		CorpusID:      "corpus",
	}
	_, ok := classicalPreparationFor(book)
	assert.False(t, ok, "a published analysis is ordinary evidence, not a preparation failure")
}

func TestClassicalPreparationReadingActionAndLabel(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureFetch, CurrentRevision: 1})
	item := readingBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "greek", Title: "Anabasis"}}, BookID: "greek", ReadingUnit: book.ReadingUnit, ClassicalPreparation: book.ClassicalPreparation}
	assert.Equal(t, classicalAssessmentUnavailable, readingEvidenceLabel(item))
	assert.Contains(t, readingEvidenceDescription(item), "Retry acquisition")
	action := readingAnalysisAction(item)
	assert.Equal(t, classicalAssessmentUnavailable, action.Status)
	assert.Equal(t, classicalRetryAcquisitionLabel, action.Label)
	assert.Equal(t, "/reading/books/greek/reanalyze", action.URL)
	assert.True(t, action.Submit)

	terminal := readingBookView{Book: item.Book, BookID: "greek", ReadingUnit: book.ReadingUnit, ClassicalPreparation: &domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureSource, CurrentRevision: 1}}
	action = readingAnalysisAction(terminal)
	assert.Equal(t, classicalSourceMismatchLabel, action.Status)
	assert.Empty(t, action.Label, "a terminal source failure offers no retry")
	assert.False(t, action.Submit)
}

func TestClassicalPreparationRecoveryAfterCompletionOffersReadingAgain(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Cleared: true, FailureRevision: 1, CurrentRevision: 2})
	book.Disposition = domain.BookDispositionInbox
	book.CompletionCount = 1
	assert.Equal(t, classicalRetryPreparationLabel+" by reading again", myBookEvidenceRecovery(book))
	assert.Equal(t, "/library/books/greek/read-again", myBookEvidenceRecoveryURL(book))
}

func TestReadingClassicalWithdrawnFallbackAndCatalogState(t *testing.T) {
	t.Parallel()
	withdrawn := readingBookView{
		Book:        domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "greek"}},
		BookID:      "greek",
		ReadingUnit: &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 2, Withdrawn: true},
	}
	status, ok := readingClassicalPreparationStatus(withdrawn)
	require.True(t, ok)
	assert.True(t, status.Withdrawn)
	assert.Equal(t, classicalWithdrawnLabel, readingEvidenceLabel(withdrawn))
	assert.Equal(t, readingEvidenceFailed, readingEvidenceState(withdrawn))

	catalog := readingBookView{
		Book:   domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "greek"}},
		BookID: "greek",
		ClassicalPreparation: &domain.ClassicalPreparationStatus{
			Failure: domain.ClassicalPreparationFailureCatalog, CurrentRevision: 1,
		},
	}
	assert.Equal(t, readingEvidenceUnavailable, readingEvidenceState(catalog))
	assert.Empty(t, readingAnalysisAction(catalog).Label)
}

func TestRetryClassicalPreparationRoutesEveryOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		service    ClassicalPreparation
		wantError  string
		wantSubmit bool
	}{
		{name: "unavailable", service: nil, wantError: "Classical preparation is unavailable"},
		{name: "legacy", service: fakeClassicalPreparation{err: classical.ErrLegacyRows}, wantError: classical.LegacyRowsCode},
		{name: "withdrawn", service: fakeClassicalPreparation{err: analysis.ErrWithdrawnClassicalDefinition}, wantError: classicalWithdrawnLabel},
		{name: "generic failure", service: fakeClassicalPreparation{err: errors.New("boom")}, wantError: "Preparation could not be started"},
		{name: "complete", service: fakeClassicalPreparation{handle: analysis.ClassicalPreparationHandle{Complete: true}}},
		{name: "terminal", service: fakeClassicalPreparation{handle: analysis.ClassicalPreparationHandle{Terminal: true}}, wantError: classicalSourceMismatchLabel},
		{name: "queued", service: fakeClassicalPreparation{handle: analysis.ClassicalPreparationHandle{JobID: 7, Queued: true}}, wantSubmit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &Handler{services: Services{ClassicalPreparation: tt.service}}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/reading/books/greek/reanalyze", nil)
			h.retryClassicalPreparation(recorder, request, "owner", "greek")
			location := recorder.Header().Get("Location")
			if tt.wantError != "" {
				assert.Contains(t, location, "error=")
				assert.Contains(t, location, url.QueryEscape(tt.wantError))
				return
			}
			if tt.wantSubmit {
				assert.Contains(t, location, "message=")
				assert.Contains(t, location, "Preparation+job")
			}
		})
	}
}

func TestQueueClassicalPreparationOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		service ClassicalPreparation
		message string
	}{
		{name: "unavailable", service: nil, message: ""},
		{name: "legacy", service: fakeClassicalPreparation{err: classical.ErrLegacyRows}, message: classical.LegacyRowsReason},
		{name: "withdrawn", service: fakeClassicalPreparation{err: analysis.ErrWithdrawnClassicalDefinition}, message: classicalWithdrawnLabel},
		{name: "generic", service: fakeClassicalPreparation{err: errors.New("boom")}, message: "preparation could not start"},
		{name: "complete", service: fakeClassicalPreparation{handle: analysis.ClassicalPreparationHandle{Complete: true}}, message: "already current"},
		{name: "terminal", service: fakeClassicalPreparation{handle: analysis.ClassicalPreparationHandle{Terminal: true}}, message: classicalSourceMismatchLabel},
		{name: "queued", service: fakeClassicalPreparation{handle: analysis.ClassicalPreparationHandle{JobID: 7, Queued: true}}, message: "Preparation job #7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := &Handler{services: Services{ClassicalPreparation: tt.service}}
			message, ok := h.queueClassicalPreparation(context.Background(), "owner", "greek")
			if tt.service == nil {
				assert.False(t, ok)
				return
			}
			assert.True(t, ok)
			assert.Contains(t, message, tt.message)
		})
	}
}

func TestClassicalPreparationChooserRecovery(t *testing.T) {
	t.Parallel()
	h := &Handler{}
	candidate := readingChooserBookView{Book: classicalStatusBook(domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureImport, CurrentRevision: 1})}
	require.NoError(t, h.classifyReadingChooserCandidate(context.Background(), "owner", &candidate))
	assert.Equal(t, readingChooserNeedsAttention, candidate.State)
	assert.Equal(t, classicalRetryAnalysisLabel, candidate.RecoveryLabel)
	assert.True(t, candidate.RecoveryAllowed)

	terminal := readingChooserBookView{Book: classicalStatusBook(domain.ClassicalPreparationStatus{Failure: domain.ClassicalPreparationFailureSource, CurrentRevision: 1})}
	require.NoError(t, h.classifyReadingChooserCandidate(context.Background(), "owner", &terminal))
	assert.Equal(t, readingChooserNeedsAttention, terminal.State)
	assert.Empty(t, terminal.RecoveryLabel)
	assert.False(t, terminal.RecoveryAllowed)

	queued := readingChooserBookView{Book: classicalStatusBook(domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationQueued, CurrentRevision: 1})}
	require.NoError(t, h.classifyReadingChooserCandidate(context.Background(), "owner", &queued))
	assert.Equal(t, readingChooserInProgress, queued.State)
	assert.False(t, queued.RecoveryAllowed)
}
