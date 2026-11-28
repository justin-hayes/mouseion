package analyzer

import (
	"time"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
)

// IsSeparableParticle exposes the unexported rule to the external tests.
func IsSeparableParticle(tokens []Token, index int) bool {
	return isSeparableParticle(tokens, index, dependentOrdinals(tokens))
}

// ToProto builds the protobuf message FromProto reads, so tests can round-trip
// a Result. Production code only converts from protobuf.
func ToProto(result Result) *mouseionv1.NormalizedCorpus {
	corpus := &mouseionv1.NormalizedCorpus{
		SchemaVersion: result.SchemaVersion,
		Language:      result.Language,
		Analysis: &mouseionv1.AnalysisProvenance{
			RunId:           result.Analysis.RunID,
			AnalyzedAt:      formatTime(result.Analysis.AnalyzedAt),
			AnalyzerName:    result.Analysis.AnalyzerName,
			AnalyzerVersion: result.Analysis.AnalyzerVersion,
		},
		NormalizationProfile: &mouseionv1.NormalizationProfile{
			Name:    result.NormalizationProfile.Name,
			Version: result.NormalizationProfile.Version,
		},
	}

	for _, document := range result.SourceDocuments {
		corpus.SourceDocuments = append(corpus.SourceDocuments, &mouseionv1.SourceDocument{
			Id:               document.ID,
			SourceIdentifier: document.SourceIdentifier,
			Title:            document.Title,
		})
	}
	for _, sentence := range result.Sentences {
		converted := &mouseionv1.Sentence{
			Text:     sentence.Text,
			Location: locationToProto(sentence.Location),
		}
		for _, token := range sentence.Tokens {
			converted.Tokens = append(converted.Tokens, &mouseionv1.Token{
				Surface:        token.Surface,
				RawLemma:       token.RawLemma,
				CanonicalLemma: token.CanonicalLemma,
				Pos:            token.UPOS,
				Dependency:     token.Dependency,
				Head:           token.Head,
				Morphology:     cloneMap(token.Morphology),
				Location:       locationToProto(token.Location),

				HeadUnprintedNode: token.HeadUnprintedNode,
			})
		}
		for _, node := range sentence.UnprintedNodes {
			converted.UnprintedNodes = append(converted.UnprintedNodes, &mouseionv1.UnprintedSyntaxNode{
				SourceId:          node.SourceID,
				Dependency:        node.Dependency,
				HeadTokenOrdinal:  node.HeadTokenOrdinal,
				HeadUnprintedNode: node.HeadUnprintedNode,
			})
		}
		corpus.Sentences = append(corpus.Sentences, converted)
	}
	return corpus
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func locationToProto(location SourceLocation) *mouseionv1.SourceLocation {
	return &mouseionv1.SourceLocation{
		SourceDocumentId: location.SourceDocumentID,
		Chapter:          location.Chapter,
		Section:          location.Section,
		StartOffset:      location.StartOffset,
		EndOffset:        location.EndOffset,
	}
}
