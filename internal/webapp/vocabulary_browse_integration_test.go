//go:build integration

package webapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

type customDeckFixtureTranslation struct {
	requests []enrichment.TranslationRequest
}

type customDeckFailTranslation struct{}

func (*customDeckFailTranslation) Name() string    { return "custom-deck-fixture" }
func (*customDeckFailTranslation) Version() string { return "1" }
func (*customDeckFailTranslation) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	return enrichment.TranslationResponse{}, errors.New("temporary translation transport failure")
}

type customDeckCancelTranslation struct {
	cancel func() error
	calls  int
}

type customDeckMalformedTranslation struct{}

func (*customDeckMalformedTranslation) Name() string    { return "custom-deck-fixture" }
func (*customDeckMalformedTranslation) Version() string { return "1" }
func (*customDeckMalformedTranslation) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	return enrichment.TranslationResponse{}, nil
}

func (*customDeckCancelTranslation) Name() string    { return "custom-deck-fixture" }
func (*customDeckCancelTranslation) Version() string { return "1" }
func (p *customDeckCancelTranslation) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.calls++
	if err := p.cancel(); err != nil {
		return enrichment.TranslationResponse{}, err
	}
	return enrichment.TranslationResponse{Translation: "tree", Gloss: "a plant", ContextOnly: true, SentenceTranslation: "The children see the tree in the garden.", SentenceTranslationTargets: []string{"tree"}}, nil
}

func (*customDeckFixtureTranslation) Name() string    { return "custom-deck-fixture" }
func (*customDeckFixtureTranslation) Version() string { return "1" }
func (p *customDeckFixtureTranslation) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.requests = append(p.requests, request)
	if request.CanonicalLemma == "baum" {
		return enrichment.TranslationResponse{UnresolvedReason: "The context does not distinguish this meaning."}, nil
	}
	return enrichment.TranslationResponse{Translation: "house", Gloss: "a building", ContextOnly: true, SentenceTranslation: "In the morning, the children visit the house and speak with their neighbors.", SentenceTranslationTargets: []string{"house"}}, nil
}

func TestCustomDeckPreparationDownloadsOwnerScopedAPKGOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "custom-deck-preparation-http-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "custom-prep-alice", "alice-password", false)
	createAccount(t, ctx, store, "custom-prep-bob", "bob-password", false)
	book, source, corpus, bookDeck := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-prep", "Private source title", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	missingBook, missingSource, missingCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-prep-missing", "Missing German source", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "missingwort", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, missingSource, missingCorpus)
	var missingRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, missingCorpus.ID).Scan(&missingRunID))
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text='Ein missingwort bleibt.',end_offset=23 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3`, alice.ID, missingCorpus.ID, missingRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_tokens SET surface='missingwort',raw_lemma='missingwort',canonical_lemma='missingwort',start_offset=4,end_offset=15 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, missingCorpus.ID, missingRunID)
	require.NoError(t, err)
	otherBook, otherSource, otherCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-prep-other", "Other German source", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, otherSource, otherCorpus)
	var otherRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, otherCorpus.ID).Scan(&otherRunID))
	baumSentence := "Kinder sehen heute einen Baum im Garten."
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text=$4,end_offset=$5 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3`, alice.ID, otherCorpus.ID, otherRunID, baumSentence, len(baumSentence))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, otherCorpus.ID, otherRunID)
	require.NoError(t, err)
	baumTokens := []struct {
		surface, lemma, upos, dependency string
		head                             int
		morphology                       map[string]string
	}{
		{"Kinder", "Kind", "NOUN", "nsubj", 1, nil}, {"sehen", "sehen", "VERB", "root", 1, map[string]string{"VerbForm": "Fin"}},
		{"heute", "heute", "ADV", "advmod", 1, nil}, {"einen", "ein", "DET", "det", 4, nil},
		{"Baum", "baum", "NOUN", "obj", 1, nil}, {"im", "in", "ADP", "case", 6, nil}, {"Garten", "Garten", "NOUN", "obl", 1, nil},
	}
	for ordinal, token := range baumTokens {
		features := token.morphology
		if features == nil {
			features = map[string]string{}
		}
		morphology, marshalErr := json.Marshal(features)
		require.NoError(t, marshalErr)
		start := strings.Index(baumSentence, token.surface)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,0,$4,$5,$6,$6,$7,$8,$9,$10,$11,$12)`, alice.ID, otherRunID, otherCorpus.ID, ordinal, token.surface, token.lemma, token.upos, token.dependency, token.head, morphology, start, start+len(token.surface))
		require.NoError(t, err)
	}
	qualityBook, qualitySource, qualityCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-prep-quality", "Low quality source", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "badwort", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, qualitySource, qualityCorpus)
	var qualityRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, qualityCorpus.ID).Scan(&qualityRunID))
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text='badwort',end_offset=7 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3`, alice.ID, qualityCorpus.ID, qualityRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_tokens SET surface='badwort',raw_lemma='badwort',canonical_lemma='badwort',start_offset=0,end_offset=7 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, qualityCorpus.ID, qualityRunID)
	require.NoError(t, err)
	seedBrowseHTTPToken(t, ctx, store, source, corpus)
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, corpus.ID).Scan(&runID))
	sentence := "Die Kinder besuchen heute das alte Haus."
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text=$4,end_offset=$5 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, corpus.ID, runID, sentence, len(sentence))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, corpus.ID, runID)
	require.NoError(t, err)
	tokens := []struct {
		surface, lemma, upos, dependency string
		head                             int
		morphology                       map[string]string
	}{
		{"Die", "die", "DET", "det", 1, nil},
		{"Kinder", "Kind", "NOUN", "nsubj", 2, nil},
		{"besuchen", "besuchen", "VERB", "root", 2, map[string]string{"VerbForm": "Fin"}},
		{"heute", "heute", "ADV", "advmod", 2, nil},
		{"das", "das", "DET", "det", 5, nil},
		{"alte", "alt", "ADJ", "amod", 5, nil},
		{"Haus", "haus", "NOUN", "obj", 2, nil},
	}
	for ordinal, token := range tokens {
		start := strings.Index(sentence, token.surface)
		morphology := token.morphology
		if morphology == nil {
			morphology = map[string]string{}
		}
		encodedMorphology, marshalErr := json.Marshal(morphology)
		require.NoError(t, marshalErr)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,0,$4,$5,$6,$6,$7,$8,$9,$10,$11,$12)`, alice.ID, runID, corpus.ID, ordinal, token.surface, token.lemma, token.upos, token.dependency, token.head, encodedMorphology, start, start+len(token.surface))
		require.NoError(t, err)
	}
	require.NoError(t, store.SetVocabularyBrowseSelection(ctx, alice.ID, "de", "haus", "NOUN", true))
	customDeck, err := store.CreateCustomVocabularyDeck(ctx, alice.ID, "de", "German practice", "d7c38a7e-777d-4fbd-a234-67115c7f92ab")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos) VALUES($1,$2,'de','missingwort','NOUN')`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos) VALUES($1,$2,'de','baum','NOUN')`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos) VALUES($1,$2,'de','badwort','NOUN')`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	bookDeckBefore, err := store.GetDeckPreparation(ctx, alice.ID, bookDeck.ID)
	require.NoError(t, err)

	workers := river.NewWorkers()
	provider := &customDeckFixtureTranslation{}
	presentation := cardexport.NewPresentation(nil)
	prepareddeck.AddCustomDeckPreparationWorker(workers, store, presentation, provider, true)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{prepareddeck.Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	preparationService := prepareddeck.NewCustomDeckPreparationService(store, client, presentation, provider)

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), CustomDeckPreparation: preparationService, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "custom-prep-alice", "alice-password")
	deckPage := perform(t, h, http.MethodGet, "/vocabulary/decks/"+customDeck.ID, nil, cookies)
	require.Equal(t, http.StatusOK, deckPage.Code)
	assert.Contains(t, deckPage.Body.String(), "Prepare deck")
	csrf := hiddenToken(t, deckPage.Body.String())
	postCookies := append(append([]*http.Cookie{}, cookies...), cookieNamed(t, cookies, csrfCookie))
	hausUnitID := domain.EPUBUnitID(0, strings.TrimPrefix(source.SourceIdentifier, "migration-"))
	evidenceFingerprint, err := preparationService.EvidenceFingerprint(ctx, alice.ID, customDeck.ID)
	require.NoError(t, err)
	selectionEdit, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		if rollbackErr := selectionEdit.Rollback(context.Background()); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			t.Errorf("rollback concurrent deck edit: %v", rollbackErr)
		}
	})
	_, err = selectionEdit.Exec(ctx, `SELECT id FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2 FOR UPDATE`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	type submissionResult struct {
		preparation domain.CustomDeckPreparation
		err         error
	}
	submittedDuringEdit := make(chan submissionResult, 1)
	go func() {
		p, submitErr := preparationService.Submit(ctx, alice.ID, customDeck.ID, "c365b0d3-1f35-4360-94bd-9b2e4e501294", evidenceFingerprint)
		submittedDuringEdit <- submissionResult{preparation: p, err: submitErr}
	}()
	deadline := time.Now().Add(3 * time.Second)
	waitingForDeckLock := false
	for time.Now().Before(deadline) {
		err = store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
		 WHERE wait_event_type='Lock' AND query LIKE 'SELECT id::text FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2 FOR UPDATE%')`).Scan(&waitingForDeckLock)
		require.NoError(t, err)
		if waitingForDeckLock {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, waitingForDeckLock, "preparation should wait for an in-flight deck edit")
	_, err = selectionEdit.Exec(ctx, `DELETE FROM custom_vocabulary_deck_identities WHERE owner_id=$1 AND deck_id=$2 AND canonical_lemma='haus' AND upos='NOUN'`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	_, err = selectionEdit.Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos) VALUES($1,$2,'de','neuwort','NOUN')`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	require.NoError(t, selectionEdit.Commit(ctx))
	concurrentSubmission := <-submittedDuringEdit
	require.ErrorIs(t, concurrentSubmission.err, persistence.ErrCustomDeckPreparationEvidenceChanged)
	_, err = store.Pool().Exec(ctx, `DELETE FROM custom_vocabulary_deck_identities WHERE owner_id=$1 AND deck_id=$2 AND canonical_lemma='neuwort' AND upos='NOUN'`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos) VALUES($1,$2,'de','haus','NOUN')`, alice.ID, customDeck.ID)
	require.NoError(t, err)
	countOnlySentence := "Am Morgen sehen die Kinder heute ein Haus im Garten."
	countOnlyTargetOffset := strings.Index(countOnlySentence, "Haus")
	countOnlyStart := 100
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,1,$5,$6,$7)`, alice.ID, runID, corpus.ID, hausUnitID, countOnlySentence, countOnlyStart, countOnlyStart+len(countOnlySentence))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,1,0,'Haus','haus','haus','NOUN','root',0,'{}',$4,$5)`, alice.ID, runID, corpus.ID, countOnlyStart+countOnlyTargetOffset, countOnlyStart+countOnlyTargetOffset+len("Haus"))
	require.NoError(t, err)
	countChangedFingerprint, err := preparationService.EvidenceFingerprint(ctx, alice.ID, customDeck.ID)
	require.NoError(t, err)
	assert.Equal(t, evidenceFingerprint, countChangedFingerprint, "adding another occurrence without losing identity evidence does not require reconfirmation")
	missingUnitID := domain.EPUBUnitID(0, strings.TrimPrefix(missingSource.SourceIdentifier, "migration-"))
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,4,15,NULL,NULL,NULL,true)`, alice.ID, missingBook.ID, missingCorpus.ID, missingRunID, missingUnitID)
	require.NoError(t, err)
	changedFingerprint, err := preparationService.EvidenceFingerprint(ctx, alice.ID, customDeck.ID)
	require.NoError(t, err)
	require.NotEqual(t, evidenceFingerprint, changedFingerprint, "the review token tracks all-evidence loss per identity")
	changedPost := perform(t, h, http.MethodPost, "/vocabulary/decks/"+customDeck.ID+"/preparations", url.Values{
		"csrf_token": {csrf}, "action_key": {"814900c1-0130-4e43-ade3-4c0d97bbf30e"}, "expected_evidence": {evidenceFingerprint},
	}, postCookies)
	require.Equal(t, http.StatusSeeOther, changedPost.Code)
	assert.Contains(t, changedPost.Header().Get("Location"), "evidence_changed=true")
	var preparationCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2`, alice.ID, customDeck.ID).Scan(&preparationCount))
	assert.Zero(t, preparationCount, "a changed all-evidence state must return to review before freezing")
	refreshedReview := perform(t, h, http.MethodGet, changedPost.Header().Get("Location"), nil, cookies)
	require.Equal(t, http.StatusOK, refreshedReview.Code)
	assert.Contains(t, refreshedReview.Body.String(), "Current evidence availability changed since your review")
	assert.Contains(t, refreshedReview.Body.String(), "Custom decks can overlap")
	assert.Contains(t, refreshedReview.Body.String(), "Custom packages use a stable Anki deck name")
	evidenceFingerprint, err = preparationService.EvidenceFingerprint(ctx, alice.ID, customDeck.ID)
	require.NoError(t, err)
	post := perform(t, h, http.MethodPost, "/vocabulary/decks/"+customDeck.ID+"/preparations", url.Values{
		"csrf_token": {csrf}, "action_key": {"7c99d59c-28a3-45a7-8b53-66054b780044"}, "expected_evidence": {evidenceFingerprint},
	}, postCookies)
	require.Equal(t, http.StatusSeeOther, post.Code, post.Body.String())
	statusURL := post.Header().Get("Location")
	preparationID := strings.TrimPrefix(statusURL, "/vocabulary/deck-preparations/")
	submitted, err := store.GetCustomDeckPreparation(ctx, alice.ID, preparationID)
	require.NoError(t, err)
	require.NotEmpty(t, submitted.FrozenSpec)
	var frozen struct {
		Evidence  []domain.CustomDeckPreparationEvidence `json:"evidence"`
		Omissions []domain.CustomDeckPreparationOmission `json:"omissions"`
	}
	require.NoError(t, json.Unmarshal(submitted.FrozenSpec, &frozen))
	require.Len(t, frozen.Evidence, 2)
	var hausEvidence *domain.CustomDeckPreparationEvidence
	for i := range frozen.Evidence {
		if frozen.Evidence[i].Lemma == "haus" {
			hausEvidence = &frozen.Evidence[i]
		}
	}
	require.NotNil(t, hausEvidence)
	assert.Equal(t, book.ID, hausEvidence.BookID)
	assert.Equal(t, source.ID, hausEvidence.SourceMaterialID)
	assert.Equal(t, runID, hausEvidence.AnalysisRunID)
	assert.Equal(t, corpus.ID, hausEvidence.CorpusID)
	assert.Equal(t, "NOUN", hausEvidence.UPOS)
	assert.Equal(t, "Haus", hausEvidence.Target)
	assert.Equal(t, sentence, hausEvidence.Sentence)
	require.Len(t, frozen.Omissions, 2)
	frozenOmissionKinds := map[string]string{}
	for _, omission := range frozen.Omissions {
		frozenOmissionKinds[omission.Lemma] = omission.Kind
	}
	assert.Equal(t, "evidence", frozenOmissionKinds["missingwort"])
	assert.Equal(t, "quality", frozenOmissionKinds["badwort"])
	repeatedPost := perform(t, h, http.MethodPost, "/vocabulary/decks/"+customDeck.ID+"/preparations", url.Values{
		"csrf_token": {csrf}, "action_key": {"7c99d59c-28a3-45a7-8b53-66054b780044"}, "expected_evidence": {evidenceFingerprint},
	}, postCookies)
	require.Equal(t, http.StatusSeeOther, repeatedPost.Code)
	assert.Equal(t, statusURL, repeatedPost.Header().Get("Location"), "uncertain repeat resolves to the same durable generation")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2`, alice.ID, customDeck.ID).Scan(&preparationCount))
	assert.Equal(t, 1, preparationCount)
	changedSentence := "The source changed after this Custom deck was submitted."
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text=$3 WHERE owner_id=$1 AND corpus_id=$2`, alice.ID, corpus.ID, changedSentence)
	require.NoError(t, err)
	hausStart := strings.Index(sentence, "Haus")
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,$6,$7,NULL,NULL,NULL,true)`, alice.ID, book.ID, corpus.ID, runID, hausUnitID, hausStart, hausStart+len("Haus"))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.Cleanup(t, "river client", func() error { return client.Stop(context.Background()) })
	var ready domain.CustomDeckPreparation
	for {
		status := perform(t, h, http.MethodGet, statusURL, nil, cookies)
		require.Equal(t, http.StatusOK, status.Code)
		ready, err = preparationService.Get(ctx, alice.ID, preparationID)
		require.NoError(t, err)
		if ready.State == "complete_with_omissions" || ready.State == "failed" {
			break
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, "custom deck preparation did not finish")
		case <-time.After(20 * time.Millisecond):
		}
	}
	require.Equal(t, "complete_with_omissions", ready.State, ready.Error)
	require.Greater(t, ready.TotalCards, 0)
	require.Len(t, ready.Omissions, 3)
	completedOmissionKinds := map[string]string{}
	for _, omission := range ready.Omissions {
		completedOmissionKinds[omission.Lemma] = omission.Kind
	}
	assert.Equal(t, "evidence", completedOmissionKinds["missingwort"])
	assert.Equal(t, "quality", completedOmissionKinds["badwort"])
	assert.Equal(t, "meaning", completedOmissionKinds["baum"])
	translationRequestsBeforeRedelivery := len(provider.requests)
	redeliveryWorker := &prepareddeck.CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: provider, Configured: true}
	require.NoError(t, redeliveryWorker.Work(ctx, &river.Job[prepareddeck.CustomDeckPreparationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
		Args:   prepareddeck.CustomDeckPreparationJobArgs{OwnerID: alice.ID, PreparationID: preparationID},
	}))
	assert.Len(t, provider.requests, translationRequestsBeforeRedelivery, "redelivery of a complete-with-omissions generation is terminal")
	status := perform(t, h, http.MethodGet, statusURL, nil, cookies)
	assert.Contains(t, status.Body.String(), "ready")
	assert.Contains(t, status.Body.String(), "missingwort")
	assert.Contains(t, status.Body.String(), "baum")
	assert.Contains(t, status.Body.String(), "badwort")
	assert.Contains(t, status.Body.String(), "Private source title")
	assert.Contains(t, status.Body.String(), "Die Kinder besuchen heute das alte Haus.")
	assert.Contains(t, status.Body.String(), "does not remove notes or packages already imported or downloaded")
	assert.NotContains(t, status.Body.String(), changedSentence)
	download := perform(t, h, http.MethodGet, statusURL+"/download", nil, cookies)
	require.Equal(t, http.StatusOK, download.Code)
	assert.Equal(t, "application/vnd.anki", download.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(download.Body.String(), "PK"), "download must be a real APKG ZIP")
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='unknown',language_tag=NULL WHERE owner_id=$1 AND id=$2`, alice.ID, book.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='unknown',language_tag=NULL WHERE owner_id=$1 AND id=$2`, alice.ID, otherBook.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='unknown',language_tag=NULL WHERE owner_id=$1 AND id=$2`, alice.ID, qualityBook.ID)
	require.NoError(t, err)
	allMissingPage := perform(t, h, http.MethodGet, "/vocabulary/decks/"+customDeck.ID, nil, cookies)
	require.Equal(t, http.StatusOK, allMissingPage.Code)
	assert.Contains(t, allMissingPage.Body.String(), "no selected identity has current eligible evidence")
	assert.Contains(t, allMissingPage.Body.String(), "Preparation history", "generation history remains available without current evidence")
	assert.Contains(t, allMissingPage.Body.String(), preparationID)
	assert.Contains(t, allMissingPage.Body.String(), "Evidence changes on this page since the previous Ready generation")
	assert.Contains(t, allMissingPage.Body.String(), "current eligible evidence is absent")
	assert.NotContains(t, allMissingPage.Body.String(), ">Prepare deck</button>")
	allMissingFingerprint, err := preparationService.EvidenceFingerprint(ctx, alice.ID, customDeck.ID)
	require.NoError(t, err)
	allMissingPost := perform(t, h, http.MethodPost, "/vocabulary/decks/"+customDeck.ID+"/preparations", url.Values{
		"csrf_token": {csrf}, "action_key": {"59feee1c-6859-44f5-8dc7-2fdf298d7482"}, "expected_evidence": {allMissingFingerprint},
	}, postCookies)
	require.Equal(t, http.StatusConflict, allMissingPost.Code)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2`, alice.ID, customDeck.ID).Scan(&preparationCount))
	assert.Equal(t, 1, preparationCount, "all-zero current evidence must not create another generation")
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='chosen',language_tag='de' WHERE owner_id=$1 AND id=$2`, alice.ID, book.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='chosen',language_tag='de' WHERE owner_id=$1 AND id=$2`, alice.ID, otherBook.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='chosen',language_tag='de' WHERE owner_id=$1 AND id=$2`, alice.ID, qualityBook.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND corpus_id=$3 AND analysis_run_id=$4 AND source_document_id=$5 AND start_offset=$6 AND end_offset=$7`, alice.ID, book.ID, corpus.ID, runID, hausUnitID, hausStart, hausStart+len("Haus"))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_sentences SET sentence_text=$4,end_offset=$5 WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, alice.ID, corpus.ID, runID, sentence, len(sentence))
	require.NoError(t, err)
	retryFingerprint, err := preparationService.EvidenceFingerprint(ctx, alice.ID, customDeck.ID)
	require.NoError(t, err)
	require.NoError(t, client.Stop(ctx))
	failingProvider := &customDeckFailTranslation{}
	failureWorkers := river.NewWorkers()
	prepareddeck.AddCustomDeckPreparationWorker(failureWorkers, store, presentation, failingProvider, true)
	failureClient, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{prepareddeck.Queue: {MaxWorkers: 1}}, Workers: failureWorkers})
	require.NoError(t, err)
	recoveryService := prepareddeck.NewCustomDeckPreparationService(store, failureClient, presentation, failingProvider)
	h.services.CustomDeckPreparation = recoveryService
	cancelledPreparation, err := recoveryService.Submit(ctx, alice.ID, customDeck.ID, "550a15b8-2068-43bd-86ad-2e1f09f460de", retryFingerprint)
	require.NoError(t, err)
	cancelPost := perform(t, h, http.MethodPost, "/vocabulary/deck-preparations/"+cancelledPreparation.ID+"/cancel", url.Values{"csrf_token": {csrf}}, postCookies)
	require.Equal(t, http.StatusSeeOther, cancelPost.Code)
	cancelledStatus := perform(t, h, http.MethodGet, cancelPost.Header().Get("Location"), nil, cookies)
	require.Equal(t, http.StatusOK, cancelledStatus.Code)
	assert.Contains(t, cancelledStatus.Body.String(), "Preparation cancelled")
	cancelledPreparation, err = recoveryService.Get(ctx, alice.ID, cancelledPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", cancelledPreparation.State)
	inFlightPreparation, err := recoveryService.Submit(ctx, alice.ID, customDeck.ID, "bc4e1589-68a8-42b1-92e4-727132766de2", retryFingerprint)
	require.NoError(t, err)
	cancelProvider := &customDeckCancelTranslation{cancel: func() error {
		return store.CancelCustomDeckPreparation(ctx, alice.ID, inFlightPreparation.ID)
	}}
	cancelWorker := &prepareddeck.CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: cancelProvider, Configured: true}
	require.NoError(t, cancelWorker.Work(ctx, &river.Job[prepareddeck.CustomDeckPreparationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   prepareddeck.CustomDeckPreparationJobArgs{OwnerID: alice.ID, PreparationID: inFlightPreparation.ID},
	}))
	inFlightPreparation, err = recoveryService.Get(ctx, alice.ID, inFlightPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", inFlightPreparation.State)
	assert.Equal(t, 1, cancelProvider.calls, "an in-flight cancellation stops further provider requests")
	failedPreparation, err := recoveryService.Submit(ctx, alice.ID, customDeck.ID, "1eb7d21b-a6e9-42c4-9339-6feeb8dd29e8", retryFingerprint)
	require.NoError(t, err)
	failureWorker := &prepareddeck.CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: failingProvider, Configured: true}
	require.NoError(t, failureWorker.Work(ctx, &river.Job[prepareddeck.CustomDeckPreparationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1},
		Args:   prepareddeck.CustomDeckPreparationJobArgs{OwnerID: alice.ID, PreparationID: failedPreparation.ID},
	}))
	failedPreparation, err = recoveryService.Get(ctx, alice.ID, failedPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failedPreparation.State)
	malformedPreparation, err := recoveryService.Submit(ctx, alice.ID, customDeck.ID, "a7e59d94-a40f-42c1-ae54-a9673fbda021", retryFingerprint)
	require.NoError(t, err)
	malformedWorker := &prepareddeck.CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: &customDeckMalformedTranslation{}, Configured: true}
	require.NoError(t, malformedWorker.Work(ctx, &river.Job[prepareddeck.CustomDeckPreparationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   prepareddeck.CustomDeckPreparationJobArgs{OwnerID: alice.ID, PreparationID: malformedPreparation.ID},
	}))
	malformedPreparation, err = recoveryService.Get(ctx, alice.ID, malformedPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", malformedPreparation.State, "malformed enrichment outcomes fail closed")
	failedStatusURL := "/vocabulary/deck-preparations/" + failedPreparation.ID
	failedStatus := perform(t, h, http.MethodGet, failedStatusURL, nil, cookies)
	require.Equal(t, http.StatusOK, failedStatus.Code)
	assert.Contains(t, failedStatus.Body.String(), "failed")
	assert.Contains(t, failedStatus.Body.String(), "Previous Ready preparation")
	assert.Contains(t, failedStatus.Body.String(), preparationID+"/download")
	previousDownload := perform(t, h, http.MethodGet, "/vocabulary/deck-preparations/"+preparationID+"/download", nil, cookies)
	assert.Equal(t, http.StatusOK, previousDownload.Code, "a failed replacement must keep the old Ready result downloadable")
	retryKey := "12894429-9e3b-4a97-9f98-507ad734359b"
	retryPost := perform(t, h, http.MethodPost, "/vocabulary/decks/"+customDeck.ID+"/preparations", url.Values{
		"csrf_token": {csrf}, "action_key": {retryKey}, "expected_evidence": {retryFingerprint},
	}, postCookies)
	require.Equal(t, http.StatusSeeOther, retryPost.Code, retryPost.Body.String())
	repeatedRetry := perform(t, h, http.MethodPost, "/vocabulary/decks/"+customDeck.ID+"/preparations", url.Values{
		"csrf_token": {csrf}, "action_key": {retryKey}, "expected_evidence": {retryFingerprint},
	}, postCookies)
	require.Equal(t, http.StatusSeeOther, repeatedRetry.Code)
	assert.Equal(t, retryPost.Header().Get("Location"), repeatedRetry.Header().Get("Location"))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2`, alice.ID, customDeck.ID).Scan(&preparationCount))
	assert.Equal(t, 6, preparationCount, "retry creates one new durable generation despite repeated submission")
	retryPreparationID := strings.TrimPrefix(retryPost.Header().Get("Location"), "/vocabulary/deck-preparations/")
	retryProvider := &customDeckFixtureTranslation{}
	retryWorker := &prepareddeck.CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: retryProvider, Configured: true}
	require.NoError(t, retryWorker.Work(ctx, &river.Job[prepareddeck.CustomDeckPreparationJobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 1},
		Args:   prepareddeck.CustomDeckPreparationJobArgs{OwnerID: alice.ID, PreparationID: retryPreparationID},
	}))
	retried, err := recoveryService.Get(ctx, alice.ID, retryPreparationID)
	require.NoError(t, err)
	require.Equal(t, "complete_with_omissions", retried.State, retried.Error)
	newestDownload := perform(t, h, http.MethodGet, "/vocabulary/deck-preparations/"+retryPreparationID+"/download", nil, cookies)
	assert.Equal(t, http.StatusOK, newestDownload.Code)
	previousDownload = perform(t, h, http.MethodGet, "/vocabulary/deck-preparations/"+preparationID+"/download", nil, cookies)
	assert.Equal(t, http.StatusNotFound, previousDownload.Code, "only the newest successful generation remains downloadable through Mouseion")
	deckAfterReplacement := perform(t, h, http.MethodGet, "/vocabulary/decks/"+customDeck.ID, nil, cookies)
	assert.Contains(t, deckAfterReplacement.Body.String(), "Prepare again")
	assert.Contains(t, deckAfterReplacement.Body.String(), "Preparation history")
	assert.Contains(t, deckAfterReplacement.Body.String(), retryPreparationID)
	historicalStatus := perform(t, h, http.MethodGet, statusURL, nil, cookies)
	assert.Contains(t, historicalStatus.Body.String(), "This generation is historical")
	require.Len(t, provider.requests, 2)
	require.Len(t, retryProvider.requests, 2)
	providerRequests := fmt.Sprint(append(provider.requests, retryProvider.requests...))
	assert.NotContains(t, providerRequests, "Private source title", "provider requests must not include Book title")
	assert.Contains(t, providerRequests, sentence, "provider request must use the submission-time sentence")
	assert.NotContains(t, providerRequests, changedSentence, "queued evidence changes must not alter provider inputs")
	bobCookies, _ := loginCookies(t, h, "custom-prep-bob", "bob-password")
	assert.Equal(t, http.StatusNotFound, perform(t, h, http.MethodGet, statusURL, nil, bobCookies).Code)

	stillOwnerDeck, err := store.GetDeckPreparation(ctx, alice.ID, bookDeck.ID)
	require.NoError(t, err)
	assert.Equal(t, bookDeckBefore.State, stillOwnerDeck.State)
	assert.Equal(t, bookDeckBefore.Artifact, stillOwnerDeck.Artifact)
	var generated, goals int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='haus'`, alice.ID).Scan(&generated))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1`, alice.ID).Scan(&goals))
	assert.Zero(t, generated, "Custom deck export must not mark Generated vocabulary")
	assert.Zero(t, goals, "Custom deck export must not change Reading")
	_ = book
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
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, aliceBook.ID, domain.BookDispositionSetAside))

	beforeDeck, err := store.GetDeckPreparation(ctx, alice.ID, aliceDeck.ID)
	require.NoError(t, err)
	var readingCountBefore int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1`, alice.ID).Scan(&readingCountBefore))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "browse-http-alice", "alice-password")
	response := perform(t, h, http.MethodGet, "/vocabulary?q=HA", nil, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Alice German")
	assert.Contains(t, response.Body.String(), "haus")
	assert.Contains(t, response.Body.String(), "Contributes current eligible vocabulary evidence")
	assert.NotContains(t, response.Body.String(), "Bob German")
	assert.NotContains(t, response.Body.String(), `action="/vocabulary/import"`)
	assert.Contains(t, response.Body.String(), `href="/vocabulary/import"`)

	afterDeck, err := store.GetDeckPreparation(ctx, alice.ID, aliceDeck.ID)
	require.NoError(t, err)
	assert.Equal(t, beforeDeck.State, afterDeck.State)
	assert.Equal(t, beforeDeck.Artifact, afterDeck.Artifact)
	var readingCountAfter int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1`, alice.ID).Scan(&readingCountAfter))
	assert.Equal(t, readingCountBefore, readingCountAfter, "Browse must not create or change Reading state")
	_, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "haus", "NOUN")
	require.NoError(t, err)

	// Keep URL encoding on a normal form submission as well as the direct request.
	search := perform(t, h, http.MethodGet, "/vocabulary?"+url.Values{"q": {"haus"}, "page": {"1"}}.Encode(), nil, cookies)
	assert.Equal(t, http.StatusOK, search.Code)
	filteredQuery := url.Values{
		"q":        {"ha"},
		"book":     {aliceBook.ID, aliceOtherBook.ID},
		"pos":      {"NOUN"},
		"known":    {"known"},
		"reserved": {"not-reserved"},
		"sort":     {"occurrences"},
	}
	filtered := perform(t, h, http.MethodGet, "/vocabulary?"+filteredQuery.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, filtered.Code)
	assert.Contains(t, filtered.Body.String(), "2 of 2 Books in the applied Book scope contribute current vocabulary evidence")
	assert.Contains(t, filtered.Body.String(), "scope contains 1 of 1 identities in the full active-language corpus")
	assert.Contains(t, filtered.Body.String(), "haus")
	assert.Contains(t, filtered.Body.String(), "<td>2</td><td>2</td>", "the authenticated page renders multi-Book scoped occurrence and Book counts")
	assert.NotContains(t, filtered.Body.String(), "heim")

	selectedBooks := []string{aliceBook.ID, aliceOtherBook.ID}
	scoped, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{BookIDs: selectedBooks, UPOS: []string{"NOUN"}, KnownFilter: "known", ReservedFilter: "not-reserved", Sort: "books", Page: 1})
	require.NoError(t, err)
	require.Len(t, scoped.Rows, 1)
	assert.Equal(t, "haus", scoped.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(2), scoped.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(2), scoped.Rows[0].BookCount)
	assert.Equal(t, int64(1), scoped.InventoryTotal)
	assert.Equal(t, int64(1), scoped.ScopedInventoryTotal)
	assert.ElementsMatch(t, selectedBooks, scoped.SelectedBooks)
	assert.Equal(t, "books", scoped.Sort)
	unfilteredState, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{BookIDs: selectedBooks, UPOS: []string{"NOUN"}, ReservedFilter: "not-reserved", Sort: "books", Page: 1})
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
	for ordinal := int64(1); ordinal <= 26; ordinal++ {
		lemma := fmt.Sprintf("wort%02d", ordinal-1)
		start := ordinal * 10
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, alice.ID, runID, aliceCorpus.ID, unitID, ordinal, lemma, start, start+int64(len(lemma)))
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,$4,0,$5,$5,$5,'NOUN','root',0,'{}',$6,$7)`, alice.ID, runID, aliceCorpus.ID, ordinal, lemma, start, start+int64(len(lemma)))
		require.NoError(t, err)
	}
	firstPage := perform(t, h, http.MethodGet, "/vocabulary?sort=lemma&page=1", nil, cookies)
	require.Equal(t, http.StatusOK, firstPage.Code)
	assert.Contains(t, firstPage.Body.String(), "Page 1 of 2")
	assert.Contains(t, firstPage.Body.String(), "wort00")
	assert.NotContains(t, firstPage.Body.String(), "wort24")
	assert.Contains(t, firstPage.Body.String(), `href="/vocabulary?page=2&amp;rev=`)
	revisionPage, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Sort: "lemma", Page: 1})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,260,266,'wort25changed','de','1',false)`, alice.ID, aliceBook.ID, aliceCorpus.ID, runID, unitID)
	require.NoError(t, err)
	changedPage := perform(t, h, http.MethodGet, "/vocabulary?sort=lemma&page=2&rev="+revisionPage.CorpusRevision, nil, cookies)
	assert.Equal(t, http.StatusConflict, changedPage.Code)
	assert.Contains(t, changedPage.Body.String(), "Current evidence changed")
	assert.Contains(t, changedPage.Body.String(), "Restart from results")
	secondPage := perform(t, h, http.MethodGet, "/vocabulary?sort=lemma&page=2", nil, cookies)
	require.Equal(t, http.StatusOK, secondPage.Code)
	assert.Contains(t, secondPage.Body.String(), "Page 2 of 2")
	assert.Contains(t, secondPage.Body.String(), "wort24")
	assert.Contains(t, secondPage.Body.String(), "wort25")
	assert.NotContains(t, secondPage.Body.String(), "wort00")
}

func TestBrowseSelectionReviewAndCustomDeckCreationAreDurableAndIdempotentOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "vocabulary-selection-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "selection-http-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "selection-http-bob", "bob-password", false)
	sourceBook, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "selection-http-alice", "Selection German", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 2}})
	_, otherSource, otherCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "selection-http-other", "Selection German 2", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	_, bobSource, bobCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, bob.ID, "selection-http-bob", "Private German", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, source, corpus)
	seedBrowseHTTPToken(t, ctx, store, otherSource, otherCorpus)
	seedBrowseHTTPToken(t, ctx, store, bobSource, bobCorpus)
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, source.OwnerID, corpus.ID).Scan(&runID))
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(source.SourceIdentifier, "migration-"))
	for ordinal := int64(1); ordinal <= 26; ordinal++ {
		lemma := fmt.Sprintf("wort%02d", ordinal-1)
		start := ordinal * 10
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, alice.ID, runID, corpus.ID, unitID, ordinal, lemma, start, start+int64(len(lemma)))
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,'de',$2,$3,$4,0,$5,$5,$5,'NOUN','root',0,'{}',$6,$7)`, alice.ID, runID, corpus.ID, ordinal, lemma, start, start+int64(len(lemma)))
		require.NoError(t, err)
	}
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "selection-http-alice", "alice-password")
	page := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Browse selection (0)", "the active selection count is visible from Browse")
	token := hiddenToken(t, page.Body.String())
	csrf := cookieNamed(t, cookies, csrfCookie)
	postCookies := append(append([]*http.Cookie{}, cookies...), csrf)
	create := url.Values{"csrf_token": {token}, "creation_key": {"d7c38a7e-777d-4fbd-a234-67115c7f92ab"}, "name": {"German shortlist"}}
	failedAttempt := perform(t, h, http.MethodPost, "/vocabulary/decks", create, postCookies)
	assert.Equal(t, http.StatusInternalServerError, failedAttempt.Code, "an empty selection must not create a deck")
	assert.Contains(t, failedAttempt.Body.String(), `name="creation_key" value="d7c38a7e-777d-4fbd-a234-67115c7f92ab"`, "retry form retains the same idempotency key")
	add := url.Values{"csrf_token": {token}, "lemma": {"haus"}, "upos": {"NOUN"}}
	added := perform(t, h, http.MethodPost, "/vocabulary/selection/add", add, postCookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	// Repeating a set operation is safe, and the review sees evidence across both Books.
	added = perform(t, h, http.MethodPost, "/vocabulary/selection/add", add, postCookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	pageTwo := perform(t, h, http.MethodGet, "/vocabulary?page=2", nil, cookies)
	require.Equal(t, http.StatusOK, pageTwo.Code)
	assert.Contains(t, pageTwo.Body.String(), "wort25", "selection can be added from a later Browse page")
	secondPageAdd := url.Values{"csrf_token": {token}, "lemma": {"wort25"}, "upos": {"NOUN"}}
	added = perform(t, h, http.MethodPost, "/vocabulary/selection/add", secondPageAdd, postCookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	review := perform(t, h, http.MethodGet, "/vocabulary/selection", nil, cookies)
	require.Equal(t, http.StatusOK, review.Code)
	assert.Contains(t, review.Body.String(), "2 selected identities")
	assert.Contains(t, review.Body.String(), "2 occurrences · 2 Books")
	assert.NotContains(t, review.Body.String(), "Private German")
	oversizedPage := perform(t, h, http.MethodGet, "/vocabulary/selection?page=9223372036854775807", nil, cookies)
	assert.Equal(t, http.StatusOK, oversizedPage.Code, "out-of-range page numbers should clamp safely")
	confirmClear := perform(t, h, http.MethodGet, "/vocabulary/selection/clear-confirm", nil, cookies)
	require.Equal(t, http.StatusOK, confirmClear.Code)
	assert.Contains(t, confirmClear.Body.String(), "Confirm clear selection")
	stillSelected, err := store.ListVocabularyBrowseSelection(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Len(t, stillSelected, 2, "opening the confirmation page does not clear selection")
	cleared := perform(t, h, http.MethodPost, "/vocabulary/selection/clear", url.Values{"csrf_token": {token}}, postCookies)
	require.Equal(t, http.StatusSeeOther, cleared.Code)
	stillSelected, err = store.ListVocabularyBrowseSelection(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, stillSelected, "only the explicit confirmation POST clears the selection")
	for _, identity := range []domain.VocabularyIdentity{{CanonicalLemma: "haus", UPOS: "NOUN"}, {CanonicalLemma: "wort25", UPOS: "NOUN"}} {
		form := url.Values{"csrf_token": {token}, "lemma": {identity.CanonicalLemma}, "upos": {identity.UPOS}}
		added = perform(t, h, http.MethodPost, "/vocabulary/selection/add", form, postCookies)
		require.Equal(t, http.StatusSeeOther, added.Code)
	}
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, sourceBook.ID)
	require.NoError(t, err)
	review = perform(t, h, http.MethodGet, "/vocabulary/selection", nil, cookies)
	require.Equal(t, http.StatusOK, review.Code)
	assert.Contains(t, review.Body.String(), "1 currently lack evidence")
	assert.Contains(t, review.Body.String(), "No current evidence")
	missingOnly := perform(t, h, http.MethodGet, "/vocabulary/selection?missing=true", nil, cookies)
	assert.Contains(t, missingOnly.Body.String(), "wort25")
	assert.NotContains(t, missingOnly.Body.String(), "<strong>haus</strong>")
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_selections(owner_id,language,canonical_lemma,upos) VALUES($1,'it','casa','NOUN')`, alice.ID)
	require.NoError(t, err)
	created := perform(t, h, http.MethodPost, "/vocabulary/decks", create, postCookies)
	require.Equal(t, http.StatusSeeOther, created.Code)
	createdAgain := perform(t, h, http.MethodPost, "/vocabulary/decks", create, postCookies)
	require.Equal(t, http.StatusSeeOther, createdAgain.Code)
	assert.Equal(t, created.Header().Get("Location"), createdAgain.Header().Get("Location"), "same action retry must resolve to the same deck")
	deckPage := perform(t, h, http.MethodGet, created.Header().Get("Location"), nil, cookies)
	require.Equal(t, http.StatusOK, deckPage.Code)
	assert.Contains(t, deckPage.Body.String(), "German shortlist")
	assert.Contains(t, deckPage.Body.String(), "haus")
	assert.Contains(t, deckPage.Body.String(), "No current evidence", "naming a deck retains missing identities")
	bobCookies, _ := loginCookies(t, h, "selection-http-bob", "bob-password")
	otherOwnerDeck := perform(t, h, http.MethodGet, created.Header().Get("Location"), nil, bobCookies)
	assert.Equal(t, http.StatusNotFound, otherOwnerDeck.Code, "Custom deck ids must remain owner-scoped")
	var deckCount, identityCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_decks WHERE owner_id=$1`, alice.ID).Scan(&deckCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_identities WHERE owner_id=$1`, alice.ID).Scan(&identityCount))
	assert.Equal(t, 1, deckCount)
	assert.Equal(t, 2, identityCount)
	remaining, err := store.ListVocabularyBrowseSelection(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, remaining, "confirmed creation clears only the active language selection")
	otherOwner, err := store.ListVocabularyBrowseSelection(ctx, bob.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, otherOwner)
	otherLanguage, err := store.ListVocabularyBrowseSelection(ctx, alice.ID, "it")
	require.NoError(t, err)
	require.Len(t, otherLanguage, 1)
	assert.Equal(t, "casa", otherLanguage[0].CanonicalLemma, "creating a German deck must preserve the Italian selection")
}

func TestCustomDeckEditingIsOwnerScopedAndReadOnlyWhenLanguageDisappears(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "custom-deck-edit-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "custom-edit-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "custom-edit-bob", "bob-password", false)
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-edit", "Custom deck evidence", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, source, corpus)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_selections(owner_id,language,canonical_lemma,upos) VALUES($1,'de','haus','NOUN')`, alice.ID)
	require.NoError(t, err)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "custom-edit-alice", "alice-password")
	csrf := cookieNamed(t, cookies, csrfCookie)
	postCookies := append(append([]*http.Cookie{}, cookies...), csrf)
	created := perform(t, h, http.MethodPost, "/vocabulary/decks", url.Values{"csrf_token": {hiddenToken(t, perform(t, h, http.MethodGet, "/vocabulary/selection", nil, cookies).Body.String())}, "creation_key": {"98e01219-011f-482b-b3cd-26093d45fd81"}, "name": {"First deck"}}, postCookies)
	require.Equal(t, http.StatusSeeOther, created.Code)
	deckURL := created.Header().Get("Location")
	deckID := strings.TrimPrefix(deckURL, "/vocabulary/decks/")
	deckPage := perform(t, h, http.MethodGet, deckURL, nil, cookies)
	require.Equal(t, http.StatusOK, deckPage.Code)
	assert.Contains(t, deckPage.Body.String(), "Save name")
	assert.Contains(t, deckPage.Body.String(), "1 occurrences · 1 Books")

	rename := url.Values{"csrf_token": {hiddenToken(t, deckPage.Body.String())}, "name": {"Renamed deck"}}
	for range 2 {
		response := perform(t, h, http.MethodPost, "/vocabulary/decks/"+deckID+"/rename", rename, postCookies)
		require.Equal(t, http.StatusSeeOther, response.Code, "retrying the same rename is safe")
	}
	add := url.Values{"csrf_token": {rename.Get("csrf_token")}, "lemma": {"unseen"}, "upos": {"VERB"}}
	for range 2 {
		response := perform(t, h, http.MethodPost, "/vocabulary/decks/"+deckID+"/identities/add", add, postCookies)
		require.Equal(t, http.StatusSeeOther, response.Code, "retrying add does not duplicate an identity")
	}
	page := perform(t, h, http.MethodGet, deckURL, nil, cookies)
	assert.Contains(t, page.Body.String(), "Renamed deck")
	assert.Contains(t, page.Body.String(), "unseen")
	assert.Contains(t, page.Body.String(), "No current evidence", "an explicitly added missing identity remains reviewable")
	missingPage := perform(t, h, http.MethodGet, deckURL+"?missing=true", nil, cookies)
	assert.Contains(t, missingPage.Body.String(), "unseen")
	assert.NotContains(t, missingPage.Body.String(), "<strong>haus</strong>")
	remove := url.Values{"csrf_token": {rename.Get("csrf_token")}, "lemma": {"unseen"}, "upos": {"VERB"}}
	removed := perform(t, h, http.MethodPost, "/vocabulary/decks/"+deckID+"/identities/remove", remove, postCookies)
	require.Equal(t, http.StatusSeeOther, removed.Code)
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, alice.ID, corpus.ID).Scan(&runID))
	unitID := domain.EPUBUnitID(0, strings.TrimPrefix(source.SourceIdentifier, "migration-"))
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,0,4,'heim','de','1',false)`, alice.ID, book.ID, corpus.ID, runID, unitID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID)
	require.NoError(t, err)
	stale := perform(t, h, http.MethodGet, deckURL, nil, cookies)
	assert.Contains(t, stale.Body.String(), "1 missing current evidence", "selection remains the originally saved haus identity when its evidence disappears")
	replacementBook, replacementSource, replacementCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-edit-replacement", "Replacement analysis", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, replacementSource, replacementCorpus)
	recovered := perform(t, h, http.MethodGet, deckURL, nil, cookies)
	assert.Contains(t, recovered.Body.String(), "haus")
	assert.NotContains(t, recovered.Body.String(), "<strong>heim</strong>", "the prior run's correction does not transfer to replacement evidence")
	assert.Contains(t, recovered.Body.String(), "current analysis of ", "current evidence links to its Book analysis")
	assert.Contains(t, recovered.Body.String(), ">Replacement analysis</a>", "analysis link names its Book")
	assert.Contains(t, recovered.Body.String(), "Review occurrences", "current evidence links to occurrence correction review")

	bobCookies, _ := loginCookies(t, h, "custom-edit-bob", "bob-password")
	assert.Equal(t, http.StatusNotFound, perform(t, h, http.MethodGet, deckURL, nil, bobCookies).Code)
	_, err = store.GetCustomVocabularyDeck(ctx, bob.ID, deckID)
	require.ErrorIs(t, err, persistence.ErrCustomVocabularyDeckNotFound)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='unknown',language_tag=NULL WHERE owner_id=$1 AND id=ANY($2::uuid[])`, alice.ID, []string{book.ID, replacementBook.ID})
	require.NoError(t, err)
	readOnly := perform(t, h, http.MethodGet, deckURL, nil, cookies)
	require.Equal(t, http.StatusOK, readOnly.Code)
	assert.Contains(t, readOnly.Body.String(), "currently unavailable")
	assert.NotContains(t, readOnly.Body.String(), "Save name")
	assert.Equal(t, http.StatusConflict, perform(t, h, http.MethodPost, "/vocabulary/decks/"+deckID+"/rename", rename, postCookies).Code)
	assert.Equal(t, http.StatusConflict, perform(t, h, http.MethodPost, "/vocabulary/selection/add", url.Values{"csrf_token": {rename.Get("csrf_token")}, "lemma": {"heim"}, "upos": {"NOUN"}}, postCookies).Code)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_state='chosen',language_tag='de' WHERE owner_id=$1 AND id=ANY($2::uuid[])`, alice.ID, []string{book.ID, replacementBook.ID})
	require.NoError(t, err)
	confirm := perform(t, h, http.MethodGet, "/vocabulary/decks/"+deckID+"/delete-confirm", nil, cookies)
	require.Equal(t, http.StatusOK, confirm.Code)
	assert.Contains(t, confirm.Body.String(), "cannot revoke APKG files already downloaded")
	assert.Contains(t, confirm.Body.String(), `method="post"`)
	deleted := perform(t, h, http.MethodPost, "/vocabulary/decks/"+deckID+"/delete", url.Values{"csrf_token": {rename.Get("csrf_token")}}, postCookies)
	require.Equal(t, http.StatusSeeOther, deleted.Code)
	assert.Equal(t, http.StatusNotFound, perform(t, h, http.MethodGet, deckURL, nil, cookies).Code)
}

func TestCustomDeckReviewPagesThousandMissingIdentitiesOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "custom-deck-scale-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "custom-scale-alice", "alice-password", false)
	_, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "custom-scale-source", "Scale source", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, source, corpus)
	var deckID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_decks(owner_id,language,name,creation_key) VALUES($1,'de','Large review','d7c38a7e-777d-4fbd-a234-67115c7f92ab') RETURNING id::text`, alice.ID).Scan(&deckID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos)
SELECT $1::uuid,$2::uuid,'de','lemma-'||lpad(n::text,4,'0'),'NOUN' FROM generate_series(0,998) n
UNION ALL SELECT $1::uuid,$2::uuid,'de','haus','NOUN'`, alice.ID, deckID)
	require.NoError(t, err)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "custom-scale-alice", "alice-password")
	pageOne := perform(t, h, http.MethodGet, "/vocabulary/decks/"+deckID, nil, cookies)
	require.Equal(t, http.StatusOK, pageOne.Code)
	assert.Contains(t, pageOne.Body.String(), "1000 selected identities")
	assert.Contains(t, pageOne.Body.String(), "999 missing current evidence")
	assert.Contains(t, pageOne.Body.String(), "Page 1 of 40")
	assert.Contains(t, pageOne.Body.String(), "lemma-0000")
	assert.NotContains(t, pageOne.Body.String(), "lemma-0025")
	pageForty := perform(t, h, http.MethodGet, "/vocabulary/decks/"+deckID+"?missing=true&page=40", nil, cookies)
	require.Equal(t, http.StatusOK, pageForty.Code)
	assert.Contains(t, pageForty.Body.String(), "1000 selected identities")
	assert.Contains(t, pageForty.Body.String(), "999 missing current evidence")
	assert.Contains(t, pageForty.Body.String(), "Page 40 of 40")
	assert.Contains(t, pageForty.Body.String(), "lemma-0998")
	assert.NotContains(t, pageForty.Body.String(), "lemma-0974")
	token := hiddenToken(t, pageForty.Body.String())
	csrf := cookieNamed(t, cookies, csrfCookie)
	postCookies := append(append([]*http.Cookie{}, cookies...), csrf)
	removed := perform(t, h, http.MethodPost, "/vocabulary/decks/"+deckID+"/identities/remove", url.Values{"csrf_token": {token}, "lemma": {"lemma-0998"}, "upos": {"NOUN"}}, postCookies)
	require.Equal(t, http.StatusSeeOther, removed.Code)
	var remaining int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM custom_vocabulary_deck_identities WHERE owner_id=$1 AND deck_id=$2`, alice.ID, deckID).Scan(&remaining)
	require.NoError(t, err)
	assert.Equal(t, 999, remaining, "editing a late-page identity updates the durable selection")
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
	assert.Contains(t, focusedPage.Body.String(), `<h2 id="concordance-summary" tabindex="-1" aria-live="polite" autofocus>Current results</h2>`)
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
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
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
	assert.Contains(t, response.Body.String(), "Concordance lookup was not applied")
	assert.Contains(t, response.Body.String(), "Retry Concordance lookup")
	assert.NotContains(t, response.Body.String(), "No current analyzed occurrences match")
	partialResponse := performWithHeader(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, cookies, "HX-Request-Type", "partial")
	require.Equal(t, http.StatusGatewayTimeout, partialResponse.Code)
	assert.Contains(t, partialResponse.Body.String(), `id="concordance-recovery"`)
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
