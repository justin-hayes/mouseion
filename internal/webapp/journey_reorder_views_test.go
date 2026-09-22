package webapp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJourneyPageRendersAccessibleReorderControls(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "ready")
	first := testJourneyBook("first", "First provisional book", "ready")
	second := testJourneyBook("second", "Second provisional book", "ready")
	first.Position, second.Position = 1, 2
	first.CanMoveLater, second.CanMoveEarlier = true, true
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: []journeyBookView{first, second}, Revision: 7}, "", "")

	for _, want := range []string{
		`id="provisional-journey-list"`,
		`role="status" aria-live="polite"`,
		`id="journey-book-first"`,
		`id="journey-book-second"`,
		`name="csrf_token" value="csrf-token"`,
		`name="expected_revision" value="7"`,
		`id="provisional-journey-content"`,
		`hx-target="#provisional-journey-content"`,
		`hx-swap="outerHTML"`,
		`data-journey-reorder`,
		`aria-label="Move First provisional book earlier"`,
		`aria-label="Move Second provisional book later"`,
		`<details class="more-actions"><summary>More actions</summary>`,
		`action="/journey/books/first/remove"`,
	} {
		assert.True(t, strings.Contains(html, want), "journey reorder markup missing %q: %s", want, html)
	}
	assert.Equal(t, 4, strings.Count(html, `name="expected_revision" value="7"`), "expected revision was not included in each move form: %s", html)
	firstCard := html[strings.Index(html, `id="journey-book-first"`):strings.Index(html, `id="journey-book-second"`)]
	secondCard := html[strings.Index(html, `id="journey-book-second"`):]
	assert.True(t, strings.Contains(firstCard, "disabled") && strings.Contains(secondCard, "disabled"), "first/last boundary controls were not disabled: first=%s second=%s", firstCard, secondCard)
	goalCard := html[strings.Index(html, `id="journey-book-goal"`):strings.Index(html, `id="provisional-journey-heading"`)]
	assert.False(t, strings.Contains(goalCard, "Move earlier") || strings.Contains(goalCard, "Move later"), "Primary Goal rendered reorder controls: %s", goalCard)
	assert.Contains(t, goalCard, `<details class="more-actions"><summary>More actions</summary>`)
	assert.Contains(t, goalCard, "Clear Primary Goal")
}

func TestJourneyForecastFailureKeepsSavedOrderActionable(t *testing.T) {
	view := journeyPageView{
		ForecastUnavailable: true,
		Provisional:         []journeyBookView{testJourneyBook("first", "First provisional book", "ready")},
	}
	html := renderJourney(t, view, "Moved First provisional book to position 1 in Your order. Coverage forecast unavailable; the saved order remains in place. Retry Reading Journey.", "")
	assert.Contains(t, html, "saved order remains in place")
	assert.Contains(t, html, `href="/journey">Retry forecast</a>`)
}
