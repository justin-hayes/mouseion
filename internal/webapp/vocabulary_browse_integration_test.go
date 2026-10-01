//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Contains(t, firstPage.Body.String(), `href="/vocabulary?page=2"`)
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
	assert.Contains(t, corrected.Body.String(), "corrected for this occurrence")
	assert.NotContains(t, corrected.Body.String(), "Bob Concordance Book")
	assert.NotContains(t, strings.SplitN(corrected.Body.String(), `<section id="concordance-results"`, 2)[1], "Stale Concordance Book")
	assert.NotContains(t, corrected.Body.String(), "excluded from effective vocabulary")
	excludedEffective := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN&book="+aliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, excludedEffective.Code)
	assert.Contains(t, excludedEffective.Body.String(), "No current analyzed occurrences match this exact lookup.")

	surface := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&book="+aliceBook.ID, nil, cookies)
	require.Equal(t, http.StatusOK, surface.Code)
	assert.Contains(t, surface.Body.String(), "excluded from effective vocabulary")
	assert.Contains(t, surface.Body.String(), "retained as analyzer evidence")

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
	assert.Contains(t, firstPage.Body.String(), `href="/vocabulary/concordance?mode=surface&amp;page=2&amp;term=Haus"`)
	secondPage := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus&page=2", nil, cookies)
	require.Equal(t, http.StatusOK, secondPage.Code)
	assert.Contains(t, secondPage.Body.String(), "Results 26–28")
	assert.Contains(t, secondPage.Body.String(), `href="/vocabulary/concordance?mode=surface&amp;page=1&amp;term=Haus"`)
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
