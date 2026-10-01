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

func TestVocabularyConcordanceServesExactModesAndOccurrenceDecisionsOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-http-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "concordance-http-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "concordance-http-bob", "bob-password", false)
	_, aliceSource, aliceCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "concordance-http-alice", "Alice Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	_, bobSource, bobCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, bob.ID, "concordance-http-bob", "Bob Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	staleBook, staleSource, staleCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "concordance-http-stale", "Stale Concordance Book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	seedBrowseHTTPToken(t, ctx, store, aliceSource, aliceCorpus)
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
	corrected := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=heim&upos=NOUN", nil, cookies)
	require.Equal(t, http.StatusOK, corrected.Code)
	assert.Contains(t, corrected.Body.String(), "Alice Concordance Book")
	assert.Contains(t, corrected.Body.String(), "Analyzer lemma (evidence)")
	assert.Contains(t, corrected.Body.String(), "corrected for this occurrence")
	assert.NotContains(t, corrected.Body.String(), "Bob Concordance Book")
	assert.NotContains(t, corrected.Body.String(), "Stale Concordance Book")
	assert.NotContains(t, corrected.Body.String(), "excluded from effective vocabulary")
	excludedEffective := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN", nil, cookies)
	require.Equal(t, http.StatusOK, excludedEffective.Code)
	assert.Contains(t, excludedEffective.Body.String(), "No current analyzed occurrences match this exact lookup.")

	surface := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, cookies)
	require.Equal(t, http.StatusOK, surface.Code)
	assert.Contains(t, surface.Body.String(), "excluded from effective vocabulary")
	assert.Contains(t, surface.Body.String(), "retained as analyzer evidence")

	analyzer := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=analyzer&term=haus&upos=NOUN", nil, cookies)
	require.Equal(t, http.StatusOK, analyzer.Code)
	assert.Contains(t, analyzer.Body.String(), "Applied analyzer lemma evidence")
	assert.Contains(t, analyzer.Body.String(), "Haus")

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
	assert.Contains(t, secondPage.Body.String(), "Results 26–26")
	assert.Contains(t, secondPage.Body.String(), `href="/vocabulary/concordance?mode=surface&amp;page=1&amp;term=Haus"`)
	unchangedEffective := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=effective&term=haus&upos=NOUN", nil, cookies)
	require.Equal(t, http.StatusOK, unchangedEffective.Code)
	assert.Equal(t, 24, strings.Count(unchangedEffective.Body.String(), `class="concordance-row"`))
	assert.NotContains(t, unchangedEffective.Body.String(), "corrected for this occurrence")
	assert.NotContains(t, unchangedEffective.Body.String(), "excluded from effective vocabulary")

	noLeak := perform(t, h, http.MethodGet, "/vocabulary/concordance?mode=surface&term=Haus", nil, func() []*http.Cookie {
		cookies, _ := loginCookies(t, h, "concordance-http-bob", "bob-password")
		return cookies
	}())
	require.Equal(t, http.StatusOK, noLeak.Code)
	assert.Contains(t, noLeak.Body.String(), "Bob Concordance Book")
	assert.NotContains(t, noLeak.Body.String(), "Alice Concordance Book")
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
