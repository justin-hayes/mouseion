//go:build integration

package persistence

import (
	"context"
	"testing"

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
	var runID string
	var snapshotID string
	if err = store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source.ID).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, owner, source.ID, source.ContentRevisionID, snapshotID, identifier).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner, source.ID, artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,analyzable_token_count=$2,distinct_lemma_count=$3,status='complete' WHERE owner_id=$4 AND id=$5`, runID, analyzable, int64(len(lemmas)), owner, corpus.ID); err != nil {
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
