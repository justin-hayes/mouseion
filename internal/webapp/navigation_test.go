package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func renderShell(t *testing.T, navContext NavigationContext) string {
	t.Helper()
	var output bytes.Buffer
	if err := ShellLayout("Shell test", &domain.User{Username: "learner"}, "csrf", navContext).Render(context.Background(), &output); err != nil {
		t.Fatalf("render shell: %v", err)
	}
	return output.String()
}

func renderedPrimaryNavigation(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `<nav class="site-header__nav"`)
	if start < 0 {
		t.Fatalf("primary navigation missing: %s", html)
	}
	end := strings.Index(html[start:], `</nav>`)
	if end < 0 {
		t.Fatalf("primary navigation missing: %s", html)
	}
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
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderShell(t, test.context)
			navigation := renderedPrimaryNavigation(t, html)
			if got := strings.Count(navigation, `aria-current="page"`); got != 1 {
				t.Fatalf("aria-current count = %d, want 1: %s", got, html)
			}
			if !strings.Contains(navigation, test.currentLink+` aria-current="page"`) || !strings.Contains(navigation, test.currentCSS) {
				t.Fatalf("current destination missing semantic current class: %s", html)
			}
			if !strings.Contains(html, `data-navigation-context="`+string(test.context)+`"`) {
				t.Fatalf("shell context missing: %s", html)
			}
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
		`<form class="inline" method="post" action="/logout"`,
	}
	previous := -1
	for _, fragment := range ordered {
		position := strings.Index(navigation, fragment)
		if position < 0 {
			t.Fatalf("shell missing %q: %s", fragment, navigation)
		}
		if position <= previous {
			t.Fatalf("shell order moved %q before prior control", fragment)
		}
		previous = position
	}
	if !strings.Contains(html, `aria-label="Primary navigation"`) {
		t.Error("primary navigation name changed")
	}
	if strings.Index(html, `class="skip-link"`) > strings.Index(html, `<nav class="site-header__nav"`) || strings.Index(html, `</nav>`) > strings.Index(html, `id="main-content"`) {
		t.Error("skip link or main content moved out of keyboard/document order")
	}
}

func TestAuthenticatedShellCompactClassContract(t *testing.T) {
	html := renderShell(t, NavigationLibrary)
	if !strings.Contains(html, `class="site-header__nav"`) || !strings.Contains(html, `class="site-header__navigation"`) {
		t.Fatalf("shell compact layout hooks missing: %s", html)
	}

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
		if !strings.Contains(css, want) {
			t.Errorf("compact/current class contract missing %q", want)
		}
	}
}

func TestRootCompatibilityRedirectsToLibrary(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	(&Handler{}).dashboard(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("root status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/library" {
		t.Fatalf("root location = %q, want /library", location)
	}
}
