//go:build integration

package analysis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/riverqueue/river/rivertype"
)

func TestRiverAnalysisLifecycleDedupAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
	if _, err = conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = MigrateRiver(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	alice, _ := store.CreateUser(ctx, "jobs-alice", false)
	bob, _ := store.CreateUser(ctx, "jobs-bob", false)
	fullText := strings.Repeat("Häuser. ", 13_000)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "job-source", Title: "Job", MediaType: "text/plain", ContentHash: "sha256:job-success", Content: []byte(fullText), FullText: fullText})
	if err != nil {
		t.Fatal(err)
	}
	var (
		analyzedChunksMu sync.Mutex
		analyzedChunks   []string
	)
	fake := &analyzertest.Fake{AnalyzeFunc: func(analyzeCtx context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		if req.Document.Text == "fail" {
			return analyzer.Result{}, fmt.Errorf("stanza failed")
		}
		if req.Document.Text == "cancel" {
			<-analyzeCtx.Done()
			return analyzer.Result{}, analyzeCtx.Err()
		}
		if req.Document.SourceIdentifier == "job-source" {
			analyzedChunksMu.Lock()
			analyzedChunks = append(analyzedChunks, req.Document.Text)
			analyzedChunksMu.Unlock()
		}
		return analyzer.Result{SchemaVersion: "1.0.0", Language: req.Language, Analysis: analyzer.AnalysisProvenance{AnalyzerName: "fake", AnalyzerVersion: "1"}, NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"}, Sentences: []analyzer.Sentence{{Tokens: []analyzer.Token{{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: map[string]string{"Number": "Plur"}}}}}}, nil
	}}
	client, err := NewClient(store.Pool(), fake, selection.NewService(store))
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	service := NewService(store.Pool(), client)
	bobSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: bob.ID, Language: "de", SourceIdentifier: "bob-job-source", Title: "Bob Job", MediaType: "text/plain", ContentHash: "sha256:bob-job", Content: []byte("Haus."), FullText: "Haus."})
	if err != nil {
		t.Fatal(err)
	}
	bobHandle, err := service.SubmitAnalysis(ctx, bob.ID, bobSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bobHandle.DisplayNumber != 1 {
		t.Fatalf("Bob's first display number = %d, want 1", bobHandle.DisplayNumber)
	}
	handle, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if handle.DisplayNumber != 1 {
		t.Fatalf("Alice's first display number = %d, want 1 (River ID %d)", handle.DisplayNumber, handle.ID)
	}
	if handle.ID == handle.DisplayNumber {
		t.Fatalf("River ID %d unexpectedly matches display number; regression setup did not decouple sequences", handle.ID)
	}
	duplicate, err := service.SubmitAnalysis(ctx, alice.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != handle.ID {
		t.Fatalf("dedup IDs differ: %d != %d", duplicate.ID, handle.ID)
	}
	if duplicate.DisplayNumber != handle.DisplayNumber {
		t.Fatalf("dedup display numbers differ: %d != %d", duplicate.DisplayNumber, handle.DisplayNumber)
	}
	if _, err = service.Get(ctx, bob.ID, handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner get = %v", err)
	}
	if _, err = service.Cancel(ctx, bob.ID, handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner cancel = %v", err)
	}
	status, err := service.Wait(ctx, alice.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != rivertype.JobStateCompleted || status.Progress != 100 {
		t.Fatalf("status = %+v", status)
	}
	analyzedChunksMu.Lock()
	if len(analyzedChunks) != 2 {
		t.Fatalf("Analyze() call count = %d, want 2", len(analyzedChunks))
	}
	for i, chunk := range analyzedChunks {
		if len([]rune(chunk)) > DefaultMaxChunkChars {
			t.Fatalf("analyzed chunk %d length = %d, exceeds %d", i, len([]rune(chunk)), DefaultMaxChunkChars)
		}
	}
	if got := strings.Join(analyzedChunks, ""); got != fullText {
		t.Fatal("analyzed chunks do not reconstruct source text")
	}
	analyzedChunksMu.Unlock()
	corpus, err := service.Result(ctx, alice.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.OwnerID != alice.ID || corpus.ArtifactHash != source.ContentHash {
		t.Fatalf("corpus = %+v", corpus)
	}
	if corpus.Statistics == nil || corpus.Statistics.AnalyzableTokenCount != 2 || corpus.Statistics.DistinctLemmaCount != 1 {
		t.Fatalf("corpus statistics = %+v, want 2 analyzable tokens and 1 distinct lemma", corpus.Statistics)
	}
	if corpus.Statistics.TextProfile == nil || corpus.Statistics.TextProfile.SentenceCount != 2 || corpus.Statistics.TextProfile.MedianSentenceTokenCount != 1 {
		t.Fatalf("text profile = %+v, want two one-token sentences", corpus.Statistics.TextProfile)
	}
	var candidateCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2`, alice.ID, corpus.ID).Scan(&candidateCount); err != nil {
		t.Fatal(err)
	}
	if candidateCount != 1 {
		t.Fatalf("candidates = %d", candidateCount)
	}
	var bobCandidateCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCandidateCount); err != nil {
		t.Fatal(err)
	}
	if bobCandidateCount != 0 {
		t.Fatalf("bob candidates = %d, want 0", bobCandidateCount)
	}
	if _, err = service.Result(ctx, bob.ID, handle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner result = %v", err)
	}

	failing, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "job-fail", Title: "Fail", MediaType: "text/plain", ContentHash: "sha256:job-fail", Content: []byte("fail"), FullText: "fail"})
	if err != nil {
		t.Fatal(err)
	}
	failureHandle, err := service.SubmitAnalysis(ctx, alice.ID, failing.ID)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := service.Wait(ctx, alice.ID, failureHandle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failure.State != rivertype.JobStateDiscarded || failure.Error == "" {
		t.Fatalf("failure status = %+v", failure)
	}

	cancelSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "job-cancel", Title: "Cancel", MediaType: "text/plain", ContentHash: "sha256:job-cancel", Content: []byte("cancel"), FullText: "cancel"})
	if err != nil {
		t.Fatal(err)
	}
	cancelHandle, err := service.SubmitAnalysis(ctx, alice.ID, cancelSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(ctx, alice.ID, cancelHandle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != rivertype.JobStateCancelled {
		cancelled, err = service.Wait(ctx, alice.ID, cancelHandle.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if cancelled.State != rivertype.JobStateCancelled {
		t.Fatalf("cancelled status = %+v", cancelled)
	}
}
