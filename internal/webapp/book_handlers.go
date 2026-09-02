package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	u := user(r)
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
	render(w, r, BookPageWithHistory(u, h.csrf(w, r), summary, coverage, statisticsUnavailable, bookHistory, r.URL.Query().Get("message")))
}

func (h *Handler) analysisResult(w http.ResponseWriter, r *http.Request) {
	reader, ok := h.services.Analysis.(CompletedAnalysisReader)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u := user(r)
	result, err := reader.GetCompletedAnalysis(r.Context(), u.ID, r.PathValue("id"), r.PathValue("runID"))
	if errors.Is(err, analysis.ErrNotFound) || errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if result.OwnerID != u.ID || result.SourceMaterialID != r.PathValue("id") || result.RunID != r.PathValue("runID") || result.ScopeID == "" {
		http.NotFound(w, r)
		return
	}
	var preparation *domain.DeckPreparation
	if reader, ok := h.services.PreparedDeck.(PreparedDeckForAnalysis); ok {
		value, preparationErr := reader.GetForAnalysis(r.Context(), u.ID, result.SourceMaterialID, result.RunID)
		if preparationErr == nil {
			preparation = &value
		} else if !errors.Is(preparationErr, persistence.ErrNotFound) {
			handlePreparationError(w, r, preparationErr)
			return
		}
	}
	var coverage *domain.AnalysisCoverage
	statisticsUnavailable := h.services.AnalysisInsights == nil
	if h.services.AnalysisInsights != nil {
		value, coverageErr := h.services.AnalysisInsights.Coverage(r.Context(), u.ID, result.Corpus.ID)
		if errors.Is(coverageErr, analysisinsights.ErrStatisticsUnavailable) {
			statisticsUnavailable = true
		} else if coverageErr != nil {
			fail(w, coverageErr)
			return
		} else {
			coverage = &value
		}
	}
	journeyAction := emptyDeckJourneyAction()
	if preparation != nil && preparation.State == domain.DeckPreparationReady {
		journeyAction, err = h.deckJourneyAction(r.Context(), u.ID, preparation.ID, preparation.SourceMaterialID)
		if err != nil {
			fail(w, err)
			return
		}
	}
	render(w, r, AnalysisResultPageWithPreparation(u, h.csrf(w, r), result, coverage, statisticsUnavailable, preparation, journeyAction))
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
