package knownvocab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestParseReportsMalformedRows(t *testing.T) {
	input := strings.NewReader("Haus\ngehen\tverb\n\nzu\tviele\tSpalten\n  Straße  \n")
	got, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].RawLemma != "Haus" || got.Entries[0].UPOS != "" || got.Entries[1].RawLemma != "Straße" || got.Entries[1].UPOS != "" {
		t.Fatalf("entries = %+v", got.Entries)
	}
	if len(got.Rejected) != 2 || got.Rejected[0].Row != 2 || got.Rejected[1].Row != 4 || !strings.Contains(got.Rejected[0].Error, "no tab-separated columns") {
		t.Fatalf("rejections = %+v", got.Rejected)
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	got, err := Parse(strings.NewReader("Haus\n" + string([]byte{0xff, '\n'}) + "gehen\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || len(got.Rejected) != 1 || got.Rejected[0].Row != 2 || got.Rejected[0].Error != "invalid UTF-8" {
		t.Fatalf("result = %+v", got)
	}
}

type memoryStore struct {
	known map[string]domain.KnownVocabulary
	state map[string]domain.VocabularyState
}

func newMemoryStore() *memoryStore {
	return &memoryStore{known: map[string]domain.KnownVocabulary{}, state: map[string]domain.VocabularyState{}}
}

func importKey(owner, language, lemma, upos string) string {
	return owner + "\x00" + language + "\x00" + lemma + "\x00" + upos
}

func (s *memoryStore) IsKnownVocabularyIdentity(_ context.Context, owner, language, lemma, upos string) (bool, error) {
	_, exact := s.known[importKey(owner, language, lemma, upos)]
	_, wildcard := s.known[importKey(owner, language, lemma, "")]
	return exact || wildcard, nil
}

func (s *memoryStore) PutKnownVocabulary(_ context.Context, owner, language, lemma, upos string) (domain.KnownVocabulary, error) {
	key := importKey(owner, language, lemma, upos)
	value, ok := s.known[key]
	if !ok {
		value = domain.KnownVocabulary{ID: key, OwnerID: owner, Language: language, CanonicalLemma: lemma, UPOS: upos, CreatedAt: time.Now()}
		s.known[key] = value
	}
	return value, nil
}

func (s *memoryStore) PutVocabularyState(_ context.Context, owner, language, lemma, upos, state string) (domain.VocabularyState, error) {
	key := importKey(owner, language, lemma, upos)
	value := domain.VocabularyState{ID: key, OwnerID: owner, Language: language, CanonicalLemma: lemma, UPOS: upos, State: state}
	s.state[key] = value
	return value, nil
}

func (s *memoryStore) ListKnownVocabulary(_ context.Context, owner, language string) ([]domain.KnownVocabulary, error) {
	var result []domain.KnownVocabulary
	for _, value := range s.known {
		if value.OwnerID == owner && value.Language == language {
			result = append(result, value)
		}
	}
	return result, nil
}

func TestImportCanonicalizesUpsertsAndReportsProvenance(t *testing.T) {
	store := newMemoryStore()
	service := NewService(store)
	input := " Daß \nHaus\ninvalid\tNOPE\n"

	first, err := service.Import(context.Background(), "alice", "de", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if first.Imported != 2 || first.AlreadyKnown != 0 || len(first.Rejected) != 1 {
		t.Fatalf("first = %+v", first)
	}
	if got := first.Entries[0]; got.Original != " Daß " || got.RawLemma != "Daß" || got.CanonicalLemma != "dass" || got.UPOS != "" || got.ProfileName == "" || got.ProfileVersion == "" {
		t.Fatalf("normalized entry = %+v", got)
	}
	if store.state[importKey("alice", "de", "dass", "")].State != "known" || store.state[importKey("alice", "de", "haus", "")].State != "known" {
		t.Fatalf("states = %+v", store.state)
	}

	second, err := service.Import(context.Background(), "alice", "de", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 || second.AlreadyKnown != 2 || len(store.known) != 2 {
		t.Fatalf("second = %+v, known = %+v", second, store.known)
	}
}

func TestImportPreservesModernGermanSharpS(t *testing.T) {
	store := newMemoryStore()
	result, err := NewService(store).Import(
		context.Background(), "alice", "de", strings.NewReader("Straße\nDaß\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 || result.Entries[0].CanonicalLemma != "straße" ||
		result.Entries[1].CanonicalLemma != "dass" ||
		result.Entries[0].ProfileName != "german-standard-post-1996" ||
		result.Entries[0].ProfileVersion != "2" {
		t.Fatalf("entries = %+v", result.Entries)
	}
}

func TestImportRejectsNonLexicalLemmasAndPreservesUnicodeWords(t *testing.T) {
	store := newMemoryStore()
	got, err := NewService(store).Import(context.Background(), "alice", "de", strings.NewReader("5\n—\nl'acqua\nStraße\nB2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Imported != 3 || len(got.Rejected) != 2 || len(store.known) != 3 {
		t.Fatalf("result=%+v known=%+v", got, store.known)
	}
	for _, rejection := range got.Rejected {
		if !strings.Contains(rejection.Error, "at least one letter") {
			t.Fatalf("rejection = %+v", rejection)
		}
	}
}

func TestImportRequiresOwnerLanguageAndSupportedProfile(t *testing.T) {
	service := NewService(newMemoryStore())
	for _, tc := range []struct{ owner, language string }{{"", "de"}, {"alice", ""}} {
		if _, err := service.Import(context.Background(), tc.owner, tc.language, strings.NewReader("Haus\n")); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("Import(%q, %q) error = %v", tc.owner, tc.language, err)
		}
	}
	got, err := service.Import(context.Background(), "alice", "zz", strings.NewReader("word\n"))
	if err != nil || len(got.Rejected) != 1 || !strings.Contains(got.Rejected[0].Error, "unsupported language") {
		t.Fatalf("unsupported profile result=%+v err=%v", got, err)
	}
}
