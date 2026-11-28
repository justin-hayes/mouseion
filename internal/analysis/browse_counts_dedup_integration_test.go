//go:build integration

package analysis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
)

type browseRebuildQueue struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	store   *persistence.PostgresStore
	service *Service
	owner   string
	book    string
	run     string
}

// newBrowseRebuildQueue prepares a current Book with one countable token and
// no ready Browse counts, plus a Service that enqueues through a real River client.
func newBrowseRebuildQueue(t *testing.T, name string) browseRebuildQueue {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	book, _ := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	runID, _ := addCountableTagToken(t, ctx, pool, owner.ID, book.ID)
	q := browseRebuildQueue{ctx: ctx, pool: pool, store: store, owner: owner.ID, book: book.ID, run: runID}
	q.removeReadiness(t)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	q.service = NewService(pool, client)
	return q
}

// jobs counts this Book's Browse-count rebuild jobs in the given River states.
func (q browseRebuildQueue) jobs(t *testing.T, states ...rivertype.JobState) int {
	t.Helper()
	names := make([]string, len(states))
	for i, state := range states {
		names[i] = string(state)
	}
	var n int
	require.NoError(t, q.pool.QueryRow(q.ctx, `SELECT count(*) FROM river_job
		WHERE kind=$1 AND state::text=ANY($2) AND args->>'owner_id'=$3 AND args->>'book_id'=$4 AND args->>'run_id'=$5`,
		(BrowseCountsRebuildArgs{}).Kind(), names, q.owner, q.book, q.run).Scan(&n))
	return n
}

// moveLiveJobs stands in for River's own transitions: it moves this Book's
// waiting or running rebuild to state.
func (q browseRebuildQueue) moveLiveJobs(t *testing.T, state rivertype.JobState) {
	t.Helper()
	_, err := q.pool.Exec(q.ctx, `UPDATE river_job SET state=$1::river_job_state,
		attempted_at=CASE WHEN $1='running' THEN now() ELSE attempted_at END,
		finalized_at=CASE WHEN $1 IN ('completed','discarded','cancelled') THEN now() ELSE finalized_at END
		WHERE kind=$2 AND args->>'owner_id'=$3 AND args->>'book_id'=$4 AND state IN ('available','pending','scheduled','retryable','running')`,
		string(state), (BrowseCountsRebuildArgs{}).Kind(), q.owner, q.book)
	require.NoError(t, err)
}

func (q browseRebuildQueue) removeReadiness(t *testing.T) {
	t.Helper()
	_, err := q.pool.Exec(q.ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, q.owner, q.book)
	require.NoError(t, err)
}

// The expected states are written out rather than taken from production, so
// the tests catch a change to which states deduplicate.
var waitingBrowseRebuildStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
}

func TestBrowseCountRebuildRepeatedEnqueuesQueueOneWaitingJob(t *testing.T) {
	enqueues := map[string]func(browseRebuildQueue) error{
		"startup reconciliation": func(q browseRebuildQueue) error { return q.service.EnqueueMissingBrowseCountRebuilds(q.ctx) },
		"decision enqueue":       func(q browseRebuildQueue) error { return q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book) },
	}
	for name, enqueue := range enqueues {
		t.Run(name, func(t *testing.T) {
			q := newBrowseRebuildQueue(t, "browse-rebuild-dedup-repeat")
			for range 3 {
				require.NoError(t, enqueue(q))
			}
			require.Equal(t, 1, q.jobs(t, waitingBrowseRebuildStates...))
		})
	}
}

func TestBrowseCountRebuildCompletedJobDoesNotBlockLaterRebuild(t *testing.T) {
	q := newBrowseRebuildQueue(t, "browse-rebuild-dedup-completed")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book))
	require.NoError(t, (&BrowseCountsRebuildWorker{Pool: q.pool}).Work(q.ctx, &river.Job[BrowseCountsRebuildArgs]{Args: BrowseCountsRebuildArgs{OwnerID: q.owner, BookID: q.book, RunID: q.run}}))
	q.moveLiveJobs(t, rivertype.JobStateCompleted)
	// Readiness goes again without a decision, as after a builder-version bump,
	// so the next job has exactly the retained completed job's key.
	q.removeReadiness(t)
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book))
	t.Run("a new job waits", func(t *testing.T) {
		require.Equal(t, 1, q.jobs(t, waitingBrowseRebuildStates...))
	})
	t.Run("the completed job is retained", func(t *testing.T) {
		require.Equal(t, 1, q.jobs(t, rivertype.JobStateCompleted))
	})
}

func TestBrowseCountRebuildNewDecisionQueuesBesideRunningJob(t *testing.T) {
	q := newBrowseRebuildQueue(t, "browse-rebuild-dedup-decision")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book))
	q.moveLiveJobs(t, rivertype.JobStateRunning)
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book))
	require.Zero(t, q.jobs(t, waitingBrowseRebuildStates...), "the running job already covers the unchanged decisions")

	occurrences, err := q.store.ListLemmaReviewOccurrences(q.ctx, q.owner, q.book, "Tag")
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	require.NoError(t, q.store.PutLemmaDecisions(q.ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "tageszeit", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book))
	t.Run("the earlier job keeps running", func(t *testing.T) {
		require.Equal(t, 1, q.jobs(t, rivertype.JobStateRunning))
	})
	t.Run("a job for the new decision waits", func(t *testing.T) {
		require.Equal(t, 1, q.jobs(t, waitingBrowseRebuildStates...), "a new decision must not be absorbed by a job that may already have published")
	})
}

func TestBrowseCountRebuildEnqueueSurfacesFailures(t *testing.T) {
	queueDown := errors.New("queue unavailable")
	enqueues := map[string]func(*Service, browseRebuildQueue) error{
		"startup reconciliation": func(s *Service, q browseRebuildQueue) error { return s.EnqueueMissingBrowseCountRebuilds(q.ctx) },
		"decision enqueue": func(s *Service, q browseRebuildQueue) error {
			return s.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book)
		},
	}
	for name, enqueue := range enqueues {
		t.Run(name+" surfaces a queue failure", func(t *testing.T) {
			q := newBrowseRebuildQueue(t, "browse-rebuild-queue-failure")
			require.ErrorIs(t, enqueue(NewService(q.pool, &failingInsertAnalysisClient{err: queueDown}), q), queueDown)
			require.Zero(t, q.jobs(t, waitingBrowseRebuildStates...))
		})
		t.Run(name+" surfaces an unreadable decision revision", func(t *testing.T) {
			q := newBrowseRebuildQueue(t, "browse-rebuild-revision-failure")
			// Each test owns its database clone, so hiding the decisions table
			// breaks only the revision read; readiness checks never touch it.
			_, err := q.pool.Exec(q.ctx, `ALTER TABLE occurrence_lemma_corrections RENAME TO hidden_occurrence_lemma_corrections`)
			require.NoError(t, err)
			require.ErrorContains(t, enqueue(q.service, q), "read decision revision for Browse rebuild")
			require.Zero(t, q.jobs(t, waitingBrowseRebuildStates...))
		})
	}
}
