//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentReadingFreezesAndReleasesVocabularySnapshot(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-snapshot", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "snapshot-one")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID)
	require.NoError(t, err)
	analysis := analyzer.Result{Language: "de"}
	for occurrence := range 5 {
		analysis.Sentences = append(analysis.Sentences, analyzer.Sentence{
			Text: "Wir reisen heute.",
			Tokens: []analyzer.Token{{Surface: "reisen", CanonicalLemma: "reisen", UPOS: "VERB", Location: analyzer.SourceLocation{
				SourceDocumentID: source.ID, StartOffset: uint64(occurrence + 4), EndOffset: uint64(occurrence + 10),
			}}},
		})
	}
	selected, err := selection.NewService(store).Select(ctx, owner.ID, analysis, selection.DefaultConfig(corpusID))
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, 5, selected[0].OccurrenceCount)
	assert.Len(t, selected[0].SentenceReferences, 5)

	goal, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, goal.SnapshotID)
	assert.Equal(t, corpusID, goal.CorpusID)
	assert.Equal(t, 1, goal.SnapshotSize)
	reserved, err := store.ListReservedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, reserved, 1)
	assert.Equal(t, "reisen", reserved[0].CanonicalLemma)

	_, err = store.Pool().Exec(ctx, `UPDATE selection_candidates SET occurrence_count=99 WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpusID)
	require.NoError(t, err)
	stored, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, goal.SnapshotID, stored.SnapshotID)
	assert.Equal(t, 1, stored.SnapshotSize)
	snapshotVocabulary, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, goal.SnapshotID)
	require.NoError(t, err)
	require.Len(t, snapshotVocabulary, 1)
	assert.Equal(t, 5, snapshotVocabulary[0].OccurrenceCount)
	var frozenReferences []selection.SentenceReference
	require.NoError(t, json.Unmarshal(snapshotVocabulary[0].SentenceReferences, &frozenReferences))
	require.Len(t, frozenReferences, 5)
	assert.Equal(t, "Wir reisen heute.", frozenReferences[0].Text)

	err = store.EndCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	reserved, err = store.ListReservedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reserved)
	var released int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner.ID, goal.SnapshotID).Scan(&released)
	require.NoError(t, err)
	assert.Equal(t, 1, released)
}

func TestCurrentReadingFreezesCorpusQualifiedTwoOccurrenceCandidates(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-corpus-qualified", false)
	require.NoError(t, err)
	target, targetSource, _ := createReadingFixture(t, ctx, store, owner.ID, "corpus-qualified-target")
	makeAnalyzedToReadBook(t, ctx, store, target, targetSource)
	other, otherSource, _ := createReadingFixture(t, ctx, store, owner.ID, "corpus-qualified-other")
	makeAnalyzedToReadBook(t, ctx, store, other, otherSource)
	italian, italianSource, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "it", "corpus-qualified-italian")
	makeAnalyzedToReadBook(t, ctx, store, italian, italianSource)
	otherOwner, err := store.CreateUser(ctx, "goal-corpus-qualified-other-owner", false)
	require.NoError(t, err)
	foreign, foreignSource, _ := createReadingFixture(t, ctx, store, otherOwner.ID, "corpus-qualified-foreign")
	makeAnalyzedToReadBook(t, ctx, store, foreign, foreignSource)

	for _, candidate := range []struct {
		lemma string
		upos  string
		count int64
	}{
		{"three-local", "NOUN", 3},
		{"nine-total", "NOUN", 2},
		{"ten-total", "NOUN", 2},
		{"generated-total", "NOUN", 2},
		{"known-total", "NOUN", 2},
		{"pos-total", "NOUN", 2},
		{"singleton", "NOUN", 1},
	} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance)
VALUES($1,(SELECT corpus_id FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2),'de',$3,$4,$5,'[]','[]','{}')`, owner.ID, target.ID, candidate.lemma, candidate.upos, candidate.count)
		require.NoError(t, err)
	}
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "known-total", "")
	require.NoError(t, err)
	generatedDeck, err := store.PutDeck(ctx, owner.ID, "de", "Generated provenance")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{
		OwnerID: owner.ID, Language: "de", CanonicalLemma: "generated-total", UPOS: "NOUN",
		FirstDeckID: generatedDeck.ID, FirstSourceMaterialID: &targetSource.ID,
	})
	require.NoError(t, err)

	type countRow struct {
		lemma string
		pos   string
		count int64
	}
	putProjection := func(book domain.Book, rows []countRow) {
		t.Helper()
		var run, corpus string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, book.OwnerID, book.ID).Scan(&run, &corpus))
		for _, item := range rows {
			_, insertErr := store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count)
	VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, book.OwnerID, book.ID, book.LanguageTag, run, corpus, item.lemma, item.pos, item.count)
			require.NoError(t, insertErr)
		}
		_, insertErr := store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,$3,$4,$5,2)`, book.OwnerID, book.ID, book.LanguageTag, run, corpus)
		require.NoError(t, insertErr)
	}
	putProjection(target, []countRow{
		{lemma: "three-local", pos: "NOUN", count: 3},
		{lemma: "nine-total", pos: "NOUN", count: 2},
		{lemma: "ten-total", pos: "NOUN", count: 2},
		{lemma: "generated-total", pos: "NOUN", count: 2},
		{lemma: "known-total", pos: "NOUN", count: 2},
		{lemma: "pos-total", pos: "NOUN", count: 2},
	})
	putProjection(other, []countRow{
		{lemma: "nine-total", pos: "NOUN", count: 7},
		{lemma: "ten-total", pos: "NOUN", count: 8},
		{lemma: "generated-total", pos: "NOUN", count: 8},
		{lemma: "known-total", pos: "NOUN", count: 8},
		{lemma: "pos-total", pos: "VERB", count: 8},
	})
	putProjection(italian, []countRow{{lemma: "nine-total", pos: "NOUN", count: 100}})
	putProjection(foreign, []countRow{{lemma: "nine-total", pos: "NOUN", count: 100}})

	goal, err := store.StartCurrentReading(ctx, owner.ID, "de", target.ID)
	require.NoError(t, err)
	snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, goal.SnapshotID)
	require.NoError(t, err)
	identities := make(map[string]int, len(snapshot))
	for _, candidate := range snapshot {
		identities[candidate.CanonicalLemma] = candidate.OccurrenceCount
	}
	assert.Equal(t, map[string]int{"three-local": 3, "ten-total": 2, "generated-total": 2}, identities,
		"nine does not meet the total threshold; language, POS, Known, owner, and Generated-provenance boundaries remain exact")
}

func TestCurrentReadingCrossBookReadinessIsAtomicAndOnlyNeededForTwoOccurrenceCandidates(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-count-readiness", false)
	require.NoError(t, err)
	current, currentSource, _ := createReadingFixture(t, ctx, store, owner.ID, "count-readiness-current")
	makeAnalyzedToReadBook(t, ctx, store, current, currentSource)
	var currentCorpus string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, current.ID).Scan(&currentCorpus))
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','three-local','NOUN',3,'[]','[]','{}')`, owner.ID, currentCorpus)
	require.NoError(t, err)
	other, otherSource, _ := createReadingFixture(t, ctx, store, owner.ID, "count-readiness-other")
	makeAnalyzedToReadBook(t, ctx, store, other, otherSource)
	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", current.ID)
	require.NoError(t, err, "a three-occurrence-only freeze does not wait for another Book's count projection")

	target, targetSource, _ := createReadingFixture(t, ctx, store, owner.ID, "count-readiness-target")
	makeAnalyzedToReadBook(t, ctx, store, target, targetSource)
	var targetCorpus, targetRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT corpus_id::text,analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, target.ID).Scan(&targetCorpus, &targetRun))
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','crossing','NOUN',2,'[]','[]','{}')`, owner.ID, targetCorpus)
	require.NoError(t, err)

	_, err = store.SwitchCurrentReading(ctx, owner.ID, "de", target.ID, current.ID, reading.SnapshotID)
	require.ErrorIs(t, err, ErrVocabularyBrowseCountsPending)
	stillCurrent, getErr := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, getErr)
	assert.Equal(t, reading, stillCurrent, "an incomplete count projection cannot partially switch Reading")

	_, err = store.Pool().Exec(ctx, `INSERT INTO river_job(kind,args,queue,state,max_attempts,finalized_at)
VALUES('rebuild_vocabulary_browse_counts',jsonb_build_object('owner_id',$1::uuid,'book_id',$2::uuid,'run_id',$3::uuid),'default','discarded',1,now())`, owner.ID, target.ID, targetRun)
	require.NoError(t, err)
	_, err = store.SwitchCurrentReading(ctx, owner.ID, "de", target.ID, current.ID, reading.SnapshotID)
	require.ErrorIs(t, err, ErrVocabularyBrowseCountsUnavailable)
	stillCurrent, getErr = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, getErr)
	assert.Equal(t, reading, stillCurrent, "an unavailable count projection leaves the former reading intact")

	type countItem struct {
		lemma string
		count int64
	}
	putReady := func(book domain.Book, items ...countItem) {
		t.Helper()
		var run, corpus string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&run, &corpus))
		for _, item := range items {
			_, insertErr := store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count) VALUES($1,$2,'de',$3,$4,$5,'NOUN',$6)`, owner.ID, book.ID, run, corpus, item.lemma, item.count)
			require.NoError(t, insertErr)
		}
		_, insertErr := store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,'de',$3,$4,2)`, owner.ID, book.ID, run, corpus)
		require.NoError(t, insertErr)
	}
	putReady(current, countItem{lemma: "three-local", count: 3})
	putReady(other, countItem{lemma: "crossing", count: 8})
	putReady(target, countItem{lemma: "crossing", count: 2})
	// Remove the discarded terminal job to model a successful durable rebuild.
	_, err = store.Pool().Exec(ctx, `DELETE FROM river_job WHERE kind='rebuild_vocabulary_browse_counts' AND args->>'owner_id'=$1 AND args->>'book_id'=$2 AND args->>'run_id'=$3`, owner.ID, target.ID, targetRun)
	require.NoError(t, err)

	switched, err := store.SwitchCurrentReading(ctx, owner.ID, "de", target.ID, current.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, target.ID, switched.BookID)
	snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, switched.SnapshotID)
	require.NoError(t, err)
	require.Len(t, snapshot, 1)
	assert.Equal(t, "crossing", snapshot[0].CanonicalLemma)
	assert.Equal(t, 2, snapshot[0].OccurrenceCount)
}

func TestCurrentReadingCompletionGraduatesFrozenVocabularyWithProvenance(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-graduation", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "graduation")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID)
	require.NoError(t, err)
	for _, candidate := range []struct {
		lemma string
		upos  string
	}{
		{lemma: "reisen", upos: "VERB"},
		{lemma: "bleiben", upos: "VERB"},
		{lemma: "exact-known", upos: "NOUN"},
		{lemma: "known-before-completion", upos: "VERB"},
		{lemma: "known-other-pos", upos: "VERB"},
	} {
		_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
			OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: candidate.lemma, UPOS: candidate.upos,
			OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
		})
		require.NoError(t, err)
	}
	goal, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "known-before-completion", "VERB")
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "known-other-pos", "NOUN")
	require.NoError(t, err)
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Graduation provenance deck")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{
		OwnerID: owner.ID, Language: "de", CanonicalLemma: "bleiben", UPOS: "VERB",
		FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID,
	})
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "reisen", "")
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "exact-known", "NOUN")
	require.NoError(t, err)
	eligibleCount, err := store.CountCurrentReadingVocabularyToAccept(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, 2, eligibleCount)

	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_goal_graduation_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced goal completion failure'; END; $$;
CREATE TRIGGER test_goal_graduation_failure
BEFORE UPDATE ON book_dispositions
FOR EACH ROW EXECUTE FUNCTION test_goal_graduation_failure();`)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.Error(t, err)
	var graduatedAfterRollback int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='bleiben'`, owner.ID).Scan(&graduatedAfterRollback)
	require.NoError(t, err)
	assert.Zero(t, graduatedAfterRollback)
	goalAfterRollback, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, goalAfterRollback.BookID)
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_goal_graduation_failure ON book_dispositions; DROP FUNCTION test_goal_graduation_failure();`)
	require.NoError(t, err)

	result, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, goal.SnapshotID, result.Completion.SnapshotID)
	assert.Equal(t, 5, result.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 2, result.Completion.EligibleVocabularyCount)
	assert.Equal(t, 2, result.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 3, result.Completion.AlreadyKnownVocabularyCount)

	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, known, 6)
	var knownBeforeCompletionCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='known-before-completion'`, owner.ID).Scan(&knownBeforeCompletionCount)
	require.NoError(t, err)
	assert.Equal(t, 1, knownBeforeCompletionCount)
	var completionBook, completionSnapshot, completionAnalysis string
	var completionAt, generatedAt *string
	err = store.Pool().QueryRow(ctx, `
SELECT completion_book_id::text, completion_goal_snapshot_id::text, completion_analysis_run_id::text,
       completion_at::text, generated_first_at::text
FROM known_vocabulary
WHERE owner_id=$1 AND canonical_lemma='bleiben'`, owner.ID).Scan(&completionBook, &completionSnapshot, &completionAnalysis, &completionAt, &generatedAt)
	require.NoError(t, err)
	assert.Equal(t, book.ID, completionBook)
	assert.Equal(t, goal.SnapshotID, completionSnapshot)
	assert.NotEmpty(t, completionAnalysis)
	assert.NotNil(t, completionAt)
	assert.NotNil(t, generatedAt)
	knownByLemma := make(map[string]domain.KnownVocabulary, len(known))
	for _, item := range known {
		knownByLemma[item.CanonicalLemma] = item
	}
	assert.Equal(t, "Accepted on Primary Goal completion", knownByLemma["bleiben"].Provenance)

	assert.NotEmpty(t, result.Completion.BookID)
	repeated, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, repeated.Completion)
	reserved, err := store.ListReservedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reserved)
}

func TestCurrentReadingCompletionAcceptsEmptySnapshot(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-empty-completion", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "empty-completion")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	goal, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Zero(t, goal.SnapshotSize)

	result, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 0, result.Completion.EligibleVocabularyCount)
	assert.Equal(t, 0, result.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 0, result.Completion.AlreadyKnownVocabularyCount)
	assert.NotEmpty(t, result.Completion.SnapshotID)
}

func TestCurrentReadingCompletionHandlesMissingSnapshotIdempotently(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-missing-snapshot", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "goal-missing-snapshot-other", false)
	require.NoError(t, err)

	book, _, _ := createReadingFixture(t, ctx, store, owner.ID, "missing-snapshot")
	italianBook, _, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "it", "missing-snapshot-italian")
	otherBook, _, _ := createReadingFixture(t, ctx, store, otherOwner.ID, "missing-snapshot-other")
	for _, item := range []struct {
		owner, language, book string
	}{
		{owner.ID, "de", book.ID},
		{owner.ID, "it", italianBook.ID},
		{otherOwner.ID, "de", otherBook.ID},
	} {
		require.NoError(t, store.SetBookDisposition(ctx, item.owner, item.book, domain.BookDispositionToRead))
	}
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id, language, book_id) VALUES ($1, 'de', $2), ($1, 'it', $3), ($4, 'de', $5)`, owner.ID, book.ID, italianBook.ID, otherOwner.ID, otherBook.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan domain.CurrentReadingFinishResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			result, finishErr := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, "")
			results <- result
			errors <- finishErr
		}()
	}
	close(start)
	for range 2 {
		require.NoError(t, <-errors)
	}
	first, second := <-results, <-results
	assert.Equal(t, first.Completion, second.Completion)
	assert.Empty(t, first.Completion.SnapshotID)
	assert.Zero(t, first.Completion.SnapshotVocabularyCount)
	assert.Zero(t, first.Completion.EligibleVocabularyCount)
	assert.Zero(t, first.Completion.GraduatedVocabularyCount)
	assert.Zero(t, first.Completion.AlreadyKnownVocabularyCount)

	repeated, err := store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, "")
	require.NoError(t, err)
	assert.Equal(t, first.Completion, repeated.Completion)
	var historyCount, knownCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&knownCount)
	require.NoError(t, err)
	assert.Zero(t, knownCount)

	goal, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID)
	italianGoal, err := store.GetCurrentReading(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Equal(t, italianBook.ID, italianGoal.BookID, "completion crossed the language boundary")
	otherGoal, err := store.GetCurrentReading(ctx, otherOwner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, otherBook.ID, otherGoal.BookID, "completion crossed the owner boundary")
}
