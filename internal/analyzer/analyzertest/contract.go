package analyzertest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// RunContract checks behavior required of every Analyzer implementation.
// Backend packages should call it from their tests with a factory configured
// to return wantResult for wantRequest.
func RunContract(
	t *testing.T,
	factory func(testing.TB, analyzer.AnalyzeRequest, analyzer.Result) analyzer.Analyzer,
) {
	t.Helper()

	request := analyzer.AnalyzeRequest{
		Language: "de",
		Document: analyzer.SourceDocument{
			ID:               "document-1",
			SourceIdentifier: "opds:1",
			Title:            "Die Verwandlung",
			Text:             "Gregor erwachte.",
		},
	}
	want := analyzer.Result{
		SchemaVersion: "1.1.0",
		Language:      "de",
		SourceDocuments: []analyzer.SourceDocumentMetadata{{
			ID:               "document-1",
			SourceIdentifier: "opds:1",
			Title:            "Die Verwandlung",
		}},
		Sentences: []analyzer.Sentence{{
			Text: "Gregor erwachte.",
			Tokens: []analyzer.Token{
				{
					Surface:        "Gregor",
					RawLemma:       "Gregor",
					CanonicalLemma: "gregor",
					UPOS:           "PROPN",
					Dependency:     "nsubj",
					Head:           1,
					Morphology:     map[string]string{"Case": "Nom"},
					Location:       analyzer.SourceLocation{SourceDocumentID: "document-1", StartOffset: 0, EndOffset: 6},
				},
				{
					Surface:        "erwachte",
					RawLemma:       "erwachen",
					CanonicalLemma: "erwachen",
					UPOS:           "VERB",
					Dependency:     "root",
					Head:           1,
					Location:       analyzer.SourceLocation{SourceDocumentID: "document-1", StartOffset: 7, EndOffset: 15},
				},
			},
			Location: analyzer.SourceLocation{SourceDocumentID: "document-1", StartOffset: 0, EndOffset: 16},
		}},
		Analysis: analyzer.AnalysisProvenance{
			RunID:           "run-1",
			AnalyzedAt:      time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
			AnalyzerName:    "contract-backend",
			AnalyzerVersion: "1.0.0",
		},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "de-standard", Version: "1"},
	}

	got, err := factory(t, request, want).Analyze(context.Background(), request)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Analyze() = %#v, want %#v", got, want)
	}
}
