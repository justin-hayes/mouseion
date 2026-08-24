package cardexport

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryStore struct {
	entries      []Entry
	candidates   []domain.SelectionCandidate
	known        []domain.KnownVocabulary
	history      []domain.GeneratedVocabulary
	generated    []Note
	bookID       string
	historyCalls map[string]int
}

func (m *memoryStore) ListGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	if m.historyCalls == nil {
		m.historyCalls = make(map[string]int)
	}
	m.historyCalls[language]++
	var out []domain.GeneratedVocabulary
	for _, word := range m.history {
		if word.OwnerID == owner && word.Language == language {
			out = append(out, word)
		}
	}
	return out, nil
}

func (m *memoryStore) ListSelectionCandidatesForBook(_ context.Context, _ string, bookID string) ([]domain.SelectionCandidate, error) {
	if bookID != m.bookID {
		return nil, nil
	}
	return append([]domain.SelectionCandidate(nil), m.candidates...), nil
}
func (m *memoryStore) ListKnownVocabulary(_ context.Context, _ string, language string) ([]domain.KnownVocabulary, error) {
	var out []domain.KnownVocabulary
	for _, word := range m.known {
		if word.Language == language {
			out = append(out, word)
		}
	}
	return out, nil
}
func (m *memoryStore) GetCoverageEntryForBook(_ context.Context, owner, _ string, candidate domain.SelectionCandidate) (Entry, error) {
	for _, entry := range m.entries {
		if entry.OwnerID == owner && entry.Language == candidate.Language && entry.CanonicalLemma == candidate.CanonicalLemma && entry.UPOS == candidate.UPOS {
			return entry, nil
		}
	}
	return Entry{}, errors.New("missing entry")
}

func (m *memoryStore) RecordGeneratedForBook(_ context.Context, owner, bookID, _ string, e Entry, n Note) error {
	if owner != e.OwnerID {
		return ErrInvalidInput
	}
	if bookID != m.bookID {
		return ErrInvalidInput
	}
	m.generated = append(m.generated, n)
	return nil
}

func TestDedupKeyStableAndOwnerScoped(t *testing.T) {
	a := DedupKey("de", "Haus", "NOUN", "alice")
	if a != DedupKey("de", "Haus", "NOUN", "alice") || len(a) != 64 {
		t.Fatalf("unstable key %q", a)
	}
	for _, b := range []string{DedupKey("de", "Haus", "NOUN", "bob"), DedupKey("fr", "Haus", "NOUN", "alice"), DedupKey("de", "haus", "NOUN", "alice"), DedupKey("de", "Haus", "VERB", "alice")} {
		if a == b {
			t.Fatal("distinct identity produced same key")
		}
	}
}

func TestClozeWithAndWithoutHint(t *testing.T) {
	got, err := Cloze("Das Haus ist groß.", "Haus", "house")
	if err != nil || got != "Das {{c1::Haus::house}} ist groß." {
		t.Fatalf("got=%q err=%v", got, err)
	}
	got, err = Cloze("Das Haus ist groß.", "haus", "")
	if err != nil || got != "Das {{c1::Haus}} ist groß." {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err = Cloze("Kein Treffer.", "Haus", ""); err == nil {
		t.Fatal("missing target accepted")
	}
}

func TestRenderTSVEscapesAndOrdersFields(t *testing.T) {
	n := Note{Key: "key", Text: "Grüße\t{{c1::Welt}}", BackExtra: "line one\nline two", Tags: []string{"mouseion", "de", "My_Book"}}
	got, err := RenderTSV([]Note{n})
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(got))
	r.Comma = '\t'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 4 || rows[0][0] != "key" || rows[0][1] != n.Text || rows[0][2] != n.BackExtra || rows[0][3] != "mouseion de My_Book" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestMakeNoteFormatsLemmaWithoutChangingTargetOrIdentity(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Die Häuser sind alt.", TargetWord: "Häuser",
	}
	note, err := makeNote("alice", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note.Text, "{{c1::Häuser}}") || !strings.Contains(note.BackExtra, "Target: Häuser\nLemma: Haus\n") {
		t.Fatalf("note = %#v", note)
	}
	if want := DedupKey("de", "haus", "NOUN", "alice"); note.Key != want {
		t.Fatalf("note key = %q, want canonical identity key %q", note.Key, want)
	}
}

func TestCoverageCandidatesExcludesKnownAndGeneratedBeforeCutoff(t *testing.T) {
	otherBook := "other-book"
	store := &memoryStore{known: []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}}}
	store.history = []domain.GeneratedVocabulary{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", FirstSourceMaterialID: &otherBook},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN"},
	}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "one", UPOS: "NOUN", OccurrenceCount: 96},
		{Language: "de", CanonicalLemma: "singleton-a", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-b", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-c", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-d", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", "current-book", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].CanonicalLemma != "one" || got[1].CanonicalLemma != "singleton-a" {
		t.Fatalf("coverage candidates = %#v", got)
	}
	if store.historyCalls["de"] != 1 {
		t.Fatalf("generated vocabulary loaded %d times", store.historyCalls["de"])
	}
}

func TestCoverageCandidatesAllowsSameBookAndIsolatesOwners(t *testing.T) {
	currentBook := "current-book"
	store := &memoryStore{history: []domain.GeneratedVocabulary{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "same", UPOS: "NOUN", FirstSourceMaterialID: &currentBook},
		{OwnerID: "bob", Language: "de", CanonicalLemma: "bob-word", UPOS: "NOUN"},
	}}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "same", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "bob-word", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", currentBook, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("coverage candidates = %#v", got)
	}
}

func TestCoverageCandidatesKnownLemmaWildcardAndSingleton(t *testing.T) {
	store := &memoryStore{known: []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known"}}}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "known", UPOS: "VERB", OccurrenceCount: 10},
		{Language: "de", CanonicalLemma: "only", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", "book", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CanonicalLemma != "only" {
		t.Fatalf("coverage candidates = %#v", got)
	}
}

func TestScoreSentenceQuality(t *testing.T) {
	tests := []struct {
		name, sentence, target, reason string
		location                       int64
		accepted                       bool
	}{
		{"long contextual", "Vor dem alten Haus spielen heute mehrere fröhliche Kinder, während ihre Eltern im sonnigen Garten gemeinsam das Abendessen vorbereiten.", "Haus", "usable length", 42, true},
		{"too short", "Altes Haus.", "Haus", "too short or fragmented", 42, false},
		{"too long", "Das Haus " + strings.Repeat("steht weit außerhalb der alten Stadt ", 9) + "still.", "Haus", "too long", 42, false},
		{"missing target", "Vor dem alten Gebäude spielen heute mehrere fröhliche Kinder.", "Haus", "target not present as a word", 42, false},
		{"fragmented boundary", "Vor dem alten Haus spielen heute mehrere Kinder", "Haus", "incomplete sentence boundaries", 42, false},
		{"contents", "Inhaltsverzeichnis: Das Haus und seine lange Geschichte ..... 12.", "Haus", "structural noise or boilerplate", 42, false},
		{"bibliography", "Bibliography: Das Haus in der europäischen Literatur.", "Haus", "structural noise or boilerplate", 42, false},
		{"boilerplate", "All rights reserved for this edition of Haus und Garten.", "Haus", "structural noise or boilerplate", 42, false},
		{"invalid location", "Vor dem alten Haus spielen heute mehrere fröhliche Kinder.", "Haus", "invalid source location", -1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreSentenceQuality(tc.sentence, tc.target, tc.location)
			if got.Accepted != tc.accepted || !contains(got.Reasons, tc.reason) {
				t.Fatalf("quality=%+v", got)
			}
		})
	}
}

func TestExportCoverageOmitsBadEvidenceAndRecordsOnlyAcceptedNotes(t *testing.T) {
	store := &memoryStore{bookID: "book"}
	store.candidates = []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1, FirstEncounter: 10, ObservedForms: []byte(`["Haus"]`)},
		{Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN", OccurrenceCount: 1, FirstEncounter: 20, ObservedForms: []byte(`["Baum"]`)},
	}
	store.entries = []Entry{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Haus.", SourceDocument: "Book", FirstEncounter: 10},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN", Sentence: "Unter dem alten Baum warten heute mehrere müde Wanderer.", SourceDocument: "Book", FirstEncounter: 20},
	}

	artifact, err := NewService(store).ExportCoverage(context.Background(), "alice", "book")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || len(artifact.Omitted) != 1 || artifact.Omitted[0].CanonicalLemma != "Haus" || len(store.generated) != 1 || !strings.Contains(artifact.TSV, "Baum") || strings.Contains(artifact.TSV, "Haus") {
		t.Fatalf("artifact=%+v generated=%+v", artifact, store.generated)
	}

	// Rejected evidence was not added to generated history, so improved evidence
	// remains eligible on a later export.
	store.entries[0].Sentence = "Vor dem alten Haus spielen heute mehrere fröhliche Kinder."
	artifact, err = NewService(store).ExportCoverage(context.Background(), "alice", "book")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 2 || len(artifact.Omitted) != 0 || len(store.generated) != 3 || !strings.Contains(artifact.TSV, "Haus") {
		t.Fatalf("later artifact=%+v generated=%+v", artifact, store.generated)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
