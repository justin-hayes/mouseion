package cardexport

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type lexicalStub struct {
	result enrichment.LexicalEntry
	found  bool
}

func (d lexicalStub) Name() string    { return "fixture" }
func (d lexicalStub) Version() string { return "fixture-v1" }
func (d lexicalStub) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return d.result, d.found, nil
}

func TestLexicalFieldsAreFrozenBeforeManifestAndRender(t *testing.T) {
	service := &lexicalResolver{lexical: lexicalStub{found: true, result: enrichment.LexicalEntry{
		Gender:                "Neut",
		Article:               "das",
		Plural:                "Häuser",
		IPA:                   "/haʊ̯s/",
		PrincipalParts:        "geht · ging · gegangen",
		Senses:                []enrichment.LexicalSense{{Gloss: "building"}},
		CandidateSenses:       []enrichment.LexicalSense{{Gloss: "building", EvidenceID: "wiktionary:de:haus:noun:1", Source: "wiktionary", Kind: "meaning", Origin: "Kaikki.org Wiktextract enwiktionary", Version: "fixture-v1", MatchStrength: "exact_lemma_pos"}},
		OmittedCandidateCount: 3,
	}}}
	entry := Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus"}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))
	assert.Equal(t, "building", entry.Gloss)
	assert.Equal(t, "Häuser", entry.Plural)
	assert.Equal(t, "/haʊ̯s/", entry.IPA)
	assert.Equal(t, "geht · ging · gegangen", entry.PrincipalParts)
	assert.Equal(t, "fixture-v1", entry.DictionaryProviderVersion)

	manifest := newManifest("owner", "Book", []Entry{entry})
	snapshot := manifest.Snapshot()
	assert.Equal(t, entry.Gloss, snapshot.Items[0].Entry.Gloss)
	assert.Equal(t, entry.IPA, snapshot.Items[0].Entry.IPA)
	assert.Equal(t, entry.PrincipalParts, snapshot.Items[0].Entry.PrincipalParts)
	assert.Equal(t, entry.DictionaryProviderVersion, snapshot.Items[0].Entry.DictionaryProviderVersion)
	assert.Equal(t, 3, snapshot.Items[0].Entry.OmittedEvidenceCount)
	assert.Equal(t, "wiktionary:de:haus:noun:1", snapshot.Items[0].Entry.CandidateSenses[0].EvidenceID)
	assert.Equal(t, "exact_lemma_pos", snapshot.Items[0].Entry.CandidateSenses[0].MatchStrength)
	note, err := makeNote("owner", renderInputFromEntry(snapshot.Items[0].Entry))
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Equal(t, "Häuser", note.Plural)
	assert.Equal(t, "/haʊ̯s/", note.IPA)
	assert.Equal(t, "geht · ging · gegangen", note.PrincipalParts)
	assert.Equal(t, entry.Gloss, note.Gloss)
	assert.Contains(t, note.BackExtra, "das Haus (Pl. Häuser)")
	assert.Contains(t, note.BackExtra, "/haʊ̯s/")
	assert.Contains(t, note.BackExtra, "geht · ging · gegangen")
}

func TestIdenticalPluralIsRenderedAndNoPluralSelfSuppresses(t *testing.T) {
	service := &lexicalResolver{lexical: lexicalStub{found: true, result: enrichment.LexicalEntry{
		Gender: "Masc", Article: "der", Plural: "Gauner",
		Senses: []enrichment.LexicalSense{{Gloss: "rogue"}},
	}}}
	entry := Entry{Language: "de", CanonicalLemma: "gauner", UPOS: "NOUN", Sentence: "Viele Gauner wurden gestern verhaftet.", TargetWord: "Gauner"}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))
	note, err := makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Equal(t, "Gauner", note.Plural)
	assert.Contains(t, note.BackExtra, "der Gauner (Pl. Gauner)")

	service = &lexicalResolver{lexical: lexicalStub{found: true, result: enrichment.LexicalEntry{
		Gender: "", Article: "", Plural: "",
		Senses: []enrichment.LexicalSense{{Gloss: "parents"}},
	}}}
	entry = Entry{Language: "de", CanonicalLemma: "eltern", UPOS: "NOUN", Sentence: "Meine Eltern wohnen seit Jahren am See.", TargetWord: "Eltern"}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))
	note, err = makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Empty(t, note.Plural)
	assert.NotContains(t, note.BackExtra, "(Pl.")
}

func TestMissingLexicalEntryKeepsMorphologyFallback(t *testing.T) {
	service := &lexicalResolver{lexical: lexicalStub{}}
	entry := Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Morphology: `{"Gender":"Neut"}`}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))
	assert.Equal(t, "fixture-v1", entry.DictionaryProviderVersion)
	note, err := makeNote("owner", renderInputFromEntry(Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus", Morphology: `{"Gender":"Neut"}`}))
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Empty(t, note.Gloss)
}

func TestConfiguredIndexMissIsReportedAsConfiguredWithoutEvidence(t *testing.T) {
	service := &lexicalResolver{lexical: lexicalStub{}}
	entry := Entry{Language: "de", CanonicalLemma: "unindexed", UPOS: "NOUN", Sentence: "Das unbekannte Wort steht heute neben dem Bahnhof.", TargetWord: "Wort"}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))
	diagnostics := manifestDiagnostics(newManifest("owner", "Book", []Entry{entry}))
	assert.Equal(t, []EvidenceCoverage{{Source: "wiktionary", Configured: true, Selected: 1}}, diagnostics.EvidenceCoverage)
}

func TestNonNounPluralIsNotRenderedOnCard(t *testing.T) {
	service := &lexicalResolver{lexical: lexicalStub{found: true, result: enrichment.LexicalEntry{
		Plural: "stehen auf",
		Senses: []enrichment.LexicalSense{{Gloss: "to get up"}},
	}}}
	for _, test := range []struct {
		name string
		upos string
	}{
		{name: "verb", upos: "VERB"},
		{name: "adjective", upos: "ADJ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := Entry{Language: "de", CanonicalLemma: "aufstehen", UPOS: test.upos, Sentence: "Wir stehen auf.", TargetWord: "stehen auf"}
			require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))
			assert.Equal(t, "to get up", entry.Gloss)
			assert.Empty(t, entry.Morphology)
		})
	}
}

func TestDictionaryIndexMorphologyRendersOnCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Neut","Article":"das","Plural":"Häuser","IPA":""}]', 'Neut', 'das', 'Häuser', '', ''); INSERT INTO entries VALUES ('de', 'see', 'NOUN', '[{"Gloss":"lake","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"der","Plural":"Seen","IPA":""},{"Gloss":"sea","Examples":[],"Topics":["tief","salzig"],"Tags":[],"Phrase":"","Gender":"Fem","Article":"die","Plural":"Meere","IPA":""}]', 'Masc', 'der', 'Seen', '', '')`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO entries VALUES ('de', 'straße', 'NOUN', '[{"Gloss":"street","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Fem","Article":"die","Plural":"Straßen","IPA":""}]', 'Fem', 'die', 'Straßen', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := dictionary.OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("dictionary index cleanup failed: %v", err)
		}
	})
	service := &lexicalResolver{lexical: index}
	entry := Entry{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das Haus steht heute neben dem Bahnhof.", TargetWord: "Haus",
		Morphology: `{"Gender":"Masc","Article":"der"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))

	note, err := makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Equal(t, "das", note.Article)
	assert.Contains(t, note.Gloss, "house")
	assert.Equal(t, "Häuser", note.Plural)
	assert.NotContains(t, note.Gloss, "(Pl.")
	assert.Contains(t, note.BackExtra, "das Haus")
	assert.Contains(t, note.BackExtra, "(Pl. Häuser)")

	sharpS := Entry{
		Language: "de", CanonicalLemma: "straße", UPOS: "NOUN",
		Sentence: "Die Straße führt zum Bahnhof.", TargetWord: "Straße",
		Morphology: `{"Gender":"Masc"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &sharpS))
	assert.Equal(t, `{"Article":"die","Gender":"Fem","Plural":"Straßen"}`, sharpS.Morphology)
	assert.Equal(t, "street", sharpS.Gloss)
	assert.Equal(t, "Straßen", sharpS.Plural)

	manifest := newManifest("owner", "Book", []Entry{sharpS})
	snapshot := manifest.Snapshot()
	assert.Equal(t, sharpS.Gloss, snapshot.Items[0].Entry.Gloss)
	note, err = makeNote("owner", renderInputFromEntry(snapshot.Items[0].Entry))
	require.NoError(t, err)
	assert.Equal(t, "die", note.Article)
	assert.Equal(t, "street", note.Gloss)
	assert.Equal(t, "Straßen", note.Plural)
	assert.Contains(t, note.BackExtra, "die Straße")

	ranked := Entry{
		Language: "de", CanonicalLemma: "see", UPOS: "NOUN",
		Sentence: "Die See ist tief und salzig.", TargetWord: "See",
		Morphology: `{"Gender":"Masc","Article":"der"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &ranked))
	note, err = makeNote("owner", renderInputFromEntry(ranked))
	require.NoError(t, err)
	assert.Equal(t, "die", note.Article)
	assert.True(t, strings.HasPrefix(note.Gloss, "sea · lake"), "ranked gloss = %q", note.Gloss)
	assert.Equal(t, "Meere", note.Plural)

	unindexed := Entry{
		Language: "de", CanonicalLemma: "baum", UPOS: "NOUN",
		Sentence: "Der Baum steht dort.", TargetWord: "Baum",
		Morphology: `{"Gender":"Masc"}`,
	}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &unindexed))
	assert.Empty(t, unindexed.Gloss)
	assert.Equal(t, "fixture-v1", unindexed.DictionaryProviderVersion)
	note, err = makeNote("owner", renderInputFromEntry(unindexed))
	require.NoError(t, err)
	assert.Equal(t, "der", note.Article)
	assert.NotContains(t, deckDescription([]RenderInput{renderInputFromEntry(unindexed)}), dictionary.AttributionNotice)
}

func TestItalianDictionaryIndexMorphologyRendersOnCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('it', 'casa', 'NOUN', '[{"Gloss":"house","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Fem","Article":"la","Plural":"case","IPA":""},{"Gloss":"home","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Fem","Article":"la","Plural":"case","IPA":""}]', 'Fem', 'la', 'case', '', ''); INSERT INTO entries VALUES ('it', 'libro', 'NOUN', '[{"Gloss":"book","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"il","Plural":"libri","IPA":""}]', 'Masc', 'il', 'libri', '', ''); INSERT INTO entries VALUES ('it', 'zaino', 'NOUN', '[{"Gloss":"backpack","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"lo","Plural":"zaini","IPA":""}]', 'Masc', 'lo', 'zaini', '', ''); INSERT INTO entries VALUES ('it', 'albero', 'NOUN', '[{"Gloss":"tree","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"l''","Plural":"alberi","IPA":""}]', 'Masc', 'l''', 'alberi', '', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := dictionary.OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("dictionary index cleanup failed: %v", err)
		}
	})
	service := &lexicalResolver{lexical: index}
	for _, test := range []struct {
		lemma, target, sentence, article, plural, gloss string
	}{
		{lemma: "casa", target: "casa", sentence: "La casa è grande.", article: "la", plural: "case", gloss: "house · home"},
		{lemma: "libro", target: "libro", sentence: "Il libro è nuovo.", article: "il", plural: "libri", gloss: "book"},
		{lemma: "zaino", target: "zaino", sentence: "Lo zaino è pieno.", article: "lo", plural: "zaini", gloss: "backpack"},
		{lemma: "albero", target: "L'albero", sentence: "L'albero è alto.", article: "l'", plural: "alberi", gloss: "tree"},
	} {
		t.Run(test.lemma, func(t *testing.T) {
			entry := Entry{
				Language: "it", CanonicalLemma: test.lemma, UPOS: "NOUN",
				Sentence: test.sentence, TargetWord: test.target,
				Morphology: `{"Gender":"Masc","Article":"il"}`,
			}
			require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))

			note, err := makeNote("owner", renderInputFromEntry(entry))
			require.NoError(t, err)
			assert.Equal(t, strings.ReplaceAll(test.article, "'", "&#39;"), note.Article)
			assert.Equal(t, test.gloss, note.Gloss)
			assert.Equal(t, test.plural, note.Plural)
			escapedArticle := strings.ReplaceAll(test.article, "'", "&#39;")
			articleLemma := escapedArticle + " " + test.lemma
			if strings.HasSuffix(test.article, "'") {
				articleLemma = escapedArticle + test.lemma
			}
			assert.Contains(t, note.BackExtra, articleLemma)
		})
	}
}

func TestGreekDictionaryIndexMorphologyRendersOnCard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1'); INSERT INTO entries VALUES ('el', 'άνθρωποσ', 'NOUN', '[{"Gloss":"person","Examples":[],"Topics":[],"Tags":[],"Phrase":"","Gender":"Masc","Article":"ο","Plural":"άνθρωποι","IPA":"/ˈanθropos/"}]', 'Masc', 'ο', 'άνθρωποι', '/ˈanθropos/', '')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := dictionary.OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := index.Close(); err != nil {
			t.Errorf("dictionary index cleanup failed: %v", err)
		}
	})
	service := &lexicalResolver{lexical: index}
	entry := Entry{
		Language: "el", CanonicalLemma: "άνθρωπος", UPOS: "NOUN",
		Sentence: "Ο άνθρωπος διαβάζει σήμερα.", TargetWord: "άνθρωπος",
	}
	require.NoError(t, service.resolveLexicalEntry(t.Context(), &entry))

	note, err := makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)
	assert.Equal(t, "ο", note.Article)
	assert.Equal(t, "άνθρωπος", note.Lemma)
	assert.Equal(t, "άνθρωποι", note.Plural)
	assert.Equal(t, "/ˈanθropos/", note.IPA)
	assert.True(t, strings.HasPrefix(note.BackExtra, "ο άνθρωπος (Pl. άνθρωποι)\n"))
}
