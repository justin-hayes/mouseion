package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	require.GreaterOrEqual(t, start, 0, "primary navigation missing: %s", html)
	end := strings.Index(html[start:], `</nav>`)
	require.GreaterOrEqual(t, end, 0, "primary navigation missing: %s", html)
	return html[start : start+end]
}

func TestAuthenticatedShellMarksEachPeerDestination(t *testing.T) {
	for _, test := range []struct {
		name        string
		context     NavigationContext
		currentLink string
		currentCSS  string
	}{
		{name: "reading", context: NavigationReading, currentLink: `href="/reading"`, currentCSS: "site-nav__link--current"},
		{name: "study", context: NavigationStudy, currentLink: `href="/study"`, currentCSS: "site-nav__link--current"},
		{name: "corpus", context: NavigationCorpus, currentLink: `href="/corpus"`, currentCSS: "site-nav__link--current"},
		{name: "catalog", context: NavigationCatalogs, currentLink: `href="/catalogs"`, currentCSS: "site-nav__link--current"},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderShell(t, test.context)
			navigation := renderedPrimaryNavigation(t, html)
			assert.Equal(t, 1, strings.Count(navigation, `aria-current="page"`), "aria-current count: %s", html)
			assert.True(t, strings.Contains(navigation, test.currentLink+` aria-current="page"`) && strings.Contains(navigation, test.currentCSS), "current destination missing semantic current class: %s", html)
			assert.Contains(t, html, `data-navigation-context="`+string(test.context)+`"`, "shell context missing: %s", html)
		})
	}
}

func TestAuthenticatedShellPreservesKeyboardOrderAndNativeControls(t *testing.T) {
	html := renderShell(t, NavigationReading)
	navigation := renderedPrimaryNavigation(t, html)
	ordered := []string{
		`<a class="site-header__brand" href="/">`,
		`<ul class="site-header__navigation">`,
		`href="/reading"`,
		`href="/study"`,
		`href="/corpus"`,
		`href="/catalogs"`,
		`</ul><div class="site-header__tools">`,
		`<details class="site-header__account">`,
	}
	previous := -1
	for _, fragment := range ordered {
		position := strings.Index(navigation, fragment)
		require.GreaterOrEqual(t, position, 0, "shell missing %q: %s", fragment, navigation)
		require.Greater(t, position, previous, "shell order moved %q before prior control", fragment)
		previous = position
	}
	assert.Contains(t, html, `aria-label="Primary navigation"`, "primary navigation name changed")
	assert.Equal(t, 1, strings.Count(html, `<nav `), "shell keeps one navigation landmark")
	assert.False(t, strings.Index(html, `class="skip-link"`) > strings.Index(html, `<nav class="site-header__nav"`) || strings.Index(html, `</nav>`) > strings.Index(html, `id="main-content"`), "skip link or main content moved out of keyboard/document order")
}

func TestAuthenticatedShellAccountDisclosureNamesLearnerAndKeepsLogoutNative(t *testing.T) {
	navigation := renderedPrimaryNavigation(t, renderShell(t, NavigationReading))
	assert.Equal(t, 1, strings.Count(navigation, `<details class="site-header__account">`))
	assert.Contains(t, navigation, `<span class="site-header__account-name">learner</span>`)
	assert.Contains(t, navigation, `<span class="site-header__account-compact">Account</span>`)
	assert.Contains(t, navigation, `Signed in as <strong>learner</strong>`)
	assert.Contains(t, navigation, `<summary`)
	assert.Contains(t, navigation, `<a href="/devices">Devices</a>`)
	assert.Contains(t, navigation, `<form method="post" action="/logout"`)
	assert.Contains(t, navigation, `<button class="button button--outline button--quiet" type="submit">Log out</button>`)
}

func TestAuthenticatedNavigationUsesLinksNotButtonControls(t *testing.T) {
	for _, context := range []NavigationContext{NavigationReading, NavigationStudy, NavigationCorpus, NavigationCatalogs} {
		t.Run(string(context), func(t *testing.T) {
			navigation := renderedPrimaryNavigation(t, renderShell(t, context))
			assert.NotContains(t, navigation, `class="site-nav__link btn`)
			assert.NotContains(t, navigation, `role="button"`)
			assert.Equal(t, 5, strings.Count(navigation, `<a href="/`), "four destinations and the account panel Devices link remain native links")
			assert.Contains(t, navigation, `<a class="site-header__brand" href="/">`)
		})
	}
}

func TestAuthenticatedShellCompactClassContract(t *testing.T) {
	html := renderShell(t, NavigationReading)
	assert.True(t, strings.Contains(html, `class="site-header__nav"`) && strings.Contains(html, `class="site-header__navigation"`), "shell compact layout hooks missing: %s", html)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/app.css", nil)
	response := httptest.NewRecorder()
	StaticHandler().ServeHTTP(response, request)
	css := response.Body.String()
	for _, want := range []string{
		".site-header__nav",
		".site-header__navigation",
		".site-header__account",
		"grid-template-columns: auto minmax(0, 1fr)",
		"@media (max-width: 40rem)",
		".site-nav__link--current",
		"box-shadow: inset 0 -2px 0 var(--mouseion-color-accent)",
		".site-header__language select",
		"min-height: 44px",
		".site-header a:focus-visible",
		".site-header__brand",
		"--mouseion-focus-width: 0.125rem",
		".table-region:focus-visible",
		"border-collapse: collapse",
		"padding: var(--mouseion-space-2) var(--mouseion-space-3)",
	} {
		assert.Contains(t, css, want, "compact/current class contract missing %q", want)
	}
}

func TestRootRedirectsToReading(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	(&Handler{}).dashboard(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	location := response.Header().Get("Location")
	assert.Equal(t, "/reading", location)
}

func TestStudyTabDueChip(t *testing.T) {
	render := func(t *testing.T, view *shellView) string {
		t.Helper()
		ctx := context.WithValue(context.Background(), shellViewContextKey{}, view)
		var output bytes.Buffer
		require.NoError(t, ShellLayout("Shell test", &domain.User{Username: "learner"}, "csrf", NavigationStudy).Render(ctx, &output))
		return renderedPrimaryNavigation(t, output.String())
	}
	for _, test := range []struct {
		name string
		view *shellView
		want string
	}{
		{name: "plural", view: &shellView{LastLanguage: "de", StudyDue: 12}, want: `aria-label="Study, 12 reviews due"`},
		{name: "singular", view: &shellView{LastLanguage: "de", StudyDue: 1}, want: `aria-label="Study, 1 review due"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			navigation := render(t, test.view)
			assert.Contains(t, navigation, test.want)
			assert.Contains(t, navigation, `<span class="site-nav__count" aria-hidden="true">`+strconv.Itoa(test.view.StudyDue)+`</span>`)
			assert.Equal(t, 1, strings.Count(navigation, `aria-current="page"`))
		})
	}
	for name, view := range map[string]*shellView{
		"zero":              {LastLanguage: "de"},
		"no study language": {},
		"no shell view":     nil,
	} {
		t.Run("absent with "+name, func(t *testing.T) {
			navigation := render(t, view)
			assert.NotContains(t, navigation, "site-nav__count")
			assert.NotContains(t, navigation, "aria-label=\"Study")
		})
	}
}
