// Package sentences deterministically selects representative, owner-scoped
// examples from a vocabulary candidate's eligible source references.
package sentences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

var ErrInvalidInput = errors.New("sentences: invalid input")

type Store interface {
	ReplaceSelectedSentences(context.Context, string, string, string, string, string, []domain.ExampleSentence) error
}

type Config struct {
	MinWords, PreferredMinWords, PreferredMaxWords, MaxWords, MaxCharacters, MaxAlternatives int
}

func DefaultConfig() Config {
	return Config{MinWords: 3, PreferredMinWords: 8, PreferredMaxWords: 24, MaxWords: 50, MaxCharacters: 400, MaxAlternatives: 4}
}

type ScoredSentence struct {
	Text     string
	Location selection.SentenceReference
	Score    int
	Reasons  []string
}

type Result struct {
	Chosen       *ScoredSentence
	Alternatives []ScoredSentence
}

type Service struct{ store Store }

var (
	authorYearEntry     = regexp.MustCompile(`(?i)^[\p{L}'’\-]+(?:,\s*(?:[\p{L}'’\-]+|(?:\p{L}\.\s*)+)|\s+(?:und|&|/)\s*[\p{L}'’\-]+)?\s+\(?(?:1[5-9]|20)\d{2}[a-z]?\)?\s*[:.,]`)
	bibliographicMarker = regexp.MustCompile(`(?i)(?:^|[[:space:]([])(?:in\s*:|(?:hgg?|hrsg)\.|(?:hgg?|hrsg)\s*(?:von|durch)|isbn(?:-1[03])?\s*:|doi\s*:|https?://|www\.)`)
	contentsLeader      = regexp.MustCompile(`(?:\.{3,}|…{2,}|\s[-–—]{2,}\s)\s*\d{1,4}\s*$`)
	barePageTail        = regexp.MustCompile(`\s(?:S\.|Seite(?:n)?|pp?\.)\s*\d+(?:\s*[-–]\s*\d+)?\.?\s*$`)
	bareNumericTail     = regexp.MustCompile(`\s\d{1,4}\.(?:\s+und\s+\d{1,4}\.)?\s*$`)
	trailingFootnote    = regexp.MustCompile(`\[\d+\]\s*[.!?]?\s*$`)
)

func NewService(store Store) *Service { return &Service{store: store} }

// Select scores only references already attached to the owner's candidate.
// Score: length 30 preferred/20 short/15 long; context 20 with at least six
// non-target words; representative usage 10 for one target occurrence, 10
// each for no quotation and no parenthetical, and 5 for terminal punctuation.
func (s *Service) Select(ctx context.Context, owner, corpusID string, candidate selection.Candidate, cfg Config) (Result, error) {
	if owner == "" || corpusID == "" || candidate.Identity.Language == "" || candidate.Identity.CanonicalLemma == "" || candidate.Identity.UPOS == "" || !validConfig(cfg) {
		return Result{}, ErrInvalidInput
	}
	forms := append([]string(nil), candidate.ObservedForms...)
	forms = append(forms, candidate.Identity.CanonicalLemma)
	seen := make(map[string]bool)
	ranked := make([]ScoredSentence, 0, len(candidate.SentenceReferences))
	for _, ref := range candidate.SentenceReferences {
		if seen[ref.Text] {
			continue
		}
		seen[ref.Text] = true
		words := strings.Fields(ref.Text)
		if !validReference(ref, words, cfg) || structuralFragment(ref.Text) {
			continue
		}
		occurrences := targetOccurrences(words, forms)
		if occurrences == 0 {
			continue
		}
		score, reasons := score(ref.Text, len(words), occurrences, cfg)
		ranked = append(ranked, ScoredSentence{Text: ref.Text, Location: ref, Score: score, Reasons: reasons})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		if ranked[i].Location.SentenceIndex != ranked[j].Location.SentenceIndex {
			return ranked[i].Location.SentenceIndex < ranked[j].Location.SentenceIndex
		}
		return ranked[i].Text < ranked[j].Text
	})
	limit := cfg.MaxAlternatives + 1
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	examples := make([]domain.ExampleSentence, len(ranked))
	for i, item := range ranked {
		location, err := json.Marshal(item.Location.Location)
		if err != nil {
			return Result{}, fmt.Errorf("marshal location: %w", err)
		}
		reasons, err := json.Marshal(item.Reasons)
		if err != nil {
			return Result{}, fmt.Errorf("marshal reasons: %w", err)
		}
		examples[i] = domain.ExampleSentence{SentenceKey: fmt.Sprintf("%s:%s:%s:%d", candidate.Identity.Language, candidate.Identity.CanonicalLemma, candidate.Identity.UPOS, item.Location.SentenceIndex), Text: item.Text, SourceLocation: location, SelectionReasons: reasons, SelectionRank: i + 1, SelectionScore: item.Score, Chosen: i == 0}
	}
	if err := s.store.ReplaceSelectedSentences(ctx, owner, corpusID, candidate.Identity.Language, candidate.Identity.CanonicalLemma, candidate.Identity.UPOS, examples); err != nil {
		return Result{}, fmt.Errorf("persist selected sentences: %w", err)
	}
	if len(ranked) == 0 {
		return Result{}, nil
	}
	result := Result{Chosen: &ranked[0]}
	result.Alternatives = append(result.Alternatives, ranked[1:]...)
	return result, nil
}

func structuralFragment(text string) bool {
	if authorYearEntry.MatchString(text) || bibliographicMarker.MatchString(text) || contentsLeader.MatchString(text) || barePageTail.MatchString(text) || bareNumericTail.MatchString(text) || trailingFootnote.MatchString(text) {
		return true
	}
	// Sentence segmentation also emits headings and standalone book/article
	// titles. Without sentence-final punctuation they are not usable prose.
	trimmed := strings.TrimRight(text, `”"')]}»`)
	return trimmed == "" || !strings.ContainsAny(trimmed[len(trimmed)-1:], ".!?")
}

func validConfig(c Config) bool {
	return c.MinWords > 0 && c.PreferredMinWords >= c.MinWords && c.PreferredMaxWords >= c.PreferredMinWords && c.MaxWords >= c.PreferredMaxWords && c.MaxCharacters > 0 && c.MaxAlternatives >= 0
}
func validReference(ref selection.SentenceReference, words []string, c Config) bool {
	return utf8.ValidString(ref.Text) && strings.TrimSpace(ref.Text) == ref.Text && utf8.RuneCountInString(ref.Text) <= c.MaxCharacters && len(words) >= c.MinWords && len(words) <= c.MaxWords && ref.Location.SourceDocumentID != "" && ref.Location.EndOffset > ref.Location.StartOffset
}
func normalizeWord(word string) string {
	return strings.ToLower(strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }))
}
func targetOccurrences(words, forms []string) int {
	targets := make(map[string]bool, len(forms))
	for _, form := range forms {
		if normalized := normalizeWord(form); normalized != "" {
			targets[normalized] = true
		}
	}
	count := 0
	for _, word := range words {
		if targets[normalizeWord(word)] {
			count++
		}
	}
	return count
}
func score(text string, words, occurrences int, c Config) (int, []string) {
	total := 0
	reasons := make([]string, 0, 5)
	switch {
	case words >= c.PreferredMinWords && words <= c.PreferredMaxWords:
		total += 30
		reasons = append(reasons, "preferred length")
	case words < c.PreferredMinWords:
		total += 20
		reasons = append(reasons, "short but contextual")
	default:
		total += 15
		reasons = append(reasons, "long sentence penalty")
	}
	if words-occurrences >= 6 {
		total += 20
		reasons = append(reasons, "sufficient surrounding context")
	}
	if occurrences == 1 {
		total += 10
		reasons = append(reasons, "single unambiguous target use")
	} else {
		reasons = append(reasons, "repeated target form")
	}
	if !strings.ContainsAny(text, `“”„\"'`) {
		total += 10
		reasons = append(reasons, "not a quotation")
	}
	if !strings.ContainsAny(text, "()[]{}") {
		total += 10
		reasons = append(reasons, "not parenthetical")
	}
	if strings.ContainsAny(text[len(text)-1:], ".!?") {
		total += 5
		reasons = append(reasons, "complete sentence punctuation")
	}
	return total, reasons
}
