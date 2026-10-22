package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSourceStructures struct {
	offers    map[string]domain.SourceStructureUpdateCandidate
	offersErr error
	submitErr error
	submitted []domain.SourceStructureUpdateExpectation
}

func (f *fakeSourceStructures) SourceStructureUpdateCandidates(context.Context, string, []string) (map[string]domain.SourceStructureUpdateCandidate, error) {
	return f.offers, f.offersErr
}

func (f *fakeSourceStructures) SubmitSourceStructureUpdate(_ context.Context, _, _, _ string, expected domain.SourceStructureUpdateExpectation) (analysis.Handle, error) {
	f.submitted = append(f.submitted, expected)
	if f.submitErr != nil {
		return analysis.Handle{}, f.submitErr
	}
	return analysis.Handle{ID: 51, DisplayNumber: 9, RunID: "structure-run"}, nil
}

func structureUpdateCandidate() domain.SourceStructureUpdateCandidate {
	return domain.SourceStructureUpdateCandidate{
		ContentRevisionID: "revision-1", SnapshotID: "snapshot-1",
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2,
		ConventionSet:         domain.StructureConventionSet,
		ResultIdentity:        "sha256:checked",
		LemmaCorrections:      1, OccurrenceExclusions: 2,
	}
}

func TestReadingChooserOffersAConfirmedSourceStructureUpdate(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	sourceID := fixtureSourceID(t, store, "fixture-route-match")
	handler.services.SourceStructures = &fakeSourceStructures{offers: map[string]domain.SourceStructureUpdateCandidate{sourceID: structureUpdateCandidate()}}
	handler.services.AnalysisInsights = readingChooserInsights{Insights: fixtures.Insights{}}

	body := readingGet(t, h, "/reading", cookies)
	assert.Equal(t, 1, strings.Count(body, "A source structure update is available."), "only the checked Book shows the notice")
	assert.Contains(t, body, `<details class="confirmation`, "the confirmation is a native disclosure that works without JavaScript")
	assert.Contains(t, body, "Update source structure")
	assert.Contains(t, body, "switches it to the new source structure and queues its re-analysis")
	assert.Contains(t, body, "The current analysis becomes stale immediately and is replaced when the new analysis publishes.")
	assert.Contains(t, body, "Your 1 confirmed lemma correction and 2 occurrence exclusions on the current analysis do not apply to the new analysis.")
	assert.Contains(t, body, "Earlier Readings and prepared decks keep the evidence they froze.")
	assert.Contains(t, body, "There is no way back to the old structure.")
	assert.Contains(t, body, `action="/reading/books/fixture-route-match/source-structure-update"`)
	assert.Contains(t, body, `name="expected_content_revision_id" value="revision-1"`)
	assert.Contains(t, body, `name="expected_snapshot_id" value="snapshot-1"`)
	assert.Contains(t, body, `name="expected_contract_version" value="2"`)
	assert.Contains(t, body, `name="expected_convention_set" value="epub-structure/1"`)
	assert.Contains(t, body, `name="expected_result_identity" value="sha256:checked"`)
	assert.Contains(t, body, "Confirm source structure update")
	assert.Contains(t, body, "Confirm start reading", "the offer does not change readiness")
}

func TestCurrentReadingCanAdoptSourceStructureWithoutLosingEvidence(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	sourceID := fixtureSourceID(t, store, fixtures.BookID)
	handler.services.SourceStructures = &fakeSourceStructures{offers: map[string]domain.SourceStructureUpdateCandidate{sourceID: structureUpdateCandidate()}}

	body := readingGet(t, h, "/library", cookies)
	assert.Contains(t, body, "A source structure update is available.")
	assert.Contains(t, body, "Your active Reading keeps its exact current evidence, Reserved vocabulary and prepared deck; new evidence needs End, restart and re-preparation.")
	assert.Contains(t, body, "Confirm source structure update", "the current Reading may adopt; its own evidence stays pinned")
}

func TestSourceStructureUpdateConfirmationSubmitsTheCheckedOffer(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	updates := &fakeSourceStructures{}
	handler.services.SourceStructures = updates

	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/source-structure-update", url.Values{
		"csrf_token":                   {csrf},
		"expected_content_revision_id": {"revision-1"},
		"expected_snapshot_id":         {"snapshot-1"},
		"expected_contract_version":    {"2"},
		"expected_convention_set":      {domain.StructureConventionSet},
		"expected_result_identity":     {"sha256:checked"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/reading", location.Path)
	assert.Contains(t, location.Query().Get("message"), "adopted as analysis job #9")
	assert.Contains(t, location.Query().Get("message"), "Earlier Readings and decks keep the evidence they froze.")
	assert.Equal(t, []domain.SourceStructureUpdateExpectation{structureUpdateCandidate().Expectation()}, updates.submitted)
}

func TestSourceStructureUpdateConfirmationReportsRefusals(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"offer changed", analysis.ErrSourceStructureUpdateChanged, http.StatusSeeOther, sourceStructureUpdateChangedMessage},
		{"not found", analysis.ErrNotFound, http.StatusNotFound, ""},
		{"no EPUB", analysis.ErrEPUBRequired, http.StatusSeeOther, sourceStructureUpdateChangedMessage},
		{"missing units", domain.ErrExtractedUnitsUnavailable, http.StatusSeeOther, sourceStructureUpdateChangedMessage},
		{"operational", errors.New("database down"), http.StatusInternalServerError, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, cookies, csrf, store := readingFixtureSession(t)
			endGermanFixtureReading(t, store)
			handler, ok := h.(*Handler)
			require.True(t, ok)
			handler.services.SourceStructures = &fakeSourceStructures{submitErr: tt.err}

			response := readingTestRequest(t, h, "/reading/books/fixture-route-match/source-structure-update", url.Values{"csrf_token": {csrf}}, cookies)
			assert.Equal(t, tt.status, response.Code)
			if tt.message != "" {
				location, err := url.Parse(response.Header().Get("Location"))
				require.NoError(t, err)
				assert.Contains(t, location.Query().Get("error"), tt.message)
			}
		})
	}
}

//nolint:dupl // mirrors the Analysis update refusal contract with its own route and copy
func TestSourceStructureUpdateConfirmationWithoutASeamSubmitsNothing(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)

	t.Run("no structure seam", func(t *testing.T) {
		response := readingTestRequest(t, h, "/reading/books/fixture-route-match/source-structure-update", url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Contains(t, response.Header().Get("Location"), url.QueryEscape(sourceStructureUpdateChangedMessage))
	})
	t.Run("unknown Book", func(t *testing.T) {
		missing := readingTestRequest(t, h, "/reading/books/no-such-book/source-structure-update", url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equal(t, http.StatusNotFound, missing.Code)
	})
	t.Run("missing CSRF token", func(t *testing.T) {
		forged := readingTestRequest(t, h, "/reading/books/fixture-route-match/source-structure-update", url.Values{}, cookies)
		assert.Equal(t, http.StatusForbidden, forged.Code)
	})
}

func TestSourceStructureAnnotationSkipsBooksWithoutAcquiredSources(t *testing.T) {
	handler := &Handler{}
	handler.annotateSourceStructureUpdates(context.Background(), "owner", []domain.MyBook{{}})
	handler.services.SourceStructures = &fakeSourceStructures{}
	handler.annotateSourceStructureUpdates(context.Background(), "owner", []domain.MyBook{{}})
	book := domain.MyBook{Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source"}}}
	handler.services.SourceStructures = &fakeSourceStructures{offers: map[string]domain.SourceStructureUpdateCandidate{"source": structureUpdateCandidate()}}
	handler.annotateSourceStructureUpdates(context.Background(), "owner", []domain.MyBook{book})
}

func TestSourceStructureUpdateDecisionsText(t *testing.T) {
	for _, tt := range []struct {
		name                    string
		corrections, exclusions int
		want                    string
	}{
		{"none", 0, 0, "You have no confirmed lemma corrections or occurrence exclusions on the current analysis."},
		{"both", 2, 1, "Your 2 confirmed lemma corrections and 1 occurrence exclusion on the current analysis do not apply to the new analysis."},
		{"exclusions only", 0, 3, "Your 3 occurrence exclusions on the current analysis do not apply to the new analysis."},
		{"one correction", 1, 0, "Your 1 confirmed lemma correction on the current analysis does not apply to the new analysis."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sourceStructureUpdateDecisionsText(tt.corrections, tt.exclusions))
		})
	}
}

func TestMyBookSourceStructureUpdateNeedsAToReadCheckedOffer(t *testing.T) {
	assert.Nil(t, myBookSourceStructureUpdate(domain.MyBook{}))
	assert.Nil(t, myBookSourceStructureUpdate(domain.MyBook{Disposition: domain.BookDispositionInbox, Acquired: &domain.SourceMaterialSummary{SourceStructureUpdate: &domain.SourceStructureUpdateCandidate{}}}))
	book := domain.MyBook{Disposition: domain.BookDispositionToRead, Acquired: &domain.SourceMaterialSummary{SourceStructureUpdate: &domain.SourceStructureUpdateCandidate{SnapshotID: "snapshot"}}}
	require.NotNil(t, myBookSourceStructureUpdate(book))
	assert.Equal(t, "snapshot", myBookSourceStructureUpdate(book).SnapshotID)
}

func TestSourceStructureUpdateCandidateLookupFailureOffersNothing(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	handler.services.SourceStructures = &fakeSourceStructures{offersErr: errors.New("check store unavailable")}
	handler.services.AnalysisInsights = readingChooserInsights{Insights: fixtures.Insights{}}

	body := readingGet(t, h, "/reading", cookies)
	assert.NotContains(t, body, "A source structure update is available.")
	assert.Contains(t, body, "Confirm start reading")
}
