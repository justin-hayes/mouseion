//go:build integration

package webapp

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

func seedMigrationAnalyzedBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, suffix, title string, lemmas []domain.LemmaOccurrence) (domain.Book, domain.SourceMaterial, domain.Corpus) {
	t.Helper()
	return seedAnalyzedBookIn(t, ctx, store, "de", owner, suffix, title, lemmas)
}

// seedAnalyzedBookIn is seedMigrationAnalyzedBook for a Book in any language.
func seedAnalyzedBookIn(t *testing.T, ctx context.Context, store *persistence.PostgresStore, language, owner, suffix, title string, lemmas []domain.LemmaOccurrence) (domain.Book, domain.SourceMaterial, domain.Corpus) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
	require.NoError(t, err)
	text := suffix + " content"
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: "migration-" + suffix, Title: title, MediaType: "application/epub+zip", Content: []byte(text), FullText: text}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, suffix), Order: 0, SpineIndex: 0, ManifestID: suffix, Text: text, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true}}})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: language, SchemaVersion: "1", NormalizationProfile: "migration", NormalizationVersion: "1", AnalyzerName: "migration-fixture", AnalyzerVersion: "1"}, toSharedLemmas(lemmas)))
	corpus, err := store.PutCorpus(ctx, owner, source.ID, source.ContentHash)
	require.NoError(t, err)
	var tokenCount int64
	for _, lemma := range lemmas {
		tokenCount += lemma.OccurrenceCount
	}
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=$2,distinct_lemma_count=$3,sentence_count=1,normalized_token_count=$2,empty_sentence_count=0,median_sentence_token_count=1,p90_sentence_token_count=1,long_sentence_count=0 WHERE owner_id=$1 AND id=$4`, owner, tokenCount, len(lemmas), corpus.ID)
	require.NoError(t, err)
	var snapshotID string
	err = store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source.ID).Scan(&snapshotID)
	require.NoError(t, err)
	var runID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'migration-fixture','1',$5,'completed',now()) RETURNING id::text`, owner, source.ID, source.ContentRevisionID, snapshotID, suffix).Scan(&runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, owner, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, book.ID, source.ID, runID)
	require.NoError(t, err)
	return book, source, corpus
}

func toSharedLemmas(lemmas []domain.LemmaOccurrence) []domain.SharedLemma {
	result := make([]domain.SharedLemma, 0, len(lemmas))
	for _, lemma := range lemmas {
		result = append(result, domain.SharedLemma{Language: lemma.Language, CanonicalLemma: lemma.CanonicalLemma, UPOS: lemma.UPOS, Frequency: lemma.OccurrenceCount, Morphology: []byte(`{}`)})
	}
	return result
}

func migrationMyBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, bookID string) domain.MyBook {
	t.Helper()
	books, err := store.ListMyBooksWithEvidence(ctx, owner)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.ID == bookID {
			return book
		}
	}
	require.Failf(t, "book absent from My Books", "book %q absent from My Books: %+v", bookID, books)
	return domain.MyBook{}
}
