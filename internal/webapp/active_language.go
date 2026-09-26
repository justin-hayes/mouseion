package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

type activeStudyLanguageOption struct {
	domain.StudyLanguage
	HasBooks   bool
	NewArrival bool
}

type shellView struct {
	ActiveLanguage string
	ReturnTo       string
	Options        []activeStudyLanguageOption
}

type shellViewContextKey struct{}

func activeStudyLanguageLabel(option activeStudyLanguageOption) string {
	label := option.DisplayName + " (" + option.Language + ")"
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

func (h *Handler) loadShellView(ctx context.Context, owner, returnTo string) (*shellView, error) {
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
		ReturnTo:       returnTo,
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

func shellReturnPath(r *http.Request) string {
	if r.Method == http.MethodGet {
		return r.URL.RequestURI()
	}
	referer, err := url.Parse(strings.TrimSpace(r.Header.Get("Referer")))
	if err == nil && referer.Path != "" {
		return webauth.SafeReturnPath(referer.RequestURI())
	}
	return "/library"
}

func (h *Handler) activeStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	language := canonicalization.NormalizeLanguage(strings.TrimSpace(r.FormValue("language")))
	if language == "" {
		redirect(w, r, activeStudyLanguageReturnPath(r.FormValue("return_to"), language))
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
	redirect(w, r, activeStudyLanguageReturnPath(r.FormValue("return_to"), language))
}

func activeStudyLanguageReturnPath(raw, language string) string {
	path := webauth.SafeReturnPath(strings.TrimSpace(raw))
	u, err := url.Parse(path)
	if err != nil {
		return "/"
	}
	legacyJourneyURL := ""
	if after, ok := strings.CutPrefix(u.Path, "/books/"); ok {
		bookID := after
		if bookID != "" && !strings.Contains(bookID, "/") {
			legacyJourneyURL = journeyEntryURL(bookID)
		}
	}
	if legacyJourneyURL != "" {
		if u.RawQuery != "" {
			base, fragment, hasFragment := strings.Cut(legacyJourneyURL, "#")
			if hasFragment {
				return base + "?" + u.RawQuery + "#" + fragment
			}
			return legacyJourneyURL + "?" + u.RawQuery
		}
		return legacyJourneyURL
	}
	if u.Path == "/library" || u.Path == "/reading" || u.Path == "/journey" || u.Path == "/vocabulary" {
		if u.Path == "/journey" {
			u.Path = "/reading"
		}
		query := u.Query()
		if u.Path == "/library" {
			query.Del("language")
		} else if u.Path == "/reading" {
			// An explicit language on a bookmark selects only that request. Once
			// the learner changes the saved mode, let the new active language win.
			query.Del("language")
		} else if u.Path == "/vocabulary" {
			query.Del("language")
		}
		u.RawQuery = query.Encode()
	}
	if result := u.RequestURI(); result != "" {
		if u.Fragment != "" {
			return result + "#" + url.PathEscape(u.Fragment)
		}
		return result
	}
	return "/"
}
