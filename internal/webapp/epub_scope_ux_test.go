package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
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
			for _, want := range []string{title, `<fieldset class="scope-actions">`, `<legend>Apply a selection</legend>`, `type="button"`, `for="scope-unit-0"`, `id="scope-unit-0"`, `name="unit_id"`, `aria-live="polite"`, `role="status"`, "Review carefully", "fallback title from manifest ID", "Estimated size", "Reasons"} {
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
