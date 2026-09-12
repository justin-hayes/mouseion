package analyzer

import (
	"fmt"
	"time"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
)

// ToProto converts a project-owned Result to the normalized-corpus transport
// and persistence schema.
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
				NamedEntity:    cloneString(token.NamedEntity),
				Location:       locationToProto(token.Location),
			})
		}
		corpus.Sentences = append(corpus.Sentences, converted)
	}
	return corpus
}

// FromProto converts the normalized-corpus transport and persistence schema to
// project-owned analyzer types.
func FromProto(corpus *mouseionv1.NormalizedCorpus) (Result, error) {
	if corpus == nil {
		return Result{}, fmt.Errorf("convert normalized corpus: nil corpus")
	}

	result := Result{SchemaVersion: corpus.GetSchemaVersion(), Language: corpus.GetLanguage()}
	if analysis := corpus.GetAnalysis(); analysis != nil {
		analyzedAt, err := parseTime(analysis.GetAnalyzedAt())
		if err != nil {
			return Result{}, fmt.Errorf("convert normalized corpus analysis time: %w", err)
		}
		result.Analysis = AnalysisProvenance{
			RunID:           analysis.GetRunId(),
			AnalyzedAt:      analyzedAt,
			AnalyzerName:    analysis.GetAnalyzerName(),
			AnalyzerVersion: analysis.GetAnalyzerVersion(),
		}
	}
	if profile := corpus.GetNormalizationProfile(); profile != nil {
		result.NormalizationProfile = NormalizationProfile{Name: profile.GetName(), Version: profile.GetVersion()}
	}
	for _, document := range corpus.GetSourceDocuments() {
		result.SourceDocuments = append(result.SourceDocuments, SourceDocumentMetadata{
			ID:               document.GetId(),
			SourceIdentifier: document.GetSourceIdentifier(),
			Title:            document.GetTitle(),
		})
	}
	for _, sentence := range corpus.GetSentences() {
		converted := Sentence{Text: sentence.GetText(), Location: locationFromProto(sentence.GetLocation())}
		for _, token := range sentence.GetTokens() {
			var namedEntity *string
			if token != nil {
				namedEntity = cloneString(token.NamedEntity)
			}
			converted.Tokens = append(converted.Tokens, Token{
				Surface:        token.GetSurface(),
				RawLemma:       token.GetRawLemma(),
				CanonicalLemma: token.GetCanonicalLemma(),
				UPOS:           token.GetPos(),
				Dependency:     token.GetDependency(),
				Head:           token.GetHead(),
				Morphology:     cloneMap(token.GetMorphology()),
				NamedEntity:    namedEntity,
				Location:       locationFromProto(token.GetLocation()),
			})
		}
		result.Sentences = append(result.Sentences, converted)
	}
	return result, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
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

func locationFromProto(location *mouseionv1.SourceLocation) SourceLocation {
	if location == nil {
		return SourceLocation{}
	}
	return SourceLocation{
		SourceDocumentID: location.GetSourceDocumentId(),
		Chapter:          location.GetChapter(),
		Section:          location.GetSection(),
		StartOffset:      location.GetStartOffset(),
		EndOffset:        location.GetEndOffset(),
	}
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
