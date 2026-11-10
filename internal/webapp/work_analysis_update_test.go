package webapp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func workUpdateCandidate(corrections, exclusions int) domain.AnalysisUpdateCandidate {
	return domain.AnalysisUpdateCandidate{
		PublishedRunID:       "published-run",
		Target:               testUpdateTarget,
		LemmaCorrections:     corrections,
		OccurrenceExclusions: exclusions,
	}
}

func workUpdateBook(id string, position int, current bool, candidate domain.AnalysisUpdateCandidate) domain.MyBook {
	return domain.MyBook{
		Book:             domain.Book{ID: id, Title: "Synthetic Work"},
		Disposition:      domain.BookDispositionToRead,
		IsCurrentReading: current,
		ReadingUnit: &domain.BookReadingUnit{
			WorkKey: "synthetic-work", WorkTitle: "Synthetic Work", ConstituentBook: position, ConstituentBooks: 24,
		},
		Acquired: &domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{ID: "source-" + id},
			Signals: domain.AnalysisSignals{
				Content:   domain.ContentCurrentImported,
				Published: domain.PublishedCurrent,
				LatestRun: domain.RunCompleted,
			},
			AnalysisUpdate: &candidate,
		},
	}
}

func TestWorkAnalysisUpdateGroupsEligibleBooksAndSkipsTheCurrentReading(t *testing.T) {
	first := workUpdateCandidate(0, 0)
	second := workUpdateCandidate(2, 1)
	third := workUpdateCandidate(1, 0)
	books := []domain.MyBook{
		workUpdateBook("book-1", 1, false, first),
		workUpdateBook("book-2", 2, true, second),
		workUpdateBook("book-3", 3, false, third),
	}

	update := workAnalysisUpdateFor("synthetic-work", "Synthetic Work", books, nil)
	require.Len(t, update.Books, 2, "the current Reading's Book is never part of the bulk action")
	require.NotNil(t, update.Current)
	assert.Equal(t, "book-2", update.Current.Book.ID)
	assert.Equal(t, 1, update.Corrections, "totals cover only the eligible Books")
	assert.Equal(t, 0, update.Exclusions)
	assert.Equal(t, "A newer analysis is available for 2 books.", update.Notice())
	assert.Equal(t, "Update 2 books of Synthetic Work", update.ActionLabel())
	assert.Equal(t, "Book 2: End the Reading first", update.CurrentText())
	assert.Contains(t, update.ConfirmationText(), "Together, the Books in this work have 1 confirmed lemma correction, which will not carry over to their new analyses.")
	assert.Contains(t, update.ConfirmationText(), workAnalysisUpdatePublished)
	assert.Contains(t, update.ConfirmationText(), workAnalysisUpdateFrozen)
	assert.Contains(t, update.ConfirmationText(), "Book 2: End the Reading first; nothing is queued for it.")
	assert.Equal(t, []workAnalysisUpdateLoss{
		{BookLabel: "Synthetic Work, Book 3", Text: "1 confirmed lemma correction"},
	}, update.Losses(), "the breakdown names only Books with losses")
}

func TestWorkAnalysisUpdateDoesNotDuplicateTheCurrentBookWhenItIsAlreadyAmongTheBooks(t *testing.T) {
	current := workUpdateBook("book-2", 2, true, workUpdateCandidate(0, 0))
	update := workAnalysisUpdateFor("synthetic-work", "Synthetic Work", []domain.MyBook{current}, &current)
	assert.Empty(t, update.Books)
	require.NotNil(t, update.Current)
	assert.Equal(t, "A newer analysis is available.", update.Notice(), "a Work with only a current Book keeps the ordinary notice")
	assert.Empty(t, update.Losses())
}

func TestWorkAnalysisUpdateOffersNothingWithoutCandidates(t *testing.T) {
	book := workUpdateBook("book-1", 1, false, workUpdateCandidate(0, 0))
	book.Acquired.AnalysisUpdate = nil
	update := workAnalysisUpdateFor("synthetic-work", "Synthetic Work", []domain.MyBook{book}, nil)
	assert.False(t, update.Offered())
	assert.NotContains(t, update.ConfirmationText(), "Together")
}

func TestWorkAnalysisUpdateExpectationsReadParallelFields(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/library/works/synthetic-work/analysis-update", nil)
	request.Form = url.Values{
		"book_id":         {"book-1", "book-2"},
		"source_id":       {"source-1", "source-2"},
		"expected_run_id": {"run-1", "run-2"},
		"expected_target": {"target-1", "target-2"},
	}
	expectations, ok := workAnalysisUpdateExpectations(request)
	require.True(t, ok)
	assert.Equal(t, workAnalysisUpdateExpectation{SourceID: "source-1", PublishedRunID: "run-1", Target: "target-1"}, expectations["book-1"])
	assert.Equal(t, workAnalysisUpdateExpectation{SourceID: "source-2", PublishedRunID: "run-2", Target: "target-2"}, expectations["book-2"])

	request.Form["expected_target"] = request.Form["expected_target"][:1]
	_, ok = workAnalysisUpdateExpectations(request)
	assert.False(t, ok, "an incomplete confirmation is refused rather than guessed")

	request.Form = url.Values{
		"book_id":         {"book-1", "book-1"},
		"source_id":       {"source-1", "source-1"},
		"expected_run_id": {"run-1", "run-1"},
		"expected_target": {"target-1", "target-1"},
	}
	_, ok = workAnalysisUpdateExpectations(request)
	assert.False(t, ok, "a duplicated Book is refused")
}

func TestWorkAnalysisUpdateFallsBackToTheCurrentBookTitle(t *testing.T) {
	book := workUpdateBook("book-1", 1, false, workUpdateCandidate(0, 0))
	update := workAnalysisUpdateFor("synthetic-work", "", []domain.MyBook{book}, nil)
	assert.Equal(t, "Synthetic Work", update.WorkTitle, "the reviewed Work title survives an empty label")

	book.ReadingUnit.WorkTitle = ""
	update = workAnalysisUpdateFor("synthetic-work", "", []domain.MyBook{book}, nil)
	assert.Equal(t, "Synthetic Work, Book 1", update.WorkTitle)

	current := workUpdateBook("book-2", 2, true, workUpdateCandidate(0, 0))
	current.ReadingUnit.WorkTitle = ""
	update = workAnalysisUpdateFor("synthetic-work", "", nil, &current)
	assert.Equal(t, "Synthetic Work, Book 2", update.WorkTitle)
	assert.Equal(t, "A newer analysis is available.", update.Notice())
	assert.Empty(t, workAnalysisUpdateFor("synthetic-work", "", nil, nil).WorkTitle)
}

func TestWorkAnalysisUpdateCurrentTextAndMessageBranches(t *testing.T) {
	assert.Empty(t, (workAnalysisUpdate{}).CurrentText())
	wholeWork := workUpdateBook("book-1", 0, true, workUpdateCandidate(0, 0))
	assert.Equal(t, "This Book: End the Reading first", workAnalysisUpdate{Current: &wholeWork}.CurrentText())

	assert.Equal(t, "No analysis update was queued.", workAnalysisUpdateMessage(0, 0, 0, 0))
	message := workAnalysisUpdateMessage(2, 1, 1, 1)
	assert.Contains(t, message, "Queued analysis updates for 2 books.")
	assert.Contains(t, message, "Skipped 1 book whose offer changed")
	assert.Contains(t, message, "Skipped 1 current Book; end the Reading first.")
	assert.Contains(t, message, "Could not update 1 book.")
}

func TestSubmitWorkAnalysisBookClassifiesIndependentOutcomes(t *testing.T) {
	available := workUpdateBook("book-1", 1, false, workUpdateCandidate(0, 0))
	expected := workAnalysisUpdateExpectation{SourceID: available.Acquired.Source.ID, PublishedRunID: "published-run", Target: testUpdateTarget.Key()}
	assert.Equal(t, workAnalysisUpdateQueued, (&Handler{services: Services{AnalysisUpdates: &fakeAnalysisUpdates{}}}).submitWorkAnalysisBook(t.Context(), fixtures.OwnerID, available, expected))

	wrongSource := expected
	wrongSource.SourceID = "other-source"
	assert.Equal(t, workAnalysisUpdateSkipped, (&Handler{}).submitWorkAnalysisBook(t.Context(), fixtures.OwnerID, available, wrongSource))

	current := workUpdateBook("book-2", 2, true, workUpdateCandidate(0, 0))
	currentExpected := workAnalysisUpdateExpectation{SourceID: current.Acquired.Source.ID, PublishedRunID: "published-run", Target: testUpdateTarget.Key()}
	assert.Equal(t, workAnalysisUpdateCurrentSkipped, (&Handler{}).submitWorkAnalysisBook(t.Context(), fixtures.OwnerID, current, currentExpected))

	noOffer := workUpdateBook("book-3", 3, false, workUpdateCandidate(0, 0))
	noOffer.Acquired.AnalysisUpdate = nil
	noOfferExpected := workAnalysisUpdateExpectation{SourceID: noOffer.Acquired.Source.ID, PublishedRunID: "published-run", Target: testUpdateTarget.Key()}
	assert.Equal(t, workAnalysisUpdateSkipped, (&Handler{}).submitWorkAnalysisBook(t.Context(), fixtures.OwnerID, noOffer, noOfferExpected))

	for _, tt := range []struct {
		name string
		err  error
		want workAnalysisUpdateResult
	}{
		{"current reading", analysis.ErrCurrentBook, workAnalysisUpdateCurrentSkipped},
		{"offer changed", analysis.ErrAnalysisUpdateChanged, workAnalysisUpdateSkipped},
		{"not found", analysis.ErrNotFound, workAnalysisUpdateSkipped},
		{"operational", errors.New("database down"), workAnalysisUpdateFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{services: Services{AnalysisUpdates: &fakeAnalysisUpdates{submitErr: tt.err}}}
			assert.Equal(t, tt.want, handler.submitWorkAnalysisBook(t.Context(), fixtures.OwnerID, available, expected))
		})
	}
}

func TestSubmitWorkAnalysisUpdatesCountsEachOutcome(t *testing.T) {
	current := workUpdateBook("book-1", 1, true, workUpdateCandidate(0, 0))
	available := workUpdateBook("book-2", 2, false, workUpdateCandidate(0, 0))
	expectations := map[string]workAnalysisUpdateExpectation{
		current.Book.ID:   {SourceID: current.Acquired.Source.ID, PublishedRunID: "published-run", Target: testUpdateTarget.Key()},
		available.Book.ID: {SourceID: available.Acquired.Source.ID, PublishedRunID: "published-run", Target: testUpdateTarget.Key()},
	}
	handler := &Handler{services: Services{AnalysisUpdates: &fakeAnalysisUpdates{submitErr: errors.New("database down")}}}
	outcome := handler.submitWorkAnalysisUpdates(t.Context(), fixtures.OwnerID, []domain.MyBook{current, available}, expectations)
	assert.Equal(t, 1, outcome.CurrentSkipped)
	assert.Equal(t, 1, outcome.Failed)
}

func TestWorkAnalysisUpdateExpectationsRejectBlankFields(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/library/works/synthetic-work/analysis-update", nil)
	request.Form = url.Values{
		"book_id":         {""},
		"source_id":       {"source-1"},
		"expected_run_id": {"run-1"},
		"expected_target": {"target-1"},
	}
	_, ok := workAnalysisUpdateExpectations(request)
	assert.False(t, ok)
}

func TestRedirectAnalysisUpdateNoticeRecoversFromAnUnparseableTarget(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), "POST", "/library/works/synthetic-work/analysis-update", nil)
	redirectAnalysisUpdateNotice(response, request, "://bad", "message", "queued")
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "/library?message=queued")
}

func TestAnalysisUpdateReturnURLKeepsOnlyLearnerSurfaces(t *testing.T) {
	assert.Equal(t, "/library?disposition=to_read", analysisUpdateReturnURL("/library?disposition=to_read"))
	assert.Equal(t, "/reading/switch", analysisUpdateReturnURL("/reading/switch"))
	assert.Equal(t, "/library", analysisUpdateReturnURL("https://example.com/elsewhere"))
	assert.Equal(t, "/library", analysisUpdateReturnURL("/jobs"))
}
