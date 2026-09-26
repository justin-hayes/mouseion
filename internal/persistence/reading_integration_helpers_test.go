//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

func createReadingFixture(t *testing.T, ctx context.Context, store *PostgresStore, owner, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	return createReadingFixtureInLanguage(t, ctx, store, owner, "de", suffix)
}

func createReadingFixtureInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner, language, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Reading " + suffix, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
	require.NoError(t, err)
	return createReadingSourceAndPreparation(t, ctx, store, owner, book, suffix)
}

func createReadingSourceAndPreparation(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	return createReadingSourceAndPreparationInLanguage(t, ctx, store, owner, book, book.LanguageTag, suffix)
}

func createReadingSourceAndPreparationInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, language, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	source := putBookSourceInLanguage(t, ctx, store, owner, language, "reading-"+suffix, "Reading "+suffix, []byte("reading-"+suffix), suffix)
	err := store.LinkSourceToBook(ctx, owner, book.ID, source.ID)
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: "Reading " + suffix, ContentHash: source.ContentHash})
	require.NoError(t, err)
	prep, err = store.ClaimDeckPreparation(ctx, owner, prep.ID)
	require.NoError(t, err)
	prep, err = store.CompleteDeckPreparation(ctx, owner, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg-" + suffix), Filename: suffix + ".apkg", DeckName: "Reading " + suffix})
	require.NoError(t, err)
	return book, source, prep
}

func makeAnalyzedToReadBook(t *testing.T, ctx context.Context, store *PostgresStore, book domain.Book, source domain.SourceMaterial) {
	t.Helper()
	require.NoError(t, store.SetBookDisposition(ctx, book.OwnerID, book.ID, domain.BookDispositionToRead))
	err := store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash: source.ContentHash, Language: source.Language, SchemaVersion: "1",
		NormalizationProfile: source.Language, NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1",
	}, nil)
	require.NoError(t, err)
	var snapshotID string
	err = store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID).Scan(&snapshotID)
	require.NoError(t, err)
	var runID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, source.OwnerID, source.ID, source.ContentRevisionID, snapshotID, source.ID).Scan(&runID)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, source.OwnerID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, source.OwnerID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, source.OwnerID, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, book.OwnerID, book.ID, source.ID, runID)
	require.NoError(t, err)
}
