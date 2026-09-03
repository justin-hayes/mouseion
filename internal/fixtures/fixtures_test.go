package fixtures

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestStoreMoveReadingJourneyEntryMutatesAndProtectsRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.MoveReadingJourneyEntry(ctx, OwnerID, edgeBookID, 1, journey.Revision)
	if err != nil || revision != journey.Revision+1 {
		t.Fatalf("move revision=%d err=%v", revision, err)
	}
	journey, _ = store.GetReadingJourney(ctx, OwnerID)
	if journey.Entries[0].BookID != edgeBookID || journey.Entries[0].Position != 1 || journey.Entries[1].Position != 2 {
		t.Fatalf("reordered journey=%+v", journey.Entries)
	}
	if _, err = store.MoveReadingJourneyEntry(ctx, OwnerID, "fixture-empty", 1, revision-1); !errors.Is(err, persistence.ErrJourneyStale) {
		t.Fatalf("stale move error=%v", err)
	}
	if unchanged, err := store.MoveReadingJourneyEntry(ctx, OwnerID, edgeBookID, 0, revision); err != nil || unchanged != revision {
		t.Fatalf("clamped move revision=%d err=%v", unchanged, err)
	}
}

func TestFixtureJourneyProjectionCoversComparisonStatesDeterministically(t *testing.T) {
	store := NewStore()
	result, err := (Insights{JourneyStore: store}).JourneyProjection(context.Background(), OwnerID, "")
	if err != nil {
		t.Fatal(err)
	}
	wantLearner := []string{"fixture-empty", edgeBookID, routeMatchBookID, routeDiffersBookID, routeTieABookID, routeTieBBookID, routeUnavailableBookID}
	wantAdvisory := []string{"fixture-empty", edgeBookID, routeMatchBookID, routeTieABookID, routeTieBBookID, routeDiffersBookID, routeUnavailableBookID}
	for i, want := range wantLearner {
		if result.LearnerOrder[i].BookID != want {
			t.Fatalf("learner order[%d]=%q, want %q", i, result.LearnerOrder[i].BookID, want)
		}
	}
	for i, want := range wantAdvisory {
		if result.AdvisoryOrder[i].BookID != want {
			t.Fatalf("advisory order[%d]=%q, want %q", i, result.AdvisoryOrder[i].BookID, want)
		}
	}
	if result.ComparableCount != 4 || result.IncomparableCount != 3 {
		t.Fatalf("comparison counts=%d/%d", result.ComparableCount, result.IncomparableCount)
	}
	if result.AdvisoryOrder[0].Rank != nil || result.AdvisoryOrder[6].Rank != nil {
		t.Fatalf("incomparable ranks=%+v", result.AdvisoryOrder)
	}
	if result.AdvisoryOrder[3].Coverage.KnownTokenCount != result.AdvisoryOrder[4].Coverage.KnownTokenCount {
		t.Fatalf("tie coverage=%+v", result.AdvisoryOrder)
	}
	if len(result.ConditionalAdvisoryOrder) != len(result.AdvisoryOrder) {
		t.Fatalf("conditional order=%+v", result.ConditionalAdvisoryOrder)
	}
	if result.ConditionalAdvisoryOrder[2].BookID != routeDiffersBookID {
		t.Fatalf("conditional order did not differ=%+v", result.ConditionalAdvisoryOrder)
	}

	journey, err := store.GetReadingJourney(context.Background(), OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.MoveReadingJourneyEntry(context.Background(), OwnerID, routeDiffersBookID, 1, journey.Revision); err != nil {
		t.Fatal(err)
	}
	result, err = (Insights{JourneyStore: store}).JourneyProjection(context.Background(), OwnerID, "")
	if err != nil || result.LearnerOrder[0].BookID != routeDiffersBookID {
		t.Fatalf("projection did not follow canonical fixture order=%+v err=%v", result.LearnerOrder, err)
	}
}

func TestFixtureLanguageCorpusViewIsStableAndOwnerScoped(t *testing.T) {
	view, err := (Insights{}).LanguageCorpus(context.Background(), "another-owner", "DE")
	if err != nil {
		t.Fatal(err)
	}
	if view.OwnerID != "another-owner" || view.Language != "de" || view.AnalyzedBookCount != 3 || view.KnownTokenCount != 45678 || view.AnalyzableTokenCount != 123456 {
		t.Fatalf("language view identity/counts=%+v", view)
	}
	if len(view.TopUnknownLemmas) != 3 || len(view.PerBook) != 5 || view.PerBook[0].BookID != BookID || view.PerBook[3].Included || view.PerBook[3].ExclusionReason == "" {
		t.Fatalf("language view evidence=%+v", view)
	}
	empty, err := (Insights{}).LanguageCorpus(context.Background(), OwnerID, "it")
	if err != nil || empty.AnalyzedBookCount != 0 || len(empty.PerBook) != 0 {
		t.Fatalf("empty Italian language view=%+v err=%v", empty, err)
	}
}

func TestStoreConcurrentJourneyMovesAcceptOnlyOneRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			_, moveErr := store.MoveReadingJourneyEntry(ctx, OwnerID, edgeBookID, 1, journey.Revision)
			results <- moveErr
		}()
	}
	wait.Wait()
	close(results)
	var successes, stale int
	for moveErr := range results {
		switch {
		case moveErr == nil:
			successes++
		case errors.Is(moveErr, persistence.ErrJourneyStale):
			stale++
		default:
			t.Fatalf("concurrent move error=%v", moveErr)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent move results successes=%d stale=%d", successes, stale)
	}
}

func TestStorePrimaryGoalFinishFixturesCoverReadingOnlyResidualAndReviewed(t *testing.T) {
	ctx := context.Background()
	readingOnly := NewStore()
	readingOnly.campaigns = nil
	result, err := readingOnly.FinishReadingPrimaryGoal(ctx, OwnerID, BookID)
	if err != nil || result.Campaign != nil || result.ResidualVocabularyCount != 0 || result.Goal.ReadingFinishedAt == nil {
		t.Fatalf("reading-only result=%+v err=%v", result, err)
	}

	residual, err := NewStore().FinishReadingPrimaryGoal(ctx, OwnerID, BookID)
	if err != nil || residual.Campaign == nil || residual.Campaign.Status != domain.CampaignActive || residual.ResidualVocabularyCount != 2 || len(residual.Graduated) != 0 {
		t.Fatalf("residual result=%+v err=%v", residual, err)
	}

	reviewed := NewStore()
	campaign, err := reviewed.GetLearningCampaign(ctx, OwnerID, CampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reviewed.UpdateLearningCampaignProgress(ctx, OwnerID, CampaignID, persistence.LearningCampaignExpectedState{}, domain.BookReading, domain.DeckReviewed); err != nil {
		t.Fatal(err)
	}
	result, err = reviewed.FinishReadingPrimaryGoal(ctx, OwnerID, BookID)
	if err != nil || result.Campaign == nil || result.Campaign.Status != domain.CampaignComplete || len(result.Graduated) != 2 || result.ResidualVocabularyCount != 0 {
		t.Fatalf("reviewed-before-finish result=%+v campaign-before=%+v err=%v", result, campaign, err)
	}
}

func TestMigrationFixturesPinLegacyAndKnownVocabularyCategories(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	campaigns, err := store.ListLearningCampaigns(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[domain.CampaignStatus]bool{}
	for _, campaign := range campaigns {
		statuses[campaign.Status] = true
	}
	for _, status := range []domain.CampaignStatus{domain.CampaignQueued, domain.CampaignActive, domain.CampaignComplete, domain.CampaignAbandoned} {
		if !statuses[status] {
			t.Fatalf("migration fixture missing campaign status %q: %+v", status, campaigns)
		}
	}

	legacy, err := store.ListLegacyGeneratedVocabulary(ctx, OwnerID, "de")
	if err != nil || len(legacy) != 1 || legacy[0].CanonicalLemma != LegacyGeneratedLemma || legacy[0].FirstSourceMaterialID != nil {
		t.Fatalf("legacy generated fixture=%+v err=%v", legacy, err)
	}
	known, err := store.ListKnownVocabulary(ctx, OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	provenance := map[string]string{}
	for _, item := range known {
		provenance[item.CanonicalLemma] = item.Provenance
	}
	if provenance[IndependentKnownLemma] != "Explicitly recorded" || provenance[GraduatedKnownLemma] != "Graduated from completed campaign" {
		t.Fatalf("known vocabulary provenance=%v", provenance)
	}
	active, err := store.ListActiveLearningCampaignVocabulary(ctx, OwnerID, "de")
	if err != nil || len(active) != 2 {
		t.Fatalf("active reservation fixture=%+v err=%v", active, err)
	}
	books, err := store.ListMyBooksWithEvidence(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		if book.Book.ID == "fixture-metadata-only" && (book.Acquired != nil || book.EvidenceState != domain.MyBookNotAcquired) {
			t.Fatalf("metadata-only fixture acquired evidence=%+v", book)
		}
	}
}
