package cardexport

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type coverageLexicalStub struct {
	entries map[string]enrichment.LexicalEntry
}

func (s coverageLexicalStub) Name() string    { return "fixture" }
func (s coverageLexicalStub) Version() string { return "fixture-v1" }
func (s coverageLexicalStub) Lookup(_ context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	entry, found := s.entries[request.Language+"\x00"+request.CanonicalLemma+"\x00"+request.UPOS]
	return entry, found, nil
}

func TestPrepareCoverageLogsGlossCoverageByLanguageAndPOS(t *testing.T) {
	const owner, bookID = "alice", "book"
	store := &memoryStore{bookID: bookID, entries: []Entry{
		{OwnerID: owner, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das Haus ist heute groß.", TargetWord: "Haus", SourceDocument: "Book"},
		{OwnerID: owner, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", Sentence: "Die Kinder gehen heute nach Hause.", TargetWord: "gehen", SourceDocument: "Book"},
		{OwnerID: owner, Language: "it", CanonicalLemma: "casa", UPOS: "NOUN", Sentence: "La casa è molto grande.", TargetWord: "casa", SourceDocument: "Book"},
	}, candidates: []domain.SelectionCandidate{
		{OwnerID: owner, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 3, FirstEncounter: 10},
		{OwnerID: owner, Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 3, FirstEncounter: 20},
		{OwnerID: owner, Language: "it", CanonicalLemma: "casa", UPOS: "NOUN", OccurrenceCount: 3, FirstEncounter: 30},
	}}
	provider := coverageLexicalStub{entries: map[string]enrichment.LexicalEntry{
		"de\x00Haus\x00NOUN": {Senses: []enrichment.LexicalSense{{Gloss: "house"}}},
	}}

	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	manifest, err := NewServiceWithLexicalProvider(store, provider).PrepareCoverage(context.Background(), owner, bookID)

	require.NoError(t, err)
	assert.Len(t, manifest.Snapshot().Items, 3)
	assert.Equal(t, `gloss_coverage {"event":"gloss_coverage","groups":[{"language":"de","pos":"NOUN","selected":1,"with_gloss":1,"without_gloss":0},{"language":"de","pos":"VERB","selected":1,"with_gloss":0,"without_gloss":1},{"language":"it","pos":"NOUN","selected":1,"with_gloss":0,"without_gloss":1}]}`+"\n", output.String())
}
