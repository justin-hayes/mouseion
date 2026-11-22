// Package download streams a bounded, digested HTTP response body to a
// temporary file. Catalog EPUB acquisition and Classical pinned-source
// acquisition share it so both get the same size cap, digest, no-progress
// watchdog and overall deadline.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// Distinct failure kinds. Callers map these onto their own error vocabulary.
var (
	// ErrRequest reports that no response was obtained.
	ErrRequest = errors.New("download: request failed")
	// ErrRead reports a transport or read failure after the response began.
	ErrRead = errors.New("download: read failed")
	// ErrStalled reports that the body made no progress within IdleTimeout.
	ErrStalled = errors.New("download: no progress")
	// ErrDeadline reports that the overall Timeout expired.
	ErrDeadline = errors.New("download: deadline exceeded")
	// ErrTooLarge reports that the body exceeded Limit.
	ErrTooLarge = errors.New("download: exceeds the size limit")
	// ErrMismatch reports that the body did not match Expected.
	ErrMismatch = errors.New("download: does not match the expected size and digest")
)

// Error is a typed failure. Kind is one of the Err* sentinels and Cause is the
// underlying transport or context error, so callers can map the kind while
// still formatting the original cause.
type Error struct {
	Kind  error
	Cause error
}

func (e *Error) Error() string { return e.Kind.Error() + ": " + e.Cause.Error() }

func (e *Error) Unwrap() []error { return []error{e.Kind, e.Cause} }

// Expectation is a reviewed size and digest a download must match.
type Expectation struct {
	Size   int64
	Digest string
}

// Options bounds one download. Limit, IdleTimeout and Timeout are required.
// CheckResponse, when set, runs before any temporary file is created and its
// error is returned unchanged.
type Options struct {
	// Dir is where the temporary file is created. Empty uses the system
	// temporary directory.
	Dir string
	// Limit caps the body, counted while streaming rather than from
	// Content-Length.
	Limit int64
	// Expected, when set, is the reviewed size and hex SHA-256 the body must
	// match.
	Expected *Expectation
	// IdleTimeout fails a body that makes no progress.
	IdleTimeout time.Duration
	// Timeout is the overall ceiling for the request and body.
	Timeout time.Duration
	// CheckResponse inspects the response before the body is streamed.
	CheckResponse func(*http.Response) error
}

// Result is a downloaded temporary file. The caller owns the file and must
// remove it on every outcome.
type Result struct {
	Path   string
	Size   int64
	Digest [sha256.Size]byte
}

// Fetch performs the request and streams its body to a temporary file in
// Options.Dir. The request context is bounded by Options.Timeout and its body
// by Options.IdleTimeout; on a stall the request context is cancelled so a
// blocked read returns. The temporary file is removed on every failure.
func Fetch(ctx context.Context, client *http.Client, request *http.Request, options Options) (result Result, err error) {
	ctx, cancelDownload := context.WithTimeout(ctx, options.Timeout)
	defer cancelDownload()

	ctx, cancelStalled := context.WithCancel(ctx)
	defer cancelStalled()

	//nolint:gosec // callers build and validate the request target before fetching it.
	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{}, &Error{Kind: ErrDeadline, Cause: err}
		}
		return Result{}, &Error{Kind: ErrRequest, Cause: err}
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if options.CheckResponse != nil {
		if err := options.CheckResponse(response); err != nil {
			return Result{}, err
		}
	}
	return streamAndVerify(ctx, response.Body, options, cancelStalled)
}

// streamAndVerify streams body to a temporary file and verifies the expected
// pin. The file is removed on every failure and committed to the caller on
// success.
func streamAndVerify(ctx context.Context, body io.Reader, options Options, cancelStalled context.CancelFunc) (result Result, err error) {
	file, err := os.CreateTemp(options.Dir, "mouseion-download-")
	if err != nil {
		return Result{}, fmt.Errorf("download: create temporary file: %w", err)
	}
	path := file.Name()
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(path))
		}
	}()

	hasher := sha256.New()
	written, err := copyBounded(ctx, body, io.MultiWriter(file, hasher), options, cancelStalled)
	if err != nil {
		return Result{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	if err := checkExpectation(options.Expected, written, sum); err != nil {
		return Result{}, err
	}
	return Result{Path: path, Size: written, Digest: sum}, nil
}

// copyBounded enforces the streaming limit and no-progress watchdog.
func copyBounded(ctx context.Context, body io.Reader, destination io.Writer, options Options, cancelStalled context.CancelFunc) (int64, error) {
	var stalled atomic.Bool
	tracked := newIdleTimeoutReader(body, options.IdleTimeout, func() {
		stalled.Store(true)
		cancelStalled()
	})
	written, err := io.Copy(destination, io.LimitReader(tracked, options.Limit+1))
	switch {
	case err != nil && stalled.Load():
		return written, &Error{Kind: ErrStalled, Cause: err}
	case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return written, &Error{Kind: ErrDeadline, Cause: err}
	case err != nil:
		return written, &Error{Kind: ErrRead, Cause: err}
	case written > options.Limit:
		return written, ErrTooLarge
	}
	return written, nil
}

// checkExpectation compares an observed size and digest with a reviewed pin.
func checkExpectation(expected *Expectation, written int64, sum [sha256.Size]byte) error {
	if expected == nil {
		return nil
	}
	digest := hex.EncodeToString(sum[:])
	if written != expected.Size || digest != expected.Digest {
		return fmt.Errorf("%w: expected %d bytes with digest %s, got %d bytes with digest %s",
			ErrMismatch, expected.Size, expected.Digest, written, digest)
	}
	return nil
}

// idleTimeoutReader fails a blocked body read after idle without progress.
// onTimeout is expected to cancel the request context, which unblocks the
// read; Read stops the timer whenever it returns.
type idleTimeoutReader struct {
	reader  io.Reader
	timer   *time.Timer
	timeout time.Duration
}

func newIdleTimeoutReader(reader io.Reader, idle time.Duration, onTimeout func()) *idleTimeoutReader {
	return &idleTimeoutReader{reader: reader, timeout: idle, timer: time.AfterFunc(idle, onTimeout)}
}

func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	r.timer.Reset(r.timeout)
	n, err := r.reader.Read(p)
	r.timer.Stop()
	return n, err
}
