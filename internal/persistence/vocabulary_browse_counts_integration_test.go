//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVocabularyBrowseCountProjectionIsReadyForNonemptyAndEmptyBooks(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-count-owner", false)
	require.NoError(t, err)

	for _, fixture := range []struct {
		name      string
		text      string
		tokens    []concordanceToken
		wantRows  int
		wantCount int64
	}{
		{name: "nonempty", text: "Haus Haus", tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 5, End: 9},
		}, wantRows: 1, wantCount: 2},
		{name: "empty", text: "no eligible tokens", wantRows: 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			book, source := createConcordanceBook(t, ctx, store, owner.ID, "Browse count "+fixture.name, "browse-count-"+fixture.name, false, []domain.ExtractedUnit{
				concordanceUnit(0, "browse-count-"+fixture.name, fixture.text, 0, uint64(len(fixture.text))),
			})
			var sentences []concordanceSentence
			if len(fixture.tokens) > 0 {
				sentences = []concordanceSentence{{
					UnitID: "epub-unit-v1:0:browse-count-" + fixture.name, Ordinal: 0,
					Text: fixture.text, Start: 0, End: int64(len(fixture.text)), Tokens: fixture.tokens,
				}}
			}
			insertConcordanceAnalysis(t, ctx, store, source, false, sentences)
			var runID, corpusID string
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT r.id::text,r.corpus_id::text FROM analysis_runs r WHERE r.owner_id=$1 AND r.source_material_id=$2`, owner.ID, source.ID).Scan(&runID, &corpusID))
			tx, beginErr := store.Pool().Begin(ctx)
			require.NoError(t, beginErr)
			require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
			_, err = tx.Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner.ID, book.ID, source.ID, runID)
			require.NoError(t, err)
			require.NoError(t, tx.Commit(ctx))

			var readyRun, readyCorpus string
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&readyRun, &readyCorpus))
			assert.Equal(t, runID, readyRun)
			assert.Equal(t, corpusID, readyCorpus)
			var count int
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&count))
			assert.Equal(t, fixture.wantRows, count)
			page, browseErr := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
			require.NoError(t, browseErr)
			assert.Len(t, page.Rows, fixture.wantRows)
			assert.Equal(t, int64(fixture.wantRows), page.InventoryTotal)
			if fixture.wantRows > 0 {
				var occurrences int64
				var projectedLanguage string
				require.NoError(t, store.Pool().QueryRow(ctx, `SELECT language,occurrence_count FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2 AND canonical_lemma='haus' AND upos='NOUN'`, owner.ID, book.ID).Scan(&projectedLanguage, &occurrences))
				assert.Equal(t, "de", projectedLanguage)
				assert.Equal(t, fixture.wantCount, occurrences)
				assert.Equal(t, fixture.wantCount, page.Rows[0].OccurrenceCount)
			}
		})
	}
}
