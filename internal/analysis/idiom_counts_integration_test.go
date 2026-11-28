//go:build integration

package analysis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"
)

// fakeIdiomIndex serves idiom entries by component word.
type fakeIdiomIndex struct{ entries []idiom.Entry }

func (f fakeIdiomIndex) IdiomEntries(_ context.Context, _ string, word string) ([]idiom.Entry, error) {
	var found []idiom.Entry
	for _, entry := range f.entries {
		for _, w := range entry.Words {
			if w == strings.ToLower(word) {
				found = append(found, entry)
				break
			}
		}
	}
	return found, nil
}

func idiomEntry(headword string) idiom.Entry {
	return idiom.Entry{Language: "de", Headword: headword, UPOS: "VERB", Words: strings.Fields(strings.ToLower(headword)), Source: "test", Version: "1"}
}

type idiomCountFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	store   *persistence.PostgresStore
	service *Service
	owner   string
	book    string
	run     string
	corpus  string
	unit    string
	next    int
	words   map[int][]string
}

func newIdiomCountFixture(t *testing.T) *idiomCountFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, "idiom-counts", false)
	require.NoError(t, err)
	book, _ := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	f := &idiomCountFixture{ctx: ctx, pool: pool, store: store, owner: owner.ID, book: book.ID}
	require.NoError(t, pool.QueryRow(ctx, `SELECT cai.analysis_run_id::text,cai.corpus_id::text,u.unit_id
		FROM current_analysis_identity cai JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2 LIMIT 1`, f.owner, f.book).Scan(&f.run, &f.corpus, &f.unit))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	f.service = NewService(pool, client)
	return f
}

// sentence stores one analysed sentence; every word is both its surface and its lemma.
func (f *idiomCountFixture) sentence(t *testing.T, words ...string) {
	t.Helper()
	f.sentenceIn(t, f.book, f.run, f.corpus, f.unit, words...)
}

func (f *idiomCountFixture) sentenceIn(t *testing.T, _, run, corpus, unit string, words ...string) {
	t.Helper()
	ordinal := f.next
	f.next++
	if f.words == nil {
		f.words = map[int][]string{}
	}
	f.words[ordinal] = words
	_, err := f.pool.Exec(f.ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, f.owner, run, corpus, unit, ordinal, strings.Join(words, " "), ordinal*1000, ordinal*1000+999)
	require.NoError(t, err)
	offset := ordinal * 1000
	for i, word := range words {
		_, err = f.pool.Exec(f.ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset)
			VALUES($1,'de',$2,$3,$4,$5,$6,$6,$6,'VERB','root',0,'{}',$7,$8)`, f.owner, run, corpus, ordinal, i, word, offset+i*10, offset+i*10+5)
		require.NoError(t, err)
	}
}

// rebuild derives the Book's counts as the job does and reports the readiness.
func (f *idiomCountFixture) rebuild(t *testing.T, index idiom.Index, indexVersion string) {
	t.Helper()
	worker := &IdiomCountsRebuildWorker{Pool: f.pool, Index: index, IndexVersion: indexVersion}
	require.NoError(t, worker.Work(f.ctx, &river.Job[IdiomCountsRebuildArgs]{Args: IdiomCountsRebuildArgs{OwnerID: f.owner, BookID: f.book, RunID: f.run}}))
}

func (f *idiomCountFixture) corpusCounts(t *testing.T) map[string]int64 {
	t.Helper()
	return f.corpusCountsAt(t, "v1")
}

func (f *idiomCountFixture) pending(t *testing.T, version string) int64 {
	t.Helper()
	counts, err := f.store.ListCorpusIdiomCandidateCounts(f.ctx, f.owner, "de", version)
	require.NoError(t, err)
	return counts.PendingBooks
}

func (f *idiomCountFixture) corpusCountsAt(t *testing.T, version string) map[string]int64 {
	t.Helper()
	counts, err := f.store.ListCorpusIdiomCandidateCounts(f.ctx, f.owner, "de", version)
	require.NoError(t, err)
	result := make(map[string]int64, len(counts.Counts))
	for _, count := range counts.Counts {
		result[count.Headword] = count.Candidates
	}
	return result
}

func (f *idiomCountFixture) jobs(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND state IN ('available','pending','scheduled','retryable','running') AND args->>'book_id'=$2`,
		(IdiomCountsRebuildArgs{}).Kind(), f.book).Scan(&n))
	return n
}

var spoon = fakeIdiomIndex{entries: []idiom.Entry{idiomEntry("den Löffel abgeben")}}

func TestIdiomCountsRecordEachHeadwordsCandidates(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "gibt", "den", "löffel", "ab") // no match: "abgeben" is not here
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	f.sentence(t, "sie", "will", "den", "löffel", "abgeben")
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 2}, f.corpusCounts(t))
}

func TestIdiomCountsRecordNothingForABookWithoutCandidates(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "guten", "tag")
	f.rebuild(t, spoon, "v1")
	require.Empty(t, f.corpusCounts(t))
	var ready int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM idiom_candidate_count_readiness WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book).Scan(&ready))
	require.Equal(t, 1, ready, "a Book without candidates is still ready")
}

func TestIdiomCountsFollowLemmaCorrectionsAndExclusions(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	f.sentence(t, "er", "gab", "den", "löffel", "ab", "gabe")
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCounts(t))

	decide := func(sentence, token int, lemma string, excluded bool) {
		var start, end int
		require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT start_offset,end_offset FROM corpus_tokens WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=$3 AND token_ordinal=$4`,
			f.owner, f.run, sentence, token).Scan(&start, &end))
		var lemmaArg *string
		if lemma != "" {
			lemmaArg = &lemma
		}
		_, err := f.pool.Exec(f.ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $9 THEN NULL ELSE 'de' END,CASE WHEN $9 THEN NULL ELSE '1' END,$9)`, f.owner, f.book, f.corpus, f.run, f.unit, start, end, lemmaArg, excluded)
		require.NoError(t, err)
	}
	// Correcting "ab" in the second sentence to "abgeben" makes it a match.
	decide(1, 4, "abgeben", false)
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 2}, f.corpusCounts(t))
	// Excluding the first sentence's "löffel" removes its match.
	decide(0, 3, "", true)
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCounts(t))
}

func TestIdiomCountsFollowTheIdiomIndex(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	f.rebuild(t, fakeIdiomIndex{}, "v1")
	require.Empty(t, f.corpusCounts(t))
	require.NoError(t, f.service.EnqueueMissingIdiomCountRebuilds(f.ctx, "v1"))
	require.Zero(t, f.jobs(t), "counts built against this index version are ready")
	require.NoError(t, f.service.EnqueueMissingIdiomCountRebuilds(f.ctx, "v2"))
	require.Equal(t, 1, f.jobs(t), "a dictionary or reviewed-list change queues a rebuild")
	f.rebuild(t, spoon, "v2")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCountsAt(t, "v2"))
}

func TestIdiomCountsOfARemovedBookLeaveTheCorpus(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	f.rebuild(t, spoon, "v1")
	want := map[string]int64{"den Löffel abgeben": 1}
	require.Equal(t, want, f.corpusCounts(t))
	require.NoError(t, f.store.RemoveBookFromMyBooks(f.ctx, f.owner, f.book))
	require.Empty(t, f.corpusCounts(t), "a removed Book leaves the Corpus sum")
	_, err := f.pool.Exec(f.ctx, `UPDATE book_membership SET state='active',activated_at=now(),removed_at=NULL WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book)
	require.NoError(t, err)
	require.Equal(t, want, f.corpusCounts(t), "adding the Book back restores its counts")
	// A deleted Book row takes its rows with it.
	_, err = f.pool.Exec(f.ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `DELETE FROM source_materials WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book)
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `DELETE FROM books WHERE owner_id=$1 AND id=$2`, f.owner, f.book)
	require.NoError(t, err)
	var rows int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM idiom_candidate_counts)+(SELECT count(*) FROM idiom_candidate_count_readiness)`).Scan(&rows))
	require.Zero(t, rows)
}

func TestIdiomCountsOfASupersededAnalysisDoNotCount(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCounts(t))
	// An Analysis update points the Book at a new run. Until that run's rebuild
	// publishes, the previous run's counts are not summed.
	var newRun string
	require.NoError(t, f.pool.QueryRow(f.ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at)
		SELECT owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,'2','second','completed',now() FROM analysis_runs WHERE owner_id=$1 AND id=$2 RETURNING id::text`, f.owner, f.run).Scan(&newRun))
	_, err := f.pool.Exec(f.ctx, `UPDATE book_current_analyses SET analysis_run_id=$3 WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book, newRun)
	require.NoError(t, err)
	require.Empty(t, f.corpusCounts(t), "the previous run's counts wait for the new run's rebuild")
}

func TestIdiomCountsAreQueuedByPublishAndDecisions(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	// Publication queues the rebuild in its own transaction.
	_, err := f.pool.Exec(f.ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book)
	require.NoError(t, err)
	var source string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT source_material_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, f.owner, f.run).Scan(&source))
	require.NoError(t, (&Worker{Pool: f.pool}).publishCompletedRun(f.ctx, JobArgs{OwnerID: f.owner, SourceMaterialID: source, RunID: f.run, Language: "de"}, 0))
	require.Equal(t, 1, f.jobs(t), "publishing queues the Book's Idiom counts")
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCounts(t))
	// A decision withdraws readiness and its post-commit hook queues a rebuild.
	_, err = f.pool.Exec(f.ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE kind=$1`, (IdiomCountsRebuildArgs{}).Kind())
	require.NoError(t, err)
	_, err = f.pool.Exec(f.ctx, `DELETE FROM idiom_candidate_count_readiness WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book)
	require.NoError(t, err)
	require.NoError(t, f.service.EnqueueBrowseCountRebuild(f.ctx, f.owner, f.book))
	require.Equal(t, 1, f.jobs(t))
	require.NoError(t, f.service.EnqueueBrowseCountRebuild(f.ctx, f.owner, f.book))
	require.Equal(t, 1, f.jobs(t), "repeating it queues no second rebuild")
}

func TestIdiomCountsReportBooksWhoseCountsAreNotReady(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "guten", "tag")
	require.EqualValues(t, 1, f.pending(t, "v1"), "a Book never counted is pending, not empty")
	f.rebuild(t, spoon, "v1")
	require.Zero(t, f.pending(t, "v1"), "a counted Book without candidates is ready")
	require.EqualValues(t, 1, f.pending(t, "v2"), "counts built against another idiom index are pending")
}

// confirmDecision stores a Lemma decision the way a confirmed review does and
// runs the hook that follows its commit.
func (f *idiomCountFixture) confirmDecision(t *testing.T, surface string, sentence int, lemma string, excluded bool) {
	t.Helper()
	occurrences, err := f.store.ListLemmaReviewOccurrences(f.ctx, f.owner, f.book, surface)
	require.NoError(t, err)
	var target *domain.LemmaReviewOccurrence
	for i := range occurrences {
		if strings.Contains(occurrences[i].SentenceText, strings.Join(f.words[sentence], " ")) {
			target = &occurrences[i]
		}
	}
	require.NotNil(t, target)
	decision := domain.LemmaReviewDecision{Occurrence: *target, CanonicalLemma: lemma, Excluded: excluded, NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}
	if excluded {
		decision.CanonicalLemma, decision.NormalizationProfile, decision.NormalizationVersion = "", "", ""
	}
	require.NoError(t, f.store.PutLemmaDecisions(f.ctx, []domain.LemmaReviewDecision{decision}))
	require.NoError(t, f.service.EnqueueBrowseCountRebuild(f.ctx, f.owner, f.book))
}

func TestIdiomCountsRebuildAfterAConfirmedLemmaCorrection(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "gab", "den", "löffel", "ab")
	f.rebuild(t, spoon, "v1")
	require.Empty(t, f.corpusCounts(t))
	f.confirmDecision(t, "ab", 0, "abgeben", false)
	require.EqualValues(t, 1, f.pending(t, "v1"), "the decision withdraws readiness until the rebuild")
	require.Equal(t, 1, f.jobs(t), "the confirmation queues the rebuild")
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCounts(t))
	require.Zero(t, f.pending(t, "v1"))
}

func TestIdiomCountsRebuildAfterAnOccurrenceExclusion(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	f.rebuild(t, spoon, "v1")
	require.Len(t, f.corpusCounts(t), 1)
	f.confirmDecision(t, "löffel", 0, "", true)
	require.EqualValues(t, 1, f.pending(t, "v1"))
	require.Equal(t, 1, f.jobs(t))
	f.rebuild(t, spoon, "v1")
	require.Empty(t, f.corpusCounts(t))
	require.Zero(t, f.pending(t, "v1"))
}

func TestIdiomCountsRebuildAfterAnAnalysisUpdate(t *testing.T) {
	f := newIdiomCountFixture(t)
	f.sentence(t, "guten", "tag")
	f.rebuild(t, spoon, "v1")
	require.Empty(t, f.corpusCounts(t))
	oldRun := f.run
	// An Analysis update publishes a new run, with its own Corpus, for the same
	// source; the new analysis holds a candidate the old one did not.
	var source, revision, snapshot string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT source_material_id::text,content_revision_id::text,snapshot_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, f.owner, f.run).Scan(&source, &revision, &snapshot))
	var hash string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT artifact_hash FROM corpora WHERE owner_id=$1 AND id=$2`, f.owner, f.corpus).Scan(&hash))
	require.NoError(t, f.pool.QueryRow(f.ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,status) VALUES($1,$2,$3,'complete') RETURNING id::text`, f.owner, source, hash).Scan(&f.corpus))
	require.NoError(t, f.pool.QueryRow(f.ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,corpus_id,completed_at)
		VALUES($1,$2,$3,$4,'test','2','second','completed',$5,now()) RETURNING id::text`, f.owner, source, revision, snapshot, f.corpus).Scan(&f.run))
	_, err := f.pool.Exec(f.ctx, `UPDATE corpora SET analysis_run_id=$1 WHERE owner_id=$2 AND id=$3`, f.run, f.owner, f.corpus)
	require.NoError(t, err)
	f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
	_, err = f.pool.Exec(f.ctx, `UPDATE book_current_analyses SET analysis_run_id=$3 WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book, f.run)
	require.NoError(t, err)
	require.EqualValues(t, 1, f.pending(t, "v1"), "the new analysis has no counts yet")
	require.NoError(t, f.service.EnqueueMissingIdiomCountRebuilds(f.ctx, "v1"))
	require.Equal(t, 1, f.jobs(t), "reconciliation queues the new run")
	f.rebuild(t, spoon, "v1")
	require.Equal(t, map[string]int64{"den Löffel abgeben": 1}, f.corpusCounts(t))
	var oldRows int
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM idiom_candidate_counts WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, f.owner, f.book, oldRun).Scan(&oldRows))
	require.Zero(t, oldRows)
}

func TestIdiomCountsRebuildQueueIsNotTheAnalysisQueue(t *testing.T) {
	f := newIdiomCountFixture(t)
	require.NoError(t, f.service.EnqueueMissingIdiomCountRebuilds(f.ctx, "v1"))
	var queue string
	require.NoError(t, f.pool.QueryRow(f.ctx, `SELECT queue FROM river_job WHERE kind=$1`, (IdiomCountsRebuildArgs{}).Kind()).Scan(&queue))
	require.Equal(t, IdiomCountsQueue, queue)
	require.NotEqual(t, Queue, queue)
}

// failingIdiomIndex fails every lookup.
type failingIdiomIndex struct{}

func (failingIdiomIndex) IdiomEntries(context.Context, string, string) ([]idiom.Entry, error) {
	return nil, errors.New("index unavailable")
}

// breakTable removes a table the rebuild reads or writes, so the step that
// needs it fails.
func (f *idiomCountFixture) breakTable(t *testing.T, table string) {
	t.Helper()
	_, err := f.pool.Exec(f.ctx, `DROP TABLE `+table+` CASCADE`)
	require.NoError(t, err)
}

func TestIdiomCountRebuildReportsWhatItCouldNotDo(t *testing.T) {
	work := func(f *idiomCountFixture, ctx context.Context, index idiom.Index) error {
		worker := &IdiomCountsRebuildWorker{Pool: f.pool, Index: index, IndexVersion: "v1"}
		return worker.Work(ctx, &river.Job[IdiomCountsRebuildArgs]{Args: IdiomCountsRebuildArgs{OwnerID: f.owner, BookID: f.book, RunID: f.run}})
	}
	t.Run("a cancelled job does not begin", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		ctx, cancel := context.WithCancel(f.ctx)
		cancel()
		require.Error(t, work(f, ctx, spoon))
	})
	t.Run("a Book with no current analysis is skipped", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		_, err := f.pool.Exec(f.ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, f.owner, f.book)
		require.NoError(t, err)
		require.NoError(t, work(f, f.ctx, spoon))
	})
	t.Run("an index that fails leaves no counts", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.sentence(t, "er", "wird", "den", "löffel", "abgeben")
		require.Error(t, work(f, f.ctx, failingIdiomIndex{}))
	})
	t.Run("counts that cannot be cleared fail the job", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.breakTable(t, "idiom_candidate_counts")
		require.Error(t, work(f, f.ctx, spoon))
	})
	t.Run("readiness that cannot be published fails the job", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.breakTable(t, "idiom_candidate_count_readiness")
		require.Error(t, work(f, f.ctx, spoon))
	})
}

func TestIdiomCountSchedulingReportsWhatItCouldNotDo(t *testing.T) {
	t.Run("an unconfigured service cannot schedule", func(t *testing.T) {
		require.Error(t, (&Service{}).EnqueueMissingIdiomCountRebuilds(t.Context(), "v1"))
	})
	t.Run("a cancelled scheduling does not begin", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		ctx, cancel := context.WithCancel(f.ctx)
		cancel()
		require.Error(t, f.service.EnqueueMissingIdiomCountRebuilds(ctx, "v1"))
	})
	t.Run("readiness that cannot be read fails the scheduling", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.breakTable(t, "idiom_candidate_count_readiness")
		require.Error(t, f.service.EnqueueMissingIdiomCountRebuilds(f.ctx, "v1"))
	})
	t.Run("a job that cannot be queued fails the scheduling", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.breakTable(t, "river_job")
		require.Error(t, f.service.EnqueueMissingIdiomCountRebuilds(f.ctx, "v1"))
	})
}

func TestCorpusIdiomCountsReportAStorageFailure(t *testing.T) {
	t.Run("a cancelled read does not begin", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		ctx, cancel := context.WithCancel(f.ctx)
		cancel()
		_, err := f.store.ListCorpusIdiomCandidateCounts(ctx, f.owner, "de", "v1")
		require.Error(t, err)
	})
	t.Run("counts that cannot be read fail the read", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.breakTable(t, "idiom_candidate_counts")
		_, err := f.store.ListCorpusIdiomCandidateCounts(f.ctx, f.owner, "de", "v1")
		require.Error(t, err)
	})
	t.Run("readiness that cannot be read fails the read", func(t *testing.T) {
		f := newIdiomCountFixture(t)
		f.breakTable(t, "idiom_candidate_count_readiness")
		_, err := f.store.ListCorpusIdiomCandidateCounts(f.ctx, f.owner, "de", "v1")
		require.Error(t, err)
	})
}
