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
	return token + csrfLanguageSeparator + canonicalization.NormalizeLanguage(view.pageLanguage())
}

func languageChangedPath() string {
	return "/library?message=" + url.QueryEscape(languageChangedMessage)
}

func redirectLanguageChanged(w http.ResponseWriter, r *http.Request) {
	redirectMyBooksBrowse(w, r, languageChangedPath())
}
