//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestLearningCampaignResidualWorkSurvivesGoalChangeAndGraduatesOnce(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "campaign-474-residual", false)
	if err != nil {
		t.Fatal(err)
	}
	source, prep := readyCampaignFixture(t, ctx, store, owner.ID, "474-residual")
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Issue 474 residual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{
		OwnerID: owner.ID, Language: "de", CanonicalLemma: "residual", UPOS: "NOUN",
		FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID,
	}); err != nil {
		t.Fatal(err)
	}
	campaign, err := store.CreateLearningCampaign(ctx, owner.ID, source.ID, prep.ID)
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(campaign), domain.BookReading, domain.DeckStudying)
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(campaign), domain.BookFinished, domain.DeckStudying)
	if err != nil || campaign.Status != domain.CampaignActive || campaign.BookFinishedAt == nil || campaign.DeckReviewedAt != nil || campaign.VocabularyGraduatedAt != nil {
		t.Fatalf("reading-finished residual campaign=%+v err=%v", campaign, err)
	}
	assertKnownCount(t, ctx, store, owner.ID, 0)
	active, err := store.ListActiveLearningCampaignVocabulary(ctx, owner.ID, "de")
	if err != nil || len(active) != 1 || active[0].CanonicalLemma != "residual" {
		t.Fatalf("residual reservation=%+v err=%v", active, err)
	}

	// A stale/regressive submission cannot turn the residual campaign into a
	// reviewed one or partially mutate its already-finished reading fact.
	if _, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(campaign), domain.BookReading, domain.DeckReviewed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reversed residual transition error=%v, want ErrInvalidTransition", err)
	}
	unchanged, err := store.GetLearningCampaign(ctx, owner.ID, campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.BookProgress != domain.BookFinished || unchanged.DeckProgress != domain.DeckStudying || unchanged.VocabularyGraduatedAt != nil {
		t.Fatalf("reversed transition changed residual campaign=%+v", unchanged)
	}

	oldBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Issue 474 old Goal", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	newBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Issue 474 new Goal", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, owner.ID, oldBook.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.ChangePrimaryGoal(ctx, owner.ID, newBook.ID, oldBook.ID); err != nil {
		t.Fatal(err)
	}
	goal, err := store.GetPrimaryGoal(ctx, owner.ID)
	if err != nil || goal.BookID != newBook.ID {
		t.Fatalf("changed Goal=%+v err=%v", goal, err)
	}
	unchanged, err = store.GetLearningCampaign(ctx, owner.ID, campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err = store.ListActiveLearningCampaignVocabulary(ctx, owner.ID, "de")
	if err != nil || len(active) != 1 || active[0].CanonicalLemma != "residual" {
		t.Fatalf("Goal change altered residual reservation=%+v err=%v", active, err)
	}
	assertKnownCount(t, ctx, store, owner.ID, 0)

	campaign, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(unchanged), domain.BookFinished, domain.DeckReviewed)
	if err != nil || campaign.Status != domain.CampaignComplete || campaign.VocabularyGraduatedAt == nil {
		t.Fatalf("justified residual completion=%+v err=%v", campaign, err)
	}
	graduatedAt := *campaign.VocabularyGraduatedAt
	assertKnownCount(t, ctx, store, owner.ID, 1)
	active, err = store.ListActiveLearningCampaignVocabulary(ctx, owner.ID, "de")
	if err != nil || len(active) != 0 {
		t.Fatalf("active reservation after graduation=%+v err=%v", active, err)
	}

	repeated, err := store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(campaign), domain.BookFinished, domain.DeckReviewed)
	if err != nil || repeated.VocabularyGraduatedAt == nil || !repeated.VocabularyGraduatedAt.Equal(graduatedAt) {
		t.Fatalf("repeated justified completion=%+v err=%v", repeated, err)
	}
	assertKnownCount(t, ctx, store, owner.ID, 1)
	if _, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(repeated), domain.BookFinished, domain.DeckStudying); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("post-completion deck regression error=%v, want ErrInvalidTransition", err)
	}
}

func TestLearningCampaignGraduationFailureRollsBackAllProgressAndCanRetry(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "campaign-474-rollback", false)
	if err != nil {
		t.Fatal(err)
	}
	source, prep := readyCampaignFixture(t, ctx, store, owner.ID, "474-rollback")
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Issue 474 rollback")
	if err != nil {
		t.Fatal(err)
	}
	for _, lemma := range []string{"rollback-safe", "rollback-failure"} {
		if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{
			OwnerID: owner.ID, Language: "de", CanonicalLemma: lemma, UPOS: "NOUN",
			FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	campaign, err := store.CreateLearningCampaign(ctx, owner.ID, source.ID, prep.ID)
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(campaign), domain.BookReading, domain.DeckStudying)
	if err != nil {
		t.Fatal(err)
	}

	const functionName = "issue_474_reject_known_insert"
	const triggerName = "issue_474_reject_known_insert_trigger"
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), "DROP TRIGGER IF EXISTS "+triggerName+" ON known_vocabulary")
		_, _ = store.Pool().Exec(context.Background(), "DROP FUNCTION IF EXISTS "+functionName+"()")
	}()
	if _, err = store.Pool().Exec(ctx, `CREATE FUNCTION issue_474_reject_known_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.canonical_lemma = 'rollback-failure' THEN
		RAISE EXCEPTION 'issue 474 injected graduation failure';
	END IF;
	RETURN NEW;
END
$$`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `CREATE TRIGGER issue_474_reject_known_insert_trigger BEFORE INSERT ON known_vocabulary FOR EACH ROW EXECUTE FUNCTION issue_474_reject_known_insert()`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(campaign), domain.BookFinished, domain.DeckReviewed); err == nil {
		t.Fatal("graduation failure unexpectedly committed")
	}

	rolledBack, err := store.GetLearningCampaign(ctx, owner.ID, campaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.BookProgress != domain.BookReading || rolledBack.DeckProgress != domain.DeckStudying || rolledBack.CompletedAt != nil || rolledBack.VocabularyGraduatedAt != nil {
		t.Fatalf("failed graduation left campaign progress=%+v", rolledBack)
	}
	assertKnownCount(t, ctx, store, owner.ID, 0)
	var stateCount, graduatedCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_states WHERE owner_id=$1 AND canonical_lemma IN ('rollback-safe','rollback-failure') AND state='known'`, owner.ID).Scan(&stateCount); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM learning_campaign_vocabulary WHERE owner_id=$1 AND campaign_id=$2 AND graduated_at IS NOT NULL`, owner.ID, campaign.ID).Scan(&graduatedCount); err != nil {
		t.Fatal(err)
	}
	if stateCount != 0 || graduatedCount != 0 {
		t.Fatalf("failed graduation left vocabulary state=%d graduated=%d", stateCount, graduatedCount)
	}

	if _, err = store.Pool().Exec(ctx, "DROP TRIGGER "+triggerName+" ON known_vocabulary"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, "DROP FUNCTION "+functionName+"()"); err != nil {
		t.Fatal(err)
	}
	completed, err := store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, campaignExpectedState(rolledBack), domain.BookFinished, domain.DeckReviewed)
	if err != nil || completed.Status != domain.CampaignComplete || completed.VocabularyGraduatedAt == nil {
		t.Fatalf("retry after rollback=%+v err=%v", completed, err)
	}
	assertKnownCount(t, ctx, store, owner.ID, 2)
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM learning_campaign_vocabulary WHERE owner_id=$1 AND campaign_id=$2 AND graduated_at IS NOT NULL`, owner.ID, campaign.ID).Scan(&graduatedCount); err != nil {
		t.Fatal(err)
	}
	if graduatedCount != 2 {
		t.Fatalf("retry graduated %d snapshot rows, want 2", graduatedCount)
	}
}

func assertKnownCount(t *testing.T, ctx context.Context, store *PostgresStore, owner string, want int) {
	t.Helper()
	var got int
	if err := store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("known vocabulary count=%d, want %d", got, want)
	}
}
