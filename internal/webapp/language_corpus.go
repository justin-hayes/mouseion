package webapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
)

type languageCorpusProvider interface {
	LanguageCorpus(context.Context, string, string) (domain.LanguageCorpusView, error)
}

type languageCorpusPanelView struct {
	Language             string
	LanguageLabel        string
	AnalyzedBookCount    int
	KnownTokenCount      int64
	AnalyzableTokenCount int64
	CoveragePercent      string
	TopUnknownLemmas     []languageCorpusLemmaView
	PerBook              []languageCorpusBookView
	State                string
	PanelUnavailable     bool
}

type languageCorpusLemmaView struct {
	Lemma           string
	UPOS            string
	OccurrenceCount int64
}

type languageCorpusBookView struct {
	BookID               string
	Title                string
	KnownTokenCount      int64
	AnalyzableTokenCount int64
	CoveragePercent      string
	EvidenceLabel        string
	Included             bool
	ExclusionReason      string
}

func buildLanguageCorpusPanel(ctx context.Context, provider languageCorpusProvider, owner, language string) (languageCorpusPanelView, error) {
	result, err := provider.LanguageCorpus(ctx, owner, language)
	if err != nil {
		return languageCorpusPanelView{}, err
	}
	view := languageCorpusPanelView{
		Language:             result.Language,
		LanguageLabel:        languageCorpusLanguageLabel(result.Language),
		AnalyzedBookCount:    result.AnalyzedBookCount,
		KnownTokenCount:      result.KnownTokenCount,
		AnalyzableTokenCount: result.AnalyzableTokenCount,
		CoveragePercent:      languageCorpusPercent(result.KnownTokenCount, result.AnalyzableTokenCount),
		State:                languageCorpusState(result),
	}
	if view.Language == "" {
		view.Language = strings.ToLower(strings.TrimSpace(language))
		view.LanguageLabel = languageCorpusLanguageLabel(view.Language)
	}
	for _, lemma := range result.TopUnknownLemmas {
		view.TopUnknownLemmas = append(view.TopUnknownLemmas, languageCorpusLemmaView{
			Lemma:           lemmadisplay.Format(view.Language, lemma.CanonicalLemma, lemma.UPOS),
			UPOS:            knownVocabUPOS(lemma.UPOS),
			OccurrenceCount: lemma.OccurrenceCount,
		})
	}
	for _, book := range result.PerBook {
		view.PerBook = append(view.PerBook, languageCorpusBookView{
			BookID:               book.BookID,
			Title:                book.Title,
			KnownTokenCount:      book.KnownTokenCount,
			AnalyzableTokenCount: book.AnalyzableTokenCount,
			CoveragePercent:      languageCorpusPercent(book.KnownTokenCount, book.AnalyzableTokenCount),
			EvidenceLabel:        myBookEvidenceLabel(book.EvidenceState),
			Included:             book.Included,
			ExclusionReason:      book.ExclusionReason,
		})
	}
	return view, nil
}

func unavailableLanguageCorpusPanel(language string) languageCorpusPanelView {
	language = strings.ToLower(strings.TrimSpace(language))
	return languageCorpusPanelView{
		Language:         language,
		LanguageLabel:    languageCorpusLanguageLabel(language),
		PanelUnavailable: true,
		State:            "unavailable",
	}
}

func languageCorpusState(result domain.LanguageCorpusView) string {
	if len(result.PerBook) == 0 {
		return "none"
	}
	if result.AnalyzedBookCount == 0 {
		return "no_analyzed"
	}
	included, excluded, stale := false, false, false
	for _, book := range result.PerBook {
		if book.Included {
			included = true
			continue
		}
		excluded = true
		if strings.Contains(strings.ToLower(book.ExclusionReason), "stale") {
			stale = true
		}
	}
	switch {
	case included && excluded:
		return "mixed"
	case stale:
		return "stale"
	case included:
		return "current"
	default:
		return "no_analyzed"
	}
}

func languageCorpusLanguageLabel(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "de":
		return "German"
	case "it":
		return "Italian"
	case "en":
		return "English"
	default:
		language = strings.TrimSpace(language)
		if language == "" {
			return "the selected language"
		}
		return strings.ToUpper(language)
	}
}

func languageCorpusPercent(numerator, denominator int64) string {
	if denominator <= 0 {
		return "not reported"
	}
	if numerator < 0 {
		numerator = 0
	}
	if numerator > denominator {
		numerator = denominator
	}
	tenths := (numerator*1000 + denominator/2) / denominator
	return fmt.Sprintf("%d.%d%%", tenths/10, tenths%10)
}

func languageCorpusLiveSummary(view languageCorpusPanelView) string {
	if view.PanelUnavailable {
		return fmt.Sprintf("Language view for %s is unavailable.", view.LanguageLabel)
	}
	if view.AnalyzedBookCount == 0 {
		return fmt.Sprintf("Language view for %s: no analyzed books.", view.LanguageLabel)
	}
	return fmt.Sprintf("Language view for %s: %d analyzed books, %s current coverage.", view.LanguageLabel, view.AnalyzedBookCount, view.CoveragePercent)
}
