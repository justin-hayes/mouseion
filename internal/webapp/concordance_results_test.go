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
	continued := occurrence
	continued.SentenceOrdinal++
	otherBook := occurrence
	otherBook.BookID = "book-2"
	otherBook.BookTitle = "Zweiter Titel"
	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "Haus", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence, continued, otherBook}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "haus"}, true, false, "", "")
	var rendered bytes.Buffer
	if err := component.Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	body := rendered.String()
	for _, want := range []string{
		`<ol id="concordance-native-results" class="concordance-results" role="list">`,
		`<details class="concordance-row" name="concordance-occurrences"`,
		`<a class="concordance-study-link"`,
		`>Study</a>`,
		`class="concordance-book-label"`,
		`2 occurrences on this page`,
		`1 occurrence on this page`,
		`class="concordance-result concordance-result--book-start"`,
		`Zweiter Titel`,
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
	if got := strings.Count(body, `class="concordance-book-label"`); got != 3 {
		t.Fatalf("rendered %d marginal book labels, want one per result row", got)
	}
	if got := strings.Count(body, `class="concordance-book-title"`); got != 2 {
		t.Fatalf("rendered %d Book titles, want one at each Book group start", got)
	}
	if got := strings.Count(body, `class="concordance-results-summary"`); got != 1 {
		t.Fatalf("rendered %d results summaries, want exactly one", got)
	}
	summaryStart := strings.Index(body, `<p class="concordance-results-summary">`)
	summaryEnd := strings.Index(body[summaryStart:], `</p>`)
	summary := body[summaryStart : summaryStart+summaryEnd]
	if !strings.Contains(summary, `Results 1–3 on page 1.`) || !strings.Contains(body, `>Word form haus</h2>`) {
		t.Fatal("results omitted the applied interpretation or result range")
	}
	emptySummary := concordanceResultsSummary(domain.ConcordanceResult{Page: 1})
	if emptySummary != `No results on page 1.` {
		t.Fatalf("empty results summary is not concise and grammatical: %q", emptySummary)
	}
	for _, forbidden := range []string{`data-concordance-data=`, `<mouseion-concordance`, `concordance-keyboard.js`, `concordance.js`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("rendered page includes retired Concordance presentation code %q", forbidden)
		}
	}
	for _, want := range []string{
		`id="concordance-workflow"`, `hx-select="#concordance-workflow"`, `hx-push-url="true"`,
		`hx-sync="#concordance-workflow:replace"`, `hx-indicator="#concordance-pending"`,
		`hx-config="timeout:9s"`, `hx-status:409=`, `hx-status:4xx=`, `hx-status:5xx=`, `id="concordance-pending"`,
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
	lookup := domain.ConcordanceLookup{Term: "Haus", Page: 1}
	result := domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "haus"}
	render := func(target string, result domain.ConcordanceResult) string {
		component := VocabularyConcordancePageView(domain.User{}, "", "de", lookup, result, true, false, "", target)
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
	if !strings.Contains(got, `<h2 id="concordance-summary" tabindex="-1" autofocus>Word form haus</h2>`) {
		t.Fatal("missing returned occurrence did not focus Current results in server-rendered HTML")
	}
}

func TestSafeConcordanceReturnURLRejectsUnrelatedAndMalformedTargets(t *testing.T) {
	valid := "/vocabulary/concordance?term=Haus&as=form&page=2&focus=occurrence-book-25-1#occurrence-book-25-1"
	if got := safeConcordanceReturnURL(valid); got != valid {
		t.Fatalf("safe return URL = %q, want %q", got, valid)
	}
	for _, invalid := range []string{
		"https://example.com/", "//example.com/vocabulary/concordance?focus=x#x",
		"/unrelated?focus=x#x", "/vocabulary/concordance?term=Haus&page=2&focus=x#y",
		"/vocabulary/concordance?term=Haus&page=not-a-page&focus=x#x",
		"/vocabulary/concordance?term=Haus&page=2&focus=x&next=https://example.com#x",
		"/vocabulary/concordance?term=Haus&mode=surface&book=x&page=2&focus=x#x",
	} {
		if got := safeConcordanceReturnURL(invalid); got != "/vocabulary/concordance" {
			t.Errorf("unsafe return URL %q accepted as %q", invalid, got)
		}
	}
}

func TestConcordanceSentencePartsUseCodePointOffsets(t *testing.T) {
	for _, test := range []struct {
		name, sentence, surface, before, after string
	}{
		{
			name:     "multi-byte letters before the occurrence",
			sentence: "Wie er tatsächlich verstand, dass sie ihn zu erwürgen gedenke, falls er ihre Pita nicht aufesse.",
			surface:  "gedenke",
			before:   "Wie er tatsächlich verstand, dass sie ihn zu erwürgen ",
			after:    ", falls er ihre Pita nicht aufesse.",
		},
		{
			name:     "dash and umlaut before the occurrence",
			sentence: "Auch Sretoje – erzählten von all dem, auch gedenken.",
			surface:  "gedenken",
			before:   "Auch Sretoje – erzählten von all dem, auch ",
			after:    ".",
		},
		{
			name:     "astral symbols around the occurrence",
			sentence: "a 🐈Haus🌿 b",
			surface:  "Haus",
			before:   "a 🐈",
			after:    "🌿 b",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			start := int64(utf8.RuneCountInString(test.before))
			occurrence := domain.ConcordanceResultOccurrence{ConcordanceOccurrence: domain.ConcordanceOccurrence{
				SentenceText: test.sentence, Surface: test.surface,
				SentenceStartOffset: start, SentenceEndOffset: start + int64(utf8.RuneCountInString(test.surface)),
			}}
			before, target, after := concordanceSentenceParts(occurrence)
			if before != test.before || target != test.surface || after != test.after {
				t.Fatalf("parts = %q | %q | %q, want %q | %q | %q", before, target, after, test.before, test.surface, test.after)
			}
		})
	}
}

func TestConcordanceSentencePartsRejectOffsetsOutsideTheSentence(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{ConcordanceOccurrence: domain.ConcordanceOccurrence{
		SentenceText: "Das Haus.", Surface: "Haus", SentenceStartOffset: 4, SentenceEndOffset: 40,
	}}
	if before, target, after := concordanceSentenceParts(occurrence); before != "" || target != "" || after != "" {
		t.Fatalf("out-of-range offsets produced parts %q | %q | %q", before, target, after)
	}
}

func TestConcordanceContextTextKeepsOnlyBoundarySpaces(t *testing.T) {
	for input, want := range map[string]string{
		"Wie er  tatsächlich\n verstand ": "Wie er tatsächlich verstand ",
		" , falls  er":                   " , falls er",
		", falls er":                     ", falls er",
		" ":                              " ",
		"":                               "",
	} {
		if got := concordanceContextText(input); got != want {
			t.Errorf("concordanceContextText(%q) = %q, want %q", input, got, want)
		}
	}
}
