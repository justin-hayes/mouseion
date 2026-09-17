//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyRerenderRecoversDependencyParseWithoutChangingManifest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "legacy-rerender-owner", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Legacy Rerender",
		MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("Karam rief dem jungen Scheich entgegen."), FullText: "Karam rief dem jungen Scheich entgegen.",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, "legacy"), Order: 0, SpineIndex: 0, Title: "Legacy", TitleSource: domain.UnitTitleHeading,
		Text: "Karam rief dem jungen Scheich entgegen.", StartOffset: 0, EndOffset: 39, ManifestID: "legacy", PackagePath: "legacy.xhtml", SourceHref: "legacy.xhtml", ResolvedHref: "legacy.xhtml", MediaType: "application/xhtml+xml", Linear: true,
	}}})
	require.NoError(t, err)
	corpus := insertLegacyRerenderCorpus(t, ctx, store, source)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title),
		DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de','entgegenrufen','VERB','candidate')`, owner.ID)
	require.NoError(t, err)

	tokens := legacyRerenderTokens()
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{{
		OwnerID: owner.ID, Language: "de", CanonicalLemma: "entgegenrufen", UPOS: "VERB", CorpusID: corpus.ID,
		SentenceOrdinal: 7, Sentence: "Karam rief dem jungen Scheich entgegen.", TargetWord: "rief", SourceDocument: source.Title,
		FirstEncounter: 1, SentenceTokens: tokens,
	}}, testutil.PresentationProvider{})
	require.NoError(t, err)
	require.Equal(t, 1, deck.Summary().Accepted)
	snapshot := deck.StorageProjection()
	snapshot.Items[0].Entry.SentenceTokens = nil

	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	result, err := store.FreezePreparedDeckRunTx(ctx, tx, persistence.FreezePreparedDeckRunParams{
		OwnerID: owner.ID, PreparationID: preparation.ID, Projection: snapshot,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	claimToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	frozen, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	deck, err = cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, cardexport.RunFacts{})
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, claimToken, artifact)
	require.NoError(t, err)
	var frontBefore string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT front FROM cards WHERE owner_id=$1`, owner.ID).Scan(&frontBefore))

	manifestBefore, digestBefore, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=0,render_input_version=0 WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=0,render_input_version=0 WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)

	updated, err := (&DurableRerenderer{Store: store, Renderer: cardexport.NewPresentation(nil)}).Rerender(ctx, owner.ID, preparation.ID, result.Run.ID, cardexport.PresentationVersion)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.DeckRevision)
	assert.Equal(t, cardexport.PresentationVersion, updated.PresentationVersion)
	after, err := store.DownloadDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, after.Artifact)
	var front string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT front FROM cards WHERE owner_id=$1`, owner.ID).Scan(&front))
	assert.NotEqual(t, frontBefore, front)
	assert.Contains(t, front, "<b>rief</b>")
	assert.Contains(t, front, "<b>entgegen</b>")
	manifestAfter, digestAfter, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, manifestBefore, manifestAfter)
	assert.Equal(t, digestBefore, digestAfter)
}

func insertLegacyRerenderCorpus(t *testing.T, ctx context.Context, store *persistence.PostgresStore, source domain.SourceMaterial) domain.Corpus {
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
		INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at)
		VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, source.OwnerID, source.ID, revisionID, snapshotID, source.ID).Scan(&runID)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, source.OwnerID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, source.OwnerID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, source.OwnerID, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, source.OwnerID, runID, corpus.ID, domain.EPUBUnitID(0, "legacy"), 7, "Karam rief dem jungen Scheich entgegen.", 0, 39)
	require.NoError(t, err)
	tokens := legacyRerenderTokens()
	for ordinal, token := range tokens {
		morphology, marshalErr := json.Marshal(token.Morphology)
		require.NoError(t, marshalErr)
		if string(morphology) == "null" {
			morphology = []byte(`{}`)
		}
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset)
			VALUES($1,'de',$2,$3,7,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, source.OwnerID, runID, corpus.ID, ordinal, token.Surface, token.RawLemma, token.CanonicalLemma, token.UPOS, token.Dependency, token.Head, morphology, ordinal, ordinal+1)
		require.NoError(t, err)
	}
	return corpus
}

func legacyRerenderTokens() []analyzer.Token {
	return []analyzer.Token{
		{Surface: "Karam", RawLemma: "Karam", CanonicalLemma: "karam", UPOS: "PROPN", Dependency: "nsubj", Head: 1},
		{Surface: "rief", RawLemma: "rufen", CanonicalLemma: "rufen", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}},
		{Surface: "dem", RawLemma: "der", CanonicalLemma: "der", UPOS: "DET", Dependency: "det", Head: 4},
		{Surface: "jungen", RawLemma: "jung", CanonicalLemma: "jung", UPOS: "ADJ", Dependency: "amod", Head: 4},
		{Surface: "Scheich", RawLemma: "Scheich", CanonicalLemma: "Scheich", UPOS: "NOUN", Dependency: "obl", Head: 1},
		{Surface: "entgegen", RawLemma: "entgegen", CanonicalLemma: "entgegen", UPOS: "ADV", Dependency: "compound:prt", Head: 1},
	}
}
