package analysisinsights

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// LanguageCorpus derives the owner's evidence-only view for one study
// language. Persistence supplies current-analysis rows and lemma aggregates;
// no aggregate is stored between calls.
func (s *Service) LanguageCorpus(ctx context.Context, owner, language string) (domain.LanguageCorpusView, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return domain.LanguageCorpusView{}, fmt.Errorf("language corpus: language is required")
	}
	store, ok := s.store.(LanguageCorpusStore)
	if !ok {
		return domain.LanguageCorpusView{}, fmt.Errorf("language corpus: store does not provide language corpus evidence")
	}
	evidence, err := store.ListLanguageCorpusEvidence(ctx, owner, language)
	if err != nil {
		return domain.LanguageCorpusView{}, fmt.Errorf("load language corpus evidence: %w", err)
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		a, b := evidence[i].Book, evidence[j].Book
		if strings.ToLower(a.Title) != strings.ToLower(b.Title) {
			return strings.ToLower(a.Title) < strings.ToLower(b.Title)
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})

	result := domain.LanguageCorpusView{OwnerID: owner, Language: language}
	var vocab vocabulary
	vocabLoaded := false
	contributions := make(map[string]domain.LemmaOccurrence)
	seenBooks := make(map[string]struct{}, len(evidence))
	for _, book := range evidence {
		if _, seen := seenBooks[book.Book.ID]; seen {
			continue
		}
		seenBooks[book.Book.ID] = struct{}{}
		spread := domain.LanguageCorpusBookSpread{
			BookID:           book.Book.ID,
			Title:            book.Book.Title,
			SourceMaterialID: book.SourceMaterialID,
			CorpusID:         book.CorpusID,
			AnalysisRunID:    book.AnalysisRunID,
			EvidenceState:    book.EvidenceState,
		}
		input := domain.AnalysisCorpusVocabulary{
			CorpusID:         book.CorpusID,
			SourceMaterialID: book.SourceMaterialID,
			AnalysisRunID:    book.AnalysisRunID,
			Statistics:       book.Statistics,
			Lemmas:           book.Lemmas,
		}
		if reason := languageCorpusExclusion(book, input, language); reason != "" {
			spread.ExclusionReason = reason
			result.PerBook = append(result.PerBook, spread)
			continue
		}
		if !vocabLoaded {
			vocab, err = s.loadVocabulary(ctx, owner, language)
			if err != nil {
				return domain.LanguageCorpusView{}, err
			}
			vocabLoaded = true
		}
		current, eligible, _ := coverageWithVocabulary(input, map[string]vocabulary{language: vocab}, false)
		spread.KnownTokenCount = current.KnownTokenCount
		spread.AnalyzableTokenCount = current.AnalyzableTokenCount
		spread.Included = true
		result.PerBook = append(result.PerBook, spread)
		result.AnalyzedBookCount++
		result.KnownTokenCount += current.KnownTokenCount
		result.AnalyzableTokenCount += current.AnalyzableTokenCount
		for _, lemma := range eligible {
			key := occurrenceKey(lemma)
			aggregate := contributions[key]
			if aggregate.Language == "" {
				aggregate.Language = lemma.Language
				aggregate.CanonicalLemma = lemma.CanonicalLemma
				aggregate.UPOS = lemma.UPOS
			}
			aggregate.OccurrenceCount += lemma.OccurrenceCount
			contributions[key] = aggregate
		}
	}
	result.TopUnknownLemmas = make([]domain.LemmaOccurrence, 0, len(contributions))
	for _, lemma := range contributions {
		result.TopUnknownLemmas = append(result.TopUnknownLemmas, lemma)
	}
	sort.Slice(result.TopUnknownLemmas, func(i, j int) bool {
		if result.TopUnknownLemmas[i].OccurrenceCount != result.TopUnknownLemmas[j].OccurrenceCount {
			return result.TopUnknownLemmas[i].OccurrenceCount > result.TopUnknownLemmas[j].OccurrenceCount
		}
		return occurrenceKey(result.TopUnknownLemmas[i]) < occurrenceKey(result.TopUnknownLemmas[j])
	})
	if len(result.TopUnknownLemmas) > topUnknownLimit {
		result.TopUnknownLemmas = result.TopUnknownLemmas[:topUnknownLimit]
	}
	return result, nil
}

func languageCorpusExclusion(book domain.LanguageCorpusBookEvidence, input domain.AnalysisCorpusVocabulary, language string) string {
	switch {
	case book.SourceMaterialID == "":
		return "unavailable: no current acquired source"
	case book.EvidenceState == domain.MyBookUnavailable:
		return "unavailable: current source content unavailable"
	case book.EvidenceState == domain.MyBookStale:
		return "stale: analysis no longer matches the current book scope"
	case book.SourceLanguage != "" && !strings.EqualFold(book.SourceLanguage, language):
		return "different study language"
	case book.CurrentSourceMaterialID != "" && book.CurrentSourceMaterialID != book.SourceMaterialID:
		return "stale: corpus does not represent the current source"
	case book.CorpusID == "" && book.CurrentAnalysisRunID != "":
		return "stale: current analysis cannot be loaded"
	case book.CorpusID == "":
		return "unassessed: no current analyzed corpus"
	case book.AnalysisSourceMaterialID != "" && book.AnalysisSourceMaterialID != book.SourceMaterialID:
		return "stale: corpus does not represent the current source"
	case book.Statistics == nil:
		return "stale/incomplete: corpus statistics unavailable"
	case !corpusLanguageMatches(input, language):
		return "different study language: corpus evidence is not modeled language"
	default:
		return ""
	}
}
