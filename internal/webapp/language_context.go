package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

const (
	// csrfLanguageSeparator splits the CSRF secret from the study language the
	// form was rendered for. Base64url secrets never contain it.
	csrfLanguageSeparator = "."

	languageChangedMessage = "Your study language changed. This is My Books in your current study language; nothing was started, changed, or submitted."
)

// languageBoundCSRFToken returns the form token for the page being rendered.
// The suffix records the active study language so a stale tab's submission is
// recognized without every form carrying its own language field.
func languageBoundCSRFToken(ctx context.Context, token string) string {
	view := shellViewFromContext(ctx)
	if view == nil {
		return token
	}
	return token + csrfLanguageSeparator + canonicalization.NormalizeLanguage(view.ActiveLanguage)
}

func formLanguageIsActive(ctx context.Context, formLanguage string) bool {
	view := shellViewFromContext(ctx)
	if view == nil {
		return true
	}
	return canonicalization.NormalizeLanguage(formLanguage) == canonicalization.NormalizeLanguage(view.ActiveLanguage)
}

func languageChangedPath() string {
	return "/library?message=" + url.QueryEscape(languageChangedMessage)
}

func redirectLanguageChanged(w http.ResponseWriter, r *http.Request) {
	redirectMyBooksBrowse(w, r, languageChangedPath())
}

// languageScopedPath reports whether a GET names study-language-scoped state
// whose language identity must agree with the active study language.
func languageScopedPath(path string) bool {
	for _, prefix := range []string{"/library", "/reading", "/vocabulary"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// requestNamesOtherLanguage reports whether a supported request carries an
// explicit language that is no longer the active study language.
func requestNamesOtherLanguage(r *http.Request, view *shellView) bool {
	if view == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) || !languageScopedPath(r.URL.Path) {
		return false
	}
	requested := canonicalization.NormalizeLanguage(strings.TrimSpace(r.URL.Query().Get("language")))
	return requested != "" && requested != canonicalization.NormalizeLanguage(view.ActiveLanguage)
}
