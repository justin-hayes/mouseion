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
	render(w, r, AnalysisResultPageWithPreparation(u, h.csrf(w, r), result, coverage, statisticsUnavailable, preparation))
}

type epubScopeUnitView struct {
	Unit           domain.ExtractedUnit
	Classification domain.EPUBUnitClassification
	CharacterCount int
	TokenEstimate  int
}

type epubScopeView struct {
	Book                   domain.SourceMaterial
	SnapshotID             string
	History                []domain.AnalysisJob
	Units                  []epubScopeUnitView
	Groups                 []epubScopeGroupView
	DegradedRecommendation bool
	AllCharacters          int
	AllTokens              int
	PriorScope             *domain.EPUBReviewedScopeSnapshot
	Preset                 string
	Comparison             epubScopeComparison
}

type epubScopeComparison struct {
	Old, New, Added, Removed                           []epubScopeUnitView
	OldCharacters, OldTokens, NewCharacters, NewTokens int
}

type epubScopeGroupView struct {
	Group          epub.UnitGroup
	CharacterCount int
	TokenEstimate  int
}

func (h *Handler) loadEPUBScope(w http.ResponseWriter, r *http.Request, owner string) (epubScopeView, bool) {
	book, err := h.services.Store.GetSourceMaterial(r.Context(), owner, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return epubScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
	}
	snapshotID, units, err := h.services.Store.GetExtractedUnitSnapshot(r.Context(), owner, book.ID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		http.Error(w, "Review is unavailable because this book has no extracted EPUB units.", http.StatusConflict)
		return epubScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
	}
	classifications, err := h.services.Store.GetEPUBUnitClassifications(r.Context(), owner, book.ID, epub.ClassifierName, epub.ClassifierVersion)
	if errors.Is(err, domain.ErrEPUBClassificationsUnavailable) {
		http.Error(w, "Review is unavailable because this book has no unit classifications.", http.StatusConflict)
		return epubScopeView{}, false
	}
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
	}
	if len(units.Units) != len(classifications) {
		fail(w, errors.New("review scope: incomplete persisted classifications"))
		return epubScopeView{}, false
	}
	view := epubScopeView{Book: book, SnapshotID: snapshotID, Units: make([]epubScopeUnitView, len(units.Units))}
	for i, unit := range units.Units {
		if classifications[i].SourceUnitSnapshot.UnitID != unit.ID || classifications[i].SourceUnitSnapshot.SnapshotID != snapshotID {
			fail(w, errors.New("review scope: classification snapshot mismatch"))
			return epubScopeView{}, false
		}
		characters := len([]rune(unit.Text))
		view.Units[i] = epubScopeUnitView{Unit: unit, Classification: classifications[i], CharacterCount: characters, TokenEstimate: (characters + 3) / 4}
		view.AllCharacters += view.Units[i].CharacterCount
		view.AllTokens += view.Units[i].TokenEstimate
	}
	hasHighConfidenceMain := false
	for _, item := range view.Units {
		if item.Classification.Category == domain.EPUBCategoryMainMatter && item.Classification.Confidence >= domain.EPUBConfidenceHighMinimum {
			hasHighConfidenceMain = true
			break
		}
	}
	view.DegradedRecommendation = !hasHighConfidenceMain
	byID := make(map[string]epubScopeUnitView, len(view.Units))
	for _, item := range view.Units {
		byID[item.Unit.ID] = item
	}
	for _, group := range epub.BuildUnitGroups(units.Units) {
		item := epubScopeGroupView{Group: group}
		for _, id := range group.UnitIDs {
			item.CharacterCount += byID[id].CharacterCount
			item.TokenEstimate += byID[id].TokenEstimate
		}
		view.Groups = append(view.Groups, item)
	}
	jobs, err := h.services.Store.ListAnalysisJobs(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return epubScopeView{}, false
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
	selected, err := h.applyScopePreset(r, u.ID, &view)
	if err != nil {
		h.renderEPUBScopeError(w, r, u, view, err.Error())
		return
	}
	render(w, r, EPUBScopeReviewPage(u, h.csrf(w, r), view, "", selected))
}

func (h *Handler) applyScopePreset(r *http.Request, owner string, view *epubScopeView) (string, error) {
	preset := r.URL.Query().Get("preset")
	priorID := r.URL.Query().Get("prior_scope_id")
	if preset == "" && priorID == "" {
		return "", nil
	}
	if preset != "recommended" && preset != "all" && preset != "prior" {
		return "", errors.New("The requested scope preset is invalid.")
	}
	view.Preset = preset
	var prior domain.EPUBReviewedScopeSnapshot
	if priorID != "" {
		var err error
		prior, err = h.services.Store.GetEPUBReviewedScope(r.Context(), owner, view.Book.ID, priorID)
		if err != nil {
			return "", errors.New("The prior scope is unavailable for this book and owner.")
		}
		if prior.SchemaVersion != domain.EPUBReviewedScopeSchemaVersion || prior.SourceUnitSnapshot.SnapshotID != view.SnapshotID || prior.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != domain.ExtractedUnitsSchemaVersion || prior.Classifier.Name != epub.ClassifierName || prior.Classifier.Version != epub.ClassifierVersion {
			return "", errors.New("The prior scope is stale or uses a different schema or classifier. Review the current units instead.")
		}
		view.PriorScope = &prior
	}
	if preset == "prior" && priorID == "" {
		return "", errors.New("A prior scope ID is required for the prior-selection preset.")
	}
	selected := make(map[string]bool, len(view.Units))
	for _, item := range view.Units {
		selected[item.Unit.ID] = preset == "all" || (preset == "recommended" && item.Classification.RecommendedInclusion)
	}
	if preset == "prior" {
		for _, item := range prior.SelectedUnits {
			selected[item.UnitID] = true
		}
	}
	var ids []string
	old := map[string]bool{}
	for _, item := range prior.SelectedUnits {
		old[item.UnitID] = true
	}
	for _, item := range view.Units {
		if selected[item.Unit.ID] {
			ids = append(ids, item.Unit.ID)
			view.Comparison.New = append(view.Comparison.New, item)
			view.Comparison.NewCharacters += item.CharacterCount
			view.Comparison.NewTokens += item.TokenEstimate
		}
		if old[item.Unit.ID] {
			view.Comparison.Old = append(view.Comparison.Old, item)
			view.Comparison.OldCharacters += item.CharacterCount
			view.Comparison.OldTokens += item.TokenEstimate
		}
		if selected[item.Unit.ID] && !old[item.Unit.ID] {
			view.Comparison.Added = append(view.Comparison.Added, item)
		}
		if old[item.Unit.ID] && !selected[item.Unit.ID] {
			view.Comparison.Removed = append(view.Comparison.Removed, item)
		}
	}
	return "submitted:" + strings.Join(ids, ","), nil
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
		h.renderEPUBScopeError(w, r, u, view, "The EPUB unit snapshot changed. Reload the page and review the current hierarchy.")
		return
	}
	selected := make(map[string]struct{}, len(r.Form["unit_id"]))
	for _, id := range r.Form["unit_id"] {
		if _, duplicate := selected[id]; duplicate {
			h.renderEPUBScopeError(w, r, u, view, "A unit was submitted more than once. Please review your selection.")
			return
		}
		selected[id] = struct{}{}
	}
	groups := make(map[string]epub.UnitGroup, len(view.Groups))
	for _, item := range view.Groups {
		groups[item.Group.ID] = item.Group
	}
	includeGroups, ok := submittedGroups(r.Form["group_include"], groups)
	if !ok {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a group that does not belong to this book.")
		return
	}
	excludeGroups, ok := submittedGroups(r.Form["group_exclude"], groups)
	if !ok {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a group that does not belong to this book.")
		return
	}
	for id := range includeGroups {
		if _, contradictory := excludeGroups[id]; contradictory {
			h.renderEPUBScopeError(w, r, u, view, "A group cannot be included and excluded in the same submission.")
			return
		}
		for _, unitID := range groups[id].UnitIDs {
			selected[unitID] = struct{}{}
		}
	}
	for id := range excludeGroups {
		for _, unitID := range groups[id].UnitIDs {
			delete(selected, unitID)
		}
	}
	references := make([]domain.EPUBSelectedUnitReference, 0, len(selected))
	recommended := true
	for _, item := range view.Units {
		_, included := selected[item.Unit.ID]
		if included {
			references = append(references, domain.EPUBSelectedUnitReference{UnitID: item.Unit.ID, Order: item.Unit.Order})
			delete(selected, item.Unit.ID)
		}
		if included != item.Classification.RecommendedInclusion {
			recommended = false
		}
	}
	if len(selected) != 0 {
		h.renderEPUBScopeError(w, r, u, view, "The selection contains a unit that does not belong to this book.")
		return
	}
	if len(references) == 0 {
		h.renderEPUBScopeError(w, r, u, view, "Select at least one readable unit before confirming the analysis scope.")
		return
	}
	mode := domain.EPUBScopeSelectionOverridden
	if recommended {
		mode = domain.EPUBScopeSelectionRecommended
	}
	scope := domain.EPUBReviewedScopeSnapshot{SchemaVersion: domain.EPUBReviewedScopeSchemaVersion, ScopeID: uuid.NewString(), OwnerID: u.ID, SourceMaterialID: view.Book.ID, SourceContent: domain.EPUBContentRevisionIdentity{RevisionID: view.Book.ContentRevisionID, Digest: view.Book.ContentDigest, DigestVersion: view.Book.ContentDigestVersion}, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: view.SnapshotID, ExtractedUnitsSchemaVersion: domain.ExtractedUnitsSchemaVersion}, Classifier: domain.EPUBClassifierIdentity{Name: epub.ClassifierName, Version: epub.ClassifierVersion}, SelectionMode: mode, SelectedUnits: references}
	if _, err := h.services.Store.CreateEPUBReviewedScope(r.Context(), scope); err != nil {
		h.renderEPUBScopeError(w, r, u, view, "The scope could not be saved. Reload the page and review the current units.")
		return
	}
	redirect(w, r, "/books/"+view.Book.ID+"?message="+url.QueryEscape(fmt.Sprintf("Analysis scope saved with %d selected units. Start analysis when you are ready.", len(references))))
}

func submittedGroups(values []string, groups map[string]epub.UnitGroup) (map[string]struct{}, bool) {
	result := make(map[string]struct{}, len(values))
	for _, id := range values {
		if _, exists := groups[id]; !exists {
			return nil, false
		}
		if _, duplicate := result[id]; duplicate {
			return nil, false
		}
		result[id] = struct{}{}
	}
	return result, true
}

func (h *Handler) renderEPUBScopeError(w http.ResponseWriter, r *http.Request, u domain.User, view epubScopeView, message string) {
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
