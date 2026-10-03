package webapp

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestConcordanceResultsRenderOneNativeList(t *testing.T) {
	sentence := `Vor 🐈 Haus.`
	start := int64(strings.Index(sentence, "Haus"))
	occurrence := domain.ConcordanceResultOccurrence{ConcordanceOccurrence: domain.ConcordanceOccurrence{
		BookID: "book-1", BookTitle: `Title </script><img src=x>`, SentenceText: sentence,
		Surface: "Haus", SentenceStartOffset: start, SentenceEndOffset: start + 4,
		SentenceOrdinal: 2, TokenOrdinal: 3, AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit",
	}}
	component := VocabularyConcordancePageView(domain.User{}, "", "de", nil,
		domain.ConcordanceLookup{Mode: "surface", Term: "Haus", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1}, true, "")
	var rendered bytes.Buffer
	if err := component.Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	body := rendered.String()
	for _, want := range []string{
		`<ol id="concordance-native-results" class="concordance-results">`,
		`<details class="concordance-row" name="concordance-occurrences"`,
		`<a class="concordance-study-link"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
	if got := strings.Count(body, `class="concordance-results"`); got != 1 {
		t.Fatalf("rendered %d Concordance result lists, want exactly one", got)
	}
	if !strings.Contains(body, `Title &lt;/script&gt;&lt;img src=x&gt;`) || strings.Contains(body, `</script><img src=x>`) {
		t.Fatal("Book title was not escaped in the server-rendered list")
	}
	for _, forbidden := range []string{`data-concordance-data=`, `<mouseion-concordance`, `concordance-keyboard.js`, `concordance.js`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("rendered page includes retired Concordance presentation code %q", forbidden)
		}
	}
}

func TestConcordanceContextWindowsDoNotSplitUnicode(t *testing.T) {
	sentence := strings.Repeat("a", 48) + " 🐈Haus🌿 " + strings.Repeat("b", 48)
	start := int64(strings.Index(sentence, "Haus"))
	occurrence := domain.ConcordanceResultOccurrence{ConcordanceOccurrence: domain.ConcordanceOccurrence{
		SentenceText: sentence, Surface: "Haus", SentenceStartOffset: start, SentenceEndOffset: start + 4,
	}}
	left, right := concordanceBefore(occurrence), concordanceAfter(occurrence)
	if !utf8.ValidString(left) || !utf8.ValidString(right) {
		t.Fatalf("context windows split UTF-8: left=%q right=%q", left, right)
	}
	if !strings.HasPrefix(left, "…") || !strings.HasSuffix(left, " 🐈") || !strings.HasPrefix(right, "🌿 ") {
		t.Fatalf("context windows lost Unicode boundaries: left=%q right=%q", left, right)
	}
}
