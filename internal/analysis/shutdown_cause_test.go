package analysis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
)

func TestCancelledByClientStop(t *testing.T) {
	cancelledWith := func(cause error) context.Context {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)
		return ctx
	}
	timedOut, cancelTimeout := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancelTimeout()
	<-timedOut.Done()
	live, cancelLive := context.WithCancel(context.Background())
	defer cancelLive()

	require.True(t, cancelledByClientStop(cancelledWith(errors.New(riverStopCause))), "client stop")
	require.False(t, cancelledByClientStop(cancelledWith(river.ErrJobCancelledRemotely)), "user cancel")
	require.False(t, cancelledByClientStop(cancelledWith(nil)), "plain cancellation")
	require.False(t, cancelledByClientStop(timedOut), "job timeout")
	require.False(t, cancelledByClientStop(live), "live context")
}
