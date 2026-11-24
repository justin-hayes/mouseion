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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type browseRebuildQueue struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	store   *persistence.PostgresStore
	service *Service
	owner   string
	book    domain.Book
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
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	book, _ := makeCurrentAnalyzedBook(t, ctx, store, owner.ID)
	var runID, corpusID, unitID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT cai.analysis_run_id::text,cai.corpus_id::text,u.unit_id
		FROM current_analysis_identity cai JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2 LIMIT 1`, owner.ID, book.ID).Scan(&runID, &corpusID, &unitID))
	_, err = pool.Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		VALUES($1,$2,$3,$4,0,'Guten Tag.',0,10)`, owner.ID, runID, corpusID, unitID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset)
		VALUES($1,'de',$2,$3,0,0,'Tag','Tag','tag','NOUN','root',0,'{}',6,9)`, owner.ID, runID, corpusID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	return browseRebuildQueue{ctx: ctx, pool: pool, store: store, service: NewService(pool, client), owner: owner.ID, book: book, run: runID}
}

// jobs counts this Book's Browse-count rebuild jobs in the given River states.
func (q browseRebuildQueue) jobs(t *testing.T, states ...string) int {
	t.Helper()
	var n int
	require.NoError(t, q.pool.QueryRow(q.ctx, `SELECT count(*) FROM river_job
		WHERE kind=$1 AND state::text=ANY($2) AND args->>'owner_id'=$3 AND args->>'book_id'=$4 AND args->>'run_id'=$5`,
		(BrowseCountsRebuildArgs{}).Kind(), states, q.owner, q.book.ID, q.run).Scan(&n))
	return n
}

func (q browseRebuildQueue) setJobState(t *testing.T, state string) {
	t.Helper()
	_, err := q.pool.Exec(q.ctx, `UPDATE river_job SET state=$1::river_job_state,
		attempted_at=CASE WHEN $1='running' THEN now() ELSE attempted_at END,
		finalized_at=CASE WHEN $1 IN ('completed','discarded','cancelled') THEN now() ELSE finalized_at END
		WHERE kind=$2 AND args->>'owner_id'=$3 AND args->>'book_id'=$4 AND state IN ('available','pending','scheduled','retryable','running')`,
		state, (BrowseCountsRebuildArgs{}).Kind(), q.owner, q.book.ID)
	require.NoError(t, err)
}

var waitingBrowseRebuildStates = []string{"available", "pending", "scheduled", "retryable"}

func TestBrowseCountRebuildReconciliationQueuesOneWaitingJob(t *testing.T) {
	q := newBrowseRebuildQueue(t, "browse-rebuild-dedup-reconcile")
	require.NoError(t, q.service.EnqueueMissingBrowseCountRebuilds(q.ctx))
	require.NoError(t, q.service.EnqueueMissingBrowseCountRebuilds(q.ctx), "a repeated startup reconciliation")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID), "a repeated immediate enqueue")
	assert.Equal(t, 1, q.jobs(t, waitingBrowseRebuildStates...))
}

func TestBrowseCountRebuildEnqueueAfterCompletionQueuesNewJob(t *testing.T) {
	q := newBrowseRebuildQueue(t, "browse-rebuild-dedup-completed")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID))
	q.setJobState(t, "completed")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID))
	assert.Equal(t, 1, q.jobs(t, waitingBrowseRebuildStates...), "a retained completed job must not block a later rebuild")
	assert.Equal(t, 1, q.jobs(t, "completed"))
}

func TestBrowseCountRebuildNewDecisionQueuesBesideRunningJob(t *testing.T) {
	q := newBrowseRebuildQueue(t, "browse-rebuild-dedup-decision")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID))
	q.setJobState(t, "running")
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID))
	require.Equal(t, 0, q.jobs(t, waitingBrowseRebuildStates...), "the running job already covers the unchanged decisions")

	occurrences, err := q.store.ListLemmaReviewOccurrences(q.ctx, q.owner, q.book.ID, "Tag")
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	require.NoError(t, q.store.PutLemmaDecisions(q.ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "tageszeit", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	require.NoError(t, q.service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID))
	assert.Equal(t, 1, q.jobs(t, "running"))
	assert.Equal(t, 1, q.jobs(t, waitingBrowseRebuildStates...), "a new decision must not be absorbed by a job that may already have published")
}

func TestBrowseCountRebuildEnqueueSurfacesQueueFailure(t *testing.T) {
	q := newBrowseRebuildQueue(t, "browse-rebuild-queue-failure")
	queueDown := errors.New("queue unavailable")
	service := NewService(q.pool, &failingInsertAnalysisClient{err: queueDown})
	require.ErrorIs(t, service.EnqueueBrowseCountRebuild(q.ctx, q.owner, q.book.ID), queueDown)
	require.ErrorIs(t, service.EnqueueMissingBrowseCountRebuilds(q.ctx), queueDown)
	assert.Zero(t, q.jobs(t, waitingBrowseRebuildStates...))
}
