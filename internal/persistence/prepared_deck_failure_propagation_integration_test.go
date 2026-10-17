//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// newPreparedDeckInputsFixture builds an analysed, Book-linked source with a
// corpus-scoped preparation and one selected candidate, so input loading reads
// the direct analysis and vocabulary facts.
func newPreparedDeckInputsFixture(t *testing.T, ctx context.Context, suffix string) (*PostgresStore, domain.DeckPreparation) {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "inputs-failure-"+suffix, false)
	require.NoError(t, err)
	book, source, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "de", suffix)
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	runID, corpusID := analyzedRunAndCorpus(t, ctx, store, owner.ID, book.ID, source.ID)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID,
		Filename: suffix + ".apkg", DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de",
		CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1,
		ObservedForms: []byte(`["haus"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	})
	require.NoError(t, err)
	return store, preparation
}

// TestPreparedDeckLanguageTxReportsSourceLanguage resolves a preparation's
// source language and reports the empty string when the source or transaction
// is unavailable, so the planner keeps its default policy.
func TestPreparedDeckLanguageTxReportsSourceLanguage(t *testing.T) {
	ctx := context.Background()
	store, preparation := newPreparedDeckInputsFixture(t, ctx, "deck-language")
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()

	t.Run("the source language is reported", func(t *testing.T) {
		language, err := store.PreparedDeckLanguageTx(ctx, tx, preparation)
		require.NoError(t, err)
		require.Equal(t, "de", language)
	})
	t.Run("an unknown source reports no language", func(t *testing.T) {
		missing := preparation
		missing.SourceMaterialID = "00000000-0000-0000-0000-000000000000"
		language, err := store.PreparedDeckLanguageTx(ctx, tx, missing)
		require.NoError(t, err)
		require.Empty(t, language)
	})
	t.Run("an incomplete preparation reports no language", func(t *testing.T) {
		language, err := store.PreparedDeckLanguageTx(ctx, tx, domain.DeckPreparation{})
		require.NoError(t, err)
		require.Empty(t, language)
	})
	t.Run("a nil transaction reports no language", func(t *testing.T) {
		language, err := store.PreparedDeckLanguageTx(ctx, nil, preparation)
		require.NoError(t, err)
		require.Empty(t, language)
	})
}

// TestLoadPreparedDeckInputFactsPropagatesPersistenceFailures drops one table
// behind each input step so a database failure reaches the caller instead of
// being read as an absent fact set.
func TestLoadPreparedDeckInputFactsPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	load := func(t *testing.T, suffix, statement string) error {
		t.Helper()
		store, preparation := newPreparedDeckInputsFixture(t, ctx, suffix)
		_, err := store.Pool().Exec(ctx, statement)
		require.NoError(t, err)
		tx, err := store.Pool().Begin(ctx)
		require.NoError(t, err)
		defer func() { require.NoError(t, tx.Rollback(ctx)) }()
		_, err = store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
		return err
	}

	t.Run("the analysed token lookup fails", func(t *testing.T) {
		err := load(t, "inputs-tokens", `DROP TABLE corpus_tokens CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
	})
	t.Run("the lemma correction lookup fails", func(t *testing.T) {
		err := load(t, "inputs-corrections", `DROP TABLE occurrence_lemma_corrections CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
	})
	t.Run("the vocabulary lookup fails", func(t *testing.T) {
		err := load(t, "inputs-vocabulary", `DROP TABLE known_vocabulary CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
	})
}

// TestLoadPreparedDeckInputFactsRejectsAnUnavailableBoundSnapshot binds a
// direct preparation to another source's Current reading snapshot, so the bound
// identity cannot be resolved and the load fails instead of falling back.
func TestLoadPreparedDeckInputFactsRejectsAnUnavailableBoundSnapshot(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)
	owner, err := store.CreateUser(ctx, "inputs-unavailable-snapshot", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "de", "unavailable-a")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	runID, _ := analyzedRunAndCorpus(t, ctx, store, owner.ID, book.ID, source.ID)

	otherBook, otherSource, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "de", "unavailable-b")
	makeAnalyzedToReadBook(t, ctx, store, otherBook, otherSource)
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", otherBook.ID)
	require.NoError(t, err)
	reading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)

	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID, SnapshotID: reading.SnapshotID,
		Filename: "unavailable.apkg", DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)

	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	_, err = store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	require.ErrorContains(t, err, "is unavailable")
	require.ErrorIs(t, err, ErrNotFound)
}

// TestLoadPreparedDeckInputFactsRejectsUndecodableTokenEvidence pins that a
// persisted token whose morphology does not decode into the expected shape
// fails the load instead of yielding a partial analysis.
func TestLoadPreparedDeckInputFactsRejectsUndecodableTokenEvidence(t *testing.T) {
	ctx := context.Background()
	store, preparation := newPreparedDeckInputsFixture(t, ctx, "inputs-undecodable-token")
	var corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`, preparation.OwnerID, preparation.AnalysisRunID).Scan(&corpusID))
	_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,'unit-1',0,'Satz',0,4)`, preparation.OwnerID, preparation.AnalysisRunID, corpusID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head) VALUES($1,'de',$2,$3,0,0,'Satz','satz','satz','NOUN','{"num": 1}'::jsonb,0,4,'root',0)`, preparation.OwnerID, preparation.AnalysisRunID, corpusID)
	require.NoError(t, err)

	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	_, err = store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	require.ErrorContains(t, err, "decode persisted token morphology")
}

// TestCompletePreparedDeckPropagatesPersistenceFailures drops one table behind
// the artifact publish so a partial completion fails instead of committing.
func TestCompletePreparedDeckPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	complete := func(t *testing.T, suffix, statement string) error {
		t.Helper()
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "complete-failure-"+suffix, false)
		require.NoError(t, err)
		source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "complete-" + suffix, Title: "Book", MediaType: "text/plain", ContentHash: "complete-" + suffix, Content: []byte("Haus"), FullText: "Haus"})
		require.NoError(t, err)
		preparation := createPreparation(t, ctx, store, owner.ID, source.ID, source.ContentHash)
		_, err = store.ClaimDeckPreparation(ctx, owner.ID, preparation.ID)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, statement)
		require.NoError(t, err)
		artifact := cardexport.Artifact{
			APKG: []byte("apkg"), Filename: preparation.Filename, DeckName: preparation.DeckName,
			Generated:    []cardexport.GeneratedRecord{{Input: cardexport.RenderInput{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}, Note: cardexport.Note{Key: "haus", Text: "haus front", BackExtra: "haus back", BookTitle: preparation.DeckName}}},
			Completeness: cardexport.Completeness{TotalCards: 1},
		}
		_, err = store.CompletePreparedDeck(ctx, owner.ID, preparation.ID, artifact)
		return err
	}

	t.Run("the card write fails", func(t *testing.T) {
		require.Error(t, complete(t, "cards", `DROP TABLE cards CASCADE`))
	})
	t.Run("the generated vocabulary write fails", func(t *testing.T) {
		require.Error(t, complete(t, "generated", `DROP TABLE generated_vocabulary CASCADE`))
	})
	t.Run("the vocabulary assignment fails", func(t *testing.T) {
		require.Error(t, complete(t, "assignment", `DROP TABLE deck_preparation_vocabulary CASCADE`))
	})
}

// TestCreateDeckPreparationPropagatesPersistenceFailures drops the Book or the
// preparation table so an identity or lookup failure is not read as a reusable
// preparation.
func TestCreateDeckPreparationPropagatesPersistenceFailures(t *testing.T) {
	ctx := context.Background()
	create := func(t *testing.T, suffix, statement string) error {
		t.Helper()
		url, _ := testutil.Postgres(t, ctx, Migrate)
		store := openIntegrationStore(t, ctx, url)
		owner, err := store.CreateUser(ctx, "create-failure-"+suffix, false)
		require.NoError(t, err)
		_, source, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "de", suffix)
		_, err = store.Pool().Exec(ctx, statement)
		require.NoError(t, err)
		_, err = store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: suffix, ContentHash: source.ContentHash})
		return err
	}

	t.Run("the book lock lookup fails", func(t *testing.T) {
		err := create(t, "book", `DROP TABLE books CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
	})
	t.Run("the existing preparation lookup fails", func(t *testing.T) {
		err := create(t, "existing", `DROP TABLE deck_preparations CASCADE`)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrNotFound)
	})
}
