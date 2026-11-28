// Package jobstop tells a River job context cancelled because the client is
// stopping apart from every other cancellation, so a job interrupted by a
// server shutdown runs again instead of being recorded as failed.
package jobstop

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// riverStopCause is the message of the cause River v0.51.2 sets on every job
// context when the client is stopped (its internal rivercommon.ErrStop, which
// cannot be imported). TestJobContextCauseOnStopAndCancel in internal/analysis
// pins it against the real client.
const riverStopCause = "stop initiated"

// CancelledByClientStop reports whether ctx was cancelled by a River client
// stop, as opposed to a user's JobCancel (cause river.ErrJobCancelledRemotely),
// a timeout, or any other cancellation.
func CancelledByClientStop(ctx context.Context) bool {
	cause := context.Cause(ctx)
	return errors.Is(ctx.Err(), context.Canceled) && cause != nil && cause.Error() == riverStopCause
}

// Middleware reports a job that failed because the client stopped as the
// context error. River refunds the attempt and makes the job available only
// when the worker returns context.Canceled, but a worker may report the
// cancellation as its own error (a gRPC Canceled status, a wrapped download
// error), which River would count as a spent attempt, and a job with one
// attempt would be discarded.
func Middleware() rivertype.WorkerMiddleware {
	return river.WorkerMiddlewareFunc(func(ctx context.Context, _ *rivertype.JobRow, doInner func(context.Context) error) error {
		err := doInner(ctx)
		if err != nil && CancelledByClientStop(ctx) {
			return fmt.Errorf("job stopped for shutdown: %w", ctx.Err())
		}
		return err
	})
}
