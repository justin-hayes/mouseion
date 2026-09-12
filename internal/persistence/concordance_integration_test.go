//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcordanceOccurrencesAreCurrentOwnerScopedAndDeterministic(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	alice, err := store.CreateUser(ctx, "concordance-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "concordance-bob", false)
	require.NoError(t, err)

	bookA, sourceA := createConcordanceBook(t, ctx, store, alice.ID, "Book A", "a", true, []domain.ExtractedUnit{
		concordanceUnit(0, "intro", "Intro", 0, 5),
		concordanceUnit(1, "chapter-a", "xxDas Haus", 7, 17),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceA, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:intro", Ordinal: 0, Text: "Intro", Start: 0, End: 5},
		{UnitID: "epub-unit-v1:1:chapter-a", Ordinal: 1, Text: "Das Haus", Start: 2, End: 10, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 6, End: 10}}},
	})

	bookB, sourceB := createConcordanceBook(t, ctx, store, alice.ID, "Book B", "b", true, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter-b", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceB, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter-b", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}}},
	})

	// This is a complete analysis artifact, but it is not the Book's current
	// analysis, so it must not contribute to study-language results.
	_, sourceNotCurrent := createConcordanceBook(t, ctx, store, alice.ID, "Book Not Current", "not-current", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter-old", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceNotCurrent, false, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter-old", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}}},
	})

	bookBob, sourceBob := createConcordanceBook(t, ctx, store, bob.ID, "Bob's Book", "bob", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter-bob", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceBob, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter-bob", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}}},
	})

	bookRows, err := store.ListBookOccurrencesByLemma(ctx, alice.ID, bookA.ID, "de-DE", "haus", "NOUN")
	require.NoError(t, err)
	require.Len(t, bookRows, 1)
	assertOccurrence(t, bookRows[0], bookA.ID, "Book A", sourceA.ID, "epub-unit-v1:1:chapter-a", "Chapter 1", "Das Haus", 4, 8, 6, 10, 13, 17, 1, 1)

	surfaceRows, err := store.ListBookOccurrencesBySurface(ctx, alice.ID, bookA.ID, "de", "Haus")
	require.NoError(t, err)
	assert.Equal(t, bookRows, surfaceRows)

	languageRows, err := store.ListStudyLanguageOccurrencesByLemma(ctx, alice.ID, "de", "haus", "NOUN")
	require.NoError(t, err)
	require.Len(t, languageRows, 2)
	assertOccurrence(t, languageRows[0], bookA.ID, "Book A", sourceA.ID, "epub-unit-v1:1:chapter-a", "Chapter 1", "Das Haus", 4, 8, 6, 10, 13, 17, 1, 1)
	assertOccurrence(t, languageRows[1], bookB.ID, "Book B", sourceB.ID, "epub-unit-v1:0:chapter-b", "Chapter 1", "Haus", 0, 4, 0, 4, 0, 4, 0, 2)

	languageSurfaceRows, err := store.ListStudyLanguageOccurrencesBySurface(ctx, alice.ID, "de-DE", "Haus")
	require.NoError(t, err)
	assert.Equal(t, languageRows, languageSurfaceRows)

	bobRows, err := store.ListStudyLanguageOccurrencesBySurface(ctx, bob.ID, "de", "Haus")
	require.NoError(t, err)
	require.Len(t, bobRows, 1)
	assert.Equal(t, bookBob.ID, bobRows[0].BookID, "owner scoping")
}

type concordanceSentence struct {
	UnitID              string
	Ordinal, Start, End int64
	Text                string
	Tokens              []concordanceToken
}

type concordanceToken struct {
	Surface, Lemma, Upos string
	Start, End           int64
}

func concordanceUnit(spine uint64, manifest, text string, start, end uint64) domain.ExtractedUnit {
	return domain.ExtractedUnit{
		ID: domain.EPUBUnitID(spine, manifest), Order: spine, SpineIndex: spine,
		Title: "Chapter 1", TitleSource: domain.UnitTitleHeading, Text: text,
		StartOffset: start, EndOffset: end, ManifestID: manifest,
		PackagePath: manifest + ".xhtml", SourceHref: manifest + ".xhtml", ResolvedHref: manifest + ".xhtml",
		MediaType: "application/xhtml+xml", Linear: true,
	}
}

func createConcordanceBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, title, suffix string, journey bool, units []domain.ExtractedUnit) (domain.Book, domain.SourceMaterial) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	fullText := ""
	for i, unit := range units {
		if i > 0 {
			fullText += "\n\n"
		}
		fullText += unit.Text
	}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: "concordance-" + suffix,
		Title: title, MediaType: "application/epub+zip", Content: []byte("epub-" + suffix), FullText: fullText,
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: units})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	if journey {
		journeyState, journeyErr := store.GetReadingJourney(ctx, owner, "de")
		require.NoError(t, journeyErr)
		_, journeyErr = store.AddToReadingJourney(ctx, owner, "de", book.ID, journeyState.Revision)
		require.NoError(t, journeyErr)
	}
	return book, source
}

func insertConcordanceAnalysis(t *testing.T, ctx context.Context, store *PostgresStore, source domain.SourceMaterial, current bool, sentences []concordanceSentence) {
	t.Helper()
	var revisionID, snapshotID string
	err := store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID).Scan(&revisionID, &snapshotID)
	require.NoError(t, err)
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "de",
		NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1",
	}, nil))
	var runID string
	err = store.Pool().QueryRow(ctx, `
		INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,
			analyzer_name,analyzer_version,config_identity,state,completed_at)
		VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`,
		source.OwnerID, source.ID, revisionID, snapshotID, source.ID).Scan(&runID)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, source.OwnerID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, source.OwnerID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, source.OwnerID, runID)
	require.NoError(t, err)
	if current {
		_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) SELECT $1,b.id,$2,$3 FROM books b WHERE b.owner_id=$1 AND b.id=(SELECT book_id FROM source_materials WHERE owner_id=$1 AND id=$2)`, source.OwnerID, source.ID, runID)
		require.NoError(t, err)
	}
	for _, sentence := range sentences {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, source.OwnerID, runID, corpus.ID, sentence.UnitID, sentence.Ordinal, sentence.Text, sentence.Start, sentence.End)
		require.NoError(t, err)
		for tokenOrdinal, token := range sentence.Tokens {
			_, err = store.Pool().Exec(ctx, `
				INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,
					surface,raw_lemma,canonical_lemma,upos,start_offset,end_offset)
				VALUES($1,'de',$2,$3,$4,$5,$6,$6,$7,$8,$9,$10)`, source.OwnerID, runID, corpus.ID, sentence.Ordinal, tokenOrdinal, token.Surface, token.Lemma, token.Upos, token.Start, token.End)
			require.NoError(t, err)
		}
	}
}

func assertOccurrence(t *testing.T, occurrence domain.ConcordanceOccurrence, bookID, bookTitle, sourceID, unitID, chapterTitle, sentenceText string, sentenceStart, sentenceEnd, unitStart, unitEnd, bookStart, bookEnd, unitOrder int64, bookPosition int) {
	t.Helper()
	assert.Equal(t, bookID, occurrence.BookID)
	assert.Equal(t, bookTitle, occurrence.BookTitle)
	assert.Equal(t, sourceID, occurrence.SourceMaterialID)
	assert.Equal(t, unitID, occurrence.UnitID)
	assert.Equal(t, chapterTitle, occurrence.ChapterTitle)
	assert.Equal(t, sentenceText, occurrence.SentenceText)
	assert.Equal(t, "Haus", occurrence.Surface)
	assert.Equal(t, "haus", occurrence.CanonicalLemma)
	assert.Equal(t, "NOUN", occurrence.UPOS)
	assert.Equal(t, sentenceStart, occurrence.SentenceStartOffset)
	assert.Equal(t, sentenceEnd, occurrence.SentenceEndOffset)
	assert.Equal(t, unitStart, occurrence.UnitStartOffset)
	assert.Equal(t, unitEnd, occurrence.UnitEndOffset)
	assert.Equal(t, bookStart, occurrence.BookStartOffset)
	assert.Equal(t, bookEnd, occurrence.BookEndOffset)
	require.NotNil(t, occurrence.BookPosition)
	assert.Equal(t, bookPosition, *occurrence.BookPosition)
	assert.Equal(t, unitOrder, occurrence.UnitOrder)
}
