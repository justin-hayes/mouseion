package webapp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type activeStudyLanguageOption struct {
	domain.StudyLanguage
	HasBooks   bool
	NewArrival bool
}

type shellView struct {
	ActiveLanguage string
	Options        []activeStudyLanguageOption
}

type shellViewContextKey struct{}

func activeStudyLanguageLabel(option activeStudyLanguageOption) string {
	label := option.DisplayName + " (" + option.Language + ")"
	switch option.Language {
	case "de":
		label = "Deutsch"
	case "it":
		label = "Italiano"
	case "el":
		label = "Ελληνικά"
	}
	if !option.HasBooks {
		label += " (no books)"
	}
	if option.NewArrival {
		label += " (new)"
	}
	return label
}

func shellViewFromContext(ctx context.Context) *shellView {
	view, ok := ctx.Value(shellViewContextKey{}).(*shellView)
	if !ok {
		return nil
	}
	return view
}

func activeStudyLanguageForContext(ctx context.Context) (language, label string) {
	view := shellViewFromContext(ctx)
	if view == nil {
		return "", ""
	}
	language = view.ActiveLanguage
	for _, option := range view.Options {
		if option.Language == language {
			return language, option.DisplayName
		}
	}
	return language, language
}

func (h *Handler) loadShellView(ctx context.Context, owner string) (*shellView, error) {
	studyLanguages, err := h.services.Store.StudyLanguages.ListStudyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	stored, err := h.services.Store.StudyLanguages.GetStoredActiveStudyLanguage(ctx, owner)
	if err != nil {
		return nil, err
	}
	recent, err := h.services.Store.StudyLanguages.MostRecentlyActivatedStudyLanguage(ctx, owner)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return nil, err
	}
	recent = canonicalization.NormalizeLanguage(recent)
	stored = canonicalization.NormalizeLanguage(stored)
	active := domain.ResolveActiveStudyLanguage(studyLanguages, stored, recent)
	// Historical vocabulary remains selectable so Vocabulary can show it read-only.
	if !studyLanguagePresent(studyLanguages, stored) && studyLanguagePresent(knownLanguages, stored) {
		active = stored
	}
	view := &shellView{
		ActiveLanguage: active,
	}
	for _, language := range studyLanguages {
		view.Options = append(view.Options, activeStudyLanguageOption{
			StudyLanguage: language,
			HasBooks:      true,
			NewArrival:    language.Language == recent && language.Language != view.ActiveLanguage,
		})
	}
	for _, language := range knownLanguages {
		if studyLanguagePresent(studyLanguages, language.Language) {
			continue
		}
		view.Options = append(view.Options, activeStudyLanguageOption{StudyLanguage: language})
	}
	return view, nil
}

func (h *Handler) activeStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	u := user(r)
	language := canonicalization.NormalizeLanguage(strings.TrimSpace(r.FormValue("language")))
	if language == "" {
		redirect(w, r, "/library")
		return
	}
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		http.Error(w, "choose a current study language", http.StatusBadRequest)
		return
	}
	if err = h.services.Store.StudyLanguages.SetActiveStudyLanguage(r.Context(), u.ID, language); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	// A deliberate change resets screen, filter, and return state: the learner
	// lands at My Books in the newly active language.
	redirect(w, r, "/library")
}
