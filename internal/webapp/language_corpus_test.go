package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestLanguageCorpusLanguageLabelUsesSupportedReference(t *testing.T) {
	supported := []domain.SupportedLanguage{{Language: "de", DisplayName: "German"}}
	if got := languageCorpusLanguageLabel("de-DE", supported); got != "German" {
		t.Fatalf("known language label=%q, want German", got)
	}
	if got := languageCorpusLanguageLabel("xx", supported); got != "xx" {
		t.Fatalf("unknown language label=%q, want canonical fallback", got)
	}
}
