package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestKnownVocabularyFormatsOnlyGermanNouns(t *testing.T) {
	known := []domain.KnownVocabulary{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"},
		{Language: "en", CanonicalLemma: "house", UPOS: "NOUN"},
	}
	var output bytes.Buffer
	if err := KnownVocabResult("de", nil, known, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"<td>Haus</td>", "<td>gehen</td>", "<td>house</td>"} {
		if !strings.Contains(html, want) {
			t.Errorf("known vocabulary table missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "<td>haus</td>") {
		t.Errorf("German noun remained lowercase: %s", html)
	}
}

func TestAnalysisInsightsFormatsOnlyGermanNouns(t *testing.T) {
	tests := []struct {
		name     string
		language string
		lemma    domain.LemmaOccurrence
		want     string
	}{
		{name: "German noun", language: "de", lemma: domain.LemmaOccurrence{CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 4}, want: "Haus"},
		{name: "German verb", language: "de", lemma: domain.LemmaOccurrence{CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 3}, want: "gehen"},
		{name: "English noun", language: "en", lemma: domain.LemmaOccurrence{CanonicalLemma: "house", UPOS: "NOUN", OccurrenceCount: 2}, want: "house"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := domain.AnalysisCoverage{TopUnknownLemmas: []domain.LemmaOccurrence{tt.lemma}}
			book := domain.SourceMaterialSummary{Source: domain.SourceMaterial{Language: tt.language}}
			var output bytes.Buffer
			if err := BookPage(domain.User{}, "csrf", book, &coverage, false, "").Render(context.Background(), &output); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); !strings.Contains(got, "<strong>"+tt.want+"</strong>") {
				t.Errorf("analysis insights missing formatted lemma %q: %s", tt.want, got)
			}
		})
	}
}
