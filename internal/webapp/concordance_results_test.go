package webapp

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcordanceResultsRenderOneNativeList(t *testing.T) {
	sentence := `Vor 🐈 Haus.`
	start := int64(strings.Index(sentence, "Haus"))
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: `Title </script><img src=x>`, SentenceText: sentence,
		Surface: "Haus", SentenceStartOffset: start, SentenceEndOffset: start + 4,
		SentenceOrdinal: 2, TokenOrdinal: 3, AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit"}
	continued := occurrence
	continued.SentenceOrdinal++
	otherBook := occurrence
	otherBook.BookID = "book-2"
	otherBook.BookTitle = "Zweiter Titel"
	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "Haus", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence, continued, otherBook}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "haus"}, true, false, "", "", browseOrigin{})
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
		`class="concordance-source"`,
		`2 occurrences on this page`,
		`1 occurrence on this page`,
		`2 on page`,
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
	// A sentence disclosure is named by its own KWIC text; repeating the Book
	// title on every occurrence would bury the sentence for a screen reader.
	if strings.Contains(body, `aria-label="Occurrence of`) {
		t.Fatal("occurrence disclosures must be named by their sentence, not the Book title")
	}
	for _, retired := range []string{`concordance-book-label`, `concordance-result--book-start`, `aria-hidden="true"></div>`} {
		if strings.Contains(body, retired) {
			t.Errorf("rendered list still has retired grouping artifact %q", retired)
		}
	}
	if got := strings.Count(body, `class="concordance-source-title"`); got != 2 {
		t.Fatalf("rendered %d source labels, want one at each page-local Book group start", got)
	}
	if got := strings.Count(body, `<li class="concordance-result">`); got != 3 {
		t.Fatalf("rendered %d uniform result rows, want 3", got)
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

func TestConcordanceSummaryAndContextKeepAuthorialSourceLines(t *testing.T) {
	sentence := "Der Geist,\nist <script>leicht</script>."
	start := int64(len([]rune("Der ")))
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: "Faust", SentenceText: sentence,
		Surface: "Geist", SentenceStartOffset: start, SentenceEndOffset: start + 5,
		SentenceOrdinal: 0, TokenOrdinal: 1, AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit",
		Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: `Mephistopheles <script>`},
	}
	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "Geist", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "geist"}, true, false, "", "", browseOrigin{})
	var rendered bytes.Buffer
	require.NoError(t, component.Render(t.Context(), &rendered))
	body := rendered.String()

	require.Equal(t, 1, strings.Count(body, `class="concordance-line-break"`),
		"the one-line summary marks each authorial break")
	require.Contains(t, body, `<span class="concordance-line-break"> / </span>`)
	require.Equal(t, 2, strings.Count(body, `class="concordance-context-line"`),
		"the expanded occurrence renders actual source lines")
	require.Contains(t, body, `<span class="concordance-context-line">Der <strong class="concordance-observed-target">Geist</strong>,</span>`,
		"target emphasis stays inside the correct source line")
	require.Contains(t, body, `ist &lt;script&gt;leicht&lt;/script&gt;.`, "source text is escaped as text")
	require.NotContains(t, body, `<script>leicht</script>`)
	require.Contains(t, body, `<p class="concordance-attribution">Mephistopheles &lt;script&gt;</p>`,
		"speaker metadata is escaped as text and stays outside the sentence")
}

func TestConcordanceContextShowsStageDirectionAttribution(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: "Faust", SentenceText: "Er schlägt das Buch auf.",
		Surface: "schlägt", SentenceStartOffset: 3, SentenceEndOffset: 10,
		SentenceOrdinal: 0, TokenOrdinal: 1, AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit",
		Attribution: domain.SourceAttribution{Kind: domain.AttributionStageDirection},
	}
	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "schlägt", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "schlägt"}, true, false, "", "", browseOrigin{})
	var rendered bytes.Buffer
	require.NoError(t, component.Render(t.Context(), &rendered))
	require.Contains(t, rendered.String(), `<p class="concordance-attribution">Stage direction</p>`)
}

func TestConcordanceNoBreakProseKeepsTheShippedLayout(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: "Buch", SentenceText: "Das Haus sieht gut aus.",
		Surface: "Haus", SentenceStartOffset: 4, SentenceEndOffset: 8,
		SentenceOrdinal: 0, TokenOrdinal: 1, AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit",
	}
	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "Haus", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "haus"}, true, false, "", "", browseOrigin{})
	var rendered bytes.Buffer
	require.NoError(t, component.Render(t.Context(), &rendered))
	body := rendered.String()

	require.NotContains(t, body, `class="concordance-line-break"`)
	require.NotContains(t, body, `class="concordance-context-line"`)
	require.Contains(t, body, `<p class="reading-text">Das <strong class="concordance-observed-target">Haus</strong> sieht gut aus.</p>`)
}

func TestConcordanceContinuedBookKeepsItsPageLocalCount(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: "Der lange Weg nach Hause", SentenceText: "Ein Haus.", Surface: "Haus"}
	result := domain.ConcordanceResult{
		Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 2, Continued: true, Match: domain.ConcordanceMatchForm, Term: "haus",
	}
	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "Haus", Page: 2}, result, true, false, "", "", browseOrigin{})
	var rendered bytes.Buffer
	if err := component.Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	body := rendered.String()
	if !strings.Contains(body, `<span class="concordance-count-full">1 occurrence on this page</span>`) {
		t.Fatal("continued Book group lost its page-local count")
	}
	if !strings.Contains(body, `continues Der lange Weg nach Hause from the previous page`) {
		t.Fatal("continued Book group is not announced in the results summary")
	}
}

func TestConcordanceReturnFocusIsServerRendered(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", SentenceOrdinal: 2, TokenOrdinal: 3, Surface: "Haus", SentenceText: "Ein Haus."}
	lookup := domain.ConcordanceLookup{Term: "Haus", Page: 1}
	result := domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "haus"}
	render := func(target string, result domain.ConcordanceResult) string {
		component := VocabularyConcordancePageView(domain.User{}, "", "de", lookup, result, true, false, "", target, browseOrigin{})
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
		"/vocabulary/concordance?term=Haus&page=2&focus=x&from_book=b&from_snap=s&from_page=0#x",
		"/vocabulary/concordance?term=Haus&page=2&focus=x&from_book=b#x",
	} {
		if got := safeConcordanceReturnURL(invalid); got != "/vocabulary/concordance" {
			t.Errorf("unsafe return URL %q accepted as %q", invalid, got)
		}
	}
}

func TestSafeConcordanceReturnURLKeepsAValidBrowseOrigin(t *testing.T) {
	origin := browseOrigin{BookID: "book-1", SnapshotID: "snap-1", Prefix: "ha", IncludeAll: true, Page: 2, Row: "NOUN:haus", Revision: "rev-1"}
	valid := origin.Apply("/vocabulary/concordance?page=1&term=haus&focus=occurrence-book-1-0-0") + "#occurrence-book-1-0-0"
	if got := safeConcordanceReturnURL(valid); got != valid {
		t.Fatalf("return URL with origin = %q, want %q", got, valid)
	}
}

func TestBrowseOriginParsingDropsMalformedOrPartialState(t *testing.T) {
	complete := url.Values{originKeyBook: {"b"}, originKeySnapshot: {"s"}, originKeyPage: {"3"}, originKeyAll: {"1"}, originKeyRow: {"NOUN:haus"}}
	origin := parseBrowseOrigin(complete)
	if !origin.Active() || origin.Page != 3 || !origin.IncludeAll || origin.Row != "NOUN:haus" {
		t.Fatalf("complete origin parsed as %+v", origin)
	}
	for name, mutate := range map[string]func(url.Values){
		"no snapshot":   func(v url.Values) { v.Del(originKeySnapshot) },
		"no book":       func(v url.Values) { v.Del(originKeyBook) },
		"page zero":     func(v url.Values) { v.Set(originKeyPage, "0") },
		"page text":     func(v url.Values) { v.Set(originKeyPage, "x") },
		"all not 1":     func(v url.Values) { v.Set(originKeyAll, "true") },
		"oversized row": func(v url.Values) { v.Set(originKeyRow, strings.Repeat("x", maxOriginValueLength+1)) },
	} {
		values := url.Values{}
		maps.Copy(values, complete)
		mutate(values)
		if parseBrowseOrigin(values).Active() {
			t.Errorf("%s: malformed origin was honored", name)
		}
	}
	if (browseOrigin{}).Apply("/x?a=1") != "/x?a=1" {
		t.Error("an inactive origin must not change a URL")
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
			occurrence := domain.ConcordanceResultOccurrence{
				SentenceText: test.sentence, Surface: test.surface,
				SentenceStartOffset: start, SentenceEndOffset: start + int64(utf8.RuneCountInString(test.surface))}
			before, target, after := concordanceSentenceParts(occurrence)
			if before != test.before || target != test.surface || after != test.after {
				t.Fatalf("parts = %q | %q | %q, want %q | %q | %q", before, target, after, test.before, test.surface, test.after)
			}
		})
	}
}

func TestConcordanceSentencePartsRejectOffsetsOutsideTheSentence(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{
		SentenceText: "Das Haus.", Surface: "Haus", SentenceStartOffset: 4, SentenceEndOffset: 40}
	if before, target, after := concordanceSentenceParts(occurrence); before != "" || target != "" || after != "" {
		t.Fatalf("out-of-range offsets produced parts %q | %q | %q", before, target, after)
	}
}

func TestConcordanceSummaryChunksMarkSourceLineBreaks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []concordanceSummaryChunk
	}{
		{
			name:  "source breaks become marks and boundary spaces stay unbroken",
			input: "Wie er  tatsächlich\n verstand ",
			want: []concordanceSummaryChunk{
				{Text: "Wie er tatsächlich"},
				{Break: true},
				{Text: "verstand\u00a0"},
			},
		},
		{
			name:  "leading boundary space stays unbroken",
			input: " , falls  er",
			want:  []concordanceSummaryChunk{{Text: "\u00a0, falls er"}},
		},
		{
			name:  "literal source slash is not a break",
			input: "und/oder",
			want:  []concordanceSummaryChunk{{Text: "und/oder"}},
		},
		{
			name:  "no breaks keep today's collapse",
			input: ", falls er",
			want:  []concordanceSummaryChunk{{Text: ", falls er"}},
		},
		{
			name:  "whitespace alone stays one boundary space",
			input: " ",
			want:  []concordanceSummaryChunk{{Text: "\u00a0"}},
		},
		{
			name:  "empty text stays empty",
			input: "",
			want:  nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, concordanceSummaryChunks(test.input))
		})
	}
}

// concordanceFailStore fails one Concordance read after the application shell
// has loaded, so the handler's own failure handling can be driven directly.
type concordanceFailStore struct {
	*fixtures.Store
	currentReadingErr error
	concordanceErr    error
}

func (s concordanceFailStore) GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error) {
	if s.currentReadingErr != nil {
		return domain.CurrentReading{}, s.currentReadingErr
	}
	return s.Store.GetCurrentReading(ctx, owner, language)
}

func (s concordanceFailStore) ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	if s.concordanceErr != nil {
		return domain.ConcordanceResult{}, s.concordanceErr
	}
	return s.Store.ListVocabularyConcordance(ctx, owner, language, lookup)
}

// TestSentenceStudyTextAlternativeIsClosedByDefaultAndComplete holds the
// Sentence study framing: the retired attributed-token list is replaced by a
// named native disclosure that is server-rendered, closed by default, and lists
// every analyzed word with its stored evidence in source order.
func TestSentenceStudyTextAlternativeIsClosedByDefaultAndComplete(t *testing.T) {
	study := domain.SentenceStudy{
		BookTitle: "Der lange Weg nach Hause", ChapterTitle: "Kapitel 1",
		SentenceText: "Das Haus Haus steht.", UnitText: "Das Haus Haus steht.",
		SentenceStartOffset: 0, SentenceEndOffset: 20,
		SentenceOrdinal: 0, TargetOrdinal: 1, TargetSurface: "Haus",
		Tokens: []domain.SentenceStudyToken{
			{Surface: "Das", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Dependency: "det", HeadOrdinal: 1, HeadSurface: "Haus", Ordinal: 0, StartOffset: 0, EndOffset: 3, Morphology: map[string]string{"Case": "Nom", "Number": "Sing"}},
			{Surface: "Haus", RawLemma: "haus", EffectiveLemma: "haus", UPOS: "NOUN", Dependency: "nsubj", HeadOrdinal: 3, HeadSurface: "steht", Ordinal: 1, StartOffset: 4, EndOffset: 8},
			{Surface: "Haus", RawLemma: "haus", EffectiveLemma: "heim", UPOS: "NOUN", Dependency: "obj", HeadOrdinal: 3, HeadSurface: "steht", Ordinal: 2, StartOffset: 9, EndOffset: 13, Corrected: true, Excluded: true},
			{Surface: "steht", RawLemma: "stehen", EffectiveLemma: "stehen", UPOS: "VERB", Dependency: "root", HeadOrdinal: 3, HeadSurface: "steht", Ordinal: 3, StartOffset: 14, EndOffset: 19, Morphology: map[string]string{"Mood": "Ind", "VerbForm": "Fin"}},
		},
	}
	var rendered bytes.Buffer
	if err := VocabularySentenceStudyPageView(domain.User{}, "", study, "/vocabulary/concordance").Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	body := rendered.String()

	detailsStart := strings.Index(body, `<details id="sentence-study-text-alternative" class="sentence-study-alternative">`)
	if detailsStart < 0 {
		t.Fatal("the complete text alternative is not a named native disclosure")
	}
	details := body[detailsStart:]
	open := strings.Index(details, ">")
	if open < 0 || strings.Contains(details[:open], "open") {
		t.Fatal("the complete text alternative must be closed by default")
	}
	if !strings.Contains(body, `<summary>Complete text alternative</summary>`) {
		t.Fatal("the complete text alternative disclosure is not named")
	}
	for _, want := range []string{
		`<code>DET</code>`, `analyzer lemma <code>der</code>`, `effective lemma <code>der</code>`,
		`morphology <code>Case=Nom, Number=Sing</code>`,
		`morphology <code>none supplied</code>`,
		`dependency <code>nsubj</code>, head steht (token 4)`,
		`Corrected for this occurrence.`,
		`Excluded from effective vocabulary; retained as syntax evidence.`,
		// The full evidence line is locked so a modern-language token keeps its
		// exact wording and spacing.
		`<strong>Das</strong> <span>(token 1)</span> : <code>DET</code>, analyzer lemma <code>der</code>, effective lemma <code>der</code>; morphology <code>Case=Nom, Number=Sing</code>; dependency <code>det</code>, head Haus (token 2).`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sentence study text alternative missing %q", want)
		}
	}
	// Repeated forms and shared-span siblings stay distinguishable by source position.
	for _, want := range []string{
		`<strong>Haus — identified target</strong> <span>(token 2)</span>`,
		`<strong>Haus</strong> <span>(token 3)</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sentence study text alternative does not distinguish %q", want)
		}
	}
	if strings.Contains(body, "Analyzer-attributed tokens") {
		t.Fatal("the retired attributed-token list is still rendered")
	}

	noEvidence := domain.SentenceStudy{BookTitle: "Buch", SentenceText: "Nur Text.", TargetSurface: "Wort"}
	rendered.Reset()
	if err := VocabularySentenceStudyPageView(domain.User{}, "", noEvidence, "/vocabulary/concordance").Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	body = rendered.String()
	if !strings.Contains(body, `<a href="/vocabulary/concordance">Return to Concordance results</a>`) {
		t.Fatal("the no-evidence state dropped the return link")
	}
	if !strings.Contains(body, "Syntax evidence is unavailable for this sentence; the complete sentence remains available to study.") {
		t.Fatal("the no-evidence state changed its explanation")
	}
	if !strings.Contains(body, "Identified target: <strong>Wort (syntax evidence unavailable)</strong>") {
		t.Fatal("the missing-target state changed its fallback wording")
	}
	if strings.Contains(body, `id="sentence-study-text-alternative"`) {
		t.Fatal("the no-evidence state must not render an empty text alternative")
	}
}

// TestSentenceStudyTextAlternativeShowsUnresolvedDecisionsSeparately holds the
// correction/exclusion attribution gate in the text alternative: an unresolved
// decision is shown separately and applied to no sibling, while an attributed
// decision leaves the word's grammar evidence intact.
func TestSentenceStudyTextAlternativeShowsUnresolvedDecisionsSeparately(t *testing.T) {
	study := domain.SentenceStudy{
		BookTitle: "Buch", SentenceText: "zum Haus steht", SentenceOrdinal: 0, TargetOrdinal: 2, TargetSurface: "Haus",
		Tokens: []domain.SentenceStudyToken{
			{Surface: "zu", RawLemma: "zu", EffectiveLemma: "zu", UPOS: "ADP", Dependency: "case", HeadOrdinal: 2, HeadSurface: "Haus", Ordinal: 0, StartOffset: 0, EndOffset: 3, Morphology: map[string]string{"Case": "Dat"}},
			{Surface: "dem", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Dependency: "det", HeadOrdinal: 2, HeadSurface: "Haus", Ordinal: 1, StartOffset: 0, EndOffset: 3, Morphology: map[string]string{"Case": "Dat", "Gender": "Neut"}},
			{Surface: "Haus", RawLemma: "haus", EffectiveLemma: "heim", UPOS: "NOUN", Dependency: "root", HeadOrdinal: 2, HeadSurface: "steht", Ordinal: 2, StartOffset: 4, EndOffset: 8, Corrected: true, Morphology: map[string]string{"Case": "Dat", "Number": "Sing"}},
			{Surface: "steht", RawLemma: "stehen", EffectiveLemma: "stehen", UPOS: "VERB", Dependency: "root", HeadOrdinal: 3, HeadSurface: "steht", Ordinal: 3, StartOffset: 9, EndOffset: 14, Excluded: true, Morphology: map[string]string{"Mood": "Ind", "VerbForm": "Fin"}},
		},
		UnresolvedDecisions: []domain.SentenceStudyDecision{
			{StartOffset: 0, EndOffset: 3, CanonicalLemma: "zu", AddressedWords: 2},
			{StartOffset: 10, EndOffset: 11, Excluded: true, AddressedWords: 0},
		},
	}
	var rendered bytes.Buffer
	if err := VocabularySentenceStudyPageView(domain.User{}, "", study, "/vocabulary/concordance").Render(t.Context(), &rendered); err != nil {
		t.Fatal(err)
	}
	body := rendered.String()

	for _, want := range []string{
		`These occurrence decisions could not be attributed to a single analyzed word, so they are shown separately and applied to none:`,
		`Unresolved lemma correction to zu at source offsets 0–3: 2 analyzed words share this span, so it is not applied to any of them.`,
		`Unresolved exclusion at source offsets 10–11: no persisted analyzed word matches this span, so it is not applied to any word.`,
		`Corrected for this occurrence.`,
		`Excluded from effective vocabulary; retained as syntax evidence.`,
		`morphology <code>Case=Dat, Number=Sing</code>`,
		`dependency <code>root</code>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sentence study text alternative missing %q", want)
		}
	}
	if got := strings.Count(body, "Corrected for this occurrence."); got != 1 {
		t.Errorf("a decision was applied to %d words, want exactly the uniquely addressed word", got)
	}
	if got := strings.Count(body, "Excluded from effective vocabulary; retained as syntax evidence."); got != 1 {
		t.Errorf("an exclusion was applied to %d words, want exactly the uniquely addressed word", got)
	}
	// The shared-span siblings carry no decision of their own.
	for _, sibling := range []string{"<strong>zu</strong>", "<strong>dem</strong>"} {
		start := strings.Index(body, sibling)
		if start < 0 {
			t.Fatalf("sibling %q missing from the text alternative", sibling)
		}
		if end := strings.Index(body[start:], "</li>"); end < 0 {
			t.Fatalf("sibling %q has no list item end", sibling)
		} else if item := body[start : start+end]; strings.Contains(item, "Corrected for this occurrence") || strings.Contains(item, "Excluded from effective vocabulary") {
			t.Errorf("the unresolved decision was applied to sibling %q", sibling)
		}
	}
}

// TestConcordanceHandlerReportsReadFailures covers the current-reading priority
// and result reads: a plain failure is a server error, a deadline is a gateway
// timeout, and neither renders a partial result page.
func TestConcordanceHandlerReportsReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		store    func(*fixtures.Store) allStoreCapabilities
		wantCode int
	}{
		{
			name: "current reading unavailable",
			store: func(s *fixtures.Store) allStoreCapabilities {
				return concordanceFailStore{Store: s, currentReadingErr: errors.New("current reading unavailable")}
			},
			wantCode: http.StatusInternalServerError,
		},
		{
			name: "current reading times out",
			store: func(s *fixtures.Store) allStoreCapabilities {
				return concordanceFailStore{Store: s, currentReadingErr: context.DeadlineExceeded}
			},
			wantCode: http.StatusGatewayTimeout,
		},
		{
			name: "lookup unavailable",
			store: func(s *fixtures.Store) allStoreCapabilities {
				return concordanceFailStore{Store: s, concordanceErr: errors.New("concordance unavailable")}
			},
			wantCode: http.StatusInternalServerError,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, cookies, _, store := readingFixtureSession(t)
			requireHandler(t, h).services.Store = storeDependencies(tc.store(store))

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary/concordance?term=gehen", nil)
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)

			assert.Equal(t, tc.wantCode, response.Code)
		})
	}
}

// TestConcordanceWithholdsStudyForSpeakerLabelSentences holds ADR 0103's Study
// affordance amendment: a sentence overlapping a speaker label offers no Study,
// a label occurrence is named as evidence, and a standalone direction sentence
// keeps its Study link.
func TestConcordanceWithholdsStudyForSpeakerLabelSentences(t *testing.T) {
	base := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: "Faust", SentenceText: "Valentin fällt, das Buch liegt hier.",
		Surface: "fällt", SentenceStartOffset: 9, SentenceEndOffset: 14,
		SentenceOrdinal: 0, TokenOrdinal: 1, AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit",
	}
	labeled := base
	labeled.SpeakerLabel = true
	labeled.StudyUnavailable = true
	otherWord := base
	otherWord.Surface = "Buch"
	otherWord.SentenceStartOffset, otherWord.SentenceEndOffset = 20, 24
	otherWord.TokenOrdinal = 3
	otherWord.SpeakerLabel = false
	otherWord.StudyUnavailable = true
	direction := base
	direction.SentenceText = "Das Buch liegt hier."
	direction.Surface = "Buch"
	direction.SentenceStartOffset, direction.SentenceEndOffset = 4, 8
	direction.SentenceOrdinal = 1
	direction.TokenOrdinal = 1
	direction.StudyUnavailable = false

	component := VocabularyConcordancePageView(domain.User{}, "", "de",
		domain.ConcordanceLookup{Term: "fällt", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{labeled, otherWord, direction}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "fällt"}, true, false, "", "", browseOrigin{})
	var rendered bytes.Buffer
	require.NoError(t, component.Render(t.Context(), &rendered))
	body := rendered.String()

	if got := strings.Count(body, `class="concordance-study-link"`); got != 1 {
		t.Fatalf("rendered %d Study links, want only the standalone direction's one", got)
	}
	if got := strings.Count(body, `class="concordance-speaker-label"`); got != 1 {
		t.Fatalf("rendered %d speaker-label markers, want exactly one", got)
	}
	require.Contains(t, body, `aria-label="Speaker label evidence"`)
	require.Contains(t, body, ">Speaker label</span>")
	require.Contains(t, body, "das Buch liegt hier.", "the label sentence is kept unchanged")
}
