//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadPreparedDeckCandidateFactsUsesCorpusCoverageEntry covers the direct
// analysis path: a corpus-scoped preparation with no snapshot and no lemma
// corrections reads each selected candidate's Entry from the corpus coverage
// fact rather than a projected sentence.
func TestLoadPreparedDeckCandidateFactsUsesCorpusCoverageEntry(t *testing.T) {
	ctx := context.Background()
	f := newCandidateFactsFixture(t, ctx, "candidate-facts-corpus", "candidate-facts")

	candidate := domain.SelectionCandidate{
		OwnerID: f.ownerID, CorpusID: f.corpusID, Language: "de",
		CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3,
		ObservedForms: []byte(`["haus"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	}
	_, err := f.store.PutSelectionCandidate(ctx, candidate)
	require.NoError(t, err)

	tx, err := f.store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	facts, err := f.store.LoadPreparedDeckCandidateFactsTx(ctx, tx, f.preparation, []domain.SelectionCandidate{candidate})
	require.NoError(t, err)
	require.Len(t, facts, 1)

	t.Run("entry is read from the corpus coverage fact", func(t *testing.T) {
		assert.Equal(t, "haus", facts[0].Entry.CanonicalLemma)
		assert.Equal(t, f.source.Title, facts[0].Entry.SourceDocument, "the coverage entry names the analysed source")
	})
	t.Run("the selected candidate passes through unchanged", func(t *testing.T) {
		assert.Equal(t, candidate.CanonicalLemma, facts[0].Candidate.CanonicalLemma)
	})
	t.Run("a candidate without sentence references carries no sentence facts", func(t *testing.T) {
		assert.Empty(t, facts[0].Sentences)
	})
}

// TestLoadPreparedDeckCandidateFactsRejectsMissingPersistedSentence pins the
// documented guarantee that partially available persisted sentences are
// rejected: a candidate referencing an ordinal the corpus does not hold fails
// instead of silently dropping context.
func TestLoadPreparedDeckCandidateFactsRejectsMissingPersistedSentence(t *testing.T) {
	ctx := context.Background()
	f := newCandidateFactsFixture(t, ctx, "candidate-facts-missing-sentence", "candidate-facts-missing")
	_, err := f.store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,0,'Der alte Baum ist gross.',0,22)`, f.ownerID, f.runID, f.corpusID, "candidate-facts-unit")
	require.NoError(t, err)

	candidate := domain.SelectionCandidate{
		OwnerID: f.ownerID, CorpusID: f.corpusID, Language: "de",
		CanonicalLemma: "baum", UPOS: "NOUN", OccurrenceCount: 3,
		ObservedForms:      []byte(`["Baum"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"location":{"start_offset":0}},{"sentence_index":1,"location":{"start_offset":14}}]`),
		Provenance:         []byte(`{}`),
	}
	_, err = f.store.PutSelectionCandidate(ctx, candidate)
	require.NoError(t, err)

	tx, err := f.store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	_, err = f.store.LoadPreparedDeckCandidateFactsTx(ctx, tx, f.preparation, []domain.SelectionCandidate{candidate})
	require.ErrorContains(t, err, "missing sentence ordinal 1")
}

// TestLoadPreparedDeckCandidateFactsIgnoresUndecodableSentenceReferences pins
// that a candidate whose persisted sentence references do not decode into the
// expected shape contributes no ordinals instead of failing the load.
func TestLoadPreparedDeckCandidateFactsIgnoresUndecodableSentenceReferences(t *testing.T) {
	ctx := context.Background()
	f := newCandidateFactsFixture(t, ctx, "candidate-facts-undecodable", "candidate-facts-undecodable")

	candidate := domain.SelectionCandidate{
		OwnerID: f.ownerID, CorpusID: f.corpusID, Language: "de",
		CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3,
		ObservedForms: []byte(`["haus"]`), SentenceReferences: []byte(`{}`), Provenance: []byte(`{}`),
	}
	_, err := f.store.PutSelectionCandidate(ctx, candidate)
	require.NoError(t, err)

	tx, err := f.store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	facts, err := f.store.LoadPreparedDeckCandidateFactsTx(ctx, tx, f.preparation, []domain.SelectionCandidate{candidate})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Empty(t, facts[0].Sentences, "undecodable references carry no sentence facts")
}

// TestLoadPreparedDeckCandidateFactsPropagatesSentenceLookupFailure drops the
// sentence table so the lookup failure reaches the caller instead of being read
// as absent context.
func TestLoadPreparedDeckCandidateFactsPropagatesSentenceLookupFailure(t *testing.T) {
	ctx := context.Background()
	f := newCandidateFactsFixture(t, ctx, "candidate-facts-lookup", "candidate-facts-lookup")

	candidate := domain.SelectionCandidate{
		OwnerID: f.ownerID, CorpusID: f.corpusID, Language: "de",
		CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3,
		ObservedForms:      []byte(`["haus"]`),
		SentenceReferences: []byte(`[{"sentence_index":0}]`),
		Provenance:         []byte(`{}`),
	}
	_, err := f.store.PutSelectionCandidate(ctx, candidate)
	require.NoError(t, err)
	_, err = f.store.Pool().Exec(ctx, `DROP TABLE corpus_sentences CASCADE`)
	require.NoError(t, err)

	tx, err := f.store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	_, err = f.store.LoadPreparedDeckCandidateFactsTx(ctx, tx, f.preparation, []domain.SelectionCandidate{candidate})
	require.ErrorContains(t, err, "list persisted corpus sentences")
}

// candidateFactsFixture is the shared direct-preparation setup for candidate
// fact tests: an analysed To Read Book whose source and corpus a preparation is
// bound to, so the loaders read corpus-scoped coverage facts.
type candidateFactsFixture struct {
	store       *PostgresStore
	ownerID     string
	source      domain.SourceMaterial
	preparation domain.DeckPreparation
	runID       string
	corpusID    string
}

func newCandidateFactsFixture(t *testing.T, ctx context.Context, username, suffix string) candidateFactsFixture {
	t.Helper()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, username, false)
	require.NoError(t, err)
	book, source, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "de", suffix)
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	runID, corpusID := analyzedRunAndCorpus(t, ctx, store, owner.ID, book.ID, source.ID)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID,
		Filename: suffix + ".apkg", DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	return candidateFactsFixture{store: store, ownerID: owner.ID, source: source, preparation: preparation, runID: runID, corpusID: corpusID}
}

// analyzedRunAndCorpus returns the completed analysis run and corpus IDs that
// makeAnalyzedToReadBook bound to the given book and source.
func analyzedRunAndCorpus(t *testing.T, ctx context.Context, store *PostgresStore, owner, bookID, sourceID string) (string, string) {
	t.Helper()
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner, bookID).Scan(&runID))
	var corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, owner, sourceID).Scan(&corpusID))
	return runID, corpusID
}
