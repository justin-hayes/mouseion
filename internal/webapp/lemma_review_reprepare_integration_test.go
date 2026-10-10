//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repreparingSpy keeps the production prepared-deck service for every other
// capability and records the Reprepare calls the lemma-review handler makes.
// Reprepare is intercepted because the persistence re-preparation guard rejects
// a Book-linked preparation without a Goal snapshot (a legacy shape), so the
// real service cannot honor this request; this test pins the handler decision.
type repreparingSpy struct {
	*prepareddeck.Service
	reprepared []string
}

func (s *repreparingSpy) Reprepare(_ context.Context, _, id string) (prepareddeck.Handle, error) {
	s.reprepared = append(s.reprepared, id)
	return prepareddeck.Handle{Preparation: domain.DeckPreparation{ID: "new-preparation"}}, nil
}

func TestReadyDeckIsOfferedAndRepreparedOnIdentityChangeWithProductionWiring(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "lemma-reprepare-integration-secret-0123")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	owner := createAccount(t, ctx, store, "lemma-reprepare-owner", "learner-password", false)
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "lemma-reprepare", "Lemma reprepare", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "drach", UPOS: "NOUN", OccurrenceCount: 2},
	})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	var analysisRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&analysisRun))
	unitID := domain.EPUBUnitID(0, "lemma-reprepare")
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Der Drache sieht einen Drachen.',0,31),
		      ($1,$2,$3,$4,1,'Der Drachen steigt als Drachen.',32,63)`, owner.ID, analysisRun, corpus.ID, unitID)
	require.NoError(t, err)
	for _, token := range []struct {
		sentence   int64
		start, end int64
	}{{0, 22, 29}, {1, 36, 43}} {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,start_offset,end_offset,dependency,head)
			VALUES($1,'de',$2,$3,$4,0,'Drachen','Drach','drach','NOUN','{}',$5,$6,'root',0)`, owner.ID, analysisRun, corpus.ID, token.sentence, token.start, token.end)
		require.NoError(t, err)
	}

	buildBrowseProjection(t, ctx, store, book.ID)

	readyDeck, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRun, Filename: "ready.apkg", DeckName: "Ready", ContentHash: source.ContentHash})
	require.NoError(t, err)
	readyDeck, err = store.ClaimDeckPreparation(ctx, owner.ID, readyDeck.ID)
	require.NoError(t, err)
	readyDeck, err = store.CompleteDeckPreparation(ctx, owner.ID, readyDeck.ID, domain.DeckPreparation{Artifact: []byte("ready-deck"), Filename: "ready.apkg", DeckName: "Ready", TotalCards: 1})
	require.NoError(t, err)

	workers := river.NewWorkers()
	prepareddeck.AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), nil, nil, prepareddeck.BatchConfig{}, prepareddeck.PreparedDeckConfig{}, false)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{prepareddeck.Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	spy := &repreparingSpy{Service: prepareddeck.NewService(store, client)}
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store),
		PreparedDeck: spy, Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	lemmaPath := "/reading/books/" + book.ID + "/lemma-review"

	page := perform(t, h, http.MethodGet, lemmaPath+"?form=Drachen", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "/deck-preparations/"+readyDeck.ID+"/download", "the historical ready deck is offered for download")

	preview := perform(t, h, http.MethodPost, lemmaPath, url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "form": {"Drachen"}, "target": {"0"}, "decision": {"correct"}, "lemma": {"drache"}}, cookies)
	require.Equal(t, http.StatusOK, preview.Code)
	assert.Contains(t, preview.Body.String(), "drach (NOUN):</strong> recurrence 2 → 1", "the preview renders the impact from the count projection")
	assert.Contains(t, preview.Body.String(), "Reading snapshot/deck eligible no → no")
	fingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(preview.Body.String())
	require.Len(t, fingerprint, 2)
	assert.Contains(t, preview.Body.String(), `name="reprepare_ready_deck"`, "an identity-changing preview asks for explicit re-preparation consent")

	confirm := url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Drachen"}, "decision": {"correct"}, "lemma": {"drache"}, "fingerprint": {fingerprint[1]}, "selected": {"0"}}
	unconsented := perform(t, h, http.MethodPost, lemmaPath, confirm, cookies)
	require.Equal(t, http.StatusBadRequest, unconsented.Code, "the identity change is refused without explicit consent")
	assert.Empty(t, spy.reprepared, "a refused confirmation does not re-prepare the ready deck")

	confirm.Set("reprepare_ready_deck", "yes")
	confirmed := perform(t, h, http.MethodPost, lemmaPath, confirm, cookies)
	require.Equal(t, http.StatusSeeOther, confirmed.Code)
	assert.Equal(t, []string{readyDeck.ID}, spy.reprepared, "the confirmed identity change re-prepares that exact ready preparation")
	assert.Equal(t, "/deck-preparations/new-preparation/status", confirmed.Header().Get("Location"), "the learner lands on the new preparation status")
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Drachen")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	assert.Equal(t, "drache", occurrences[0].CorrectedLemma, "the decision is saved alongside the re-preparation")
	afterDecision := perform(t, h, http.MethodGet, lemmaPath+"?form=Drachen", nil, cookies)
	require.Equal(t, http.StatusOK, afterDecision.Code)
	assert.Contains(t, afterDecision.Body.String(), "/deck-preparations/"+readyDeck.ID+"/reprepare", "after an identity decision the ready deck offers an explicit re-prepare control")
}
