package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if metadataOnly, err := h.services.Store.IsMetadataOnlyMyBook(r.Context(), u.ID, r.PathValue("id")); err != nil {
		fail(w, err)
		return
	} else if metadataOnly {
		book, err := h.services.Store.GetBook(r.Context(), u.ID, r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		eligible, err := h.bookRefreshEligible(r.Context(), u.ID, book.ID)
		if err != nil {
			fail(w, err)
			return
		}
		var target *cataloguesync.AcquisitionTarget
		acquisitionToken := ""
		if provider, ok := h.services.CatalogueSync.(CatalogueAcquisitionTargetProvider); ok {
			if value, targetErr := provider.FindAcquisitionTarget(r.Context(), u.ID, book.ID); targetErr == nil {
				target = &value
				acquisitionToken = h.clientTargetToken(value.ConnectionID, value.Language, &value.Entry, value.Href)
			}
		}
		render(w, r, MetadataOnlyBookPageWithAcquisition(u, h.csrf(w, r), domain.MyBook{Book: book, EvidenceState: domain.MyBookNotAcquired}, r.URL.Query().Get("message"), eligible, target, acquisitionToken))
		return
	}
	summary, ok := h.loadBook(w, r, u.ID)
	if !ok {
		return
	}
	var coverage *domain.AnalysisCoverage
	statisticsUnavailable := false
	if summary.AnalysisStatus == "analyzed" && h.services.AnalysisInsights != nil {
		value, err := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, summary.CorpusID)
		if errors.Is(err, analysisinsights.ErrStatisticsUnavailable) {
			statisticsUnavailable = true
		} else if err != nil {
			fail(w, err)
			return
		} else {
			coverage = &value
		}
	}
	history, err := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	bookHistory := make([]domain.AnalysisJob, 0, len(history))
	for _, job := range history {
		if job.SourceMaterialID == summary.Source.ID {
			bookHistory = append(bookHistory, job)
		}
	}
	preparation, journeyAction, ok := h.currentBookPreparation(w, r, u.ID, summary)
	if !ok {
		return
	}
	render(w, r, BookPageWithHistoryAndPreparation(u, h.csrf(w, r), summary, coverage, statisticsUnavailable, bookHistory, r.URL.Query().Get("message"), preparation, journeyAction))
}

type catalogueAliasReader interface {
	GetBookCatalogEntryAlias(context.Context, string, string) (domain.BookAlias, error)
}

func (h *Handler) bookRefreshEligible(ctx context.Context, owner, bookID string) (bool, error) {
	reader, ok := h.services.Store.(catalogueAliasReader)
	if !ok {
		return false, nil
	}
	alias, err := reader.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if alias.AliasType != domain.AliasCatalogEntry || alias.Namespace != domain.NamespaceSourceIdentifier || strings.TrimSpace(alias.Value) == "" {
		return false, nil
	}
	connections, err := h.services.Store.ListOpdsConnections(ctx, owner)
	if err != nil {
		return false, err
	}
	return len(connections) > 0, nil
}

func (h *Handler) refreshBookMetadata(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	refresher, ok := h.services.CatalogueSync.(CatalogueMetadataRefresher)
	if !ok {
		h.renderBookRefreshFailure(w, r, u, r.PathValue("id"))
		return
	}
	result, err := refresher.RefreshEntry(r.Context(), u.ID, r.PathValue("id"))
	if errors.Is(err, cataloguesync.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		result.Failed = true
	}
	if result.Book.ID == "" {
		result.Book, _ = h.services.Store.GetBook(r.Context(), u.ID, r.PathValue("id"))
	}
	message := refreshMessage(result)
	if isHTMX(r) {
		if result.Book.ID == "" {
			http.NotFound(w, r)
			return
		}
		render(w, r, MetadataOnlyBookMetadataRegion(h.csrf(w, r), domain.MyBook{Book: result.Book, EvidenceState: domain.MyBookNotAcquired}, message))
		return
	}
	redirect(w, r, "/books/"+url.PathEscape(r.PathValue("id"))+"?message="+url.QueryEscape(message))
}

func (h *Handler) renderBookRefreshFailure(w http.ResponseWriter, r *http.Request, u domain.User, bookID string) {
	message := "Metadata could not be refreshed. Check the connection and try again."
	if isHTMX(r) {
		book, err := h.services.Store.GetBook(r.Context(), u.ID, bookID)
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		render(w, r, MetadataOnlyBookMetadataRegion(h.csrf(w, r), domain.MyBook{Book: book, EvidenceState: domain.MyBookNotAcquired}, message))
		return
	}
	redirect(w, r, "/books/"+url.PathEscape(bookID)+"?message="+url.QueryEscape(message))
}

func refreshMessage(result cataloguesync.RefreshResult) string {
	switch {
	case result.Updated:
		return "Metadata refreshed."
	case result.Missing:
		return "The catalogue entry is no longer available. Your book and its metadata are unchanged."
	case result.Failed:
		return "Metadata could not be refreshed. Check the connection and try again."
	default:
		return "Metadata is already up to date."
	}
}

func (h *Handler) analysisResult(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	book, ok := h.loadBookID(w, r, u.ID, r.PathValue("id"))
	if !ok {
		return
	}
	runID := r.PathValue("runID")
	redirectSourceID := book.Source.ID
	if book.AnalysisRunID != runID {
		jobs, err := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
		if err != nil {
			fail(w, err)
			return
		}
		found := false
		for _, job := range jobs {
			if job.SourceMaterialID == book.Source.ID && job.AnalysisRunID == runID {
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		if book.BookID != "" {
			candidates, err := h.services.Store.ListSourceMaterials(r.Context(), u.ID)
			if err != nil {
				fail(w, err)
				return
			}
			for _, candidate := range candidates {
				if candidate.BookID == book.BookID && candidate.AnalysisRunID != "" {
					redirectSourceID = candidate.Source.ID
					break
				}
			}
		}
	}
	http.Redirect(w, r, "/books/"+url.PathEscape(redirectSourceID), http.StatusSeeOther)
}

func (h *Handler) currentBookPreparation(w http.ResponseWriter, r *http.Request, owner string, book domain.SourceMaterialSummary) (*domain.DeckPreparation, deckJourneyActionView, bool) {
	journeyAction := emptyDeckJourneyAction()
	if book.AnalysisStatus != "analyzed" || book.AnalysisState != "completed" || book.AnalysisRunID == "" || h.services.PreparedDeck == nil {
		return nil, journeyAction, true
	}
	reader, ok := h.services.PreparedDeck.(PreparedDeckForAnalysis)
	if !ok {
		return nil, journeyAction, true
	}
	preparation, err := reader.GetForAnalysis(r.Context(), owner, book.Source.ID, book.AnalysisRunID)
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, journeyAction, true
	}
	if err != nil {
		handlePreparationError(w, r, err)
		return nil, journeyAction, false
	}
	if preparation.State == domain.DeckPreparationReady {
		journeyAction, err = h.deckJourneyAction(r.Context(), owner, preparation.ID, preparation.SourceMaterialID)
		if err != nil {
			fail(w, err)
			return nil, journeyAction, false
		}
	}
	return &preparation, journeyAction, true
}

func (h *Handler) resolveFullEPUBScope(ctx context.Context, owner string, book domain.SourceMaterial) (domain.EPUBReviewedScopeSnapshot, error) {
	snapshotID, units, err := h.services.Store.GetExtractedUnitSnapshot(ctx, owner, book.ID)
	if err != nil {
		return domain.EPUBReviewedScopeSnapshot{}, err
	}
	if existing, findErr := h.services.Store.FindFullBookScope(ctx, owner, book.ID, snapshotID); findErr == nil {
		return existing, nil
	} else if !errors.Is(findErr, persistence.ErrNotFound) {
		return domain.EPUBReviewedScopeSnapshot{}, findErr
	}
	selected := make([]domain.EPUBSelectedUnitReference, 0, len(units.Units))
	for _, unit := range units.Units {
		if strings.TrimSpace(unit.Text) != "" {
			selected = append(selected, domain.EPUBSelectedUnitReference{UnitID: unit.ID, Order: unit.Order})
		}
	}
	if len(selected) == 0 {
		return domain.EPUBReviewedScopeSnapshot{}, domain.ErrEPUBReviewedScopeUnavailable
	}
	return h.services.Store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{SchemaVersion: domain.EPUBReviewedScopeSchemaVersion, ScopeID: uuid.NewString(), OwnerID: owner, SourceMaterialID: book.ID, SourceContent: domain.EPUBContentRevisionIdentity{RevisionID: book.ContentRevisionID, Digest: book.ContentDigest, DigestVersion: book.ContentDigestVersion}, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: units.SchemaVersion}, SelectedUnits: selected})
}

func (h *Handler) analyzeBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	book, ok := h.loadBook(w, r, u.ID)
	if !ok {
		return
	}
	var handle analysis.Handle
	var err error
	if book.Source.MediaType == "application/epub+zip" {
		scope, scopeErr := h.resolveFullEPUBScope(r.Context(), u.ID, book.Source)
		if scopeErr != nil {
			if errors.Is(scopeErr, domain.ErrExtractedUnitsUnavailable) || errors.Is(scopeErr, domain.ErrEPUBReviewedScopeUnavailable) {
				http.Error(w, "Analysis is unavailable because this book has no extracted EPUB units.", http.StatusConflict)
				return
			}
			fail(w, scopeErr)
			return
		}
		handle, err = h.services.Analysis.SubmitScopedAnalysis(r.Context(), u.ID, book.Source.ID, scope.ScopeID)
	} else {
		handle, err = h.services.Analysis.SubmitAnalysis(r.Context(), u.ID, book.Source.ID)
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/books/%s?message=Analysis+job+%d+submitted", r.PathValue("id"), handle.DisplayNumber))
}
func (h *Handler) loadBook(w http.ResponseWriter, r *http.Request, owner string) (domain.SourceMaterialSummary, bool) {
	return h.loadBookID(w, r, owner, r.PathValue("id"))
}
func (h *Handler) loadBookID(w http.ResponseWriter, r *http.Request, owner, id string) (domain.SourceMaterialSummary, bool) {
	if strings.TrimSpace(id) == "" {
		http.NotFound(w, r)
		return domain.SourceMaterialSummary{}, false
	}
	books, err := h.services.Store.ListSourceMaterials(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return domain.SourceMaterialSummary{}, false
	}
	for _, book := range books {
		if book.Source.ID == id {
			return book, true
		}
	}
	http.NotFound(w, r)
	return domain.SourceMaterialSummary{}, false
}
