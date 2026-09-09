package fixtures

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestStoreMoveReadingJourneyEntryMutatesAndProtectsRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID, "it")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.MoveReadingJourneyEntry(ctx, OwnerID, "it", edgeBookID, 1, journey.Revision)
	if err != nil || revision != journey.Revision+1 {
		t.Fatalf("move revision=%d err=%v", revision, err)
	}
	journey, _ = store.GetReadingJourney(ctx, OwnerID, "it")
	if journey.Entries[0].BookID != edgeBookID || journey.Entries[0].Position != 1 || journey.Entries[1].Position != 2 {
		t.Fatalf("reordered journey=%+v", journey.Entries)
	}
	if _, err = store.MoveReadingJourneyEntry(ctx, OwnerID, "it", "fixture-empty", 1, revision-1); !errors.Is(err, persistence.ErrJourneyStale) {
		t.Fatalf("stale move error=%v", err)
	}
	if unchanged, err := store.MoveReadingJourneyEntry(ctx, OwnerID, "it", edgeBookID, 0, revision); err != nil || unchanged != revision {
		t.Fatalf("clamped move revision=%d err=%v", unchanged, err)
	}
}

func TestFixtureGetBookDetailResolvesBookAndSourceIDs(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	store.books = append(store.books, domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: "fixture-detail-source", OwnerID: OwnerID, Language: "de", Title: "Distinct fixture identities"},
		BookID:         "fixture-detail-book",
		AnalysisStatus: "not analyzed",
	})

	for _, id := range []string{BookID, SourceID, "fixture-metadata-only", "fixture-detail-book", "fixture-detail-source"} {
		detail, err := store.GetBookDetail(ctx, OwnerID, id)
		if err != nil {
			t.Fatalf("GetBookDetail(%q): %v", id, err)
		}
		if id == SourceID || id == "fixture-detail-source" {
			wantBookID := BookID
			if id == "fixture-detail-source" {
				wantBookID = "fixture-detail-book"
			}
			if detail.Book.ID != wantBookID || detail.Acquired == nil {
				t.Fatalf("source resolution=%+v", detail)
			}
		} else if detail.Book.ID != id {
			t.Fatalf("GetBookDetail(%q) book=%q", id, detail.Book.ID)
		}
	}

	detail, err := store.GetBookDetail(ctx, OwnerID, BookID)
	if err != nil || detail.Acquired == nil || detail.EvidenceState() != domain.BookAnalyzed {
		t.Fatalf("acquired detail=%+v err=%v", detail, err)
	}
	if _, err = store.GetBookDetail(ctx, "other-owner", BookID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("cross-owner detail error=%v", err)
	}
	if _, err = store.GetBookDetail(ctx, OwnerID, "unknown"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unknown detail error=%v", err)
	}
}

func TestFixtureJourneyProjectionCoversComparisonStatesDeterministically(t *testing.T) {
	store := NewStore()
	result, err := (Insights{JourneyStore: store}).JourneyProjection(context.Background(), OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	wantLearner := []string{BookID, "fixture-failed", routeMatchBookID, routeDiffersBookID, routeTieABookID, routeTieBBookID, routeUnavailableBookID}
	wantAdvisory := []string{BookID, "fixture-failed", routeMatchBookID, routeTieABookID, routeTieBBookID, routeDiffersBookID, routeUnavailableBookID}
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

	journey, err := store.GetReadingJourney(context.Background(), OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.MoveReadingJourneyEntry(context.Background(), OwnerID, "de", routeDiffersBookID, 1, journey.Revision); err != nil {
		t.Fatal(err)
	}
	result, err = (Insights{JourneyStore: store}).JourneyProjection(context.Background(), OwnerID, "de")
	if err != nil || result.LearnerOrder[1].BookID != routeDiffersBookID {
		t.Fatalf("projection did not follow canonical fixture order=%+v err=%v", result.LearnerOrder, err)
	}
}

func TestFixtureItalianJourneyProjectionUsesItalianEvidenceIdentity(t *testing.T) {
	result, err := (Insights{JourneyStore: NewStore()}).JourneyProjection(context.Background(), OwnerID, "it")
	if err != nil {
		t.Fatal(err)
	}
	if result.Language != "it" {
		t.Fatalf("Italian projection language=%q", result.Language)
	}
	if len(result.LearnerOrder) != 3 {
		t.Fatalf("Italian projection order=%+v", result.LearnerOrder)
	}
	for _, book := range result.LearnerOrder {
		if book.Language != "it" {
			t.Fatalf("Italian projection book=%+v", book)
		}
		if strings.Contains(book.IncomparableReason, "different study language") {
			t.Fatalf("Italian projection retained cross-language reason=%q", book.IncomparableReason)
		}
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

func TestStoreStudyLanguagesDeriveFromFixtureBooks(t *testing.T) {
	store := NewStore()
	store.supported = []domain.SupportedLanguage{{Language: " DE ", DisplayName: "German"}}
	store.books = []domain.SourceMaterialSummary{
		{Source: domain.SourceMaterial{OwnerID: OwnerID, Language: " DE "}},
		{Source: domain.SourceMaterial{OwnerID: OwnerID, Language: "it_IT"}},
		{Source: domain.SourceMaterial{OwnerID: "another-owner", Language: "fr"}},
	}
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{OwnerID: OwnerID, LanguageState: domain.LanguageChosen, LanguageTag: "pt-BR"}},
		{Book: domain.Book{OwnerID: OwnerID, LanguageState: domain.LanguageUnknown, LanguageTag: ""}},
		{Book: domain.Book{OwnerID: "another-owner", LanguageState: domain.LanguageChosen, LanguageTag: "nl"}},
	}

	languages, err := store.ListStudyLanguages(context.Background(), OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.StudyLanguage{
		{Language: "de", DisplayName: "German"},
		{Language: "it", DisplayName: "it"},
		{Language: "pt", DisplayName: "pt"},
	}
	if len(languages) != len(want) {
		t.Fatalf("study languages=%+v, want %+v", languages, want)
	}
	for i := range want {
		if languages[i] != want[i] {
			t.Fatalf("study language[%d]=%+v, want %+v", i, languages[i], want[i])
		}
	}
}

func TestStoreActiveStudyLanguageIsStoredAndNullable(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	if got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID); err != nil || got != "de" {
		t.Fatalf("initial active language=%q err=%v", got, err)
	}
	if err := store.SetActiveStudyLanguage(ctx, OwnerID, "IT_it"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID); err != nil || got != "it" {
		t.Fatalf("stored active language=%q err=%v", got, err)
	}
	if err := store.SetActiveStudyLanguage(ctx, OwnerID, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID); err != nil || got != "" {
		t.Fatalf("cleared active language=%q err=%v", got, err)
	}
}

func TestFixtureCatalogueSyncAddsPassiveStudyLanguageArrival(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)

	if got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID); err != nil || got != "de" {
		t.Fatalf("initial active language=%q err=%v", got, err)
	}
	if recent, err := store.MostRecentlyActivatedStudyLanguage(ctx, OwnerID); err != nil || recent != "it" {
		t.Fatalf("initial recent language=%q err=%v", recent, err)
	}

	if _, err := sync.Enqueue(ctx, OwnerID, "fixture-connection"); err != nil {
		t.Fatal(err)
	}

	languages, err := store.ListStudyLanguages(ctx, OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	var foundSpanish bool
	for _, language := range languages {
		if language.Language == "es" {
			foundSpanish = true
			break
		}
	}
	if !foundSpanish {
		t.Fatalf("study languages after sync=%+v, want Spanish arrival", languages)
	}
	if recent, err := store.MostRecentlyActivatedStudyLanguage(ctx, OwnerID); err != nil || recent != "es" {
		t.Fatalf("recent language after sync=%q err=%v", recent, err)
	}
	if got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID); err != nil || got != "de" {
		t.Fatalf("active language changed after sync=%q err=%v", got, err)
	}
}

func TestStoreKnownVocabularyOnlyLanguageRemainsViewable(t *testing.T) {
	store := NewStore()
	if err := store.SetActiveStudyLanguage(context.Background(), OwnerID, "fr"); err != nil {
		t.Fatal(err)
	}
	known, err := store.ListKnownVocabulary(context.Background(), OwnerID, "fr")
	if err != nil || len(known) != 1 || known[0].CanonicalLemma != "bonjour" {
		t.Fatalf("known-only vocabulary=%+v err=%v", known, err)
	}
	if languages, err := store.ListStudyLanguages(context.Background(), OwnerID); err != nil || len(languages) != 2 {
		t.Fatalf("study languages=%+v err=%v", languages, err)
	}
}

func TestStoreMyBooksBrowseUsesCanonicalLanguageIdentity(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "regional", OwnerID: OwnerID, Title: "Regional", LanguageState: domain.LanguageChosen, LanguageTag: "de-DE"}},
	}
	result, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "DE_de", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Counts[0].Tag != "de" {
		t.Fatalf("canonical browse result=%+v", result)
	}
}

func TestStoreConcurrentJourneyMovesAcceptOnlyOneRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID, "it")
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			_, moveErr := store.MoveReadingJourneyEntry(ctx, OwnerID, "it", edgeBookID, 1, journey.Revision)
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
	result, err := readingOnly.FinishReadingPrimaryGoal(ctx, OwnerID, "de", BookID)
	if err != nil || result.Campaign != nil || result.ResidualVocabularyCount != 0 || result.Goal.ReadingFinishedAt == nil {
		t.Fatalf("reading-only result=%+v err=%v", result, err)
	}

	residual, err := NewStore().FinishReadingPrimaryGoal(ctx, OwnerID, "de", BookID)
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
	result, err = reviewed.FinishReadingPrimaryGoal(ctx, OwnerID, "de", BookID)
	if err != nil || result.Campaign == nil || result.Campaign.Status != domain.CampaignComplete || len(result.Graduated) != 2 || result.ResidualVocabularyCount != 0 {
		t.Fatalf("reviewed-before-finish result=%+v campaign-before=%+v err=%v", result, campaign, err)
	}
}

func TestStorePrimaryGoalsAreIndependentByLanguageAndJourneyRemovalClearsOnlyThatLanguage(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	deGoal, err := store.GetPrimaryGoal(ctx, OwnerID, "de")
	if err != nil || deGoal.BookID != BookID {
		t.Fatalf("German Goal=%+v err=%v", deGoal, err)
	}
	itGoal, err := store.GetPrimaryGoal(ctx, OwnerID, "it")
	if err != nil || itGoal.BookID != ItalianGoalBookID {
		t.Fatalf("Italian Goal=%+v err=%v", itGoal, err)
	}

	if err = store.ClearPrimaryGoal(ctx, OwnerID, "it", ItalianGoalBookID); err != nil {
		t.Fatalf("clear Italian Goal: %v", err)
	}
	if deGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "de"); err != nil || deGoal.BookID != BookID {
		t.Fatalf("German Goal after Italian clear=%+v err=%v", deGoal, err)
	}
	if itGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "it"); err != nil || itGoal.BookID != "" {
		t.Fatalf("Italian Goal after clear=%+v err=%v", itGoal, err)
	}

	if _, err = store.CreatePrimaryGoal(ctx, OwnerID, "it", ItalianGoalBookID); err != nil {
		t.Fatalf("recreate Italian Goal: %v", err)
	}
	journey, err := store.GetReadingJourney(ctx, OwnerID, "it")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RemoveFromReadingJourney(ctx, OwnerID, "it", ItalianGoalBookID, journey.Revision); err != nil {
		t.Fatalf("remove Italian Goal member: %v", err)
	}
	if itGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "it"); err != nil || itGoal.BookID != "" {
		t.Fatalf("Italian Goal after Journey removal=%+v err=%v", itGoal, err)
	}
	if deGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "de"); err != nil || deGoal.BookID != BookID {
		t.Fatalf("German Goal after Italian Journey removal=%+v err=%v", deGoal, err)
	}
}

func TestStoreRetaggingGoalBookClearsItsLanguageGoal(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	if _, err := store.UpdateBookMetadata(ctx, OwnerID, ItalianGoalBookID, "Una meta", domain.LanguageUnknown, ""); err != nil {
		t.Fatalf("retag Italian Goal book: %v", err)
	}
	if goal, err := store.GetPrimaryGoal(ctx, OwnerID, "it"); err != nil || goal.BookID != "" {
		t.Fatalf("Italian Goal after retag=%+v err=%v", goal, err)
	}
	if goal, err := store.GetPrimaryGoal(ctx, OwnerID, "de"); err != nil || goal.BookID != BookID {
		t.Fatalf("German Goal after Italian retag=%+v err=%v", goal, err)
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
		if book.Book.ID == "fixture-metadata-only" && (book.Acquired != nil || book.EvidenceState() != domain.BookNotAcquired) {
			t.Fatalf("metadata-only fixture acquired evidence=%+v", book)
		}
	}
}

func TestFixtureCatalogueAliasScopesRefreshAndAcquisition(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)

	alias, err := store.GetBookCatalogEntryAlias(ctx, OwnerID, "fixture-metadata-only")
	if err != nil || alias.ConnectionID != "fixture-connection" || alias.Value != "fixture-entry" {
		t.Fatalf("catalogue alias=%+v err=%v", alias, err)
	}
	target, err := sync.FindAcquisitionTarget(ctx, OwnerID, "fixture-metadata-only")
	if err != nil {
		t.Fatal(err)
	}
	if target.ConnectionID != alias.ConnectionID || target.Entry.ID != alias.Value {
		t.Fatalf("acquisition target=%+v alias=%+v", target, alias)
	}
	store.myBooks[0].Book.Title = "Stale metadata"
	result, err := sync.RefreshEntry(ctx, OwnerID, "fixture-metadata-only")
	if err != nil || !result.Updated || result.Book.Title != "Metadata-only migration book" {
		t.Fatalf("refresh result=%+v err=%v", result, err)
	}

	store.aliases[0].ConnectionID = "fixture-failed-connection"
	if _, err = sync.FindAcquisitionTarget(ctx, OwnerID, "fixture-metadata-only"); !errors.Is(err, cataloguesync.ErrNotFound) {
		t.Fatalf("wrong connection acquisition error=%v", err)
	}
	result, err = sync.RefreshEntry(ctx, OwnerID, "fixture-metadata-only")
	if err != nil || !result.Missing || result.Failed {
		t.Fatalf("wrong connection refresh=%+v err=%v", result, err)
	}
}

func TestFixtureNeedsLanguageBookCannotJoinJourney(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	journey, err := store.GetReadingJourney(ctx, OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, OwnerID, "de", "fixture-metadata-only", journey.Revision); !errors.Is(err, persistence.ErrBookLanguageRequired) {
		t.Fatalf("unknown-language Journey add error=%v", err)
	}
	unchanged, err := store.GetReadingJourney(ctx, OwnerID, "de")
	if err != nil || unchanged.Revision != journey.Revision {
		t.Fatalf("unknown-language add changed Journey=%+v err=%v", unchanged, err)
	}
}

func TestFixtureCatalogueSyncAdmitsNeedsLanguageBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)
	before, err := store.ListMyBooksBrowse(ctx, OwnerID, "", domain.LanguageUnknown, 0, 25)
	if err != nil || before.Total < 1 {
		t.Fatalf("initial needs-language browse=%+v err=%v", before, err)
	}
	if _, err = sync.Enqueue(ctx, OwnerID, "fixture-browser-sync-connection"); err != nil {
		t.Fatal(err)
	}
	afterUnknown, err := store.ListMyBooksBrowse(ctx, OwnerID, "", domain.LanguageUnknown, 0, 25)
	if err != nil || afterUnknown.Total != before.Total-1 {
		t.Fatalf("needs-language book remained after sync=%+v err=%v", afterUnknown, err)
	}
	afterGerman, err := store.ListMyBooksBrowse(ctx, OwnerID, "", "de", 0, 25)
	if err != nil || afterGerman.Total == 0 {
		t.Fatalf("re-synced book missing from German browse=%+v err=%v", afterGerman, err)
	}
	found := false
	for _, book := range afterGerman.Items {
		if book.Book.ID == BrowserSyncBookID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("re-synced metadata book missing from German page=%+v", afterGerman.Items)
	}
}
