package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
)

func TestIssue287CompleteGermanBookScopeReview(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "epub", "testfixtures", "classifier", "issue-287-complete-german-book.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SnapshotID string                `json:"snapshot_id"`
		Units      domain.ExtractedUnits `json:"units"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	classifications, err := epub.ClassifyUnits(fixture.SnapshotID, fixture.Units)
	if err != nil {
		t.Fatal(err)
	}
	view := epubScopeView{Book: domain.SourceMaterial{ID: "book-287", Title: "Der Palast und die Stadt"}, SnapshotID: fixture.SnapshotID}
	byID := make(map[string]epubScopeUnitView, len(fixture.Units.Units))
	selectedCharacters := 0
	for i, unit := range fixture.Units.Units {
		characters := len([]rune(unit.Text))
		item := epubScopeUnitView{Unit: unit, Classification: classifications[i], CharacterCount: characters, TokenEstimate: (characters + 3) / 4}
		view.Units = append(view.Units, item)
		view.AllCharacters += characters
		view.AllTokens += item.TokenEstimate
		byID[unit.ID] = item
		if classifications[i].RecommendedInclusion {
			selectedCharacters += characters
		}
	}
	for _, group := range epub.BuildUnitGroups(fixture.Units.Units) {
		item := epubScopeGroupView{Group: group}
		for _, id := range group.UnitIDs {
			item.CharacterCount += byID[id].CharacterCount
			item.TokenEstimate += byID[id].TokenEstimate
		}
		view.Groups = append(view.Groups, item)
	}
	if view.AllCharacters != 1997 || selectedCharacters != 1333 {
		t.Fatalf("fixture totals changed: all=%d selected=%d", view.AllCharacters, selectedCharacters)
	}
	var output strings.Builder
	if err = EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	body := output.String()
	for _, want := range []string{"All units: 1997 characters", "Selected: 0 characters", "Haupttext", "All group units: 9 units | 1333 characters", "Apparat", "All group units: 6 units", "fallback title from manifest ID", "Category", "Confidence", "Recommendation policy", "Included because high-confidence structural evidence identifies main matter.", "Excluded because the structural category is outside the recommended main-matter scope."} {
		if !strings.Contains(body, want) {
			t.Errorf("complete scope review missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "Limited structural confidence") || strings.Count(body, " checked") != 9 {
		t.Fatalf("unsafe or degraded default selection: checked=%d degraded=%t", strings.Count(body, " checked"), strings.Contains(body, "Limited structural confidence"))
	}
	previous := -1
	for _, unit := range fixture.Units.Units {
		position := strings.Index(body, "Include "+unit.Title)
		if position <= previous {
			t.Fatalf("unit %q rendered out of spine order", unit.Title)
		}
		previous = position
	}
}

func TestEPUBScopeReviewIsAccessibleForGermanAndItalianTitles(t *testing.T) {
	for _, title := range []string{"Erstes Kapitel", "Capitolo primo"} {
		t.Run(title, func(t *testing.T) {
			view := epubScopeView{Book: domain.SourceMaterial{ID: "book-1", Title: title}, Units: []epubScopeUnitView{{Unit: domain.ExtractedUnit{ID: "epub-unit-v1:0:chapter", Order: 0, Title: title, TitleSource: domain.UnitTitleHeading}, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryMainMatter, Confidence: 95, RecommendedInclusion: true, Reasons: []domain.EPUBClassificationReason{{Message: "A chapter heading was found."}}}, CharacterCount: 120, TokenEstimate: 30}, {Unit: domain.ExtractedUnit{ID: "epub-unit-v1:1:notes", Order: 1, Title: "notes", TitleSource: domain.UnitTitleManifestID}, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryUnknown, Confidence: 20, Reasons: []domain.EPUBClassificationReason{{Message: "The evidence is limited."}}}, CharacterCount: 40, TokenEstimate: 10}}}
			var output bytes.Buffer
			if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			body := output.String()
			for _, want := range []string{title, `<form method="post"`, `<fieldset class="scope-actions">`, `<legend>Apply a selection</legend>`, `type="button"`, `for="scope-unit-0"`, `id="scope-unit-0"`, `type="checkbox"`, `name="unit_id"`, `<button type="submit">Confirm and analyze scope</button>`, `aria-live="polite"`, `aria-atomic="true"`, `role="status"`, "Review carefully", "fallback title from manifest ID", "Estimated size", "Classification evidence", "Recommendation policy"} {
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
	for _, want := range []string{`name="snapshot_id" value="snapshot-1"`, `aria-labelledby="scope-groups-heading"`, "EPUB groups", "Parte prima", "All group units: 2 units | 140 characters | about 35 tokens", `data-group-selected`, "Selected: ${selectedMembers}", `data-group-state role="status" aria-live="polite"`, `<button type="submit" class="outline" name="group_include" value="group-1">`, `<button type="submit" class="outline secondary" name="group_exclude" value="group-1">`, "Partially selected"} {
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
		Units: []epubScopeUnitView{
			{Unit: domain.ExtractedUnit{ID: "unit-0", Order: 0, Title: "I. Einleitung"}, CharacterCount: 528000, TokenEstimate: 132000, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryMainMatter, Confidence: 70, RecommendedInclusion: true, Reasons: []domain.EPUBClassificationReason{{Message: "The text contains sustained sentence-like prose."}}}},
			{Unit: domain.ExtractedUnit{ID: "unit-1", Order: 1, Title: "Register"}, CharacterCount: 47013, TokenEstimate: 11753, Classification: domain.EPUBUnitClassification{Category: domain.EPUBCategoryUnknown, Confidence: 35, RecommendedInclusion: false, Reasons: []domain.EPUBClassificationReason{{Signal: "text_reference_density", Message: "Citation and reference patterns are concentrated in this unit."}}}},
		},
	}
	var output strings.Builder
	if err := EPUBScopeReviewPage(domain.User{Username: "learner"}, "csrf", view, "", "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Limited structural confidence", "No high-confidence main matter was found", "whole-book fallback", "Include — review suggested", "Included as plausible main matter", "Register", "Exclude — review required", "reference-like evidence", "Classification evidence", "Recommendation policy", "All units: 596187 characters", "Selected: 0 characters"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("degraded recommendation UI missing %q: %s", want, output.String())
		}
	}
	body := output.String()
	if strings.Count(body, "Limited structural confidence") != 1 {
		t.Fatalf("degraded notice must appear once at book level: %s", body)
	}
	category, confidence, recommendation, policy, evidence := strings.Index(body, "<dt>Category</dt>"), strings.Index(body, "<dt>Confidence</dt>"), strings.Index(body, "<dt>Recommendation</dt>"), strings.Index(body, "<dt>Recommendation policy</dt>"), strings.Index(body, "<h3>Classification evidence</h3>")
	if category < 0 || !(category < confidence && confidence < recommendation && recommendation < policy && policy < evidence) {
		t.Fatalf("classification and policy labels rendered out of order: %s", body)
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
