//go:build integration

package analysis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type lifecycleAnalyzer struct {
	mu   sync.Mutex
	mode string
}

func (a *lifecycleAnalyzer) setMode(mode string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mode = mode
}

func (a *lifecycleAnalyzer) Analyze(ctx context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
	a.mu.Lock()
	mode := a.mode
	a.mu.Unlock()
	switch mode {
	case "fail":
		return analyzer.Result{}, errors.New("test analyzer failed")
	case "cancel":
		<-ctx.Done()
		return analyzer.Result{}, ctx.Err()
	}
	return analyzer.Result{
		SchemaVersion:        "1.0.0",
		Language:             req.Language,
		Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "lifecycle-fake", AnalyzerVersion: "1"},
		NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
		Sentences: []analyzer.Sentence{{
			Text: req.Document.Text,
			Tokens: []analyzer.Token{{
				Surface:        "Haus",
				CanonicalLemma: "haus",
				UPOS:           "NOUN",
			}},
		}},
	}, nil
}

func TestScopedAnalysisFailureCancellationRetryAndRestartReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "scoped-lifecycle-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser(ctx, "scoped-lifecycle-other", false)
	if err != nil {
		t.Fatal(err)
	}
	fake := &lifecycleAnalyzer{}
	workers := newTestWorkers()
	client, err := NewClient(store.Pool(), fake, selection.NewService(store), workers)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	service := NewService(store.Pool(), client)
	clientRunning := true
	defer func() {
		if clientRunning {
			_ = client.Stop(context.Background())
		}
	}()

	newScopedSource := func(identifier string) (domain.SourceMaterial, domain.EPUBReviewedScopeSnapshot) {
		t.Helper()
		text := "Haus ist ein sehr schönes Buch."
		source, sourceErr := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: identifier, Title: identifier, MediaType: "application/epub+zip", Content: []byte(text), FullText: text}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, identifier), Order: 0, SpineIndex: 0, ManifestID: identifier, Text: text, EndOffset: uint64(len([]rune(text)))}}})
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		snapshotID, _, snapshotErr := store.GetExtractedUnitSnapshot(ctx, owner.ID, source.ID)
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		scope, scopeErr := store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: owner.ID, SourceMaterialID: source.ID, SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1}, Classifier: domain.EPUBClassifierIdentity{Name: "deterministic", Version: "1"}, SelectionMode: domain.EPUBScopeSelectionOverridden, SelectedUnits: []domain.EPUBSelectedUnitReference{{UnitID: domain.EPUBUnitID(0, identifier), Order: 0}}})
		if scopeErr != nil {
			t.Fatal(scopeErr)
		}
		return source, scope
	}

	fake.setMode("fail")
	failingSource, failingScope := newScopedSource("scoped-failing")
	failingHandle, err := service.SubmitScopedAnalysis(ctx, owner.ID, failingSource.ID, failingScope.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := service.Wait(ctx, owner.ID, failingHandle.ID)
	if err != nil || failure.LogicalState != "failed" || failure.Error == "" {
		t.Fatalf("failed scoped analysis=%+v err=%v", failure, err)
	}
	fake.setMode("success")
	retry, err := service.Retry(ctx, owner.ID, failingHandle.ID)
	if err != nil || retry.ID != failingHandle.ID || retry.RunID != failingHandle.RunID {
		t.Fatalf("scoped retry=%+v err=%v", retry, err)
	}
	completed, err := service.Wait(ctx, owner.ID, failingHandle.ID)
	if err != nil || completed.State != rivertype.JobStateCompleted || completed.Attempt != 2 {
		t.Fatalf("retried scoped analysis=%+v err=%v", completed, err)
	}
	var attemptCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_run_attempts WHERE run_id=$1`, failingHandle.RunID).Scan(&attemptCount); err != nil || attemptCount != 2 {
		t.Fatalf("attempt history=%d err=%v", attemptCount, err)
	}

	fake.setMode("cancel")
	cancelSource, cancelScope := newScopedSource("scoped-cancelled")
	cancelHandle, err := service.SubmitScopedAnalysis(ctx, owner.ID, cancelSource.ID, cancelScope.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	for {
		status, getErr := service.Get(ctx, owner.ID, cancelHandle.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if status.LogicalState == "running" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancelled, err := service.Cancel(ctx, owner.ID, cancelHandle.ID)
	if err != nil || cancelled.LogicalState != "cancelled" {
		t.Fatalf("cancelled scoped analysis=%+v err=%v", cancelled, err)
	}
	if _, err = service.Result(ctx, owner.ID, cancelHandle.ID); err == nil {
		t.Fatal("cancelled scoped analysis produced a result")
	}
	if _, err = service.Get(ctx, other.ID, cancelHandle.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner scoped lifecycle read=%v", err)
	}

	// Submit while no client is running, remove the unavailable queue row, and
	// reconstruct the client as a process restart. Reconciliation must make the
	// queued logical run actionable with a new attempt.
	_ = client.Stop(context.Background())
	clientRunning = false
	restartedWorkers := newTestWorkers()
	preRestartClient, err := NewClient(store.Pool(), &analyzertest.Fake{}, selection.NewService(store), restartedWorkers)
	if err != nil {
		t.Fatal(err)
	}
	restartService := NewService(store.Pool(), preRestartClient)
	restartSource, restartScope := newScopedSource("scoped-restart")
	restartHandle, err := restartService.SubmitScopedAnalysis(ctx, owner.ID, restartSource.ID, restartScope.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `DELETE FROM river_job WHERE id=$1`, restartHandle.JobID); err != nil {
		t.Fatal(err)
	}
	_ = preRestartClient.Stop(context.Background())

	restartedClient, err := NewClient(store.Pool(), fake, selection.NewService(store), newTestWorkers())
	if err != nil {
		t.Fatal(err)
	}
	restartedService := NewService(store.Pool(), restartedClient)
	reconciled, err := restartedService.Reconcile(ctx, owner.ID, restartHandle.ID)
	if err != nil || reconciled.LogicalState != "queued" || reconciled.AttemptCount != 2 {
		t.Fatalf("reconciled restart=%+v err=%v", reconciled, err)
	}
	if err = restartedClient.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restartedClient.Stop(context.Background())
	restarted, err := restartedService.Wait(ctx, owner.ID, restartHandle.ID)
	if err != nil || restarted.State != rivertype.JobStateCompleted {
		t.Fatalf("restarted scoped analysis=%+v err=%v", restarted, err)
	}
	var liveJobs int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'run_id'=$2 AND state IN ('available','pending','running','retryable','scheduled')`, (JobArgs{}).Kind(), restartHandle.RunID).Scan(&liveJobs); err != nil || liveJobs != 0 {
		t.Fatalf("restart left live jobs=%d err=%v", liveJobs, err)
	}
}

func TestScopedAnalysisPersistsCorpusWithRunIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "scoped-corpus-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	text := "Haus ist ein Buch."
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: "scoped-corpus",
		Title: "Scoped corpus", MediaType: "application/epub+zip",
		Content: []byte(text), FullText: text,
	}, domain.ExtractedUnits{
		SchemaVersion: 1,
		Units: []domain.ExtractedUnit{{
			ID: domain.EPUBUnitID(0, "scoped-corpus"), Order: 0, SpineIndex: 0,
			ManifestID: "scoped-corpus", Text: text, EndOffset: uint64(len([]rune(text))),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotID, _, err := store.GetExtractedUnitSnapshot(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{
		SchemaVersion: 1, ScopeID: uuid.NewString(), OwnerID: owner.ID, SourceMaterialID: source.ID,
		SourceUnitSnapshot: domain.EPUBUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: 1},
		Classifier:         domain.EPUBClassifierIdentity{Name: "deterministic", Version: "1"},
		SelectionMode:      domain.EPUBScopeSelectionOverridden,
		SelectedUnits:      []domain.EPUBSelectedUnitReference{{UnitID: domain.EPUBUnitID(0, "scoped-corpus"), Order: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(store.Pool(), &lifecycleAnalyzer{}, selection.NewService(store), newTestWorkers())
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())

	service := NewService(store.Pool(), client)
	handle, err := service.SubmitScopedAnalysis(ctx, owner.ID, source.ID, scope.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != rivertype.JobStateCompleted || status.LogicalState != "completed" || status.CorpusID == "" {
		t.Fatalf("scoped analysis status = %+v, want completed corpus", status)
	}
}

func newTestWorkers() *river.Workers {
	return river.NewWorkers()
}
