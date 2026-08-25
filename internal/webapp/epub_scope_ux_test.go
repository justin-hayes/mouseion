package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
)

func TestEPUBScopeReviewIsAccessibleForGermanAndItalianTitles(t *testing.T) {
	for _, title := range []string{"Erstes Kapitel", "Capitolo primo"} {
		t.Run(title, func(t *testing.T) {
			view := epubScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: title}, Units: []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "epub-unit-v1:0:chapter", Order: 0, Title: title, TitleSource: domain.UnitTitleHeading}, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryMainMatter, Confidence: 95, RecommendedInclusion: true, Reasons: []domain.EPUBClassificationReason{{Message: "A chapter heading was found."}}}, CharacterCount: 120, TokenEstimate: 30}, {Unit: domain.ExtractedUnit{ID: "epub-unit-v1:1:notes", Order: 1, Title: "notes", TitleSource: domain.UnitTitleManifestID}, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryUnknown, Confidence: 20, Reasons: []domain.EPUBClassificationReason{{Message: "The evidence is limited."}}}, CharacterCount: 40, TokenEstimate: 10}}}
			var output bytes.Buffer
			if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			body := output.String()
			for _, want := range []string{title, `<form method="post"`, `<fieldset class="scope-actions">`, `<legend>Apply a selection</legend>`, `type="button"`, `for="scope-unit-0"`, `id="scope-unit-0"`, `type="checkbox"`, `name="unit_id"`, `<button type="submit">Confirm and analyze scope</button>`, `aria-live="polite"`, `aria-atomic="true"`, `role="status"`, "Review carefully", "fallback title from manifest ID", "Estimated size", "Reasons"} {
				if !strings.Contains(body, want) {
					t.Errorf("render missing %q: %s", want, body)
				}
			}
			if strings.Contains(body, `name="text"`) || strings.Contains(body, `value="A chapter heading was found."`) {
				t.Fatal("browser form included authoritative text")
			}
		})
	}
}

func TestEPUBScopeReviewErrorIsProgrammaticallyExposedAndPreservesSelection(t *testing.T) {
	view := epubScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: "Libro"}, Units: []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "unit-0", Order: 0, Title: "Capitolo"}, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryMainMatter, RecommendedInclusion: true}}}}
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

func TestEPUBScopeReviewRendersGroupsAggregatesAndPartialState(t *testing.T) {
	units := []epubScopeUnitView{
		{Unit: domain.ExtractedUnit{ID: "unit-0", Order: 0, Title: "Uno"}, CharacterCount: 100, TokenEstimate: 25, Classification: domain.EPUBUnitClassification{RecommendedInclusion: true}},
		{Unit: domain.ExtractedUnit{ID: "unit-1", Order: 1, Title: "Due"}, CharacterCount: 40, TokenEstimate: 10},
	}
	view := epubScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: "Libro"}, SnapshotID: "snapshot-1", Units: units, Groups: []epubScopeGroupView{{Group: epub.UnitGroup{ID: "group-1", Label: "Parte prima", UnitIDs: []string{"unit-0", "unit-1"}}, CharacterCount: 140, TokenEstimate: 35}}}
	var output bytes.Buffer
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{`name="snapshot_id" value="snapshot-1"`, `aria-labelledby="scope-groups-heading"`, "EPUB groups", "Parte prima", "2 units · 140 characters · about 35 tokens", `data-group-state role="status" aria-live="polite"`, `<button type="submit" class="outline" name="group_include" value="group-1">`, `<button type="submit" class="outline secondary" name="group_exclude" value="group-1">`, "Partially selected"} {
		if !strings.Contains(body, want) {
			t.Errorf("group render missing %q: %s", want, body)
		}
	}
}

func TestEPUBScopeReviewFlatFallbackAndKeyboardControls(t *testing.T) {
	view := epubScopeView{Book: domain.SourceMaterial{ID: "book-flat", Title: "Legacy flat EPUB"}, SnapshotID: "snapshot-flat", Units: []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "unit-0", Order: 0, Title: "Chapter"}, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryMainMatter, RecommendedInclusion: true}}}}
	var output strings.Builder
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{`role="note"><strong>Flat spine order.`, `<fieldset class="scope-actions"><legend>Apply a selection</legend>`, `<button type="button"`, `<label for="scope-unit-0"><input id="scope-unit-0" type="checkbox"`, `<fieldset class="scope-units"><legend>Readable units in spine order</legend>`, `scopeForm.addEventListener('change'`, `scopeForm.addEventListener('click'`} {
		if !strings.Contains(body, want) {
			t.Errorf("flat/keyboard review missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `tabindex="0"`) || strings.Contains(body, `onkeydown=`) {
		t.Fatal("native keyboard controls were replaced with custom keyboard semantics")
	}
}

func TestEPUBScopeReviewExplainsDegradedPolicyAndMediumConfidenceRecommendation(t *testing.T) {
	view := epubScopeView{
		Book: domain.SourceMaterial{ID: "book-de", Title: "Buch"}, DegradedRecommendation: true, AllCharacters: 596187, AllTokens: 149047,
		Units: []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "unit-0", Order: 0, Title: "I. Einleitung"}, CharacterCount: 528000, TokenEstimate: 132000, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryMainMatter, Confidence: 70, RecommendedInclusion: true, Reasons: []domain.EPUBClassificationReason{{Message: "The text contains sustained sentence-like prose."}}}}},
	}
	var output strings.Builder
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Limited structural confidence", "No high-confidence main matter was found", "whole-book fallback", "Include — review suggested", "All units: 596187 characters", "Selected: 0 characters"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("degraded recommendation UI missing %q: %s", want, output.String())
		}
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

func TestEPUBScopeReviewComparisonNamesIDsTitlesAndEstimatedSizes(t *testing.T) {
	view := epubScopeView{
		Book: domain.SourceMaterial{ID: "book-1", OwnerID: "owner-1", Title: "Libro"}, Preset: "recommended",
		PriorScope: &domain.EPUBReviewedScopeSnapshot{ScopeID: "scope-1", SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: "unit-0", Order: 0}, {UnitID: "unit-1", Order: 1}}},
		Comparison: epubScopeComparison{
			Old:           []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "unit-0", Title: "Capitolo"}}, {Unit: domain.ExtractedUnit{ID: "unit-1", Title: "Bibliografia"}}},
			New:           []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "unit-0", Title: "Capitolo"}}},
			Removed:       []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "unit-1", Title: "Bibliografia"}, CharacterCount: 80, TokenEstimate: 20}},
			OldCharacters: 200, OldTokens: 50, NewCharacters: 120, NewTokens: 30,
		},
	}
	var output strings.Builder
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Scope comparison", "Prior included units", "Proposed included units", "unit-0", "Capitolo", "unit-1", "Bibliografia", "200 characters", "120 characters", "80 characters"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("comparison missing %q: %s", want, output.String())
		}
	}
}
