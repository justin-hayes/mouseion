package analyzer_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"google.golang.org/protobuf/proto"
)

func TestProtoRoundTrip(t *testing.T) {
	namedEntity := "PERSON"
	want := analyzer.Result{
		SchemaVersion: "1.0.0",
		Language:      "de",
		Sentences: []analyzer.Sentence{{
			Text: "Er grüßte.",
			Tokens: []analyzer.Token{{
				Surface:        "grüßte",
				RawLemma:       "grüßen",
				CanonicalLemma: "grüßen",
				UPOS:           "VERB",
				Morphology:     map[string]string{"Tense": "Past"},
				NamedEntity:    &namedEntity,
				Location:       analyzer.SourceLocation{SourceDocumentID: "doc-1", Chapter: "1", Section: "opening", StartOffset: 3, EndOffset: 9},
			}},
			Location: analyzer.SourceLocation{SourceDocumentID: "doc-1", Chapter: "1", Section: "opening", StartOffset: 0, EndOffset: 10},
		}},
		SourceDocuments: []analyzer.SourceDocumentMetadata{{ID: "doc-1", SourceIdentifier: "opds:book", Title: "Book"}},
		Analysis: analyzer.AnalysisProvenance{
			RunID:           "run-1",
			AnalyzedAt:      time.Date(2026, 8, 21, 12, 34, 56, 123, time.UTC),
			AnalyzerName:    "fake",
			AnalyzerVersion: "1.2.3",
		},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "german-standard", Version: "1"},
	}

	message := analyzer.ToProto(want)
	bytes, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(bytes) == 0 {
		t.Fatal("Marshal() returned empty transport payload")
	}

	got, err := analyzer.FromProto(message)
	if err != nil {
		t.Fatalf("FromProto() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FromProto(ToProto()) = %#v, want %#v", got, want)
	}
}

func TestFromProtoRejectsInvalidAnalysisTime(t *testing.T) {
	message := analyzer.ToProto(analyzer.Result{})
	message.Analysis.AnalyzedAt = "not-a-time"

	if _, err := analyzer.FromProto(message); err == nil {
		t.Fatal("FromProto() error = nil, want invalid analysis time error")
	}
}
