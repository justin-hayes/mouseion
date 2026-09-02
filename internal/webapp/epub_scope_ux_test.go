package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestEPUBScopeReviewRendersAllOnTOCChecklist(t *testing.T) {
	view := tocScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: "Libro"}, SnapshotID: "snapshot-1", Choices: []tocScopeChoice{
		{Label: "Part One", UnitIDs: []string{"unit-0", "unit-1"}, First: 0, CharacterCount: 100, TokenEstimate: 25},
		{Label: "Appendix", UnitIDs: []string{"unit-2"}, First: 2, CharacterCount: 40, TokenEstimate: 10},
	}}
	var output bytes.Buffer
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{`name="snapshot_id" value="snapshot-1"`, "Part One", "Appendix", `value="unit-0,unit-1" checked`, `value="unit-2" checked`, "Check all", "Uncheck all", "All units: 140 characters, about 35 tokens", `aria-live="polite"`, `aria-atomic="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("checklist missing %q: %s", want, body)
		}
	}
	if got := strings.Count(body, `type="checkbox"`); got != 2 {
		t.Fatalf("checkbox count=%d, want 2", got)
	}
	for _, forbidden := range []string{"Category", "Confidence", "Recommendation", "Classification evidence", "scope-groups", "Scope comparison", "How recommendations work", "Limited structural confidence"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("classifier-era UI still contains %q", forbidden)
		}
	}
}

func TestEPUBScopeReviewErrorIsAccessibleAndPreservesSelection(t *testing.T) {
	view := tocScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: "Libro"}, Choices: []tocScopeChoice{{Label: "Part One", UnitIDs: []string{"unit-0"}, First: 0}}}
	var output bytes.Buffer
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "Select at least one readable unit before confirming the analysis scope.", "submitted:unit-0").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{`role="alert"`, `tabindex="-1"`, "Scope not saved.", "Select at least one readable unit", `value="unit-0" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("error render missing %q: %s", want, body)
		}
	}
}

func TestEPUBScopeReviewFlatFallbackUsesReadableUnitLabels(t *testing.T) {
	view := tocScopeView{Book: domain.SourceMaterial{ID: "book-flat", Title: "Legacy flat EPUB"}, Choices: []tocScopeChoice{
		{Label: "Chapter", UnitIDs: []string{"unit-0"}, First: 0, CharacterCount: 24, TokenEstimate: 6},
		{Label: "unit-1", UnitIDs: []string{"unit-1"}, First: 1, CharacterCount: 12, TokenEstimate: 3},
	}}
	var output strings.Builder
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{"Chapter", "unit-1", `value="unit-0" checked`, `value="unit-1" checked`, "Readable sections in spine order"} {
		if !strings.Contains(body, want) {
			t.Errorf("flat fallback missing %q: %s", want, body)
		}
	}
}

func TestEPUBScopeReviewUsesNativeBulkControlsAndLiveSummary(t *testing.T) {
	view := tocScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: "Libro"}, Choices: []tocScopeChoice{{Label: "Chapter", UnitIDs: []string{"unit-0"}, First: 0}}}
	var output strings.Builder
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{`<label for="scope-choice-0"`, `id="scope-choice-0"`, `<fieldset class="scope-actions">`, `<legend>Choose readable sections</legend>`, `data-select="all"`, `data-select="none"`, "scopeForm.addEventListener('change'", "scopeForm.addEventListener('click'", `<button type="submit">Confirm scope</button>`} {
		if !strings.Contains(body, want) {
			t.Errorf("accessible checklist missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `name="text"`) || strings.Contains(body, `tabindex="0"`) || strings.Contains(body, "onkeydown=") {
		t.Fatal("checklist submitted text or replaced native keyboard semantics")
	}
}

func TestBookPageLabelsHistoricalScopedAndLegacyFullTextCorpora(t *testing.T) {
	book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "book-1", Title: "Book", Language: "de", MediaType: "application/epub+zip"}}
	for _, test := range []struct {
		name     string
		coverage domain.AnalysisCoverage
		want     []string
	}{
		{"historical scope", domain.AnalysisCoverage{ReviewedScopeID: "scope-history", SelectedUnits: []domain.CorpusSelectedUnit{{UnitID: "unit-1", Order: 1, Title: "Kapitel"}}}, []string{"Analyzed scope", "1 selected units", "scope-history", "Kapitel"}},
		{"legacy full text", domain.AnalysisCoverage{}, []string{"Analyzed scope", "Legacy/full-text scope.", "persisted full-text corpus"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := BookPage(domain.User{Username: "learner"}, "csrf", book, &test.coverage, false, "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			for _, want := range test.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("book history missing %q: %s", want, output.String())
				}
			}
		})
	}
}
