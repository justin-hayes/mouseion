package cardexport

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeNoteBoldsEveryComponentOfSeparableVerb(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB",
		Sentence: "Karam rief dem jungen Scheich entgegen.", TargetWord: "rief",
		SentenceTokens: []analyzer.Token{
			{Surface: "Karam", UPOS: "PROPN", Dependency: "nsubj", Head: 1},
			{Surface: "rief", UPOS: "VERB", Dependency: "root", Head: 1},
			{Surface: "dem", UPOS: "DET", Dependency: "det", Head: 4},
			{Surface: "jungen", UPOS: "ADJ", Dependency: "amod", Head: 4},
			{Surface: "Scheich", UPOS: "NOUN", Dependency: "obl", Head: 1},
			{Surface: "entgegen", UPOS: "ADV", Dependency: "compound:prt", Head: 1},
		},
	}

	note, err := makeNote("owner", entry)

	require.NoError(t, err)
	assert.Equal(t, "Karam <b>rief</b> dem jungen Scheich <b>entgegen</b>.", note.Text)
}

func TestMakeNoteBoldsAttachedSeparableVerbOnce(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "aufstehen", UPOS: "VERB",
		Sentence: "Sie ist gestern aufgestanden.", TargetWord: "aufgestanden",
		SentenceTokens: []analyzer.Token{
			{Surface: "Sie", UPOS: "PRON", Dependency: "nsubj", Head: 2},
			{Surface: "ist", UPOS: "AUX", Dependency: "aux", Head: 2},
			{Surface: "gestern", UPOS: "ADV", Dependency: "advmod", Head: 2},
			{Surface: "aufgestanden", UPOS: "VERB", Dependency: "root", Head: 3},
		},
	}

	note, err := makeNote("owner", entry)

	require.NoError(t, err)
	assert.Equal(t, "Sie ist gestern <b>aufgestanden</b>.", note.Text)
}

func TestMakeNoteBoldsEverySeparableParticle(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "entgegenzurückrufen", UPOS: "VERB",
		Sentence: "Er rief ihr entgegen und zurück.", TargetWord: "rief",
		SentenceTokens: []analyzer.Token{
			{Surface: "Er", UPOS: "PRON", Dependency: "nsubj", Head: 1},
			{Surface: "rief", UPOS: "VERB", Dependency: "root", Head: 1},
			{Surface: "ihr", UPOS: "PRON", Dependency: "iobj", Head: 1},
			{Surface: "entgegen", UPOS: "ADV", Dependency: "compound:prt", Head: 1},
			{Surface: "und", UPOS: "CCONJ", Dependency: "cc", Head: 1},
			{Surface: "zurück", UPOS: "ADV", Dependency: "compound:prt", Head: 1},
		},
	}

	note, err := makeNote("owner", entry)

	require.NoError(t, err)
	assert.Equal(t, "Er <b>rief</b> ihr <b>entgegen</b> und <b>zurück</b>.", note.Text)
}

func TestMakeNoteIgnoresParticlesOfOtherVerbs(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB",
		Sentence: "Karam rief entgegen und stand auf.", TargetWord: "rief",
		SentenceTokens: []analyzer.Token{
			{Surface: "Karam", UPOS: "PROPN", Dependency: "nsubj", Head: 1},
			{Surface: "rief", UPOS: "VERB", Dependency: "root", Head: 1},
			{Surface: "entgegen", UPOS: "ADV", Dependency: "compound:prt", Head: 1},
			{Surface: "und", UPOS: "CCONJ", Dependency: "cc", Head: 4},
			{Surface: "stand", UPOS: "VERB", Dependency: "root", Head: 4},
			{Surface: "auf", UPOS: "ADV", Dependency: "compound:prt", Head: 4},
		},
	}

	note, err := makeNote("owner", entry)

	require.NoError(t, err)
	assert.Equal(t, "Karam <b>rief</b> <b>entgegen</b> und stand auf.", note.Text)
}

func TestMakeNoteWithoutParseBoldsObservedFormOnly(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB",
		Sentence: "Karam rief dem jungen Scheich entgegen.", TargetWord: "rief",
	}

	note, err := makeNote("owner", entry)

	require.NoError(t, err)
	assert.Equal(t, "Karam <b>rief</b> dem jungen Scheich entgegen.", note.Text)
}

func TestExportCoverageBoldsSeparableVerbFromPersistedParse(t *testing.T) {
	const owner, bookID = "alice", "book"
	store := &persistedMemoryStore{
		memoryStore: &memoryStore{bookID: bookID},
		sentences: map[int64]analyzer.Sentence{
			0: {
				Text: "Karam rief dem jungen Scheich entgegen.",
				Tokens: []analyzer.Token{
					{Surface: "Karam", UPOS: "PROPN", Dependency: "nsubj", Head: 1},
					{Surface: "rief", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}},
					{Surface: "dem", UPOS: "DET", Dependency: "det", Head: 4},
					{Surface: "jungen", UPOS: "ADJ", Dependency: "amod", Head: 4},
					{Surface: "Scheich", UPOS: "NOUN", Dependency: "obl", Head: 1},
					{Surface: "entgegen", UPOS: "ADV", Dependency: "compound:prt", Head: 1},
				},
			},
		},
	}
	store.candidates = []domain.SelectionCandidate{
		{OwnerID: owner, CorpusID: "corpus", Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB", OccurrenceCount: 3, FirstEncounter: 10, ObservedForms: []byte(`["rief"]`), SentenceReferences: []byte(`[{"sentence_index":0,"text":"stale text","location":{"start_offset":10}}]`)},
	}
	store.entries = []Entry{
		{OwnerID: owner, Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB", SourceDocument: "Book"},
	}

	artifact, err := NewService(store).ExportCoverage(context.Background(), owner, bookID)

	require.NoError(t, err)
	require.Equal(t, 1, artifact.Count)
	assert.Contains(t, artifact.TSV, "Karam <b>rief</b> dem jungen Scheich <b>entgegen</b>.")
	assert.NotContains(t, artifact.TSV, "stale text")
}

func TestNewManifestSnapshotPreservesFrozenParseForBolding(t *testing.T) {
	const sentence = "Im Haus des Erpressers strahlten ihre Schwestern sie an."
	tokens := []analyzer.Token{
		{Surface: "Im", UPOS: "ADP", Dependency: "case", Head: 1},
		{Surface: "Haus", UPOS: "NOUN", Dependency: "obl", Head: 4},
		{Surface: "des", UPOS: "DET", Dependency: "det", Head: 3},
		{Surface: "Erpressers", UPOS: "NOUN", Dependency: "nmod", Head: 1},
		{Surface: "strahlten", UPOS: "VERB", Dependency: "root", Head: 4, Morphology: map[string]string{"VerbForm": "Fin"}},
		{Surface: "ihre", UPOS: "DET", Dependency: "det", Head: 6},
		{Surface: "Schwestern", UPOS: "NOUN", Dependency: "nsubj", Head: 4},
		{Surface: "sie", UPOS: "PRON", Dependency: "obj", Head: 4},
		{Surface: "an", UPOS: "ADV", Dependency: "compound:prt", Head: 4},
	}
	entry := Entry{
		Language: "de", CanonicalLemma: "anstrahlen", UPOS: "VERB",
		Sentence: sentence, TargetWord: "strahlten", SourceDocument: "Book",
		SentenceTokens: tokens,
	}

	manifest := NewManifest("owner", "Buch", []Entry{entry})
	snapshot := manifest.Snapshot()
	require.Len(t, snapshot.Items, 1)
	require.True(t, snapshot.Items[0].Quality.Accepted, "representative sentence was quality-omitted: %+v", snapshot.Items[0].Quality)
	require.Equal(t, tokens, snapshot.Items[0].Entry.SentenceTokens, "snapshot dropped the frozen parse")

	rebuilt, err := ManifestFromSnapshot(snapshot)
	require.NoError(t, err)
	artifact, err := (&Service{}).RenderManifest(context.Background(), rebuilt, nil)
	require.NoError(t, err)
	assert.Contains(t, artifact.TSV, "Im Haus des Erpressers <b>strahlten</b> ihre Schwestern sie <b>an</b>.")
}

func TestRenderManifestBoldsSeparableVerbFromFrozenParse(t *testing.T) {
	const sentence = "Im Haus des Erpressers strahlten ihre Schwestern sie an."
	snapshot := ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersion,
		Owner:         "owner", DeckName: "Buch", Filename: DownloadFilename("Buch"),
		Items: []ManifestItem{{
			Ordinal: 0, Disposition: ManifestAccepted,
			Entry: Entry{
				Language: "de", CanonicalLemma: "anstrahlen", UPOS: "VERB",
				Sentence: sentence, TargetWord: "strahlten", SourceDocument: "Book",
				SentenceTokens: []analyzer.Token{
					{Surface: "Im", UPOS: "ADP", Dependency: "case", Head: 1},
					{Surface: "Haus", UPOS: "NOUN", Dependency: "obl", Head: 4},
					{Surface: "des", UPOS: "DET", Dependency: "det", Head: 3},
					{Surface: "Erpressers", UPOS: "NOUN", Dependency: "nmod", Head: 1},
					{Surface: "strahlten", UPOS: "VERB", Dependency: "root", Head: 4},
					{Surface: "ihre", UPOS: "DET", Dependency: "det", Head: 6},
					{Surface: "Schwestern", UPOS: "NOUN", Dependency: "nsubj", Head: 4},
					{Surface: "sie", UPOS: "PRON", Dependency: "obj", Head: 4},
					{Surface: "an", UPOS: "ADV", Dependency: "compound:prt", Head: 4},
				},
			},
			Quality: SentenceQuality{Accepted: true, Score: 94, Reasons: []string{"target present"}},
		}},
	}

	rebuilt, err := ManifestFromSnapshot(snapshot)
	require.NoError(t, err)
	require.Len(t, rebuilt.accepted, 1)
	assert.Equal(t, snapshot.Items[0].Entry.SentenceTokens, rebuilt.accepted[0].SentenceTokens, "frozen parse lost in snapshot round trip")

	artifact, err := (&Service{}).RenderManifest(context.Background(), rebuilt, nil)
	require.NoError(t, err)
	assert.Contains(t, artifact.TSV, "Im Haus des Erpressers <b>strahlten</b> ihre Schwestern sie <b>an</b>.")
}

func TestMakeNoteTargetBoldingIsDeterministic(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB",
		Sentence: "Karam rief dem jungen Scheich entgegen.", TargetWord: "rief",
		SentenceTokens: []analyzer.Token{
			{Surface: "rief", UPOS: "VERB", Dependency: "root", Head: 0},
			{Surface: "entgegen", UPOS: "ADV", Dependency: "compound:prt", Head: 0},
		},
	}

	first, err := makeNote("owner", entry)
	require.NoError(t, err)
	for range 5 {
		again, err := makeNote("owner", entry)
		require.NoError(t, err)
		assert.Equal(t, first.Text, again.Text)
	}
}
