//go:build integration

package knownvocab

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

func TestRiverImportLifecycleResultsRetrySafetyAndOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = analysis.MigrateRiver(ctx, store.Pool()); err != nil {
		t.Fatal(err)
	}
	alice, err := store.CreateUser(ctx, "known-job-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "known-job-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	workers := river.NewWorkers()
	AddWorker(workers, store.Pool())
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	service := NewJobService(store.Pool(), client)
	handle, err := service.Submit(ctx, alice.ID, "de", "Daß\nHaus\nbad\tNOPE\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Get(ctx, bob.ID, handle.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("cross-owner status = %v", err)
	}
	status := waitKnownVocabJob(t, ctx, service, alice.ID, handle.ID)
	if status.State != rivertype.JobStateCompleted || status.Imported != 2 || status.AlreadyKnown != 0 || len(status.Rejected) != 1 || status.Processed != 3 || status.Total != 3 {
		t.Fatalf("first status = %+v", status)
	}
	duplicate, err := service.Submit(ctx, alice.ID, "de", "Daß\nHaus\n")
	if err != nil {
		t.Fatal(err)
	}
	status = waitKnownVocabJob(t, ctx, service, alice.ID, duplicate.ID)
	if status.Imported != 0 || status.AlreadyKnown != 2 {
		t.Fatalf("duplicate status = %+v", status)
	}
	var knownRows int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&knownRows); err != nil || knownRows != 2 {
		t.Fatalf("known rows=%d err=%v", knownRows, err)
	}
	// A forged retry without its owner-scoped history handle is rejected before writes.
	forged := &river.Job[JobArgs]{JobRow: &rivertype.JobRow{ID: 999999}, Args: JobArgs{OwnerID: alice.ID, Language: "de", FileContents: "neu"}}
	if err = (&Worker{Pool: store.Pool()}).Work(ctx, forged); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("forged work = %v", err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&knownRows); err != nil || knownRows != 2 {
		t.Fatalf("rows after rejected retry=%d err=%v", knownRows, err)
	}
}

func waitKnownVocabJob(t *testing.T, ctx context.Context, service *JobService, owner string, id int64) Status {
	t.Helper()
	for {
		status, err := service.Get(ctx, owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == rivertype.JobStateCompleted || status.State == rivertype.JobStateDiscarded {
			return status
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}
