package jobstop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func cancelledWith(cause error) context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	return ctx
}

func timedOut(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	t.Cleanup(cancel)
	<-ctx.Done()
	return ctx
}

func live(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

func TestCancelledByClientStop(t *testing.T) {
	tests := []struct {
		name string
		ctx  func(t *testing.T) context.Context
		want bool
	}{
		{name: "client stop", ctx: func(*testing.T) context.Context { return cancelledWith(errors.New(riverStopCause)) }, want: true},
		{name: "user cancel", ctx: func(*testing.T) context.Context { return cancelledWith(river.ErrJobCancelledRemotely) }},
		{name: "plain cancellation", ctx: func(*testing.T) context.Context { return cancelledWith(nil) }},
		{name: "job timeout", ctx: timedOut},
		{name: "live context", ctx: live},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CancelledByClientStop(tt.ctx(t)))
		})
	}
}

func TestMiddlewareReportsAShutdownFailureAsTheContextError(t *testing.T) {
	workErr := fmt.Errorf("call NLP analyzer: %w", status.Error(codes.Canceled, "context canceled"))
	work := func(ctx context.Context, err error) error {
		return Middleware().Work(ctx, &rivertype.JobRow{}, func(context.Context) error { return err })
	}
	t.Run("a worker error after a client stop becomes context.Canceled", func(t *testing.T) {
		err := work(cancelledWith(errors.New(riverStopCause)), workErr)
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorContains(t, err, "job stopped for shutdown")
	})
	t.Run("a success after a client stop stays a success", func(t *testing.T) {
		require.NoError(t, work(cancelledWith(errors.New(riverStopCause)), nil))
	})
	t.Run("a user cancel keeps the worker error", func(t *testing.T) {
		require.ErrorIs(t, work(cancelledWith(river.ErrJobCancelledRemotely), workErr), workErr)
	})
	t.Run("a live job keeps the worker error", func(t *testing.T) {
		require.ErrorIs(t, work(live(t), workErr), workErr)
	})
}
