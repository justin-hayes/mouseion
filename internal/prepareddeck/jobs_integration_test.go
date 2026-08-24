//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestServiceEnqueuesOwnerScopedImmutablePreparationAndRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "prepared-service-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser(ctx, "prepared-service-other", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "service-book", Title: "A Book", MediaType: "text/plain", ContentHash: "immutable-hash", Content: []byte("text"), FullText: "text"})
	if err != nil {
		t.Fatal(err)
	}
	workers := river.NewWorkers()
	AddWorker(workers, store, cardexport.NewService(store), nil)
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, client)
	handle, err := service.Submit(ctx, owner.ID, source.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if handle.Preparation.State != domain.DeckPreparationQueued || handle.Preparation.ContentHash != source.ContentHash || handle.JobID == 0 {
		t.Fatalf("handle=%+v", handle)
	}
	var args JobArgs
	job, err := client.JobGet(ctx, handle.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(job.EncodedArgs, &args); err != nil {
		t.Fatal(err)
	}
	if args.OwnerID != owner.ID || args.SourceMaterialID != source.ID || args.ContentHash != source.ContentHash || !args.ExternalTranslationConsent {
		t.Fatalf("args=%+v", args)
	}
	if _, err = service.Get(ctx, other.ID, handle.Preparation.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-owner get=%v", err)
	}
	cancelled, err := service.Cancel(ctx, owner.ID, handle.Preparation.ID)
	if err != nil || cancelled.State != domain.DeckPreparationCancelled {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	retried, err := service.Retry(ctx, owner.ID, handle.Preparation.ID, false)
	if err != nil || retried.Preparation.State != domain.DeckPreparationQueued || retried.JobID == handle.JobID {
		t.Fatalf("retried=%+v err=%v", retried, err)
	}
}
