//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVocabularyBrowseCountProjectionIsReadyForNonemptyAndEmptyBooks(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-count-owner", false)
	require.NoError(t, err)

	for _, fixture := range []struct {
		name      string
		text      string
		tokens    []concordanceToken
		wantRows  int
		wantCount int64
	}{
		{name: "nonempty", text: "Haus Haus", tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 5, End: 9},
		}, wantRows: 1, wantCount: 3},
		{name: "empty", text: "no eligible tokens", wantRows: 0},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			units := []domain.ExtractedUnit{concordanceUnit(0, "browse-count-"+fixture.name, fixture.text, 0, uint64(len(fixture.text)))}
			if fixture.wantCount > 0 {
				units = append(units, concordanceUnit(1, "browse-count-ancillary-"+fixture.name, "Haus", 11, 15))
			}
			book, source := createConcordanceBook(t, ctx, store, owner.ID, "Browse count "+fixture.name, "browse-count-"+fixture.name, false, units)
			var sentences []concordanceSentence
			if len(fixture.tokens) > 0 {
				sentences = []concordanceSentence{{
					UnitID: "epub-unit-v1:0:browse-count-" + fixture.name, Ordinal: 0,
					Text: fixture.text, Start: 0, End: int64(len(fixture.text)), Tokens: fixture.tokens,
				}}
				sentences = append(sentences, concordanceSentence{
					UnitID: "epub-unit-v1:1:browse-count-ancillary-" + fixture.name, Ordinal: 1,
					Text: "Haus", Start: 11, End: 15,
					Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 11, End: 15}},
				})
			}
			insertConcordanceAnalysis(t, ctx, store, source, false, sentences)
			var runID, corpusID string
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT r.id::text,r.corpus_id::text FROM analysis_runs r WHERE r.owner_id=$1 AND r.source_material_id=$2`, owner.ID, source.ID).Scan(&runID, &corpusID))
			tx, beginErr := store.Pool().Begin(ctx)
			require.NoError(t, beginErr)
			require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
			_, err = tx.Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner.ID, book.ID, source.ID, runID)
			require.NoError(t, err)
			require.NoError(t, tx.Commit(ctx))

			var readyRun, readyCorpus string
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&readyRun, &readyCorpus))
			assert.Equal(t, runID, readyRun)
			assert.Equal(t, corpusID, readyCorpus)
			var count int
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&count))
			assert.Equal(t, fixture.wantRows, count)
			page, browseErr := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
			require.NoError(t, browseErr)
			assert.Len(t, page.Rows, fixture.wantRows)
			assert.Equal(t, int64(fixture.wantRows), page.InventoryTotal)
			if fixture.wantRows > 0 {
				var occurrences int64
				var projectedLanguage string
				require.NoError(t, store.Pool().QueryRow(ctx, `SELECT language,occurrence_count FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2 AND canonical_lemma='haus' AND upos='NOUN'`, owner.ID, book.ID).Scan(&projectedLanguage, &occurrences))
				assert.Equal(t, "de", projectedLanguage)
				assert.Equal(t, fixture.wantCount, occurrences)
				assert.Equal(t, fixture.wantCount, page.Rows[0].OccurrenceCount)

				otherOwner, userErr := store.CreateUser(ctx, "browse-count-other-owner", false)
				require.NoError(t, userErr)
				otherBook, otherSource := createConcordanceBook(t, ctx, store, otherOwner.ID, "Other owner", "browse-count-other-owner", false, []domain.ExtractedUnit{
					concordanceUnit(0, "browse-count-other-owner", "Haus", 0, 4),
				})
				insertConcordanceAnalysis(t, ctx, store, otherSource, true, []concordanceSentence{{
					UnitID: "epub-unit-v1:0:browse-count-other-owner", Ordinal: 0, Text: "Haus", Start: 0, End: 4,
					Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}},
				}})
				var otherRun, otherCorpus string
				require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, otherOwner.ID, otherBook.ID).Scan(&otherRun, &otherCorpus))
				otherTx, txErr := store.Pool().Begin(ctx)
				require.NoError(t, txErr)
				require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, otherTx, otherOwner.ID, otherBook.ID, otherSource.ID, otherRun, otherCorpus, "de"))
				require.NoError(t, otherTx.Commit(ctx))

				aliceAfterOtherOwner, browseErr := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
				require.NoError(t, browseErr)
				require.Len(t, aliceAfterOtherOwner.Rows, 1)
				assert.Equal(t, fixture.wantCount, aliceAfterOtherOwner.Rows[0].AcrossBooksOccurrenceCount, "another owner's matching projection must not leak into this Browse inventory")
				wrongLanguage, browseErr := store.ListVocabularyBrowsePage(ctx, owner.ID, "it", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
				require.NoError(t, browseErr)
				assert.Empty(t, wrongLanguage.Rows, "German projection rows must not appear in another study language")
			}
		})
	}
}

func TestVocabularyBrowseCountsTrackReadyOccurrenceDecisionsAtomically(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-decision-count-owner", false)
	require.NoError(t, err)
	units := []domain.ExtractedUnit{concordanceUnit(0, "browse-decision-count", "Haus Haus Haus Baum Baum", 0, 24)}
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Browse decision counts", "browse-decision-count", false, units)
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:browse-decision-count", Ordinal: 0, Text: "Haus Haus Haus Baum Baum", Start: 0, End: 24,
		Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 5, End: 9},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 10, End: 14},
			{Surface: "Baum", Lemma: "baum", Upos: "NOUN", Start: 15, End: 19},
			{Surface: "Baum", Lemma: "baum", Upos: "NOUN", Start: 20, End: 24},
		},
	}})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))
	projectionTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, projectionTx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, projectionTx.Commit(ctx))

	getOccurrences := func() []domain.LemmaReviewOccurrence {
		occurrences, listErr := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Haus")
		require.NoError(t, listErr)
		return occurrences
	}
	checkCounts := func(want map[string]int64) {
		t.Helper()
		page, browseErr := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
		require.NoError(t, browseErr)
		assert.False(t, page.BrowseCountsUpdating)
		got := make(map[string]int64, len(page.Rows))
		for _, row := range page.Rows {
			got[row.CanonicalLemma] = row.OccurrenceCount
			assert.Equal(t, row.OccurrenceCount, row.AcrossBooksOccurrenceCount)
		}
		assert.Equal(t, want, got)
	}
	checkCounts(map[string]int64{"haus": 3, "baum": 2})

	occurrences := getOccurrences()
	require.Len(t, occurrences, 3)
	corrected := occurrences[0]
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{
		Occurrence: corrected, CanonicalLemma: "gebäude", NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
	}}))
	checkCounts(map[string]int64{"haus": 2, "gebäude": 1, "baum": 2})

	occurrences = getOccurrences()
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[2], Excluded: true}}))
	checkCounts(map[string]int64{"haus": 1, "gebäude": 1, "baum": 2})

	occurrences = getOccurrences()
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[2], CanonicalLemma: "haus"}}))
	checkCounts(map[string]int64{"haus": 2, "gebäude": 1, "baum": 2})

	occurrences = getOccurrences()
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "haus"}}))
	checkCounts(map[string]int64{"haus": 3, "baum": 2})

	occurrences = getOccurrences()
	stale := append([]domain.LemmaReviewOccurrence(nil), occurrences[:2]...)
	proposal := []domain.LemmaReviewDecision{
		{Occurrence: occurrences[0], CanonicalLemma: "heim", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
		{Occurrence: occurrences[1], CanonicalLemma: "heim", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
	}
	require.NoError(t, store.PutLemmaDecisions(ctx, proposal))
	checkCounts(map[string]int64{"haus": 1, "heim": 2, "baum": 2})
	latest := getOccurrences()
	retry := []domain.LemmaReviewDecision{
		{Occurrence: latest[0], CanonicalLemma: "heim", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
		{Occurrence: latest[1], CanonicalLemma: "heim", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"},
	}
	require.NoError(t, store.PutLemmaDecisions(ctx, retry), "replaying the already-applied decision must not double-count")
	checkCounts(map[string]int64{"haus": 1, "heim": 2, "baum": 2})
	staleReplay := []domain.LemmaReviewDecision{
		{Occurrence: stale[0], CanonicalLemma: "haus"},
		{Occurrence: stale[1], CanonicalLemma: "haus"},
	}
	require.ErrorIs(t, store.PutLemmaDecisions(ctx, staleReplay), ErrNotFound, "a stale multi-occurrence replay must roll back without changing counts")
	checkCounts(map[string]int64{"haus": 1, "heim": 2, "baum": 2})

	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	occurrences = getOccurrences()
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "haus"}}))
	pending, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.True(t, pending.BrowseCountsUpdating, "a decision made without ready counts invalidates the old projection until its rebuild completes")
	assert.Empty(t, pending.Rows, "an unready projection never leaks a stale or partial count")
}

func buildBrowseProjectionsForOwner(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string) {
	t.Helper()
	rows, err := store.Pool().Query(ctx, `SELECT cai.book_id::text,cai.source_material_id::text,cai.analysis_run_id::text,cai.corpus_id::text,s.language
		FROM current_analysis_identity cai JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		WHERE cai.owner_id=$1`, ownerID)
	require.NoError(t, err)
	type currentBook struct{ bookID, sourceID, runID, corpusID, language string }
	var books []currentBook
	for rows.Next() {
		var book currentBook
		require.NoError(t, rows.Scan(&book.bookID, &book.sourceID, &book.runID, &book.corpusID, &book.language))
		books = append(books, book)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	for _, book := range books {
		tx, beginErr := store.Pool().Begin(ctx)
		require.NoError(t, beginErr)
		require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, ownerID, book.bookID, book.sourceID, book.runID, book.corpusID, book.language))
		require.NoError(t, tx.Commit(ctx))
	}
}

func browseCountRows(t *testing.T, ctx context.Context, store *PostgresStore, ownerID, bookID string) map[string]int64 {
	t.Helper()
	rows, err := store.Pool().Query(ctx, `SELECT canonical_lemma||'/'||upos,occurrence_count FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, ownerID, bookID)
	require.NoError(t, err)
	defer rows.Close()
	got := map[string]int64{}
	for rows.Next() {
		var key string
		var count int64
		require.NoError(t, rows.Scan(&key, &count))
		got[key] = count
	}
	require.NoError(t, rows.Err())
	return got
}

func TestVocabularyBrowseCountsMatchSelectionProjection(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-parity-owner", false)
	require.NoError(t, err)
	text := "Haus Haus Haus Ärger Ärger fängt an 123 Baum Baum Baum"
	const textLength = 54 // runes in text
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Browse parity", "browse-parity", false,
		[]domain.ExtractedUnit{concordanceUnit(0, "browse-parity", text, 0, textLength)})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:browse-parity", Ordinal: 0, Text: text, Start: 0, End: textLength,
		Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 5, End: 9},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 10, End: 14},
			{Surface: "Ärger", Lemma: "ärger", Upos: "NOUN", Start: 15, End: 21},
			{Surface: "Ärger", Lemma: " ärger ", Upos: " noun ", Start: 22, End: 28},
			{Surface: "fängt", Lemma: "anfangen", Upos: "VERB", Start: 29, End: 34},
			{Surface: "an", Lemma: "an", Upos: "VERB", Dependency: "compound:prt", Start: 35, End: 37},
			{Surface: "123", Lemma: "123", Upos: "NOUN", Start: 38, End: 41},
			{Surface: "Baum", Lemma: "baum", Upos: "NOUN", Start: 42, End: 46},
			{Surface: "Baum", Lemma: "baum", Upos: "NOUN", Start: 47, End: 51},
			{Surface: "Baum", Lemma: "baum", Upos: "NOUN", Start: 52, End: 56},
		},
	}})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))
	insertDecision := func(start, end int64, lemma *string, excluded bool) {
		_, execErr := store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded)
			VALUES($1,$2,$3,$4,'epub-unit-v1:0:browse-parity',$5,$6,$7,CASE WHEN $8 THEN NULL ELSE 'german-post-1996' END,CASE WHEN $8 THEN NULL ELSE '6' END,$8)`, owner.ID, book.ID, corpusID, runID, start, end, lemma, excluded)
		require.NoError(t, execErr)
	}
	corrected, nonLetter := "gebäude", "123"
	insertDecision(10, 14, &corrected, false)
	insertDecision(22, 28, &nonLetter, false)
	insertDecision(42, 46, nil, true)

	rebuild := func() {
		tx, beginErr := store.Pool().Begin(ctx)
		require.NoError(t, beginErr)
		require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
		require.NoError(t, tx.Commit(ctx))
	}
	wantFromProject := func() map[string]int64 {
		tx, beginErr := store.Pool().Begin(ctx)
		require.NoError(t, beginErr)
		defer func() { require.NoError(t, txcleanup.Rollback(ctx, tx)) }()
		analysis, corrections, loadErr := loadAnalysisProjectionFactsTx(ctx, tx, sqlcgen.New(tx), domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID}, corpusID)
		require.NoError(t, loadErr)
		var decisions []selection.OccurrenceDecision
		for _, c := range corrections {
			start, startErr := checked.Uint64FromInt64(c.StartOffset)
			end, endErr := checked.Uint64FromInt64(c.EndOffset)
			require.NoError(t, errors.Join(startErr, endErr))
			decisions = append(decisions, selection.OccurrenceDecision{
				Occurrence: selection.OccurrenceIdentity{SourceDocumentID: c.SourceDocumentID, StartOffset: start, EndOffset: end},
				Lemma:      c.CanonicalLemma, Excluded: c.Excluded,
			})
		}
		candidates, projectErr := selection.Project(analysis, selection.DefaultConfig(corpusID), decisions)
		require.NoError(t, projectErr)
		want := map[string]int64{}
		for _, c := range candidates {
			want[c.Identity.CanonicalLemma+"/"+c.Identity.UPOS] = int64(c.OccurrenceCount)
		}
		return want
	}
	rebuild()
	want := wantFromProject()
	assert.Equal(t, map[string]int64{"haus/NOUN": 2, "gebäude/NOUN": 1, "ärger/NOUN": 1, "anfangen/VERB": 1, "baum/NOUN": 2}, want)
	assert.Equal(t, want, browseCountRows(t, ctx, store, owner.ID, book.ID), "built counts equal selection.Project over the same facts")

	// A correction and an exclusion update the affected identities in their
	// own transaction and match a full rebuild.
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Baum")
	require.NoError(t, err)
	var live domain.LemmaReviewOccurrence
	for _, o := range occurrences {
		if o.StartOffset == 47 {
			live = o
		}
	}
	require.NotEmpty(t, live.SourceDocumentID)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: live, CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	incremental := browseCountRows(t, ctx, store, owner.ID, book.ID)
	assert.Equal(t, map[string]int64{"haus/NOUN": 3, "gebäude/NOUN": 1, "ärger/NOUN": 1, "anfangen/VERB": 1, "baum/NOUN": 1}, incremental)
	rebuild()
	assert.Equal(t, incremental, browseCountRows(t, ctx, store, owner.ID, book.ID))
	assert.Equal(t, wantFromProject(), incremental)

	occurrences, err = store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Baum")
	require.NoError(t, err)
	for _, o := range occurrences {
		if o.StartOffset == 52 {
			live = o
		}
	}
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: live, Excluded: true}}))
	incremental = browseCountRows(t, ctx, store, owner.ID, book.ID)
	assert.NotContains(t, incremental, "baum/NOUN", "an identity with no remaining occurrences is removed")
	rebuild()
	assert.Equal(t, incremental, browseCountRows(t, ctx, store, owner.ID, book.ID))
}

func TestVocabularyBrowseBuilderVersionBumpMakesOldProjectionsUpdatingUntilRebuilt(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-version-owner", false)
	require.NoError(t, err)
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Browse version", "browse-version", false,
		[]domain.ExtractedUnit{concordanceUnit(0, "browse-version", "Haus", 0, 4)})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:browse-version", Ordinal: 0, Text: "Haus", Start: 0, End: 4,
		Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}},
	}})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count) VALUES($1,$2,'de',$3,$4,'stale','NOUN',9)`, owner.ID, book.ID, runID, corpusID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,'de',$3,$4,$5)`, owner.ID, book.ID, runID, corpusID, vocabularyBrowseCountBuilderVersion-1)
	require.NoError(t, err)

	query := domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1}
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", query)
	require.NoError(t, err)
	assert.True(t, page.BrowseCountsUpdating)
	assert.Empty(t, page.Rows)

	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))
	page, err = store.ListVocabularyBrowsePage(ctx, owner.ID, "de", query)
	require.NoError(t, err)
	assert.False(t, page.BrowseCountsUpdating)
	require.Len(t, page.Rows, 1)
	assert.Equal(t, "haus", page.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(1), page.Rows[0].OccurrenceCount)
}
