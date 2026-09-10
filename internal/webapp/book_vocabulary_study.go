package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) attachVocabularyStudyPreparation(ctx context.Context, owner string, book domain.SourceMaterialSummary, preparation domain.DeckPreparation) (domain.DeckPreparation, error) {
	reader, ok := h.services.Store.(VocabularyStudyPreparationReader)
	if ok && preparation.VocabularyStudyStatus() == domain.VocabularyStudyNotStarted {
		active, err := reader.GetActiveDeckVocabularyStudy(ctx, owner, book.Source.ID)
		if err == nil {
			preparation = active
		} else if !errors.Is(err, persistence.ErrNotFound) {
			return domain.DeckPreparation{}, err
		}
	}
	if preparation.State == domain.DeckPreparationReady {
		if counter, ok := h.services.Store.(VocabularyStudyStore); ok {
			count, err := counter.CountDeckPreparationVocabularyToGraduate(ctx, owner, preparation.ID)
			if err != nil {
				return domain.DeckPreparation{}, err
			}
			preparation.VocabularyCount = count
		}
	}
	return preparation, nil
}

func (h *Handler) currentVocabularyStudyPreparation(ctx context.Context, owner string, book domain.SourceMaterialSummary, includeReady bool) (*domain.DeckPreparation, error) {
	if !bookHasCompletedAnalysis(book) {
		return nil, nil
	}
	reader, ok := h.services.Store.(VocabularyStudyPreparationReader)
	if !ok {
		return nil, nil
	}
	preparation, err := reader.GetDeckPreparationForAnalysis(ctx, owner, book.Source.ID, book.AnalysisRunID)
	if errors.Is(err, persistence.ErrNotFound) {
		preparation, err = reader.GetActiveDeckVocabularyStudy(ctx, owner, book.Source.ID)
	} else if err == nil && preparation.VocabularyStudyStatus() == domain.VocabularyStudyNotStarted && !includeReady {
		active, activeErr := reader.GetActiveDeckVocabularyStudy(ctx, owner, book.Source.ID)
		if activeErr == nil {
			preparation = active
		} else if !errors.Is(activeErr, persistence.ErrNotFound) {
			return nil, activeErr
		}
	}
	if errors.Is(err, persistence.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	preparation, err = h.attachVocabularyStudyPreparation(ctx, owner, book, preparation)
	if err != nil {
		return nil, err
	}
	return &preparation, nil
}

func (h *Handler) bookVocabularyStudyPreparation(w http.ResponseWriter, r *http.Request, allowActive bool) (domain.DeckPreparation, bool) {
	u := user(r)
	detail, ok := h.bookDetail(w, r, u.ID, r.PathValue("id"))
	if !ok || detail.Acquired == nil {
		if ok {
			http.NotFound(w, r)
		}
		return domain.DeckPreparation{}, false
	}
	if err := h.annotateBookWithJourneyLanguage(r.Context(), u.ID, detail.Book.LanguageTag, &detail); err != nil {
		fail(w, err)
		return domain.DeckPreparation{}, false
	}
	if !detail.JourneyMember || !bookHasCompletedAnalysis(*detail.Acquired) {
		http.NotFound(w, r)
		return domain.DeckPreparation{}, false
	}
	reader, ok := h.services.Store.(VocabularyStudyPreparationReader)
	if !ok {
		http.NotFound(w, r)
		return domain.DeckPreparation{}, false
	}
	preparation, err := reader.GetDeckPreparationForAnalysis(r.Context(), u.ID, detail.Acquired.Source.ID, detail.Acquired.AnalysisRunID)
	if errors.Is(err, persistence.ErrNotFound) && allowActive {
		preparation, err = reader.GetActiveDeckVocabularyStudy(r.Context(), u.ID, detail.Acquired.Source.ID)
	} else if err == nil && allowActive && preparation.VocabularyStudyStatus() == domain.VocabularyStudyNotStarted {
		active, activeErr := reader.GetActiveDeckVocabularyStudy(r.Context(), u.ID, detail.Acquired.Source.ID)
		if activeErr == nil {
			preparation = active
		} else if !errors.Is(activeErr, persistence.ErrNotFound) {
			fail(w, activeErr)
			return domain.DeckPreparation{}, false
		}
	}
	if errors.Is(err, persistence.ErrNotFound) {
		http.Error(w, "this Book has no current prepared deck", http.StatusConflict)
		return domain.DeckPreparation{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.DeckPreparation{}, false
	}
	preparation, err = h.attachVocabularyStudyPreparation(r.Context(), u.ID, *detail.Acquired, preparation)
	if err != nil {
		fail(w, err)
		return domain.DeckPreparation{}, false
	}
	return preparation, true
}

func (h *Handler) startBookVocabularyStudy(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	store, ok := h.services.Store.(VocabularyStudyStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	preparation, ok := h.bookVocabularyStudyPreparation(w, r, false)
	if !ok {
		return
	}
	alreadyStudying := preparation.VocabularyStudyStatus() == domain.VocabularyStudyStudying
	_, err := store.StartDeckVocabularyStudy(r.Context(), user(r).ID, preparation.ID)
	if errors.Is(err, persistence.ErrActiveVocabularyStudy) {
		redirectBookVocabularyStudyError(w, r, "Finish or release the other Book's vocabulary study before starting this one.")
		return
	}
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirectBookVocabularyStudyError(w, r, "Only a ready, non-empty deck can start a vocabulary study.")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	message := "Study this Book's vocabulary started. Its reserved vocabulary is not counted as known."
	if alreadyStudying {
		message = "This Book's vocabulary study is already in progress."
	}
	redirectBookVocabularyStudy(w, r, message)
}

func (h *Handler) confirmBookVocabularyReview(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	store, ok := h.services.Store.(VocabularyStudyStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	preparation, ok := h.bookVocabularyStudyPreparation(w, r, true)
	if !ok {
		return
	}
	count := preparation.VocabularyCount
	_, err := store.ConfirmDeckVocabularyReview(r.Context(), user(r).ID, preparation.ID)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirectBookVocabularyStudyError(w, r, "Start this Book's vocabulary study before confirming its deck review.")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirectBookVocabularyStudy(w, r, fmt.Sprintf("Deck review confirmed. %d vocabulary identities graduated to known vocabulary.", count))
}

func (h *Handler) releaseBookVocabularyStudy(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	store, ok := h.services.Store.(VocabularyStudyStore)
	if !ok {
		http.NotFound(w, r)
		return
	}
	preparation, ok := h.bookVocabularyStudyPreparation(w, r, true)
	if !ok {
		return
	}
	_, err := store.ReleaseDeckVocabularyStudy(r.Context(), user(r).ID, preparation.ID)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		redirectBookVocabularyStudyError(w, r, "A reviewed Book vocabulary study cannot be released.")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirectBookVocabularyStudy(w, r, "Vocabulary study released. Its provenance remains recorded and the vocabulary is eligible again.")
}

func redirectBookVocabularyStudy(w http.ResponseWriter, r *http.Request, message string) {
	redirect(w, r, "/journey/"+url.PathEscape(r.PathValue("id"))+"?message="+url.QueryEscape(message))
}

func redirectBookVocabularyStudyError(w http.ResponseWriter, r *http.Request, message string) {
	redirectBookVocabularyStudy(w, r, "Study action blocked: "+message)
}
