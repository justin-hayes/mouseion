package knownvocab

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReportsMalformedRows(t *testing.T) {
	input := strings.NewReader("Haus\ngehen\tverb\n\nzu\tviele\tSpalten\n  Straße  \n")
	got, err := Parse(input)
	require.NoError(t, err)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "Haus", got.Entries[0].RawLemma)
	assert.Empty(t, got.Entries[0].UPOS)
	assert.Equal(t, "Straße", got.Entries[1].RawLemma)
	assert.Empty(t, got.Entries[1].UPOS)
	require.Len(t, got.Rejected, 2)
	assert.Equal(t, 2, got.Rejected[0].Row)
	assert.Equal(t, 4, got.Rejected[1].Row)
	assert.Contains(t, got.Rejected[0].Error, "no tab-separated columns")
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	got, err := Parse(strings.NewReader("Haus\n" + string([]byte{0xff, '\n'}) + "gehen\n"))
	require.NoError(t, err)
	require.Len(t, got.Entries, 2)
	require.Len(t, got.Rejected, 1)
	assert.Equal(t, 2, got.Rejected[0].Row)
	assert.Equal(t, "invalid UTF-8", got.Rejected[0].Error)
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
	require.NoError(t, err)
	assert.Equal(t, 2, first.Imported)
	assert.Zero(t, first.AlreadyKnown)
	require.Len(t, first.Rejected, 1)
	require.Len(t, first.Entries, 2)
	assert.Equal(t, " Daß ", first.Entries[0].Original)
	assert.Equal(t, "Daß", first.Entries[0].RawLemma)
	assert.Equal(t, "dass", first.Entries[0].CanonicalLemma)
	assert.Empty(t, first.Entries[0].UPOS)
	assert.NotEmpty(t, first.Entries[0].ProfileName)
	assert.NotEmpty(t, first.Entries[0].ProfileVersion)
	assert.Equal(t, "known", store.state[importKey("alice", "de", "dass", "")].State)
	assert.Equal(t, "known", store.state[importKey("alice", "de", "haus", "")].State)

	second, err := service.Import(context.Background(), "alice", "de", strings.NewReader(input))
	require.NoError(t, err)
	assert.Zero(t, second.Imported)
	assert.Equal(t, 2, second.AlreadyKnown)
	assert.Len(t, store.known, 2)
}

func TestImportPreservesModernGermanSharpS(t *testing.T) {
	store := newMemoryStore()
	result, err := NewService(store).Import(
		context.Background(), "alice", "de", strings.NewReader("Straße\nDaß\ngeleiten|leiten\n"),
	)
	require.NoError(t, err)
	require.Len(t, result.Entries, 3)
	assert.Equal(t, "straße", result.Entries[0].CanonicalLemma)
	assert.Equal(t, "dass", result.Entries[1].CanonicalLemma)
	assert.Equal(t, "geleiten|leiten", result.Entries[2].RawLemma)
	assert.Equal(t, "geleiten", result.Entries[2].CanonicalLemma)
	assert.Equal(t, "german-standard-post-1996", result.Entries[0].ProfileName)
	assert.Equal(t, "4", result.Entries[0].ProfileVersion)
}

func TestImportRejectsNonLexicalLemmasAndPreservesUnicodeWords(t *testing.T) {
	store := newMemoryStore()
	got, err := NewService(store).Import(context.Background(), "alice", "de", strings.NewReader("5\n—\nl'acqua\nStraße\nB2\n"))
	require.NoError(t, err)
	assert.Equal(t, 3, got.Imported)
	require.Len(t, got.Rejected, 2)
	assert.Len(t, store.known, 3)
	for _, rejection := range got.Rejected {
		assert.Contains(t, rejection.Error, "at least one letter", "rejection = %+v", rejection)
	}
}

func TestImportRequiresOwnerLanguageAndSupportedProfile(t *testing.T) {
	service := NewService(newMemoryStore())
	for _, tc := range []struct{ owner, language string }{{"", "de"}, {"alice", ""}} {
		_, err := service.Import(context.Background(), tc.owner, tc.language, strings.NewReader("Haus\n"))
		assert.ErrorIs(t, err, ErrInvalidInput, "Import(%q, %q)", tc.owner, tc.language)
	}
	got, err := service.Import(context.Background(), "alice", "zz", strings.NewReader("word\n"))
	require.NoError(t, err)
	require.Len(t, got.Rejected, 1)
	assert.Contains(t, got.Rejected[0].Error, "unsupported language")
}
