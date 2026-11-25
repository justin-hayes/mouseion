//go:build integration

package webapp

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassicalWholeWorkBookLoop follows a multi-book Work, added whole from
// the Catalog, through My Books, the Reading chooser and Concordance (spec
// #2634, ADR 0128). The Work is one Book, one ordinary row, one chooser entry,
// one preparation and one Concordance group whose every occurrence carries a
// Citation. A learner who kept per-book Books from before the change can still
// add the whole Work, and the old Books stay ordinary rows.
func TestClassicalWholeWorkBookLoop(t *testing.T) {
	h := newClassicalLoopHarness(t)
	learner := createAccount(t, h.ctx, h.store, "whole-work-learner", "learner-password")
	cookies, csrf := loginCookies(t, h.handler, learner.Username, "learner-password")

	// A learner who owns per-book Books on the withdrawn definitions.
	legacy := createAccount(t, h.ctx, h.store, "whole-work-legacy", "learner-password")
	legacyCookies, legacyCSRF := loginCookies(t, h.handler, legacy.Username, "learner-password")
	legacyBooks := make([]string, 0, 2)
	legacyRuns := map[string]string{}
	for _, key := range []string{loopTreatiseKey + "-1", loopTreatiseKey + "-2"} {
		var definition classical.Definition
		for _, candidate := range h.definitions {
			if candidate.Key == key {
				definition = candidate
			}
		}
		require.True(t, definition.Withdrawn, "fixture per-book definition %s is a tombstone", key)
		registration, err := h.store.RegisterReadingUnitBook(h.ctx, legacy.ID, domain.ReadingUnitDefinition{
			Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle,
			WorkAuthor: definition.WorkAuthor, EditionLabel: definition.EditionLabel,
			CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
			SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
			SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
		})
		require.NoError(t, err)
		legacyBooks = append(legacyBooks, registration.BookID)
		legacyRuns[registration.BookID] = h.seedLegacyAnalysis(t, legacy.ID, registration.BookID)
	}

	// The Work page is one Add and a read-only contents list, never per-book rows.
	work := perform(t, h.handler, http.MethodGet, "/catalogs/classical/works/"+loopTreatiseKey, nil, cookies)
	require.Equal(t, http.StatusOK, work.Code, work.Body.String())
	addAction := `/catalogs/classical/units/` + loopTreatiseKey + `/add`
	assert.Equal(t, 1, strings.Count(work.Body.String(), `action="`+addAction+`"`), "one Add for the whole Work")
	assert.Equal(t, 1, strings.Count(work.Body.String(), "/catalogs/classical/units/"+loopTreatiseKey+"/previously-read"))
	assert.Equal(t, 2, strings.Count(work.Body.String(), "/catalogs/classical/units/"), "no per-book Add or I've read this")
	assert.Contains(t, work.Body.String(), "<td>Book 2</td>", "the contents list names each book")
	assert.NotContains(t, work.Body.String(), "Choose books")

	// Add creates one Book and queues exactly one preparation.
	added := perform(t, h.handler, http.MethodPost, addAction, url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books."))
	owned := h.booksByUnit(t, learner.ID)
	require.Len(t, owned, 1)
	book := owned[loopTreatiseKey]
	require.NotEmpty(t, book)
	queues, bookIDs := h.preparationJobs(t)
	require.Equal(t, []string{analysis.ClassicalPreparationQueue}, queues)
	require.Equal(t, []string{book}, bookIDs, "exactly one preparation, for the whole-work Book")

	// My Books shows one ordinary row with no position.
	require.NoError(t, h.store.SetActiveStudyLanguage(h.ctx, learner.ID, classical.Language))
	library := perform(t, h.handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	assert.Equal(t, 1, strings.Count(library.Body.String(), `id="book-row-`+book+`"`))
	assert.Equal(t, 1, strings.Count(library.Body.String(), "Synthetic Treatise"+"</"), "one title for the whole Work")
	for _, retired := range []string{"Book 1 of", "library-work", "Books in", "Move the work", "Remove the work"} {
		assert.NotContains(t, library.Body.String(), retired)
	}

	// The queued preparation publishes once.
	h.startWorkers(t)
	h.waitForBookAnalysis(t, learner.ID, book)
	assert.Equal(t, int64(1), h.sourceServed())
	assert.Equal(t, 1, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	assert.NotEmpty(t, h.preparedDefinitionRunID(t, learner.ID, book))

	// The chooser lists the Work as one ordinary entry.
	chooser := perform(t, h.handler, http.MethodGet, "/grc/reading", nil, cookies)
	require.Equal(t, http.StatusOK, chooser.Code, chooser.Body.String())
	assert.Equal(t, 1, strings.Count(chooser.Body.String(), `id="reading-chooser-book-`+book+`"`))
	assert.Contains(t, chooser.Body.String(), `action="/reading/books/`+book+`/start"`)
	assert.NotContains(t, chooser.Body.String(), "Book 1 of")

	// Concordance shows one group, one count and a Citation on every occurrence.
	query := url.Values{"term": {"λόγος"}, "as": {domain.ConcordanceMatchLemma}}
	concordance := perform(t, h.handler, http.MethodGet, "/grc/vocabulary/concordance?"+query.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, concordance.Code, concordance.Body.String())
	body := concordance.Body.String()
	occurrences := countRows(t, h.pool, `SELECT count(*) FROM corpus_tokens WHERE owner_id=$1 AND canonical_lemma=$2`,
		learner.ID, "λόγοσ")
	require.Positive(t, occurrences)
	assert.Equal(t, 1, strings.Count(body, `class="concordance-source-title"`), "one group for the Book")
	assert.Equal(t, occurrences, strings.Count(body, `class="concordance-row"`), "one result per occurrence")
	assert.Contains(t, body, fmt.Sprintf("%d occurrences on this page", occurrences), "one count for the Book")
	citations := regexp.MustCompile(`<p class="concordance-location">(1\.1\.\d)</p>`).FindAllStringSubmatch(body, -1)
	assert.Len(t, citations, occurrences, "a Citation on every occurrence")

	// The learner with per-book Books still adds the whole Work; the old
	// Books stay ordinary rows and the whole Work is a separate Book.
	require.NoError(t, h.store.SetActiveStudyLanguage(h.ctx, legacy.ID, classical.Language))
	before := countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue)
	legacyAdd := perform(t, h.handler, http.MethodPost, addAction, url.Values{"csrf_token": {legacyCSRF}}, legacyCookies)
	require.Equal(t, http.StatusSeeOther, legacyAdd.Code, legacyAdd.Body.String())
	assert.Contains(t, legacyAdd.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books."))
	legacyOwned := h.booksByUnit(t, legacy.ID)
	require.Len(t, legacyOwned, 3)
	wholeBook := legacyOwned[loopTreatiseKey]
	require.NotEmpty(t, wholeBook)
	assert.NotContains(t, legacyBooks, wholeBook)
	assert.Equal(t, before+1, countRows(t, h.pool, `SELECT count(*) FROM river_job WHERE kind=$1`, analysis.ClassicalPreparationQueue))
	page := perform(t, h.handler, http.MethodGet, "/library", nil, legacyCookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	for _, id := range append([]string{wholeBook}, legacyBooks...) {
		row := myBookRowHTML(t, page.Body.String(), id)
		assert.Contains(t, row, `action="/library/books/`+id+`/remove"`, "every Book is an ordinary row")
		_, err := h.store.GetBookDetail(h.ctx, legacy.ID, id)
		require.NoError(t, err, "the Book stays openable")
	}
	chooserPage := perform(t, h.handler, http.MethodGet, "/grc/reading", nil, legacyCookies)
	require.Equal(t, http.StatusOK, chooserPage.Code, chooserPage.Body.String())
	for _, id := range legacyBooks {
		assert.Contains(t, chooserPage.Body.String(), `action="/reading/books/`+id+`/start"`, "a legacy Book stays readable from the chooser")
		analysisPage := perform(t, h.handler, http.MethodGet, "/books/"+id+"/analyses/"+legacyRuns[id], nil, legacyCookies)
		assert.Equal(t, http.StatusSeeOther, analysisPage.Code, "a legacy Book's analysis stays reachable")
		assert.Equal(t, readingBookURL("grc", id), analysisPage.Header().Get("Location"))
	}
	assert.NotContains(t, page.Body.String(), "Remove the work")
}

// seedLegacyAnalysis gives a registered per-book Book source content and a
// published analysis, and puts it To Read, so its reading surface can be opened.
func (h *classicalLoopHarness) seedLegacyAnalysis(t *testing.T, owner, bookID string) string {
	t.Helper()
	source, err := h.store.PutSourceMaterialWithExtractedUnits(h.ctx, domain.SourceMaterial{OwnerID: owner, Language: classical.Language, SourceIdentifier: "legacy-" + bookID, Title: "Legacy book", MediaType: "application/epub+zip", Content: []byte("λόγος"), FullText: "λόγος"},
		domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "λόγος", EndOffset: 5}}})
	require.NoError(t, err)
	require.NoError(t, h.store.LinkSourceToBook(h.ctx, owner, bookID, source.ID))
	handle, err := h.service.SubmitAnalysis(h.ctx, owner, source.ID)
	require.NoError(t, err)
	artifactHash := "sha256:legacy-" + bookID
	_, err = h.pool.Exec(h.ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,$2,'1','casefold','1','fake','1')`, artifactHash, classical.Language)
	require.NoError(t, err)
	var corpusID string
	require.NoError(t, h.pool.QueryRow(h.ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count) VALUES($1,$2,$3,$4,'complete',1,1,1,1,0,1,1,0) RETURNING id::text`, owner, source.ID, artifactHash, handle.RunID).Scan(&corpusID))
	for _, statement := range []string{
		`UPDATE analysis_runs SET state='completed',corpus_id=$2,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$3`,
		`UPDATE analysis_jobs SET corpus_id=$2,progress=100,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`,
	} {
		_, err = h.pool.Exec(h.ctx, statement, owner, corpusID, handle.RunID)
		require.NoError(t, err)
	}
	_, err = h.pool.Exec(h.ctx, `UPDATE analysis_run_attempts SET state='completed',finalized_at=now() WHERE run_id=$1`, handle.RunID)
	require.NoError(t, err)
	_, err = h.pool.Exec(h.ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, bookID, source.ID, handle.RunID)
	require.NoError(t, err)
	require.NoError(t, h.store.SetBookDisposition(h.ctx, owner, bookID, domain.BookDispositionToRead))
	testutil.SeedCorpusWords(t, h.ctx, h.pool, owner, corpusID, classical.Language, testutil.CorpusWord{Lemma: "λόγοσ", UPOS: "NOUN", Occurrences: 1})
	// The chooser withholds coverage until the Book's count projection is
	// ready. The submitted analysis job would build it only whenever the
	// started workers reach it, so publish it here as that job would.
	rebuild, err := h.pool.Begin(h.ctx)
	require.NoError(t, err)
	require.NoError(t, persistence.BuildVocabularyBrowseCountsTx(h.ctx, rebuild, owner, bookID, source.ID, handle.RunID, corpusID, classical.Language))
	require.NoError(t, rebuild.Commit(h.ctx))
	return handle.RunID
}
