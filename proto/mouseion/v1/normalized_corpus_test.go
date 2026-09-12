package mouseionv1_test

import (
	"testing"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"google.golang.org/protobuf/proto"
)

func TestNormalizedCorpusRoundTrip(t *testing.T) {
	namedEntity := "BUILDING"
	want := &mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.1.0",
		Language:      "de",
		SourceDocuments: []*mouseionv1.SourceDocument{{
			Id: "book-1", SourceIdentifier: "opds:42", Title: "Das Buch",
		}},
		Analysis: &mouseionv1.AnalysisProvenance{
			RunId: "run-7", AnalyzedAt: "2026-08-21T12:00:00Z",
			AnalyzerName: "stanza", AnalyzerVersion: "1.10.1",
		},
		NormalizationProfile: &mouseionv1.NormalizationProfile{
			Name: "de-standard-1996", Version: "1.0.0",
		},
		Sentences: []*mouseionv1.Sentence{{
			Text: "Das Haus steht.",
			Location: &mouseionv1.SourceLocation{
				SourceDocumentId: "book-1", Chapter: "1", Section: "1.1", EndOffset: 15,
			},
			Tokens: []*mouseionv1.Token{{
				Surface: "Haus", RawLemma: "Haus", CanonicalLemma: "haus", Pos: "NOUN",
				Dependency: "root", Head: 0,
				Morphology:  map[string]string{"Case": "Nom", "Number": "Sing"},
				NamedEntity: &namedEntity,
				Location: &mouseionv1.SourceLocation{
					SourceDocumentId: "book-1", Chapter: "1", Section: "1.1",
					StartOffset: 4, EndOffset: 8,
				},
			}},
		}},
	}

	encoded, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("marshal normalized corpus: %v", err)
	}

	got := new(mouseionv1.NormalizedCorpus)
	if err := proto.Unmarshal(encoded, got); err != nil {
		t.Fatalf("unmarshal normalized corpus: %v", err)
	}
	if !proto.Equal(want, got) {
		t.Fatalf("round trip mismatch:\nwant: %v\n got: %v", want, got)
	}
	if got.GetSentences()[0].GetTokens()[0].GetCanonicalLemma() != "haus" {
		t.Fatal("canonical lemma was not preserved")
	}
}
