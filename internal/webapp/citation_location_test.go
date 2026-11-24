package webapp

import (
	"bytes"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A classical sentence's location is its Citation, rendered by one function for
// the Concordance line and Sentence study; a modern sentence keeps its own.

func concordanceBody(t *testing.T, occurrence domain.ConcordanceResultOccurrence) string {
	t.Helper()
	var rendered bytes.Buffer
	require.NoError(t, VocabularyConcordancePageView(domain.User{}, "", "grc",
		domain.ConcordanceLookup{Term: "Haus", Page: 1},
		domain.ConcordanceResult{Occurrences: []domain.ConcordanceResultOccurrence{occurrence}, Page: 1, Match: domain.ConcordanceMatchForm, Term: "haus"},
		true, false, "", "", browseOrigin{}).Render(t.Context(), &rendered))
	return rendered.String()
}

func TestConcordanceLineShowsAClassicalOccurrencesCitation(t *testing.T) {
	occurrence := domain.ConcordanceResultOccurrence{
		BookID: "book-1", BookTitle: "Anabasis 3", SentenceText: "λέγει ὅτι.", Surface: "ὅτι",
		SentenceStartOffset: 0, SentenceEndOffset: 10, ChapterTitle: "Book 3", SentenceOrdinal: 4,
		AnalysisRunID: "run", CorpusID: "corpus", UnitID: "unit",
		Citations: []string{"3.2.1", "3.2.2"},
	}
	body := concordanceBody(t, occurrence)
	assert.Contains(t, body, `class="concordance-location">3.2.1–2<`)

	occurrence.Citations = nil
	body = concordanceBody(t, occurrence)
	assert.NotContains(t, body, "concordance-location", "a modern occurrence keeps its current location")
}

func TestSentenceStudyShowsAClassicalSentencesCitation(t *testing.T) {
	render := func(citations []string) string {
		study := domain.SentenceStudy{
			BookTitle: "Anabasis 3", ChapterTitle: "Book 3", SentenceText: "λέγει.", UnitText: "λέγει.",
			SentenceEndOffset: 6, SentenceOrdinal: 4, Citations: citations,
		}
		var rendered bytes.Buffer
		require.NoError(t, VocabularySentenceStudyPageView(domain.User{}, "", study, "/vocabulary/concordance", "").Render(t.Context(), &rendered))
		return rendered.String()
	}
	body := render([]string{"3.2.1", "3.2.2"})
	assert.Contains(t, body, `<p class="sentence-study-location">3.2.1–2</p>`)
	assert.NotContains(t, body, "sentence 5")

	body = render(nil)
	assert.Contains(t, body, "Book 3 · sentence 5")
}

func TestOneCitationRenderingServesEverySurface(t *testing.T) {
	citations := []string{"1.2.17", "1.2.18", "1.2.20"}
	want := "1.2.17–18, 1.2.20"
	occurrence := domain.ConcordanceResultOccurrence{BookID: "b", BookTitle: "T", SentenceText: "x", Surface: "x", SentenceEndOffset: 1, Citations: citations}
	assert.Contains(t, concordanceBody(t, occurrence), ">"+want+"<")
	assert.Equal(t, want, sentenceCitation(citations))
}
