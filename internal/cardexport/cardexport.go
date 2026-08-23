// Package cardexport produces owner-scoped, Anki-compatible Cloze TSV exports.
package cardexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var ErrInvalidInput = errors.New("cardexport: invalid input")

type Entry struct {
	OwnerID, Language, CanonicalLemma, UPOS string
	Sentence, Translation, TargetWord       string
	Morphology, SourceDocument, Notes       string
	FirstEncounter                          int64
	Ranking                                 domain.RankingComponents
}

type RankingChoice string

const (
	RankingDefault   RankingChoice = "default"
	RankingEncounter RankingChoice = "encounter"
	RankingBook      RankingChoice = "book"
	RankingGlobal    RankingChoice = "global"
	RankingBalanced  RankingChoice = "balanced"
)

type KnownWord struct{ Language, CanonicalLemma, UPOS string }

type ExportConfig struct {
	BookID      string
	FilterKnown bool
	Ranking     RankingChoice
	KnownWords  []KnownWord
}

type Note struct {
	Key, Text, BackExtra string
	Tags                 []string
}

type Artifact struct {
	TSV, NoteType string
	Count         int
}

type Store interface {
	ListAcceptedCurated(context.Context, string) ([]Entry, error)
	ListSelectionCandidatesForBook(context.Context, string, string) ([]domain.SelectionCandidate, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
	GetCoverageEntryForBook(context.Context, string, string, domain.SelectionCandidate) (Entry, error)
	RecordGenerated(context.Context, string, string, Entry, Note) error
}

type configuredStore interface {
	ListAcceptedCuratedForBook(context.Context, string, string) ([]Entry, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func DedupKey(language, lemma, upos, owner string) string {
	sum := sha256.Sum256([]byte(language + " | " + lemma + " | " + upos + " | " + owner))
	return hex.EncodeToString(sum[:])
}

func Cloze(sentence, target, hint string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || strings.TrimSpace(sentence) == "" {
		return "", ErrInvalidInput
	}
	start := strings.Index(strings.ToLower(sentence), strings.ToLower(target))
	if start < 0 {
		return "", fmt.Errorf("%w: target %q not found in sentence", ErrInvalidInput, target)
	}
	end := start + len(target)
	mark := "{{c1::" + sentence[start:end]
	if hint = strings.TrimSpace(hint); hint != "" {
		mark += "::" + hint
	}
	mark += "}}"
	return sentence[:start] + mark + sentence[end:], nil
}

func makeNote(owner string, entry Entry) (Note, error) {
	target := strings.TrimSpace(entry.TargetWord)
	if target == "" {
		target = entry.CanonicalLemma
	}
	text, err := Cloze(entry.Sentence, target, entry.Translation)
	if err != nil {
		return Note{}, err
	}
	tags := uniqueTags("mouseion", entry.Language, entry.SourceDocument)
	back := strings.Join([]string{
		"Sentence: " + entry.Sentence, "Translation: " + entry.Translation,
		"Target: " + target, "Lemma: " + entry.CanonicalLemma, "POS: " + entry.UPOS,
		"Morphology: " + entry.Morphology, "Source: " + entry.SourceDocument, "Notes: " + entry.Notes,
	}, "\n")
	return Note{Key: DedupKey(entry.Language, entry.CanonicalLemma, entry.UPOS, owner), Text: text, BackExtra: back, Tags: tags}, nil
}

func uniqueTags(values ...string) []string {
	seen := map[string]bool{}
	var tags []string
	for _, value := range values {
		value = strings.Join(strings.Fields(value), "_")
		if value != "" && !seen[value] {
			seen[value] = true
			tags = append(tags, value)
		}
	}
	return tags
}

func RenderTSV(notes []Note) (string, error) {
	var out bytes.Buffer
	w := csv.NewWriter(&out)
	w.Comma, w.UseCRLF = '\t', false
	for _, n := range notes {
		if err := w.Write([]string{n.Key, n.Text, n.BackExtra, strings.Join(n.Tags, " ")}); err != nil {
			return "", err
		}
	}
	w.Flush()
	return out.String(), w.Error()
}

func NoteTypeDefinition() string {
	return "Mouseion Cloze\nFields (in order): Key, Text, Back Extra, Tags\nSet Key as the first field and use it for duplicate checking.\nCard template: {{cloze:Text}}\nBack template: {{cloze:Text}}<hr id=answer>{{Back Extra}}\nImport: UTF-8, tab-separated, allow HTML, map Tags to Tags.\n"
}

func (s *Service) Export(ctx context.Context, owner, deckName string) (Artifact, error) {
	return s.export(ctx, owner, deckName, ExportConfig{})
}

// ExportConfigured limits an export to one of the owner's books, optionally
// removes their known vocabulary, and orders cards using ADR 0005 components.
func (s *Service) ExportConfigured(ctx context.Context, owner, deckName string, cfg ExportConfig) (Artifact, error) {
	if strings.TrimSpace(cfg.BookID) == "" || !validRanking(cfg.Ranking) {
		return Artifact{}, ErrInvalidInput
	}
	return s.export(ctx, owner, deckName, cfg)
}

// ExportCoverage exports the smallest set of unknown lemmas accounting for at
// least 97 percent of the book's unknown lemma tokens, in reading order.
func (s *Service) ExportCoverage(ctx context.Context, owner, bookID string) (Artifact, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return Artifact{}, ErrInvalidInput
	}
	candidates, err := s.store.ListSelectionCandidatesForBook(ctx, owner, bookID)
	if err != nil {
		return Artifact{}, fmt.Errorf("list selection candidates: %w", err)
	}
	selected, err := s.coverageCandidates(ctx, owner, candidates)
	if err != nil {
		return Artifact{}, err
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].FirstEncounter != selected[j].FirstEncounter {
			return selected[i].FirstEncounter < selected[j].FirstEncounter
		}
		return candidateKey(selected[i]) < candidateKey(selected[j])
	})

	entries := make([]Entry, 0, len(selected))
	deckName := bookID
	for _, candidate := range selected {
		entry, err := s.store.GetCoverageEntryForBook(ctx, owner, bookID, candidate)
		if err != nil {
			return Artifact{}, fmt.Errorf("get coverage entry %s: %w", candidateKey(candidate), err)
		}
		entry.TargetWord = targetWord(entry.Sentence, candidate)
		entries = append(entries, entry)
		if strings.TrimSpace(entry.SourceDocument) != "" {
			deckName = entry.SourceDocument
		}
	}
	return s.renderAndRecord(ctx, owner, deckName, entries)
}

func targetWord(sentence string, candidate domain.SelectionCandidate) string {
	var forms []string
	if json.Unmarshal(candidate.ObservedForms, &forms) == nil {
		for _, form := range forms {
			if strings.Contains(strings.ToLower(sentence), strings.ToLower(form)) {
				return form
			}
		}
	}
	return candidate.CanonicalLemma
}

const coveragePercent = 97

func (s *Service) coverageCandidates(ctx context.Context, owner string, candidates []domain.SelectionCandidate) ([]domain.SelectionCandidate, error) {
	knownByLanguage := make(map[string]map[string]bool)
	unknown := make([]domain.SelectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		known, ok := knownByLanguage[candidate.Language]
		if !ok {
			words, err := s.store.ListKnownVocabulary(ctx, owner, candidate.Language)
			if err != nil {
				return nil, fmt.Errorf("list known vocabulary for %s: %w", candidate.Language, err)
			}
			known = make(map[string]bool, len(words))
			for _, word := range words {
				known[word.CanonicalLemma+"\x00"+word.UPOS] = true
			}
			knownByLanguage[candidate.Language] = known
		}
		if !known[candidate.CanonicalLemma+"\x00"+candidate.UPOS] && !known[candidate.CanonicalLemma+"\x00"] {
			unknown = append(unknown, candidate)
		}
	}
	sort.SliceStable(unknown, func(i, j int) bool {
		if unknown[i].OccurrenceCount != unknown[j].OccurrenceCount {
			return unknown[i].OccurrenceCount > unknown[j].OccurrenceCount
		}
		return candidateKey(unknown[i]) < candidateKey(unknown[j])
	})
	total := 0
	for _, candidate := range unknown {
		total += candidate.OccurrenceCount
	}
	cumulative, count := 0, 0
	for count < len(unknown) && cumulative*100 < total*coveragePercent {
		cumulative += unknown[count].OccurrenceCount
		count++
	}
	return unknown[:count], nil
}

func candidateKey(candidate domain.SelectionCandidate) string {
	return candidate.Language + "\x00" + candidate.CanonicalLemma + "\x00" + candidate.UPOS
}

func (s *Service) export(ctx context.Context, owner, deckName string, cfg ExportConfig) (Artifact, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(deckName) == "" {
		return Artifact{}, ErrInvalidInput
	}
	var entries []Entry
	var err error
	if cfg.BookID == "" {
		entries, err = s.store.ListAcceptedCurated(ctx, owner)
	} else if store, ok := s.store.(configuredStore); ok {
		entries, err = store.ListAcceptedCuratedForBook(ctx, owner, cfg.BookID)
	} else {
		return Artifact{}, errors.New("cardexport: configured exports are unsupported by the store")
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("list accepted curated entries: %w", err)
	}
	if cfg.FilterKnown {
		entries = filterKnown(entries, cfg.KnownWords)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if cfg.BookID != "" && (cfg.Ranking == "" || cfg.Ranking == RankingDefault || cfg.Ranking == RankingEncounter) {
			if a.FirstEncounter != b.FirstEncounter {
				return a.FirstEncounter < b.FirstEncounter
			}
		} else if cfg.Ranking != "" && cfg.Ranking != RankingDefault {
			aScore, bScore := rankingScore(a.Ranking, cfg.Ranking), rankingScore(b.Ranking, cfg.Ranking)
			if aScore != bScore {
				return aScore > bScore
			}
		}
		return a.Language+"\x00"+a.CanonicalLemma+"\x00"+a.UPOS < b.Language+"\x00"+b.CanonicalLemma+"\x00"+b.UPOS
	})
	return s.renderAndRecord(ctx, owner, deckName, entries)
}

func (s *Service) renderAndRecord(ctx context.Context, owner, deckName string, entries []Entry) (Artifact, error) {
	notes := make([]Note, 0, len(entries))
	for _, entry := range entries {
		n, err := makeNote(owner, entry)
		if err != nil {
			return Artifact{}, fmt.Errorf("render %s/%s/%s: %w", entry.Language, entry.CanonicalLemma, entry.UPOS, err)
		}
		if err := s.store.RecordGenerated(ctx, owner, deckName, entry, n); err != nil {
			return Artifact{}, fmt.Errorf("record generated %s/%s/%s: %w", entry.Language, entry.CanonicalLemma, entry.UPOS, err)
		}
		notes = append(notes, n)
	}
	tsv, err := RenderTSV(notes)
	return Artifact{TSV: tsv, NoteType: NoteTypeDefinition(), Count: len(notes)}, err
}

func validRanking(choice RankingChoice) bool {
	return choice == "" || choice == RankingDefault || choice == RankingEncounter || choice == RankingBook || choice == RankingGlobal || choice == RankingBalanced
}

func rankingScore(c domain.RankingComponents, choice RankingChoice) float64 {
	switch choice {
	case RankingBook:
		return c.CorpusPercentile
	case RankingGlobal:
		return c.GlobalPercentile
	default:
		priority := 0.0
		if c.Priority {
			priority = 1
		}
		crossText := max(1, min(c.CrossText, 5))
		return .6*c.GlobalPercentile + .3*c.CorpusPercentile + .1*priority + .05*float64(crossText-1)
	}
}

func filterKnown(entries []Entry, known []KnownWord) []Entry {
	isKnown := make(map[string]bool, len(known))
	for _, word := range known {
		isKnown[word.Language+"\x00"+word.CanonicalLemma+"\x00"+word.UPOS] = true
	}
	result := entries[:0]
	for _, entry := range entries {
		exact := entry.Language + "\x00" + entry.CanonicalLemma + "\x00" + entry.UPOS
		wildcard := entry.Language + "\x00" + entry.CanonicalLemma + "\x00"
		if !isKnown[exact] && !isKnown[wildcard] {
			result = append(result, entry)
		}
	}
	return result
}
