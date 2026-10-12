package prepareddeck

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
)

// jobLessRiverClient answers every insert with the same result. A nil result, or
// one without a live job, is what the workers must refuse to record.
type jobLessRiverClient struct{ result *rivertype.JobInsertResult }

func (c jobLessRiverClient) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return c.result, nil
}

func (jobLessRiverClient) JobCancel(context.Context, int64) (*rivertype.JobRow, error) {
	return nil, errors.New("unexpected job cancellation")
}

// noRowsTx finds no existing live job, so the caller goes on to insert one.
type noRowsTx struct{ pgx.Tx }

func (noRowsTx) QueryRow(context.Context, string, ...any) pgx.Row { return noRow{} }

type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }

// Each River insert site refuses a result that is absent or not a live job, so a
// job row is never recorded for work River did not actually accept.
func TestRiverInsertsRefuseResultsWithoutLiveJob(t *testing.T) {
	ctx := context.Background()
	results := map[string]*rivertype.JobInsertResult{
		"nil result":    nil,
		"no job":        {},
		"completed job": {Job: &rivertype.JobRow{State: rivertype.JobStateCompleted}},
	}
	for name, result := range results {
		client := jobLessRiverClient{result: result}
		t.Run(name, func(t *testing.T) {
			_, err := insertLivePreparationJobTx(ctx, nil, client, JobArgs{}, Queue, "preparation")
			require.EqualError(t, err, "prepareddeck: River did not return a live preparation job")

			_, err = (&BatchPollWorker{Client: client}).insertSubmissionJob(ctx, nil, domain.PreparedDeckBatchChunk{})
			require.EqualError(t, err, "prepareddeck: River did not return a live Batch submission job")

			err = (&BatchPollWorker{Client: client}).insertFinalizerJob(ctx, nil, domain.PreparedDeckRun{})
			require.EqualError(t, err, "prepareddeck: River did not return a live finalizer job")

			err = (&StandardTranslationWorker{Client: client}).finalizer(ctx, nil, domain.PreparedDeckRun{})
			require.EqualError(t, err, "prepareddeck: River did not return a finalizer job")

			err = (&RecoveryWorker{Client: client}).insertFinalizer(ctx, nil, domain.PreparedDeckRun{})
			require.EqualError(t, err, "prepareddeck: River did not return a live finalizer job")

			service := &Service{client: client}
			_, _, err = service.ensurePreparationJob(ctx, noRowsTx{}, domain.DeckPreparation{State: domain.DeckPreparationQueued})
			require.EqualError(t, err, "prepareddeck: River did not return a live preparation job")
		})
	}
}
