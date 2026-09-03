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
	"github.com/justin-hayes/mouseion/internal/epub"
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

type tocScopeView struct {
	Book       domain.SourceMaterial
	SnapshotID string
	History    []domain.AnalysisJob
	Choices    []tocScopeChoice
	UnitOrders map[string]uint64
}

type tocScopeChoice struct {
	Label          string
	UnitIDs        []string
	First          uint64
	CharacterCount int
	TokenEstimate  int
}

func (h *Handler) loadEPUBScope(w http.ResponseWriter, r *http.Request, owner string) (tocScopeView, bool) {
	book, err := h.services.Store.GetSourceMaterial(r.Context(), owner, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return tocScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return tocScopeView{}, false
	}
	snapshotID, units, err := h.services.Store.GetExtractedUnitSnapshot(r.Context(), owner, book.ID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		http.Error(w, "Review is unavailable because this book has no extracted EPUB units.", http.StatusConflict)
		return tocScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return tocScopeView{}, false
	}
	projected := epub.ProjectTOCChoices(book.Content, units)
	byID := make(map[string]domain.ExtractedUnit, len(units.Units))
	for _, unit := range units.Units {
		byID[unit.ID] = unit
	}
	view := tocScopeView{Book: book, SnapshotID: snapshotID, Choices: make([]tocScopeChoice, 0, len(projected)), UnitOrders: make(map[string]uint64, len(units.Units))}
	for _, unit := range units.Units {
		view.UnitOrders[unit.ID] = unit.Order
	}
	for _, choice := range projected {
		item := tocScopeChoice{Label: choice.Label, UnitIDs: append([]string(nil), choice.UnitIDs...), First: choice.First}
		for _, unitID := range choice.UnitIDs {
			unit, exists := byID[unitID]
			if !exists {
				fail(w, errors.New("review scope: TOC choice references an unknown unit"))
				return tocScopeView{}, false
			}
			characters := len([]rune(unit.Text))
			item.CharacterCount += characters
			item.TokenEstimate += (characters + 3) / 4
		}
		view.Choices = append(view.Choices, item)
	}
	jobs, err := h.services.Store.ListAnalysisJobs(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return tocScopeView{}, false
	}
	for _, job := range jobs {
		if job.SourceMaterialID == book.ID {
			view.History = append(view.History, job)
		}
	}
	return view, true
}

func (h *Handler) reviewEPUBScope(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	view, ok := h.loadEPUBScope(w, r, u.ID)
	if !ok {
		return
	}
	render(w, r, EPUBScopeReviewPage(u, h.csrf(w, r), view, "", ""))
}

func (h *Handler) confirmEPUBScope(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	view, ok := h.loadEPUBScope(w, r, u.ID)
	if !ok {
		return
	}
	if submittedSnapshot := r.FormValue("snapshot_id"); submittedSnapshot == "" || submittedSnapshot != view.SnapshotID {
		h.renderEPUBScopeError(w, r, u, view, "The EPUB unit snapshot changed. Reload the page and review the current scope.")
		return
	}
	selected := make(map[string]struct{}, len(r.Form["unit_id"]))
	for _, value := range r.Form["unit_id"] {
		for _, id := range strings.Split(value, ",") {
			if id == "" {
				h.renderEPUBScopeError(w, r, u, view, "The selection contains an invalid unit reference. Please review your selection.")
				return
			}
			if _, duplicate := selected[id]; duplicate {
				h.renderEPUBScopeError(w, r, u, view, "A unit was submitted more than once. Please review your selection.")
				return
			}
			selected[id] = struct{}{}
		}
	}
	references := make([]domain.EPUBSelectedUnitReference, 0, len(selected))
	for _, choice := range view.Choices {
		for _, id := range choice.UnitIDs {
			if _, included := selected[id]; !included {
				continue
			}
			order, exists := view.UnitOrders[id]
			if !exists {
				h.renderEPUBScopeError(w, r, u, view, "The selection contains a unit that does not belong to this book.")
				return
			}
			references = append(references, domain.EPUBSelectedUnitReference{UnitID: id, Order: order})
		}
	}
	if len(references) != len(selected) {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a unit that does not belong to this book.")
		return
	}
	if len(references) == 0 {
		h.renderEPUBScopeError(w, r, u, view, "Select at least one readable unit before confirming the analysis scope.")
		return
	}
	for i := range references {
		if i > 0 && references[i].Order <= references[i-1].Order {
			h.renderEPUBScopeError(w, r, u, view, "A unit was submitted more than once. Please review your selection.")
			return
		}
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: domain.EPUBReviewedScopeSchemaVersion, ScopeID: uuid.NewString(), OwnerID: u.ID, SourceMaterialID: view.Book.ID, SourceContent: domain.EPUBContentRevisionIdentity{RevisionID: view.Book.ContentRevisionID, Digest: view.Book.ContentDigest, DigestVersion: view.Book.ContentDigestVersion}, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: view.SnapshotID, ExtractedUnitsSchemaVersion: domain.ExtractedUnitsSchemaVersion}, SelectedUnits: references}
	if _, err := h.services.Store.CreateEPUBReviewedScope(r.Context(), scope); err != nil {
		h.renderEPUBScopeError(w, r, u, view, "The scope could not be saved. Reload the page and review the current units.")
		return
	}
	redirect(w, r, "/books/"+view.Book.ID+"?message="+url.QueryEscape(fmt.Sprintf("Analysis scope saved with %d selected units. Start analysis when you are ready.", len(references))))
}

func (h *Handler) renderEPUBScopeError(w http.ResponseWriter, r *http.Request, u domain.User, view tocScopeView, message string) {
	w.WriteHeader(http.StatusBadRequest)
	render(w, r, EPUBScopeReviewPage(u, h.csrf(w, r), view, message, "submitted:"+strings.Join(r.Form["unit_id"], ",")))
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
	scopeID := book.ConfirmedScopeID
	if scopeID == "" {
		scopeID = book.ReviewedScopeID
	}
	if book.Source.MediaType == "application/epub+zip" && scopeID == "" {
		redirect(w, r, "/books/"+book.Source.ID+"/scope")
		return
	}
	if scopeID != "" {
		handle, err = h.services.Analysis.SubmitScopedAnalysis(r.Context(), u.ID, book.Source.ID, scopeID)
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
