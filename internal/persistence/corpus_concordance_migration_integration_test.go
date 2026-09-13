//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorpusConcordanceSchemaPersistsZeroTokenSentencesAndTokens(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "concordance-owner", false)
	require.NoError(t, err)
	units := domain.ExtractedUnits{
		SchemaVersion: domain.ExtractedUnitsSchemaVersion,
		Units: []domain.ExtractedUnit{{
			ID: "epub-unit-v1:0:chapter-1", Order: 0, SpineIndex: 0,
			Title: "Chapter 1", TitleSource: domain.UnitTitleHeading, Text: "Hallo Welt",
			StartOffset: 0, EndOffset: 10, PackagePath: "chapter-1.xhtml",
			ManifestID: "chapter-1", SourceHref: "chapter-1.xhtml", ResolvedHref: "chapter-1.xhtml",
			MediaType: "application/xhtml+xml", Linear: true,
		}},
	}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: "concordance-book",
		Title: "Concordance Book", MediaType: "application/epub+zip", Content: []byte("epub"), FullText: "Hallo Welt",
	}, units)
	require.NoError(t, err)
	var revisionID, snapshotID string
	err = store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID).Scan(&revisionID, &snapshotID)
	require.NoError(t, err)

	artifact := domain.NormalizedArtifact{
		ContentHash: "sha256:concordance-artifact", Language: "de", SchemaVersion: "1.0.0",
		NormalizationProfile: "de-standard", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1",
	}
	require.NoError(t, store.PutArtifact(ctx, artifact, nil))
	var runID string
	err = store.Pool().QueryRow(ctx, `
		INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,
			analyzer_name,analyzer_version,config_identity,state,completed_at)
		VALUES($1,$2,$3,$4,'test','1','concordance','completed',now())
		RETURNING id::text`, owner.ID, source.ID, revisionID, snapshotID).Scan(&runID)
	require.NoError(t, err)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `
		INSERT INTO corpora(owner_id,source_material_id,artifact_hash,analysis_run_id,status)
		VALUES($1,$2,$3,$4,'complete') RETURNING id::text`, owner.ID, source.ID, artifact.ContentHash, runID).Scan(&corpusID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpusID, owner.ID, runID)
	require.NoError(t, err)

	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'',0,0),($1,$2,$3,$4,1,'Hallo',0,5)`, owner.ID, runID, corpusID, units.Units[0].ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,
			surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,named_entity,start_offset,end_offset)
		VALUES($1,'de',$2,$3,1,0,'Hallo','Hallo','hallo','PROPN','dep',1,'{"Number":"Sing"}', 'S-PER',0,5),
		      ($1,'de',$2,$3,1,1,'Welt','Welt','welt','NOUN','root',1,'{}',NULL,6,10)`, owner.ID, runID, corpusID)
	require.NoError(t, err)

	var sentenceCount, tokenCount, emptySentenceCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE sentence_text='') FROM corpus_sentences WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, runID).Scan(&sentenceCount, &emptySentenceCount)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM corpus_tokens WHERE owner_id=$1 AND language='de' AND analysis_run_id=$2`, owner.ID, runID).Scan(&tokenCount)
	require.NoError(t, err)
	assert.Equal(t, 2, sentenceCount)
	assert.Equal(t, 1, emptySentenceCount)
	assert.Equal(t, 2, tokenCount)

	var surface, rawLemma, canonicalLemma, upos, dependency, namedEntity string
	var head int64
	var morphology []byte
	err = store.Pool().QueryRow(ctx, `
		SELECT surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,named_entity
		FROM corpus_tokens WHERE owner_id=$1 AND language='de' AND canonical_lemma='hallo' AND upos='PROPN'`, owner.ID).
		Scan(&surface, &rawLemma, &canonicalLemma, &upos, &dependency, &head, &morphology, &namedEntity)
	require.NoError(t, err)
	assert.Equal(t, "Hallo", surface)
	assert.Equal(t, "Hallo", rawLemma)
	assert.Equal(t, "hallo", canonicalLemma)
	assert.Equal(t, "PROPN", upos)
	assert.Equal(t, "dep", dependency)
	assert.Equal(t, int64(1), head)
	assert.JSONEq(t, `{"Number":"Sing"}`, string(morphology))
	assert.Equal(t, "S-PER", namedEntity)

	var viewCount int
	err = store.Pool().QueryRow(ctx, `
		SELECT count(*) FROM information_schema.views
		WHERE table_schema='public' AND table_name='concordance_occurrences'`).Scan(&viewCount)
	require.NoError(t, err)
	assert.Equal(t, 1, viewCount)

	var indexCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND tablename='corpus_tokens' AND indexname IN ('corpus_tokens_owner_language_canonical_lemma_upos_idx','corpus_tokens_owner_language_surface_idx','corpus_tokens_owner_language_dependency_idx')`).Scan(&indexCount)
	require.NoError(t, err)
	assert.Equal(t, 3, indexCount)

	down, err := migrations.FS.ReadFile("000071_concordance_occurrence_read_model.down.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(down))
	require.NoError(t, err)

	down, err = migrations.FS.ReadFile("000070_persist_dependency_parses.down.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(down))
	require.NoError(t, err)

	down, err = migrations.FS.ReadFile("000069_corpus_concordance.down.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(down))
	require.NoError(t, err)
	var tableCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('corpus_sentences','corpus_tokens')`).Scan(&tableCount)
	require.NoError(t, err)
	assert.Zero(t, tableCount)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM information_schema.views WHERE table_schema='public' AND table_name='concordance_occurrences'`).Scan(&viewCount)
	require.NoError(t, err)
	assert.Zero(t, viewCount)
}
