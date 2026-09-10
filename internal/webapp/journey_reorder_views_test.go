package webapp

import (
	"strings"
	"testing"
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
		`hx-target="#provisional-journey-list"`,
		`hx-swap="outerHTML"`,
		`aria-label="Move First provisional book earlier"`,
		`aria-label="Move Second provisional book later"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("journey reorder markup missing %q: %s", want, html)
		}
	}
	if strings.Count(html, `name="expected_revision" value="7"`) != 4 {
		t.Fatalf("expected revision was not included in each move form: %s", html)
	}
	firstCard := html[strings.Index(html, `id="journey-book-first"`):strings.Index(html, `id="journey-book-second"`)]
	secondCard := html[strings.Index(html, `id="journey-book-second"`):]
	if !strings.Contains(firstCard, "disabled") || !strings.Contains(secondCard, "disabled") {
		t.Fatalf("first/last boundary controls were not disabled: first=%s second=%s", firstCard, secondCard)
	}
	goalCard := html[strings.Index(html, `id="journey-book-goal"`):strings.Index(html, `id="provisional-journey-heading"`)]
	if strings.Contains(goalCard, "Move earlier") || strings.Contains(goalCard, "Move later") {
		t.Fatalf("Primary Goal rendered reorder controls: %s", goalCard)
	}
}
