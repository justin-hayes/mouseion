package opds

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	// defaultDownloadLimit caps a Catalog file. The size is counted while
	// streaming, so a catalog cannot bypass it with a missing or wrong
	// Content-Length.
	defaultDownloadLimit = int64(512 << 20)
	// defaultIdleTimeout fails an EPUB body that makes no progress.
	defaultIdleTimeout = 30 * time.Second
	// defaultDownloadTimeout is the overall ceiling for one Catalog file.
	defaultDownloadTimeout = 10 * time.Minute
)

// CatalogDownload is a Catalog file streamed to a temporary file, with the
// size and SHA-256 digest observed while it arrived. The caller owns the file
// and must remove it on every outcome.
type CatalogDownload struct {
	Path   string
	Size   int64
	Digest string
}

// DownloadToFile streams an EPUB acquisition target into a temporary file in
// dir, or the system temporary directory when dir is empty, hashing it as it
// arrives. EPUB bodies use a no-progress timeout and an overall ceiling
// instead of the HTTP client's feed timeout, so a slow catalog does not fail a
// large download. The temporary file is removed when the download fails.
func (c *Client) DownloadToFile(ctx context.Context, downloadURL, dir string) (download CatalogDownload, err error) {
	scoped, err := c.withOrigin(downloadURL)
	if err != nil {
		return CatalogDownload{}, fmt.Errorf("opds: download EPUB: %w", err)
	}
	if scoped != c {
		return scoped.DownloadToFile(ctx, downloadURL, dir)
	}

	downloadCtx, cancelDownload := context.WithTimeout(ctx, c.downloadTimeout)
	defer cancelDownload()
	downloadCtx, cancelStalled := context.WithCancel(downloadCtx)
	defer cancelStalled()

	client := c.httpClient()
	// The shared client's timeout is the feed timeout. A Catalog file gets its
	// own bounds from the context and the no-progress watchdog instead.
	client.Timeout = 0
	resp, err := c.do(downloadCtx, downloadURL, client)
	if err != nil {
		return CatalogDownload{}, fmt.Errorf("opds: download EPUB: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if err := catalogResponseError(resp); err != nil {
		return CatalogDownload{}, err
	}

	file, err := os.CreateTemp(dir, "mouseion-epub-")
	if err != nil {
		return CatalogDownload{}, fmt.Errorf("opds: download EPUB: create temporary file: %w", err)
	}
	path := file.Name()
	committed := false
	defer func() {
		if committed {
			return
		}
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("opds: close temporary EPUB: %w", closeErr))
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("opds: remove temporary EPUB: %w", removeErr))
		}
	}()

	hasher := sha256.New()
	written, err := c.streamCatalogFile(downloadCtx, io.MultiWriter(file, hasher), resp.Body, cancelStalled)
	if err != nil {
		return CatalogDownload{}, err
	}
	if err := file.Close(); err != nil {
		return CatalogDownload{}, fmt.Errorf("opds: download EPUB: write temporary file: %w", err)
	}
	committed = true
	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	return CatalogDownload{Path: path, Size: written, Digest: domain.EPUBContentDigestFromHash(sum)}, nil
}

// streamCatalogFile copies the Catalog file body while enforcing the download
// limit and the no-progress timeout. cancelStalled aborts the request context,
// which unblocks a stalled read.
func (c *Client) streamCatalogFile(ctx context.Context, dst io.Writer, body io.Reader, cancelStalled context.CancelFunc) (int64, error) {
	var stalled atomic.Bool
	idle := c.idleTimeout
	tracked := newIdleTimeoutReader(body, idle, func() {
		stalled.Store(true)
		cancelStalled()
	})
	written, err := io.Copy(dst, io.LimitReader(tracked, c.downloadLimit+1))
	switch {
	case err != nil && stalled.Load():
		return written, fmt.Errorf("opds: download EPUB: no progress for %s: %w", idle, err)
	case err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return written, fmt.Errorf("opds: download EPUB: exceeded %s: %w", c.downloadTimeout, err)
	case err != nil:
		return written, fmt.Errorf("opds: download EPUB: read EPUB: %w", err)
	case written > c.downloadLimit:
		return written, ErrDownloadTooLarge
	}
	return written, nil
}

func catalogResponseError(resp *http.Response) error {
	if err := statusError(resp); err != nil {
		return fmt.Errorf("opds: download EPUB: %w", err)
	}
	mediaType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mediaType != "" && !strings.EqualFold(mediaType, EPUBMediaType) && !strings.EqualFold(mediaType, "application/octet-stream") {
		return fmt.Errorf("opds: download EPUB: incompatible media type %q", mediaType)
	}
	return nil
}

// idleTimeoutReader fails a blocked body read after idle without progress.
// The callback is expected to cancel the request context, which unblocks the
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
