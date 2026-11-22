package opds

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/download"
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
func (c *Client) DownloadToFile(ctx context.Context, downloadURL, dir string) (CatalogDownload, error) {
	scoped, err := c.withOrigin(downloadURL)
	if err != nil {
		return CatalogDownload{}, fmt.Errorf("opds: download EPUB: %w", err)
	}
	if scoped != c {
		return scoped.DownloadToFile(ctx, downloadURL, dir)
	}

	request, err := scoped.newRequest(ctx, downloadURL)
	if err != nil {
		return CatalogDownload{}, fmt.Errorf("opds: download EPUB: %w", err)
	}
	client := scoped.httpClient()
	// The shared client's timeout is the feed timeout. A Catalog file gets its
	// own bounds from the download helper instead.
	client.Timeout = 0

	staged, err := download.Fetch(ctx, client, request, download.Options{
		Dir:           dir,
		Limit:         scoped.downloadLimit,
		IdleTimeout:   scoped.idleTimeout,
		Timeout:       scoped.downloadTimeout,
		CheckResponse: catalogResponseError,
	})
	if err != nil {
		return CatalogDownload{}, scoped.catalogDownloadError(err)
	}
	return CatalogDownload{
		Path:   staged.Path,
		Size:   staged.Size,
		Digest: domain.EPUBContentDigestFromHash(staged.Digest),
	}, nil
}

// catalogDownloadError maps the shared helper's failure kinds onto the errors
// Catalog callers and acquisition-failure classification already match. It
// formats from the helper's cause so the message keeps the Catalog vocabulary.
func (c *Client) catalogDownloadError(err error) error {
	if errors.Is(err, download.ErrTooLarge) {
		return ErrDownloadTooLarge
	}
	var failure *download.Error
	if !errors.As(err, &failure) {
		// CheckResponse errors already carry the Catalog prefix.
		return err
	}
	switch {
	case errors.Is(failure.Kind, download.ErrStalled):
		return fmt.Errorf("opds: download EPUB: no progress for %s: %w", c.idleTimeout, failure.Cause)
	case errors.Is(failure.Kind, download.ErrDeadline):
		return fmt.Errorf("opds: download EPUB: exceeded %s: %w", c.downloadTimeout, failure.Cause)
	case errors.Is(failure.Kind, download.ErrRead):
		return fmt.Errorf("opds: download EPUB: read EPUB: %w", failure.Cause)
	case errors.Is(failure.Kind, download.ErrRequest):
		return fmt.Errorf("opds: download EPUB: %w", failure.Cause)
	default:
		return err
	}
}

func catalogResponseError(resp *http.Response) error {
	if err := statusError(resp); err != nil {
		return fmt.Errorf("opds: download EPUB: %w", err)
	}
	mediaType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mediaType != "" && !strings.EqualFold(mediaType, EPUBMediaType) && !strings.EqualFold(mediaType, "application/octet-stream") {
		return fmt.Errorf("opds: download EPUB: incompatible media type %q: %w", mediaType, ErrIncompatibleMediaType)
	}
	return nil
}
