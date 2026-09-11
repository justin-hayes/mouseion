package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderShell(t *testing.T, navContext NavigationContext) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, ShellLayout("Shell test", &domain.User{Username: "learner"}, "csrf", navContext).Render(context.Background(), &output))
	return output.String()
}

func renderedPrimaryNavigation(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `<nav class="site-header__nav"`)
	require.True(t, start >= 0, "primary navigation missing: %s", html)
	end := strings.Index(html[start:], `</nav>`)
	require.True(t, end >= 0, "primary navigation missing: %s", html)
	return html[start : start+end]
}

func TestAuthenticatedShellMarksEachPeerDestination(t *testing.T) {
	for _, test := range []struct {
		name        string
		context     NavigationContext
		currentLink string
		currentCSS  string
	}{
		{name: "library", context: NavigationLibrary, currentLink: `href="/library"`, currentCSS: "site-nav__link--current"},
		{name: "learning", context: NavigationLearning, currentLink: `href="/journey"`, currentCSS: "site-nav__link--current"},
		{name: "vocabulary", context: NavigationVocabulary, currentLink: `href="/vocabulary"`, currentCSS: "site-nav__link--current"},
		{name: "catalogs", context: NavigationCatalogs, currentLink: `href="/catalogs"`, currentCSS: "site-nav__link--current"},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderShell(t, test.context)
			navigation := renderedPrimaryNavigation(t, html)
			assert.Equal(t, 1, strings.Count(navigation, `aria-current="page"`), "aria-current count: %s", html)
			assert.True(t, strings.Contains(navigation, test.currentLink+` aria-current="page"`) && strings.Contains(navigation, test.currentCSS), "current destination missing semantic current class: %s", html)
			assert.True(t, strings.Contains(html, `data-navigation-context="`+string(test.context)+`"`), "shell context missing: %s", html)
		})
	}
}

func TestAuthenticatedShellPreservesKeyboardOrderAndNativeControls(t *testing.T) {
	html := renderShell(t, NavigationLibrary)
	navigation := renderedPrimaryNavigation(t, html)
	ordered := []string{
		`<ul class="site-header__brand">`,
		`href="/"`,
		`href="/library"`,
		`href="/journey"`,
		`href="/vocabulary"`,
		`href="/catalogs"`,
		`<form class="inline" method="post" action="/logout"`,
	}
	previous := -1
	for _, fragment := range ordered {
		position := strings.Index(navigation, fragment)
		require.True(t, position >= 0, "shell missing %q: %s", fragment, navigation)
		require.True(t, position > previous, "shell order moved %q before prior control", fragment)
		previous = position
	}
	assert.True(t, strings.Contains(html, `aria-label="Primary navigation"`), "primary navigation name changed")
	assert.False(t, strings.Index(html, `class="skip-link"`) > strings.Index(html, `<nav class="site-header__nav"`) || strings.Index(html, `</nav>`) > strings.Index(html, `id="main-content"`), "skip link or main content moved out of keyboard/document order")
}

func TestAuthenticatedShellCompactClassContract(t *testing.T) {
	html := renderShell(t, NavigationLibrary)
	assert.True(t, strings.Contains(html, `class="site-header__nav"`) && strings.Contains(html, `class="site-header__navigation"`), "shell compact layout hooks missing: %s", html)

	request := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)
	css := response.Body.String()
	for _, want := range []string{
		".site-header__nav",
		".site-header__navigation",
		"flex-wrap: wrap",
		"@media (max-width: 40rem)",
		".site-nav__link--current",
		"text-decoration: underline",
	} {
		assert.True(t, strings.Contains(css, want), "compact/current class contract missing %q", want)
	}
}

func TestRootCompatibilityRedirectsToLibrary(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	(&Handler{}).dashboard(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	location := response.Header().Get("Location")
	assert.Equal(t, "/library", location)
}
