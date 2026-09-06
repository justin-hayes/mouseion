//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestLanguageCorpusEvidenceIsCurrentOwnerAndLanguageScoped(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "corpus-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "corpus-bob", false)
	if err != nil {
		t.Fatal(err)
	}

	seedAnalysisBook(t, ctx, store, alice.ID, "Alice one", "de", "alice-one", "artifact-alice-one", []domain.SharedLemma{
		{Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN", Frequency: 60},
		{Language: "de", CanonicalLemma: "gemeinsam", UPOS: "NOUN", Frequency: 20},
	}, 80)
	seedHistoricalAnalysis(t, ctx, store, alice.ID, "alice-one", "artifact-alice-one-history")
	seedAnalysisBook(t, ctx, store, alice.ID, "Alice two", "de", "alice-two", "artifact-alice-two", []domain.SharedLemma{
		{Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN", Frequency: 10},
		{Language: "de", CanonicalLemma: "gemeinsam", UPOS: "NOUN", Frequency: 30},
	}, 40)
	seedAnalysisBook(t, ctx, store, bob.ID, "Bob one", "de", "bob-one", "artifact-bob-one", []domain.SharedLemma{
		{Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN", Frequency: 100},
	}, 100)
	seedAnalysisBook(t, ctx, store, alice.ID, "Alice Italian", "it", "alice-it", "artifact-alice-it", []domain.SharedLemma{
		{Language: "it", CanonicalLemma: "ciao", UPOS: "NOUN", Frequency: 100},
	}, 100)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice pending", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	seedSource(t, ctx, store, alice.ID, book.ID, "alice-pending", "Alice pending", "de")

	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "bekannt", "NOUN"); err != nil {
		t.Fatal(err)
	}
	view, err := analysisinsights.NewService(store).LanguageCorpus(ctx, alice.ID, " DE ")
	if err != nil {
		t.Fatal(err)
	}
	if view.AnalyzedBookCount != 2 || view.KnownTokenCount != 70 || view.AnalyzableTokenCount != 120 {
		t.Fatalf("alice German summary = %+v", view)
	}
	if len(view.TopUnknownLemmas) != 1 || view.TopUnknownLemmas[0].CanonicalLemma != "gemeinsam" || view.TopUnknownLemmas[0].OccurrenceCount != 50 {
		t.Fatalf("alice German unknowns = %+v", view.TopUnknownLemmas)
	}
	if len(view.PerBook) != 3 {
		t.Fatalf("alice German spread = %+v", view.PerBook)
	}
	for _, spread := range view.PerBook {
		if spread.BookID == book.ID && (spread.Included || spread.ExclusionReason != "unassessed: no current analyzed corpus") {
			t.Fatalf("pending book spread = %+v", spread)
		}
	}

	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "gemeinsam", "NOUN"); err != nil {
		t.Fatal(err)
	}
	view, err = analysisinsights.NewService(store).LanguageCorpus(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if view.KnownTokenCount != 120 || len(view.TopUnknownLemmas) != 0 {
		t.Fatalf("recomputed Alice German view = %+v", view)
	}

	italian, err := analysisinsights.NewService(store).LanguageCorpus(ctx, alice.ID, "it")
	if err != nil {
		t.Fatal(err)
	}
	if italian.AnalyzedBookCount != 1 || italian.AnalyzableTokenCount != 100 || italian.KnownTokenCount != 0 || len(italian.TopUnknownLemmas) != 1 || italian.TopUnknownLemmas[0].CanonicalLemma != "ciao" {
		t.Fatalf("Alice Italian view = %+v", italian)
	}
	bobView, err := analysisinsights.NewService(store).LanguageCorpus(ctx, bob.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if bobView.AnalyzedBookCount != 1 || bobView.KnownTokenCount != 0 || len(bobView.TopUnknownLemmas) != 1 || bobView.TopUnknownLemmas[0].CanonicalLemma != "bekannt" {
		t.Fatalf("Bob German view = %+v", bobView)
	}

	evidence, err := store.ListLanguageCorpusEvidence(ctx, alice.ID, "de")
	if err != nil || len(evidence) != 3 {
		t.Fatalf("Alice evidence = %+v, err=%v", evidence, err)
	}
	for _, item := range evidence {
		if item.Book.Title == "Alice one" && item.AnalysisRunID == "" {
			t.Fatalf("current analysis was not selected: %+v", item)
		}
		for _, lemma := range item.Lemmas {
			if lemma.CanonicalLemma == "history-only" {
				t.Fatalf("historical analysis was merged: %+v", item)
			}
		}
	}
}

func seedAnalysisBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, title, language, identifier, artifactHash string, lemmas []domain.SharedLemma, analyzable int64) domain.Book {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
	if err != nil {
		t.Fatal(err)
	}
	source := seedSource(t, ctx, store, owner, book.ID, identifier, title, language)
	for i := range lemmas {
		if lemmas[i].Morphology == nil {
			lemmas[i].Morphology = []byte(`{}`)
		}
	}
	if err = store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: artifactHash, Language: language, SchemaVersion: "1", NormalizationProfile: language, NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, lemmas); err != nil {
		t.Fatal(err)
	}
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: owner, SourceMaterialID: source.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: domain.EPUBUnitID(0, "unit"), Order: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,scope_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,$5,'test','1',$6,'completed',now()) RETURNING id::text`, owner, source.ID, source.ContentRevisionID, scope.ScopeID, snapshotID, identifier).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner, source.ID, artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET reviewed_scope_id=$1,analysis_run_id=$2,analyzable_token_count=$3,distinct_lemma_count=$4,status='complete' WHERE owner_id=$5 AND id=$6`, scope.ScopeID, runID, analyzable, int64(len(lemmas)), owner, corpus.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, book.ID, source.ID, runID); err != nil {
		t.Fatal(err)
	}
	return book
}

func seedSource(t *testing.T, ctx context.Context, store *PostgresStore, owner, bookID, identifier, title, language string) domain.SourceMaterial {
	t.Helper()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: identifier, Title: title, MediaType: "application/epub+zip", Content: []byte(identifier), FullText: identifier}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, ManifestID: "unit", Text: identifier, EndOffset: uint64(len(identifier))}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, owner, bookID, source.ID); err != nil {
		t.Fatal(err)
	}
	return source
}

func seedHistoricalAnalysis(t *testing.T, ctx context.Context, store *PostgresStore, owner, identifier, artifactHash string) {
	t.Helper()
	var sourceID, revisionID, digest string
	var digestVersion int
	if err := store.Pool().QueryRow(ctx, `SELECT s.id::text,r.revision_id::text,r.content_digest,r.digest_version FROM source_materials s JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id WHERE s.owner_id=$1 AND s.source_identifier=$2`, owner, identifier).Scan(&sourceID, &revisionID, &digest, &digestVersion); err != nil {
		t.Fatal(err)
	}
	if err := store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: artifactHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "de", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, []domain.SharedLemma{{Language: "de", CanonicalLemma: "history-only", UPOS: "NOUN", Morphology: []byte(`{}`), Frequency: 99}}); err != nil {
		t.Fatal(err)
	}
	var snapshotID string
	if err := store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, sourceID).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	scope, err := store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: owner, SourceMaterialID: sourceID, SourceContent: domain.EPUBContentRevisionIdentity{RevisionID: revisionID, Digest: digest, DigestVersion: digestVersion}, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: domain.EPUBUnitID(0, "unit"), Order: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,scope_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,$5,'test','1','historical','completed',now()) RETURNING id::text`, owner, sourceID, revisionID, scope.ScopeID, snapshotID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner, sourceID, artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET reviewed_scope_id=$1,analysis_run_id=$2,analyzable_token_count=99,distinct_lemma_count=1,status='complete' WHERE owner_id=$3 AND id=$4`, scope.ScopeID, runID, owner, corpus.ID); err != nil {
		t.Fatal(err)
	}
}
