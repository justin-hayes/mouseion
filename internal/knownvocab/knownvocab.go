// Package knownvocab imports per-user known-vocabulary lemma lists.
package knownvocab

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
)

var ErrInvalidInput = errors.New("knownvocab: owner, language, and input are required")

type Entry struct {
	Row            int
	Original       string
	RawLemma       string
	CanonicalLemma string
	UPOS           string
	ProfileName    string
	ProfileVersion string
}

type Rejection struct {
	Row      int
	Original string
	Error    string
}

type ParseResult struct {
	Entries  []Entry
	Rejected []Rejection
}

type ImportResult struct {
	Imported     int
	AlreadyKnown int
	Entries      []Entry
	Rejected     []Rejection
}

// Parse validates the UTF-8 lemma-list wire format. An empty UPOS is a
// deliberate wildcard: a one-column lemma marks every POS for that lemma known.
func Parse(reader io.Reader) (ParseResult, error) {
	if reader == nil {
		return ParseResult{}, ErrInvalidInput
	}
	var result ParseResult
	scanner := bufio.NewScanner(reader)
	// Permit long compound lemmas while retaining a finite input bound per row.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for row := 1; scanner.Scan(); row++ {
		raw := scanner.Bytes()
		original := string(append([]byte(nil), raw...))
		if !utf8.Valid(raw) {
			result.Rejected = append(result.Rejected, Rejection{Row: row, Original: original, Error: "invalid UTF-8"})
			continue
		}
		line := strings.TrimSuffix(original, "\r")
		if strings.ContainsRune(line, '\t') {
			result.Rejected = append(result.Rejected, Rejection{Row: row, Original: original, Error: "expected exactly one lemma with no tab-separated columns"})
			continue
		}
		lemma := strings.TrimSpace(line)
		if lemma == "" {
			// Blank lines are allowed and do not count as input rows.
			continue
		}
		result.Entries = append(result.Entries, Entry{Row: row, Original: original, RawLemma: lemma})
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read lemma list: %w", err)
	}
	return result, nil
}

type Store interface {
	IsKnownVocabularyIdentity(context.Context, string, string, string, string) (bool, error)
	PutKnownVocabulary(context.Context, string, string, string, string) (domain.KnownVocabulary, error)
	PutVocabularyState(context.Context, string, string, string, string, string) (domain.VocabularyState, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Import(ctx context.Context, owner, language string, reader io.Reader) (ImportResult, error) {
	owner, language = strings.TrimSpace(owner), strings.TrimSpace(language)
	if s == nil || s.store == nil || owner == "" || language == "" || reader == nil {
		return ImportResult{}, ErrInvalidInput
	}
	parsed, err := Parse(reader)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{Rejected: parsed.Rejected}
	for _, entry := range parsed.Entries {
		normalized, normalizeErr := canonicalization.Normalize(language, entry.RawLemma)
		if normalizeErr != nil {
			entry.ErrorTo(&result, normalizeErr)
			continue
		}
		entry.CanonicalLemma = normalized.CanonicalLemma
		entry.ProfileName = normalized.ProfileName
		entry.ProfileVersion = normalized.ProfileVersion
		if entry.CanonicalLemma == "" {
			entry.ErrorTo(&result, errors.New("canonical lemma is empty"))
			continue
		}
		known, lookupErr := s.store.IsKnownVocabularyIdentity(ctx, owner, language, entry.CanonicalLemma, entry.UPOS)
		if lookupErr != nil {
			return result, fmt.Errorf("check row %d: %w", entry.Row, lookupErr)
		}
		if _, putErr := s.store.PutKnownVocabulary(ctx, owner, language, entry.CanonicalLemma, entry.UPOS); putErr != nil {
			return result, fmt.Errorf("upsert known vocabulary row %d: %w", entry.Row, putErr)
		}
		if _, putErr := s.store.PutVocabularyState(ctx, owner, language, entry.CanonicalLemma, entry.UPOS, string(vocabulary.Known)); putErr != nil {
			return result, fmt.Errorf("set known state row %d: %w", entry.Row, putErr)
		}
		result.Entries = append(result.Entries, entry)
		if known {
			result.AlreadyKnown++
		} else {
			result.Imported++
		}
	}
	return result, nil
}

func (e Entry) ErrorTo(result *ImportResult, err error) {
	result.Rejected = append(result.Rejected, Rejection{Row: e.Row, Original: e.Original, Error: err.Error()})
}
