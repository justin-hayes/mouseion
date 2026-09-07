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
	view, _ := ctx.Value(shellViewContextKey{}).(*shellView)
	return view
}

func (h *Handler) loadShellView(ctx context.Context, owner, returnTo string) (*shellView, error) {
	studyLanguages, err := h.services.Store.ListStudyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	knownLanguages, err := h.services.Store.ListKnownVocabularyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	stored, err := h.services.Store.GetStoredActiveStudyLanguage(ctx, owner)
	if err != nil {
		return nil, err
	}
	recent, err := h.services.Store.MostRecentlyActivatedStudyLanguage(ctx, owner)
	if err != nil && !errors.Is(err, persistence.ErrNotFound) {
		return nil, err
	}
	recent = canonicalization.NormalizeLanguage(recent)
	view := &shellView{
		ActiveLanguage: domain.ResolveActiveStudyLanguage(studyLanguages, canonicalization.NormalizeLanguage(stored), canonicalization.NormalizeLanguage(recent)),
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
	languages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !studyLanguagePresent(languages, language) {
		http.Error(w, "choose a current study language", http.StatusBadRequest)
		return
	}
	if err = h.services.Store.SetActiveStudyLanguage(r.Context(), u.ID, language); err != nil {
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
	if u.Path == "/library" || u.Path == "/journey" || u.Path == "/vocabulary" {
		query := u.Query()
		if query.Get("language") != "" && (u.Path == "/library" || u.Path == "/vocabulary") {
			query.Set("language", language)
		}
		u.RawQuery = query.Encode()
	}
	if result := u.RequestURI(); result != "" {
		return result
	}
	return "/"
}
