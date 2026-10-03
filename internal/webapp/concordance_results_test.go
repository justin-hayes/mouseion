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
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1}, true, "", "")
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
	for _, want := range []string{
		`id="concordance-workflow"`, `hx-select="#concordance-workflow"`, `hx-push-url="true"`,
		`hx-sync="#concordance-workflow:replace"`, `hx-indicator="#concordance-pending"`,
		`hx-status:409=`, `hx-status:4xx=`, `hx-status:5xx=`, `id="concordance-pending"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page missing HTMX workflow behavior %q", want)
		}
	}
	for _, retired := range []string{"concordanceRequestID", "history.pushState", "addEventListener('popstate'"} {
		if strings.Contains(body, retired) {
			t.Errorf("rendered page still includes handwritten Concordance request/history code %q", retired)
		}
	}
}

func TestConcordanceReturnFocusIsServerRendered(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{ConcordanceOccurrence: domain.ConcordanceOccurrence{
		BookID: "book-1", SentenceOrdinal: 2, TokenOrdinal: 3, Surface: "Haus", SentenceText: "Ein Haus.",
	}}
	lookup := domain.ConcordanceLookup{Mode: "surface", Term: "Haus", Page: 1}
	result := domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1}
	render := func(target string, result domain.ConcordanceResult) string {
		component := VocabularyConcordancePageView(domain.User{}, "", "de", nil, lookup, result, true, "", target)
		var rendered bytes.Buffer
		if err := component.Render(t.Context(), &rendered); err != nil {
			t.Fatal(err)
		}
		return rendered.String()
	}

	got := render("occurrence-book-1-2-3", result)
	if !strings.Contains(got, `id="occurrence-book-1-2-3" tabindex="-1" autofocus`) {
		t.Fatal("returned occurrence was not focused by server-rendered autofocus")
	}

	got = render("occurrence-no-longer-present", result)
	if !strings.Contains(got, `<h2 id="concordance-summary" tabindex="-1" aria-live="polite" autofocus>Current results</h2>`) {
		t.Fatal("missing returned occurrence did not focus Current results in server-rendered HTML")
	}
}

func TestSafeConcordanceReturnURLRejectsUnrelatedAndMalformedTargets(t *testing.T) {
	valid := "/vocabulary/concordance?mode=surface&term=Haus&page=2&focus=occurrence-book-25-1#occurrence-book-25-1"
	if got := safeConcordanceReturnURL(valid); got != valid {
		t.Fatalf("safe return URL = %q, want %q", got, valid)
	}
	for _, invalid := range []string{
		"https://example.com/", "//example.com/vocabulary/concordance?focus=x#x",
		"/unrelated?focus=x#x", "/vocabulary/concordance?term=Haus&page=2&focus=x#y",
		"/vocabulary/concordance?term=Haus&page=not-a-page&focus=x#x",
		"/vocabulary/concordance?term=Haus&page=2&focus=x&next=https://example.com#x",
	} {
		if got := safeConcordanceReturnURL(invalid); got != "/vocabulary/concordance" {
			t.Errorf("unsafe return URL %q accepted as %q", invalid, got)
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
