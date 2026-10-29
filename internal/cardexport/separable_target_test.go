package cardexport

import (
	"testing"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func recoveredAdvmodTokens(t *testing.T, text string, tokens ...*mouseionv1.Token) []analyzer.Token {
	t.Helper()
	corpus, err := analyzer.FromProto(&mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.1.0",
		Language:      "de",
		Sentences:     []*mouseionv1.Sentence{{Text: text, Tokens: tokens}},
	})
	require.NoError(t, err)
	return corpus.Sentences[0].Tokens
}

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

	note, err := makeNote("owner", renderInputFromEntry(entry))

	require.NoError(t, err)
	assert.Equal(t, "Karam <b>rief</b> dem jungen Scheich <b>entgegen</b>.", note.Text)
}

func TestMakeNoteBoldsRecoveredAdvmodParticle(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "fernsehen", UPOS: "VERB",
		Sentence:   "Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.",
		TargetWord: "sieht",
		SentenceTokens: recoveredAdvmodTokens(t, "Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.",
			&mouseionv1.Token{Surface: "Großmutter", CanonicalLemma: "großmutter", Pos: "NOUN", Dependency: "nsubj", Head: 1},
			&mouseionv1.Token{Surface: "sieht", CanonicalLemma: "fernsehen", Pos: "VERB", Dependency: "root", Head: 1},
			&mouseionv1.Token{Surface: "im", CanonicalLemma: "in", Pos: "ADP", Dependency: "case", Head: 3},
			&mouseionv1.Token{Surface: "Wohnzimmer", CanonicalLemma: "wohnzimmer", Pos: "NOUN", Dependency: "obl", Head: 1},
			&mouseionv1.Token{Surface: "fern", CanonicalLemma: "fern", Pos: "ADV", Dependency: "advmod", Head: 1},
			&mouseionv1.Token{Surface: "ohne", CanonicalLemma: "ohne", Pos: "ADP", Dependency: "case", Head: 6},
			&mouseionv1.Token{Surface: "Bild", CanonicalLemma: "bild", Pos: "NOUN", Dependency: "obl", Head: 1},
			&mouseionv1.Token{Surface: "und", CanonicalLemma: "und", Pos: "CCONJ", Dependency: "cc", Head: 6},
			&mouseionv1.Token{Surface: "Ton", CanonicalLemma: "ton", Pos: "NOUN", Dependency: "conj", Head: 6},
		),
	}

	note, err := makeNote("owner", renderInputFromEntry(entry))

	require.NoError(t, err)
	assert.Equal(t, "Großmutter <b>sieht</b> im Wohnzimmer <b>fern</b> ohne Bild und Ton.", note.Text)
}

func TestMakeNoteLeavesTrueAdverbUnbolded(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "liegen", UPOS: "VERB",
		Sentence:   "Das Dorf liegt fern der Stadt.",
		TargetWord: "liegt",
		SentenceTokens: recoveredAdvmodTokens(t, "Das Dorf liegt fern der Stadt.",
			&mouseionv1.Token{Surface: "Das", CanonicalLemma: "der", Pos: "DET", Dependency: "det", Head: 1},
			&mouseionv1.Token{Surface: "Dorf", CanonicalLemma: "dorf", Pos: "NOUN", Dependency: "nsubj", Head: 2},
			&mouseionv1.Token{Surface: "liegt", CanonicalLemma: "liegen", Pos: "VERB", Dependency: "root", Head: 2},
			&mouseionv1.Token{Surface: "fern", CanonicalLemma: "fern", Pos: "ADV", Dependency: "advmod", Head: 2},
			&mouseionv1.Token{Surface: "der", CanonicalLemma: "der", Pos: "DET", Dependency: "det", Head: 5},
			&mouseionv1.Token{Surface: "Stadt", CanonicalLemma: "stadt", Pos: "NOUN", Dependency: "nmod", Head: 3},
		),
	}

	note, err := makeNote("owner", renderInputFromEntry(entry))

	require.NoError(t, err)
	assert.Equal(t, "Das Dorf <b>liegt</b> fern der Stadt.", note.Text)
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

	note, err := makeNote("owner", renderInputFromEntry(entry))

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

	note, err := makeNote("owner", renderInputFromEntry(entry))

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

	note, err := makeNote("owner", renderInputFromEntry(entry))

	require.NoError(t, err)
	assert.Equal(t, "Karam <b>rief</b> <b>entgegen</b> und stand auf.", note.Text)
}

func TestMakeNoteWithoutParseBoldsObservedFormOnly(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB",
		Sentence: "Karam rief dem jungen Scheich entgegen.", TargetWord: "rief",
	}

	note, err := makeNote("owner", renderInputFromEntry(entry))

	require.NoError(t, err)
	assert.Equal(t, "Karam <b>rief</b> dem jungen Scheich entgegen.", note.Text)
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

	manifest := newManifest("owner", "Buch", []Entry{entry})
	snapshot := manifest.Snapshot()
	require.Len(t, snapshot.Items, 1)
	require.True(t, snapshot.Items[0].Quality.Accepted, "representative sentence was quality-omitted: %+v", snapshot.Items[0].Quality)
	require.Equal(t, tokens, snapshot.Items[0].Entry.SentenceTokens, "snapshot dropped the frozen parse")

	deck, err := NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	artifact, _, err := NewPresentation(nil).Finalize(t.Context(), deck, nil, RunFacts{})
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

	deck, err := NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	artifact, _, err := NewPresentation(nil).Finalize(t.Context(), deck, nil, RunFacts{})
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

	first, err := makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)
	for range 5 {
		again, err := makeNote("owner", renderInputFromEntry(entry))
		require.NoError(t, err)
		assert.Equal(t, first.Text, again.Text)
	}
}
