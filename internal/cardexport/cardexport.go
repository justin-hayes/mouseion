// Package cardexport produces owner-scoped, Anki-compatible Cloze TSV exports.
package cardexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrInvalidInput = errors.New("cardexport: invalid input")

type Entry struct {
	OwnerID, Language, CanonicalLemma, UPOS string
	Sentence, Translation, TargetWord       string
	Morphology, SourceDocument, Notes       string
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
	RecordGenerated(context.Context, string, string, Entry, Note) error
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
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(deckName) == "" {
		return Artifact{}, ErrInvalidInput
	}
	entries, err := s.store.ListAcceptedCurated(ctx, owner)
	if err != nil {
		return Artifact{}, fmt.Errorf("list accepted curated entries: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		return a.Language+"\x00"+a.CanonicalLemma+"\x00"+a.UPOS < b.Language+"\x00"+b.CanonicalLemma+"\x00"+b.UPOS
	})
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
