package opds

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// downloadedBytes is the in-test shortcut for a download the caller reads
// immediately. The file lives under t.TempDir, which testing removes.
func downloadedBytes(t *testing.T, client *Client, target, dir string) ([]byte, error) {
	t.Helper()
	download, err := client.DownloadToFile(t.Context(), target, dir)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(download.Path)
}

func TestDownloadToFileWritesTheExactCatalogBytes(t *testing.T) {
	body := []byte("epub bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, string(body))
	}))
	defer server.Close()

	dir := t.TempDir()
	download, err := NewClient(server.Client(), Auth{}).DownloadToFile(t.Context(), server.URL+"/book.epub", dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(dir), filepath.Dir(download.Path))
	assert.Equal(t, int64(len(body)), download.Size)
	assert.Equal(t, domain.EPUBContentDigest(body), download.Digest)
	stored, err := os.ReadFile(download.Path)
	require.NoError(t, err)
	assert.Equal(t, body, stored)
}

func TestDownloadToFileCountsTheLimitWithoutContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "more than the limit")
		flushResponse(t, w)
	}))
	defer server.Close()

	client := NewClient(server.Client(), Auth{})
	client.downloadLimit = 8
	dir := t.TempDir()
	_, err := client.DownloadToFile(t.Context(), server.URL+"/book.epub", dir)
	require.ErrorIs(t, err, ErrDownloadTooLarge)
	assertNoTemporaryFiles(t, dir)
}

func TestDownloadToFileAllowsASlowBodyThatKeepsMakingProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		for range 5 {
			testwrite.String(t, w, "chunk")
			flushResponse(t, w)
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := NewClient(&http.Client{Timeout: 30 * time.Millisecond}, Auth{})
	client.idleTimeout = 50 * time.Millisecond
	client.downloadTimeout = 5 * time.Second
	data, err := downloadedBytes(t, client, server.URL+"/book.epub", t.TempDir())
	require.NoError(t, err, "progressing body failed on the feed timeout")
	assert.Equal(t, "chunkchunkchunkchunkchunk", string(data))
}

func TestDownloadToFileFailsAStalledBodyAndRemovesTheFile(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "chunk")
		flushResponse(t, w)
		<-release
	}))
	defer server.Close()
	defer close(release)

	client := NewClient(server.Client(), Auth{})
	client.idleTimeout = 20 * time.Millisecond
	client.downloadTimeout = 5 * time.Second
	dir := t.TempDir()
	_, err := client.DownloadToFile(t.Context(), server.URL+"/book.epub", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no progress")
	assertNoTemporaryFiles(t, dir)
}

func TestDownloadToFileHonoursTheOverallCeiling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		for {
			if _, err := io.WriteString(w, "x"); err != nil {
				return
			}
			flushResponse(t, w)
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := NewClient(server.Client(), Auth{})
	client.idleTimeout = time.Second
	client.downloadTimeout = 40 * time.Millisecond
	dir := t.TempDir()
	_, err := client.DownloadToFile(t.Context(), server.URL+"/book.epub", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded")
	assertNoTemporaryFiles(t, dir)
}

func TestDownloadToFileRemovesTheFileAfterCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", EPUBMediaType)
		testwrite.String(t, w, "chunk")
		flushResponse(t, w)
		<-release
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(t.Context())
	client := NewClient(server.Client(), Auth{})
	client.idleTimeout = time.Second
	client.downloadTimeout = 5 * time.Second
	dir := t.TempDir()
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := client.DownloadToFile(ctx, server.URL+"/book.epub", dir)
	require.ErrorIs(t, err, context.Canceled)
	assertNoTemporaryFiles(t, dir)
}

func TestFeedRequestsKeepTheHTTPClientTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	client := NewClient(&http.Client{Timeout: 30 * time.Millisecond}, Auth{})
	_, err := client.ListPage(t.Context(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Client.Timeout")
}

func TestDownloadToFileFailsHTTPStatusAndRemovesTheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	dir := t.TempDir()
	_, err := NewClient(server.Client(), Auth{}).DownloadToFile(t.Context(), server.URL, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assertNoTemporaryFiles(t, dir)
}

func assertNoTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "temporary files remained after acquisition")
}

func flushResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatalf("response writer does not support flushing")
	}
	flusher.Flush()
}
