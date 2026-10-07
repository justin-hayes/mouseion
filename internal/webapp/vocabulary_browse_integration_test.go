//go:build integration

package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredVocabularyRoutesDoNotExposeOrMutateStoredCustomData(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "retired-vocabulary-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "retired-vocabulary-owner", "owner-password", false)
	createAccount(t, ctx, store, "retired-vocabulary-other", "other-password", false)
	var deckID, preparationID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_decks(owner_id,language,name,creation_key) VALUES($1,'de','Archived custom deck','a2284277-041c-463c-a47b-6aa2060ec8fa') RETURNING id::text`, owner.ID).Scan(&deckID))
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_deck_preparations(owner_id,custom_deck_id,submission_key,language,deck_name,filename,state,artifact,total_cards,selected_identities,completed_at) VALUES($1,$2,'05e62174-9e03-48a9-8fb6-b787a7e8209d','de','Archived custom deck','archived.apkg','ready','retained-apkg-bytes',1,1,now()) RETURNING id::text`, owner.ID, deckID).Scan(&preparationID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_selections(owner_id,language,canonical_lemma,upos) VALUES($1,'de','archived','NOUN')`, owner.ID)
	require.NoError(t, err)

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "retired-vocabulary-owner", "owner-password")
	otherCookies, _ := loginCookies(t, h, "retired-vocabulary-other", "other-password")
	paths := []struct{ method, path string }{
		{http.MethodGet, "/vocabulary/selection"},
		{http.MethodGet, "/vocabulary/selection/clear-confirm"},
		{http.MethodPost, "/vocabulary/selection/add"},
		{http.MethodPost, "/vocabulary/selection/remove"},
		{http.MethodPost, "/vocabulary/selection/clear"},
		{http.MethodPost, "/vocabulary/decks"},
		{http.MethodGet, "/vocabulary/decks/" + deckID},
		{http.MethodPost, "/vocabulary/decks/" + deckID + "/rename"},
		{http.MethodPost, "/vocabulary/decks/" + deckID + "/identities/add"},
		{http.MethodPost, "/vocabulary/decks/" + deckID + "/identities/remove"},
		{http.MethodPost, "/vocabulary/decks/" + deckID + "/preparations"},
		{http.MethodGet, "/vocabulary/decks/" + deckID + "/delete-confirm"},
		{http.MethodPost, "/vocabulary/decks/" + deckID + "/delete"},
		{http.MethodGet, "/vocabulary/deck-preparations/" + preparationID},
		{http.MethodGet, "/vocabulary/deck-preparations/" + preparationID + "/download"},
		{http.MethodPost, "/vocabulary/deck-preparations/" + preparationID + "/cancel"},
	}
	for _, route := range paths {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			unauthenticated := perform(t, h, route.method, route.path, nil, nil)
			assert.Equal(t, http.StatusNotFound, unauthenticated.Code, "retired URLs disclose nothing before authentication")
			authenticated := perform(t, h, route.method, route.path, nil, cookies)
			assert.Equal(t, http.StatusNotFound, authenticated.Code, "retired URLs cannot read or mutate retained data")
			otherAuthenticated := perform(t, h, route.method, route.path, nil, otherCookies)
			assert.Equal(t, http.StatusNotFound, otherAuthenticated.Code, "another authenticated owner cannot discover retained data")
		})
	}
	var selectionCount, deckCount, preparationCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_selections WHERE owner_id=$1`, owner.ID).Scan(&selectionCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_decks WHERE owner_id=$1`, owner.ID).Scan(&deckCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_preparations WHERE owner_id=$1`, owner.ID).Scan(&preparationCount))
	assert.Equal(t, 1, selectionCount, "retirement does not run the separately scoped selection cleanup")
	assert.Equal(t, 1, deckCount, "custom-deck records are retained")
	assert.Equal(t, 1, preparationCount, "durable custom preparation records are retained")
	var artifact []byte
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT artifact FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$2`, owner.ID, preparationID).Scan(&artifact))
	assert.Equal(t, []byte("retained-apkg-bytes"), artifact, "archived APKG bytes remain stored")
}

type retainedCustomDeckTranslation struct{}

func (retainedCustomDeckTranslation) Name() string    { return "retained-custom-job-test" }
func (retainedCustomDeckTranslation) Version() string { return "1" }
func (retainedCustomDeckTranslation) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if request.CanonicalLemma == "missingwort" {
		return enrichment.TranslationResponse{UnresolvedReason: "No distinguishable contextual meaning."}, nil
	}
	return enrichment.TranslationResponse{
		Translation: "house", Gloss: "a building", ContextOnly: true,
		SentenceTranslation: "The children visit the old house.", SentenceTranslationTargets: []string{"house"},
	}, nil
}

func TestPreviouslySubmittedCustomPreparationCanFinishAfterRouteRetirement(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "retained-custom-job-owner", false)
	require.NoError(t, err)
	_, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "retained-custom-job", "House source", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, source, corpus)
	_, omittedSource, omittedCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "retained-custom-job-omission", "Omitted source", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "missingwort", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, omittedSource, omittedCorpus)
	var omittedRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, omittedCorpus.ID).Scan(&omittedRunID))
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text='Ein missingwort bleibt.',end_offset=23 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3`, owner.ID, omittedCorpus.ID, omittedRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_tokens SET surface='missingwort',raw_lemma='missingwort',canonical_lemma='missingwort',start_offset=4,end_offset=15 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, owner.ID, omittedCorpus.ID, omittedRunID)
	require.NoError(t, err)
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpus.ID).Scan(&runID))
	sentence := "Die Kinder besuchen heute das alte Haus."
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text=$4,end_offset=$5 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3`, owner.ID, corpus.ID, runID, sentence, len(sentence))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3`, owner.ID, corpus.ID, runID)
	require.NoError(t, err)
	tokens := []struct {
		surface, lemma, upos, dependency string
		head                             int
		morphology                       map[string]string
	}{
		{"Die", "die", "DET", "det", 1, nil}, {"Kinder", "Kind", "NOUN", "nsubj", 2, nil},
		{"besuchen", "besuchen", "VERB", "root", 2, map[string]string{"VerbForm": "Fin"}},
		{"heute", "heute", "ADV", "advmod", 2, nil}, {"das", "das", "DET", "det", 5, nil},
		{"alte", "alt", "ADJ", "amod", 5, nil}, {"Haus", "haus", "NOUN", "obj", 2, nil},
	}
	for ordinal, token := range tokens {
		features := token.morphology
		if features == nil {
			features = map[string]string{}
		}
		morphology, marshalErr := json.Marshal(features)
		require.NoError(t, marshalErr)
		start := strings.Index(sentence, token.surface)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,0,$4,$5,$6,$6,$7,$8,$9,$10,$11,$12)`, owner.ID, runID, corpus.ID, ordinal, token.surface, token.lemma, token.upos, token.dependency, token.head, morphology, start, start+len(token.surface))
		require.NoError(t, err)
	}
	require.NoError(t, store.SetVocabularyBrowseSelection(ctx, owner.ID, "de", "haus", "NOUN", true))
	deck, err := store.CreateCustomVocabularyDeck(ctx, owner.ID, "de", "Archived deck", "727f0e52-28ee-4cb2-b11e-3fe0a4b4eb02")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos) VALUES($1,$2,'de','missingwort','NOUN')`, owner.ID, deck.ID)
	require.NoError(t, err)

	provider := retainedCustomDeckTranslation{}
	presentation := cardexport.NewPresentation(nil)
	workers := river.NewWorkers()
	prepareddeck.AddCustomDeckPreparationWorker(workers, store, presentation, provider, true)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{prepareddeck.Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	service := prepareddeck.NewCustomDeckPreparationService(store, client, presentation, provider)
	fingerprint, err := service.EvidenceFingerprint(ctx, owner.ID, deck.ID)
	require.NoError(t, err)
	preparation, err := service.Submit(ctx, owner.ID, deck.ID, "ae910e1d-28f0-495d-967e-206cf4ae9df9", fingerprint)
	require.NoError(t, err)
	require.Equal(t, "queued", preparation.State)

	worker := &prepareddeck.CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: provider, Configured: true}
	require.NoError(t, worker.Work(ctx, &river.Job[prepareddeck.CustomDeckPreparationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1},
		Args:   prepareddeck.CustomDeckPreparationJobArgs{OwnerID: owner.ID, PreparationID: preparation.ID},
	}))
	completed, err := store.GetCustomDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, "complete_with_omissions", completed.State)
	assert.NotEmpty(t, completed.Artifact)
}

func TestVocabularyBrowseRendersCrossBookOrderFromEvidenceProjection(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "browse-render-rank", false)
	require.NoError(t, err)
	current, currentSource, currentCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "render-current", "Current", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "zeta", UPOS: "NOUN", OccurrenceCount: 1},
	})
	_, otherSource, otherCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "render-other", "Other", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "zeta", UPOS: "NOUN", OccurrenceCount: 3},
	})
	seedBrowseEvidenceTokens(t, ctx, store, currentSource, currentCorpus, []string{"alpha", "zeta"})
	seedBrowseEvidenceTokens(t, ctx, store, otherSource, otherCorpus, []string{"alpha", "zeta", "zeta", "zeta"})
	buildBrowseProjection(t, ctx, store, current.ID)
	var otherBookID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT book_id::text FROM current_analysis_identity WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, otherSource.ID).Scan(&otherBookID))
	buildBrowseProjection(t, ctx, store, otherBookID)
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{
		CurrentBookID: current.ID, Sort: "occurrences", IncludeAll: true, Page: 1,
	})
	require.NoError(t, err)
	page.CurrentBookID = current.ID
	page.ReadingBookID = current.ID
	require.Len(t, page.Rows, 2)
	require.Len(t, page.Books, 1)
	assert.Equal(t, "Current", page.Books[0].Title)
	assert.Equal(t, "zeta", page.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(1), page.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(4), page.Rows[0].AcrossBooksOccurrenceCount)
	var output strings.Builder
	require.NoError(t, VocabularyBrowsePageView(domain.User{}, "csrf", "de", page, "").Render(ctx, &output))
	html := output.String()
	assert.Less(t, strings.Index(html, ">Zeta</a>"), strings.Index(html, ">Alpha</a>"), "rendering preserves the evidence projection's cross-Book tie-break")
	assert.Contains(t, html, "In this Book: 1; Across analyzed books: 4")
	assert.Contains(t, html, "In this Book: 1; Across analyzed books: 2")

	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, otherBookID)
	require.NoError(t, err)
	updating, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: current.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	updating.CurrentBookID = current.ID
	updating.ReadingBookID = current.ID
	assert.True(t, updating.BrowseCountsUpdating)
	assert.Empty(t, updating.Rows, "a missing contributing projection must withhold the entire language result")
	var updatingHTML strings.Builder
	require.NoError(t, VocabularyBrowsePageView(domain.User{}, "csrf", "de", updating, "").Render(ctx, &updatingHTML))
	assert.Contains(t, updatingHTML.String(), "Updating Browse counts")
	assert.NotContains(t, updatingHTML.String(), "no eligible vocabulary identities", "updating must not be represented as an empty result")
}

func seedBrowseEvidenceTokens(t *testing.T, ctx context.Context, store *persistence.PostgresStore, source domain.SourceMaterial, corpus domain.Corpus, lemmas []string) {
	t.Helper()
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, source.OwnerID, corpus.ID).Scan(&runID))
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(source.SourceIdentifier, "migration-"))
	for ordinal, lemma := range lemmas {
		start := int64(ordinal * 20)
		end := start + int64(len(lemma))
		_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, source.OwnerID, runID, corpus.ID, unitID, ordinal, lemma, start, end)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,$4,0,$5,$5,$5,'NOUN','root',0,'{}',$6,$7)`, source.OwnerID, runID, corpus.ID, ordinal, lemma, start, end)
		require.NoError(t, err)
	}
}

func buildBrowseProjection(t *testing.T, ctx context.Context, store *persistence.PostgresStore, bookID string) {
	t.Helper()
	var ownerID, sourceID, runID, corpusID, language string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT cai.owner_id::text,cai.source_material_id::text,cai.analysis_run_id::text,cai.corpus_id::text,s.language FROM current_analysis_identity cai JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id WHERE cai.book_id=$1`, bookID).Scan(&ownerID, &sourceID, &runID, &corpusID, &language))
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(ctx, tx, ownerID, bookID, sourceID, runID, corpusID, language))
	require.NoError(t, tx.Commit(ctx))
}

func TestVocabularyBrowseServesOwnerScopedCurrentEvidenceOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "vocabulary-browse-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "browse-http-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "browse-http-bob", "bob-password", false)
	aliceBook, aliceSource, aliceCorpus, aliceDeck := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "browse-http-alice", "Alice German", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	aliceOtherBook, aliceOtherSource, aliceOtherCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "browse-http-alice-other", "Alice German Other", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	_, bobSource, bobCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, bob.ID, "browse-http-bob", "Bob German", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, aliceSource, aliceCorpus)
	seedBrowseHTTPToken(t, ctx, store, aliceOtherSource, aliceOtherCorpus)
	seedBrowseHTTPToken(t, ctx, store, bobSource, bobCorpus)
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, aliceBook.ID, domain.BookDispositionToRead))
	current, err := store.StartCurrentReading(ctx, alice.ID, "de", aliceBook.ID)
	require.NoError(t, err)
	beforeBookDeck, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Page: 1, IncludeAll: true})
	require.NoError(t, err)
	var firstDeckID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,'de','Earlier deck') RETURNING id::text`, alice.ID).Scan(&firstDeckID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,'de','haus','NOUN',$2,$3)`, alice.ID, firstDeckID, aliceOtherSource.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de','haus','NOUN',now())`, alice.ID, aliceDeck.ID)
	require.NoError(t, err)
	var preparationRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO deck_preparation_runs(owner_id,preparation_id,run_number,state,translation_state,external_translation_consent,external_translation_configured,manifest_schema_version,retry_policy_version,max_provider_attempts,max_batch_generations,batch_max_requests,batch_max_bytes,candidate_count,completed_count,translation_completed_at,completed_at,execution_mode) VALUES($1,$2,1,'completed','completed',false,false,1,1,1,1,1,1,2,2,now(),now(),'standard') RETURNING id::text`, alice.ID, aliceDeck.ID).Scan(&preparationRunID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_manifests(owner_id,preparation_id,run_id,schema_version,manifest_digest,deck_name,filename,selected_count,accepted_count,omitted_count) VALUES($1,$2,$3,1,repeat('b',64),'Alice German','alice.apkg',2,1,1)`, alice.ID, aliceDeck.ID, preparationRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_manifest_items(owner_id,preparation_id,run_id,ordinal,disposition,language,target_language,canonical_lemma,upos,source_sentence,tested_target,first_encounter,quality_score,quality_gdex_score,quality_reasons,render_payload,candidate_digest) VALUES($1,$2,$3,0,'accepted','de','en','haus','NOUN','Das Haus bleibt hell.','Haus',1,80,0.5,'{}','{}',repeat('a',64)),($1,$2,$3,1,'quality_omitted','de','en','wort02','NOUN','Das Wort bleibt hell.','Wort',2,20,0.2,'{}','{}',repeat('c',64))`, alice.ID, aliceDeck.ID, preparationRunID)
	require.NoError(t, err)
	afterBookDeck, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Page: 1, IncludeAll: true})
	require.NoError(t, err)
	assert.NotEqual(t, beforeBookDeck.CorpusRevision, afterBookDeck.CorpusRevision, "successful per-Book deck inclusion invalidates Browse paging")

	beforeDeck, err := store.GetDeckPreparation(ctx, alice.ID, aliceDeck.ID)
	require.NoError(t, err)
	var readingCountBefore int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1`, alice.ID).Scan(&readingCountBefore))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "browse-http-alice", "alice-password")
	defaultBrowse := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, defaultBrowse.Code)
	assert.NotContains(t, defaultBrowse.Body.String(), `term=haus&amp;upos=NOUN`)
	assert.Contains(t, defaultBrowse.Body.String(), "already Known, Reserved, or included in a prepared Book deck")
	assert.Contains(t, defaultBrowse.Body.String(), `name="all" value="1"`)
	response := perform(t, h, http.MethodGet, "/vocabulary?q=HA&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Alice German")
	assert.Contains(t, response.Body.String(), "haus")
	assert.Contains(t, response.Body.String(), "In a Book deck")
	assert.Contains(t, response.Body.String(), "Current reading")
	assert.Contains(t, response.Body.String(), "In this Book: 1; Across analyzed books: 2")
	assert.Contains(t, response.Body.String(), `href="/vocabulary/concordance?book=`+aliceBook.ID+`&amp;mode=effective&amp;term=haus&amp;upos=NOUN"`, "Browse hands exact identity and Current Book to Concordance")
	assert.NotContains(t, response.Body.String(), "Book count")
	assert.NotContains(t, response.Body.String(), "Correction")
	assert.NotContains(t, response.Body.String(), "Bob German")
	assert.NotContains(t, response.Body.String(), "Alice German Other")
	assert.NotContains(t, response.Body.String(), `action="/vocabulary/import"`)
	assert.Contains(t, response.Body.String(), `href="/vocabulary/import"`)
	handoff := perform(t, h, http.MethodGet, "/vocabulary/concordance?"+url.Values{
		"mode": {"effective"}, "term": {"haus"}, "upos": {"NOUN"}, "book": {aliceBook.ID},
	}.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, handoff.Code)
	assert.Contains(t, handoff.Body.String(), "Applied effective lemma + NOUN lookup for “haus”")
	assert.Contains(t, handoff.Body.String(), "Applied effective lemma + NOUN lookup")
	assert.Contains(t, handoff.Body.String(), "in Alice German with no grammar filter")
	assert.Contains(t, handoff.Body.String(), "Results 1–1")
	assert.NotContains(t, response.Body.String(), "Browse selection")
	assert.NotContains(t, response.Body.String(), "Custom deck")
	assert.NotContains(t, response.Body.String(), `name="lemma"`)

	afterDeck, err := store.GetDeckPreparation(ctx, alice.ID, aliceDeck.ID)
	require.NoError(t, err)
	assert.Equal(t, beforeDeck.State, afterDeck.State)
	assert.Equal(t, beforeDeck.Artifact, afterDeck.Artifact)
	var readingCountAfter int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1`, alice.ID).Scan(&readingCountAfter))
	assert.Equal(t, readingCountBefore, readingCountAfter, "Browse must not create or change Reading state")
	beforeKnown, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Page: 1, IncludeAll: true})
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "haus", "NOUN")
	require.NoError(t, err)
	afterKnown, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Page: 1, IncludeAll: true})
	require.NoError(t, err)
	assert.NotEqual(t, beforeKnown.CorpusRevision, afterKnown.CorpusRevision, "learner-state changes invalidate Browse paging")

	// Keep URL encoding on a normal form submission as well as the direct request.
	search := perform(t, h, http.MethodGet, "/vocabulary?"+url.Values{"q": {"haus"}, "page": {"1"}, "all": {"1"}}.Encode(), nil, cookies)
	assert.Equal(t, http.StatusOK, search.Code)
	filteredQuery := url.Values{
		"q":        {"ha"},
		"all":      {"1"},
		"book":     {aliceBook.ID, aliceOtherBook.ID},
		"pos":      {"VERB"},
		"known":    {"not-known"},
		"reserved": {"reserved"},
		"sort":     {"lemma"},
	}
	filtered := perform(t, h, http.MethodGet, "/vocabulary?"+filteredQuery.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, filtered.Code)
	assert.Contains(t, filtered.Body.String(), "In this Book: 1; Across analyzed books: 2")
	assert.NotContains(t, filtered.Body.String(), "Alice German Other")
	assert.Contains(t, filtered.Body.String(), "haus")
	assert.NotContains(t, filtered.Body.String(), `name="pos"`)
	assert.NotContains(t, filtered.Body.String(), `name="known"`)
	assert.NotContains(t, filtered.Body.String(), `name="reserved"`)
	assert.Contains(t, filtered.Body.String(), "In this Book: 1; Across analyzed books: 2", "the page labels local and cross-Book counts separately")
	assert.NotContains(t, filtered.Body.String(), "heim")

	selectedBooks := []string{aliceBook.ID, aliceOtherBook.ID}
	scoped, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{BookIDs: selectedBooks, UPOS: []string{"NOUN"}, KnownFilter: "known", ReservedFilter: "not-reserved", Sort: "books", Page: 1, IncludeAll: true})
	require.NoError(t, err)
	require.Len(t, scoped.Rows, 1)
	assert.Equal(t, "haus", scoped.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(2), scoped.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(2), scoped.Rows[0].BookCount)
	assert.Equal(t, int64(1), scoped.InventoryTotal)
	assert.Equal(t, int64(1), scoped.ScopedInventoryTotal)
	assert.ElementsMatch(t, selectedBooks, scoped.SelectedBooks)
	assert.Equal(t, "books", scoped.Sort)
	unfilteredState, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{BookIDs: selectedBooks, UPOS: []string{"NOUN"}, ReservedFilter: "not-reserved", Sort: "books", Page: 1, IncludeAll: true})
	require.NoError(t, err)
	require.Len(t, unfilteredState.Rows, 1)
	assert.Equal(t, unfilteredState.Rows[0].OccurrenceCount, scoped.Rows[0].OccurrenceCount, "Known filters must not change effective occurrence counts")
	assert.Equal(t, unfilteredState.Rows[0].BookCount, scoped.Rows[0].BookCount, "Known filters must not change distinct-Book counts")

	otherOwnerScope, err := store.ListVocabularyBrowsePage(ctx, bob.ID, "de", domain.VocabularyBrowseQuery{BookIDs: []string{aliceBook.ID}, Page: 1})
	require.NoError(t, err)
	assert.Empty(t, otherOwnerScope.Rows, "a Book ID from another owner must not reveal its identities")

	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, aliceSource.OwnerID, aliceCorpus.ID).Scan(&runID))
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(aliceSource.SourceIdentifier, "migration-"))
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,0,4,'heim','de','1',false)`, alice.ID, aliceBook.ID, aliceCorpus.ID, runID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, alice.ID, aliceBook.ID)
	require.NoError(t, err)
	correctedHaus := perform(t, h, http.MethodGet, "/vocabulary?q=haus&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, correctedHaus.Code)
	assert.Contains(t, correctedHaus.Body.String(), "Updating Browse counts")
	assert.NotContains(t, correctedHaus.Body.String(), `term=haus`)
	buildBrowseProjection(t, ctx, store, aliceBook.ID)
	correctedHeim := perform(t, h, http.MethodGet, "/vocabulary?q=heim&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, correctedHeim.Code)
	assert.Contains(t, correctedHeim.Body.String(), "In this Book: 1; Across analyzed books: 1", "the corrected occurrence moves to the effective identity in the Current reading only")
	correctedEvidence, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Prefix: "heim", IncludeAll: true, Page: 1})
	require.NoError(t, err)
	require.Len(t, correctedEvidence.Rows, 1)
	assert.Equal(t, int64(1), correctedEvidence.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(1), correctedEvidence.Rows[0].AcrossBooksOccurrenceCount, "a singleton corrected occurrence remains eligible")
	for ordinal := int64(1); ordinal <= 26; ordinal++ {
		lemma := fmt.Sprintf("wort%02d", ordinal-1)
		start := ordinal * 10
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, alice.ID, runID, aliceCorpus.ID, unitID, ordinal, lemma, start, start+int64(len(lemma)))
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,$4,0,$5,$5,$5,'NOUN','root',0,'{}',$6,$7)`, alice.ID, runID, aliceCorpus.ID, ordinal, lemma, start, start+int64(len(lemma)))
		require.NoError(t, err)
	}
	buildBrowseProjection(t, ctx, store, aliceBook.ID)
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,'de','wort00','')`, alice.ID)
	require.NoError(t, err)
	var activeSnapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT ps.id::text FROM primary_goal_snapshots ps JOIN primary_goals pg ON pg.owner_id=ps.owner_id AND pg.book_id=ps.book_id WHERE ps.owner_id=$1 AND ps.book_id=$2 AND ps.released_at IS NULL`, alice.ID, aliceBook.ID).Scan(&activeSnapshotID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goal_snapshot_vocabulary(owner_id,snapshot_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'fixture','de','wort00','NOUN',1,'[]','[]','{}')`, alice.ID, activeSnapshotID)
	require.NoError(t, err)
	omittedDefault := perform(t, h, http.MethodGet, "/vocabulary?q=wort02", nil, cookies)
	require.Equal(t, http.StatusOK, omittedDefault.Code)
	assert.Contains(t, omittedDefault.Body.String(), `term=wort02&amp;upos=NOUN`, "quality-omitted identities are not treated as Book-deck inclusions")
	assert.NotContains(t, omittedDefault.Body.String(), "In a Book deck")
	lemmaWideDefault := perform(t, h, http.MethodGet, "/vocabulary?q=wort00", nil, cookies)
	require.Equal(t, http.StatusOK, lemmaWideDefault.Code)
	assert.NotContains(t, lemmaWideDefault.Body.String(), `term=wort00&amp;upos=NOUN`)
	lemmaWideIncluded := perform(t, h, http.MethodGet, "/vocabulary?q=wort00&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, lemmaWideIncluded.Code)
	assert.Contains(t, lemmaWideIncluded.Body.String(), "Known · Reserved", "lemma-wide imported Known and Reserved state must both be represented")
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,30,'Zebra zebra',300,311)`, alice.ID, runID, aliceCorpus.ID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,30,0,'Zebra','zebra','zebra','NOUN','root',0,'{}',300,305),($1,'de',$2,$3,30,1,'zebra','zebra','zebra','NOUN','conj',0,'{}',306,311)`, alice.ID, runID, aliceCorpus.ID)
	require.NoError(t, err)
	buildBrowseProjection(t, ctx, store, aliceBook.ID)
	firstPage := perform(t, h, http.MethodGet, "/vocabulary?sort=lemma&page=1&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, firstPage.Code)
	assert.Contains(t, firstPage.Body.String(), "Page 1 of 2")
	assert.Contains(t, firstPage.Body.String(), "In this Book: 2; Across analyzed books: 2")
	assert.Contains(t, firstPage.Body.String(), ">Zebra</a>")
	assert.Contains(t, firstPage.Body.String(), ">Heim</a>")
	assert.Less(t, strings.Index(firstPage.Body.String(), ">Zebra</a>"), strings.Index(firstPage.Body.String(), ">Heim</a>"), "frequency outranks the legacy lemma-sort parameter")
	assert.Less(t, strings.Index(firstPage.Body.String(), ">Heim</a>"), strings.Index(firstPage.Body.String(), ">Wort00</a>"), "equal counts break ties by canonical lemma")
	assert.Contains(t, firstPage.Body.String(), "wort00")
	assert.NotContains(t, firstPage.Body.String(), "wort24")
	assert.Contains(t, firstPage.Body.String(), `href="/vocabulary?all=1&amp;page=2&amp;reading=`+aliceBook.ID+`&amp;rev=`)
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,300,305,NULL,NULL,NULL,true)`, alice.ID, aliceBook.ID, aliceCorpus.ID, runID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, alice.ID, aliceBook.ID)
	require.NoError(t, err)
	excludedBrowse := perform(t, h, http.MethodGet, "/vocabulary?q=zebra&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, excludedBrowse.Code)
	assert.Contains(t, excludedBrowse.Body.String(), "Updating Browse counts")
	buildBrowseProjection(t, ctx, store, aliceBook.ID)
	excludedBrowse = perform(t, h, http.MethodGet, "/vocabulary?q=zebra&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, excludedBrowse.Code)
	assert.Contains(t, excludedBrowse.Body.String(), "zebra")
	assert.Contains(t, excludedBrowse.Body.String(), "In this Book: 1; Across analyzed books: 1", "excluding one of two occurrences updates both effective counts over HTTP")
	revisionPage, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Sort: "lemma", Page: 1})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,260,266,'wort25changed','de','1',false)`, alice.ID, aliceBook.ID, aliceCorpus.ID, runID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, alice.ID, aliceBook.ID)
	require.NoError(t, err)
	buildBrowseProjection(t, ctx, store, aliceBook.ID)
	changedPage := perform(t, h, http.MethodGet, "/vocabulary?sort=lemma&page=2&all=1&rev="+revisionPage.CorpusRevision, nil, cookies)
	assert.Equal(t, http.StatusConflict, changedPage.Code)
	assert.Contains(t, changedPage.Body.String(), "Current evidence changed")
	assert.Contains(t, changedPage.Body.String(), "Restart in the Current reading")
	secondPage := perform(t, h, http.MethodGet, "/vocabulary?sort=lemma&page=2&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, secondPage.Code)
	assert.Contains(t, secondPage.Body.String(), "Page 2 of 2")
	assert.Contains(t, secondPage.Body.String(), ">Wort24</a>")
	assert.Contains(t, secondPage.Body.String(), ">Wort25changed</a>")
	assert.NotContains(t, secondPage.Body.String(), ">Wort00</a>")
	prefixFirst := perform(t, h, http.MethodGet, "/vocabulary?q=wort&page=1&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, prefixFirst.Code)
	assert.Contains(t, prefixFirst.Body.String(), "Page 1 of 2")
	assert.Contains(t, prefixFirst.Body.String(), ">Wort00</a>")
	assert.Contains(t, prefixFirst.Body.String(), ">Wort24</a>")
	assert.NotContains(t, prefixFirst.Body.String(), ">Heim</a>")
	assert.Contains(t, prefixFirst.Body.String(), `href="/vocabulary?all=1&amp;page=2&amp;q=wort&amp;reading=`+aliceBook.ID+`&amp;rev=`)
	prefixSecond := perform(t, h, http.MethodGet, "/vocabulary?q=wort&page=2&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, prefixSecond.Code)
	assert.Contains(t, prefixSecond.Body.String(), ">Wort25changed</a>")
	assert.NotContains(t, prefixSecond.Body.String(), ">Wort24</a>")

	// The same lemma's POS identities have a stable secondary ordering.
	for ordinal, pos := range map[int64]string{31: "VERB", 32: "NOUN"} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,'Tie lemma',310,319)`, alice.ID, runID, aliceCorpus.ID, unitID, ordinal)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,$4,0,'tie','tie','tie',$5,'root',0,'{}',310,313)`, alice.ID, runID, aliceCorpus.ID, ordinal, pos)
		require.NoError(t, err)
	}
	buildBrowseProjection(t, ctx, store, aliceBook.ID)
	tieRows, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Prefix: "tie", Page: 1})
	require.NoError(t, err)
	require.Len(t, tieRows.Rows, 2)
	assert.Equal(t, "NOUN", tieRows.Rows[0].UPOS)
	assert.Equal(t, "VERB", tieRows.Rows[1].UPOS)
	tieBrowse := perform(t, h, http.MethodGet, "/vocabulary?q=tie&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, tieBrowse.Code)
	assert.Less(t, strings.Index(tieBrowse.Body.String(), ">NOUN</code>"), strings.Index(tieBrowse.Body.String(), ">VERB</code>"), "HTTP rows break same-lemma ties by POS")

	// Current-reading scope cannot be widened by a legacy Book parameter.
	otherBookParameter := perform(t, h, http.MethodGet, "/vocabulary?book="+url.QueryEscape(aliceOtherBook.ID), nil, cookies)
	require.Equal(t, http.StatusOK, otherBookParameter.Code)
	assert.NotContains(t, otherBookParameter.Body.String(), "Alice German Other")

	// Switching the Current reading changes Browse evidence scope.
	beforeSwitch, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceBook.ID, Page: 1})
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, aliceOtherBook.ID, domain.BookDispositionToRead))
	_, err = store.SwitchCurrentReading(ctx, alice.ID, "de", aliceOtherBook.ID, aliceBook.ID, current.SnapshotID)
	require.NoError(t, err)
	staleSwitchPage := perform(t, h, http.MethodGet, "/vocabulary?page=2&reading="+url.QueryEscape(aliceBook.ID)+"&rev="+url.QueryEscape(beforeSwitch.CorpusRevision), nil, cookies)
	assert.Equal(t, http.StatusConflict, staleSwitchPage.Code, "a Book switch invalidates pagination under the old scope revision")
	assert.Contains(t, staleSwitchPage.Body.String(), "Restart in the Current reading")
	switched := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, switched.Code)
	assert.Contains(t, switched.Body.String(), "Current reading: Alice German Other")
	assert.NotContains(t, switched.Body.String(), "Browse selection")

	// Browse must not silently fall back to the former cross-Book inventory.
	current, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.NoError(t, store.StopCurrentReading(ctx, alice.ID, "de", aliceOtherBook.ID, current.SnapshotID))
	noReading := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, noReading.Code)
	assert.Contains(t, noReading.Body.String(), "No Current reading")
	assert.Contains(t, noReading.Body.String(), `href="/reading"`)
	assert.NotContains(t, noReading.Body.String(), "Alice German Other")
	assert.NotContains(t, noReading.Body.String(), "Browse selection")

	// A Book with no current completed analysis gets Book-specific recovery and
	// never falls back to stale analysis or another Book's evidence.
	_, err = store.StartCurrentReading(ctx, alice.ID, "de", aliceOtherBook.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, aliceOtherBook.ID)
	require.NoError(t, err)
	stale := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, stale.Code)
	assert.Contains(t, stale.Body.String(), "Current reading: Alice German Other")
	assert.Contains(t, stale.Body.String(), "no current completed analysis")
	assert.NotContains(t, stale.Body.String(), "haus")
	assert.NotContains(t, stale.Body.String(), "Browse selection")
	current, err = store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.NoError(t, store.StopCurrentReading(ctx, alice.ID, "de", aliceOtherBook.ID, current.SnapshotID))
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_tag='it',title='Alice Italian Other' WHERE owner_id=$1 AND id=$2`, alice.ID, aliceOtherBook.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE source_materials SET language='it' WHERE owner_id=$1 AND id=$2`, alice.ID, aliceOtherSource.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_tokens SET language='it' WHERE owner_id=$1 AND corpus_id=$2`, alice.ID, aliceOtherCorpus.ID)
	require.NoError(t, err)
	var italianRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, aliceOtherCorpus.ID).Scan(&italianRunID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, alice.ID, aliceOtherBook.ID, aliceOtherSource.ID, italianRunID)
	require.NoError(t, err)
	buildBrowseProjection(t, ctx, store, aliceOtherBook.ID)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "it"))
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, aliceOtherBook.ID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReading(ctx, alice.ID, "it", aliceOtherBook.ID)
	require.NoError(t, err)
	italianBrowse := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, italianBrowse.Code)
	assert.Contains(t, italianBrowse.Body.String(), "active study language <code>it</code>")
	assert.Contains(t, italianBrowse.Body.String(), "Current reading: Alice Italian Other")
	assert.Contains(t, italianBrowse.Body.String(), "haus")
	assert.Contains(t, italianBrowse.Body.String(), "In this Book: 1; Across analyzed books: 1", "same-lemma German Books do not contribute to the Italian total")
	assert.NotContains(t, italianBrowse.Body.String(), "Alice German")
	assert.NotContains(t, italianBrowse.Body.String(), "Browse selection")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))
	germanBrowse := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, germanBrowse.Code)
	assert.NotContains(t, germanBrowse.Body.String(), "Browse selection")
}

func TestVocabularyBrowseServesReadyProjectionOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "vocabulary-browse-projection-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "browse-projection-http", "projection-password", false)
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "browse-projection-http", "Projected German", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 2}})
	seedBrowseHTTPToken(t, ctx, store, source, corpus)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "browse-projection-http", "projection-password")
	response := perform(t, h, http.MethodGet, "/vocabulary?q=ha&all=1", nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "haus")
	assert.Contains(t, response.Body.String(), "In this Book: 1")
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	updating := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, updating.Code)
	assert.Contains(t, updating.Body.String(), "Updating Browse counts")
	assert.NotContains(t, updating.Body.String(), `href="/vocabulary/selection"`)
	assert.NotContains(t, updating.Body.String(), "no eligible vocabulary identities")
	require.NoError(t, analysis.MigrateRiver(ctx, store.Pool()))
	riverClient, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{})
	require.NoError(t, err)
	failedJob, err := riverClient.Insert(ctx, analysis.BrowseCountsRebuildArgs{OwnerID: owner.ID, BookID: book.ID, RunID: runID}, &river.InsertOpts{Queue: analysis.Queue, MaxAttempts: 1})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE river_job SET state='discarded',finalized_at=now() WHERE id=$1`, failedJob.Job.ID)
	require.NoError(t, err)
	unavailable := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, unavailable.Code)
	assert.Contains(t, unavailable.Body.String(), "Browse counts unavailable")
	assert.Contains(t, unavailable.Body.String(), "restart Mouseion")
	assert.NotContains(t, unavailable.Body.String(), `href="/vocabulary/selection"`)
}

func TestVocabularyConcordanceServesExactModesAndOccurrenceDecisionsOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "concordance-http-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "concordance-http-bob", "bob-password", false)
	aliceBook, aliceSource, aliceCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "concordance-http-alice", "Alice Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	secondAliceBook, secondAliceSource, secondAliceCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "concordance-http-alice-second", "Alice Second Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	_, bobSource, bobCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, bob.ID, "concordance-http-bob", "Bob Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	staleBook, staleSource, staleCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "concordance-http-stale", "Stale Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, aliceSource, aliceCorpus)
	seedBrowseHTTPToken(t, ctx, store, secondAliceSource, secondAliceCorpus)
	seedBrowseHTTPDependencySentence(t, ctx, store, secondAliceSource, secondAliceCorpus)
	seedBrowseHTTPToken(t, ctx, store, bobSource, bobCorpus)
	seedBrowseHTTPToken(t, ctx, store, staleSource, staleCorpus)
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, staleBook.ID)
	require.NoError(t, err)

	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, aliceCorpus.ID).Scan(&runID))
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(aliceSource.SourceIdentifier, "migration-"))
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) SELECT $1,b.id,$2,$3,$4,0,4,'heim','de','1',false FROM books b WHERE b.owner_id=$1 AND b.title='Alice Concordance Book'`, alice.ID, aliceCorpus.ID, runID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,1,'Haus',10,14)`, alice.ID, runID, aliceCorpus.ID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,1,0,'Haus','haus','haus','NOUN','root',0,'{}',10,14)`, alice.ID, runID, aliceCorpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) SELECT $1,b.id,$2,$3,$4,10,14,NULL,NULL,NULL,true FROM books b WHERE b.owner_id=$1 AND b.title='Alice Concordance Book'`, alice.ID, aliceCorpus.ID, runID, unitID)
	require.NoError(t, err)

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "concordance-http-alice", "alice-password")
	corrected := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=heim&upos=NOUN&book="+aliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, corrected.Code)
	assert.Contains(t, corrected.Body.String(), "Alice Concordance Book")
	assert.Contains(t, corrected.Body.String(), "Analyzer lemma (evidence)")
	assert.NotContains(t, corrected.Body.String(), "Bob Concordance Book")
	correctedResults := strings.SplitN(corrected.Body.String(), `<section id="concordance-results"`, 2)[1]
	assert.NotContains(t, correctedResults, "Stale Concordance Book")
	assert.NotContains(t, correctedResults, "corrected for this occurrence")
	assert.NotContains(t, correctedResults, "excluded from effective vocabulary")
	invalidPartial := performWithHeader(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=Haus&book="+aliceBook.ID, nil, cookies, "HX-Request-Type", "partial")
	assert.Equal(t, http.StatusBadRequest, invalidPartial.Code)
	assert.Contains(t, invalidPartial.Body.String(), `id="concordance-recovery"`)
	assert.Contains(t, invalidPartial.Body.String(), "Part of speech is required")
	assert.Contains(t, invalidPartial.Body.String(), "Retry Concordance lookup")
	assert.Contains(t, invalidPartial.Body.String(), "book="+aliceBook.ID)
	assert.NotContains(t, invalidPartial.Body.String(), `id="concordance-results"`)
	excludedEffective := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN&book="+aliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, excludedEffective.Code)
	assert.Contains(t, excludedEffective.Body.String(), "No current analyzed occurrences match this exact lookup.")

	surface := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&book="+aliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, surface.Code)
	surfaceResults := strings.SplitN(surface.Body.String(), `<section id="concordance-results"`, 2)[1]
	assert.NotContains(t, surfaceResults, "excluded from effective vocabulary")
	assert.NotContains(t, surfaceResults, "retained as analyzer evidence")

	analyzer := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=analyzer&term=haus&upos=NOUN&book="+aliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, analyzer.Code)
	assert.Contains(t, analyzer.Body.String(), "Applied analyzer lemma evidence")
	assert.Contains(t, analyzer.Body.String(), "Haus")
	multiBook := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&book="+aliceBook.ID+"&book="+secondAliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, multiBook.Code)
	multiBookResults := strings.SplitN(multiBook.Body.String(), `<section id="concordance-results"`, 2)[1]
	assert.Contains(t, multiBookResults, "Alice Concordance Book")
	assert.Contains(t, multiBookResults, "Alice Second Concordance Book")
	assert.Equal(t, 4, strings.Count(multiBookResults, `class="concordance-row"`))
	booksPreserveGrammar := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN&book="+aliceBook.ID+"&book="+secondAliceBook.ID+"&grammar=own&relation=root", nil, cookies)
	require.Equal(t, http.StatusOK, booksPreserveGrammar.Code)
	assert.Contains(t, booksPreserveGrammar.Body.String(), "Alice Concordance Book, Alice Second Concordance Book")
	assert.Contains(t, booksPreserveGrammar.Body.String(), "own relation: root")
	assert.Contains(t, booksPreserveGrammar.Body.String(), `name="grammar" value="own"`)
	assert.Contains(t, booksPreserveGrammar.Body.String(), `name="book" value="`+aliceBook.ID+`"`)
	ownRelation := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN&book="+secondAliceBook.ID+"&grammar=own&relation=nsubj", nil, cookies)
	require.Equal(t, http.StatusOK, ownRelation.Code)
	assert.Contains(t, ownRelation.Body.String(), "own relation: nsubj")
	assert.Equal(t, 1, strings.Count(ownRelation.Body.String(), `class="concordance-row"`))
	governorRelation := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=das&upos=DET&book="+secondAliceBook.ID+"&grammar=governor&relation=nsubj", nil, cookies)
	require.Equal(t, http.StatusOK, governorRelation.Code)
	assert.Contains(t, governorRelation.Body.String(), "governor dependents, relation: nsubj")
	assert.Equal(t, 1, strings.Count(governorRelation.Body.String(), `class="concordance-row"`))
	governorResults := strings.SplitN(governorRelation.Body.String(), `<section id="concordance-results"`, 2)[1]
	assert.Contains(t, governorResults, "Haus")
	assert.Contains(t, governorResults, "Alice Second Concordance Book")
	var aliceRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, aliceCorpus.ID).Scan(&aliceRun))
	studyPage := perform(t, h, http.MethodGet, "/vocabulary/concordance/sentence?book="+aliceBook.ID+"&run="+aliceRun+"&corpus="+aliceCorpus.ID+"&unit="+domain.EPUBUnitID(0, strings.TrimPrefix(aliceSource.SourceIdentifier, "migration-"))+"&sentence=0&target=0", nil, cookies)
	require.Equal(t, http.StatusOK, studyPage.Code)
	assert.Contains(t, studyPage.Body.String(), "Analyzer-attributed tokens")
	assert.Contains(t, studyPage.Body.String(), "Alice Concordance Book")
	assert.Contains(t, studyPage.Body.String(), "Corrected for this occurrence.")
	excludedStudyPage := perform(t, h, http.MethodGet, "/vocabulary/concordance/sentence?book="+aliceBook.ID+"&run="+aliceRun+"&corpus="+aliceCorpus.ID+"&unit="+domain.EPUBUnitID(0, strings.TrimPrefix(aliceSource.SourceIdentifier, "migration-"))+"&sentence=1&target=0&target_surface=Haus", nil, cookies)
	require.Equal(t, http.StatusOK, excludedStudyPage.Code)
	assert.Contains(t, excludedStudyPage.Body.String(), "Excluded from effective vocabulary; retained as syntax evidence.")

	for ordinal := int64(2); ordinal < 26; ordinal++ {
		start := 20 + ordinal*5
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,'Haus',$6,$7)`, alice.ID, runID, aliceCorpus.ID, unitID, ordinal, start, start+4)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,$4,0,'Haus','haus','haus','NOUN','root',0,'{}',$5,$6)`, alice.ID, runID, aliceCorpus.ID, ordinal, start, start+4)
		require.NoError(t, err)
	}
	firstPage := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, cookies)
	require.Equal(t, http.StatusOK, firstPage.Code)
	assert.Contains(t, firstPage.Body.String(), "Results 1–25")
	assert.Contains(t, firstPage.Body.String(), `href="/vocabulary/concordance?mode=surface&amp;page=2&amp;rev=`)
	pageRevision, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{Mode: "surface", Term: "Haus", Page: 1})
	require.NoError(t, err)
	require.NotEmpty(t, pageRevision.Revision)
	_, err = store.Pool().Exec(ctx, `UPDATE occurrence_lemma_corrections SET canonical_lemma='heim-changed' WHERE owner_id=$1 AND book_id=$2 AND canonical_lemma='heim'`, alice.ID, aliceBook.ID)
	require.NoError(t, err)
	stalePage := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&page=2&rev="+pageRevision.Revision, nil, cookies)
	assert.Equal(t, http.StatusConflict, stalePage.Code)
	assert.Contains(t, stalePage.Body.String(), "Current evidence changed")
	assert.Contains(t, stalePage.Body.String(), `href="/vocabulary/concordance?mode=surface&amp;page=1&amp;term=Haus">Restart from results`)
	partialStalePage := performWithHeader(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&page=2&rev="+pageRevision.Revision, nil, cookies, "HX-Request-Type", "partial")
	assert.Equal(t, http.StatusConflict, partialStalePage.Code)
	assert.Contains(t, partialStalePage.Body.String(), `id="concordance-recovery"`)
	assert.Contains(t, partialStalePage.Body.String(), "Restart from results")
	assert.NotContains(t, partialStalePage.Body.String(), "<html")
	assert.NotContains(t, partialStalePage.Body.String(), `id="concordance-results"`)
	secondPage := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&page=2", nil, cookies)
	require.Equal(t, http.StatusOK, secondPage.Code)
	assert.Contains(t, secondPage.Body.String(), "Results 26–28")
	assert.Contains(t, secondPage.Body.String(), `href="/vocabulary/concordance?mode=surface&amp;page=1&amp;rev=`)
	focusedQuery := url.Values{"mode": {"surface"}, "term": {"Haus"}, "book": {aliceBook.ID}, "grammar": {"own"}, "relation": {"root"}, "page": {"2"}, "focus": {"occurrence-" + aliceBook.ID + "-25-0"}}
	focusedPage := perform(t, h, http.MethodGet, "/vocabulary/concordance?"+focusedQuery.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, focusedPage.Code)
	assert.Contains(t, focusedPage.Body.String(), `id="occurrence-`+aliceBook.ID+`-25-0" tabindex="-1" autofocus`)
	for _, encodedTarget := range []string{
		`return=%2Fvocabulary%2Fconcordance%3F`, `book%3D` + aliceBook.ID,
		`focus%3Doccurrence-` + aliceBook.ID + `-25-0`, `grammar%3Down`, `mode%3Dsurface`,
		`page%3D2`, `relation%3Droot`, `term%3DHaus`, `rev%3D`,
	} {
		assert.Contains(t, focusedPage.Body.String(), encodedTarget)
	}
	focusedQuery.Set("focus", "occurrence-no-longer-present")
	focusedPage = perform(t, h, http.MethodGet, "/vocabulary/concordance?"+focusedQuery.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, focusedPage.Code)
	assert.Contains(t, focusedPage.Body.String(), `<h2 id="concordance-summary" tabindex="-1" autofocus>Current results</h2>`)
	outOfRangePage := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&page=99", nil, cookies)
	require.Equal(t, http.StatusOK, outOfRangePage.Code)
	assert.Contains(t, outOfRangePage.Body.String(), "No results are available on this page")
	assert.NotContains(t, outOfRangePage.Body.String(), "No current analyzed occurrences match this exact lookup.")
	unchangedEffective := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN", nil, cookies)
	require.Equal(t, http.StatusOK, unchangedEffective.Code)
	assert.Equal(t, 25, strings.Count(unchangedEffective.Body.String(), `class="concordance-row"`))
	assert.NotContains(t, unchangedEffective.Body.String(), "corrected for this occurrence")
	assert.NotContains(t, unchangedEffective.Body.String(), "excluded from effective vocabulary")

	noLeak := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, func() []*http.Cookie {
		cookies, _ := loginCookies(t, h, "concordance-http-bob", "bob-password")
		return cookies
	}())
	require.Equal(t, http.StatusOK, noLeak.Code)
	assert.Contains(t, noLeak.Body.String(), "Bob Concordance Book")
	assert.NotContains(t, noLeak.Body.String(), "Alice Concordance Book")
	_, err = store.Pool().Exec(ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, aliceCorpus.ID, runID)
	require.NoError(t, err)
	parseGapPage := perform(t, h, http.MethodGet, "/vocabulary/concordance/sentence?book="+aliceBook.ID+"&run="+runID+"&corpus="+aliceCorpus.ID+"&unit="+unitID+"&sentence=0&target=0&target_surface=Haus", nil, cookies)
	require.Equal(t, http.StatusOK, parseGapPage.Code)
	assert.Contains(t, parseGapPage.Body.String(), "Identified target:")
	assert.Contains(t, parseGapPage.Body.String(), "Haus (syntax evidence unavailable)")
	assert.Contains(t, parseGapPage.Body.String(), "Syntax evidence is unavailable")
}

func TestVocabularyConcordanceTimesOutWithRetryInsteadOfReportingNoMatches(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-timeout-secret-012345")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	account := createAccount(t, ctx, store, "concordance-timeout", "timeout-password", false)
	seedMigrationAnalyzedBook(t, ctx, store, account.ID, "concordance-timeout", "Timeout Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour, InteractiveReadTimeout: 500 * time.Millisecond})
	cookies, _ := loginCookies(t, h, "concordance-timeout", "timeout-password")
	lockConn, err := store.Pool().Acquire(ctx)
	require.NoError(t, err)
	defer lockConn.Release()
	lockTx, err := lockConn.Begin(ctx)
	require.NoError(t, err)
	_, err = lockTx.Exec(ctx, `LOCK TABLE corpus_tokens IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)

	response := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, cookies)
	require.Equal(t, http.StatusGatewayTimeout, response.Code)
	assert.Contains(t, response.Body.String(), "Concordance server timed out")
	assert.Contains(t, response.Body.String(), "Retry Concordance lookup")
	assert.NotContains(t, response.Body.String(), "No current analyzed occurrences match")
	partialResponse := performWithHeader(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, cookies, "HX-Request-Type", "partial")
	require.Equal(t, http.StatusGatewayTimeout, partialResponse.Code)
	assert.Contains(t, partialResponse.Body.String(), `id="concordance-recovery"`)
	assert.Contains(t, partialResponse.Body.String(), "Concordance server timed out")
	assert.Contains(t, partialResponse.Body.String(), "Retry Concordance lookup")
	assert.NotContains(t, partialResponse.Body.String(), "<html")
	assert.NotContains(t, partialResponse.Body.String(), `id="concordance-results"`)
	require.NoError(t, lockTx.Rollback(ctx))
}

func seedBrowseHTTPToken(t *testing.T, ctx context.Context, store *persistence.PostgresStore, source domain.SourceMaterial, corpus domain.Corpus) {
	t.Helper()
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(source.SourceIdentifier, "migration-"))
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, source.OwnerID, corpus.ID).Scan(&runID))
	_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,0,'Haus',0,4)`, source.OwnerID, runID, corpus.ID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,0,0,'Haus','haus','haus','NOUN','root',0,'{}',0,4)`, source.OwnerID, runID, corpus.ID)
	require.NoError(t, err)
	var bookID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT book_id::text FROM current_analysis_identity WHERE owner_id=$1 AND source_material_id=$2`, source.OwnerID, source.ID).Scan(&bookID))
	buildBrowseProjection(t, ctx, store, bookID)
}

func seedBrowseHTTPDependencySentence(t *testing.T, ctx context.Context, store *persistence.PostgresStore, source domain.SourceMaterial, corpus domain.Corpus) {
	t.Helper()
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, source.OwnerID, corpus.ID).Scan(&runID))
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(source.SourceIdentifier, "migration-"))
	_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,1,'Das Haus',20,28)`, source.OwnerID, runID, corpus.ID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,1,0,'Das','das','das','DET','root',0,'{}',20,23),($1,'de',$2,$3,1,1,'Haus','haus','haus','NOUN','nsubj',0,'{}',24,28)`, source.OwnerID, runID, corpus.ID)
	require.NoError(t, err)
}
