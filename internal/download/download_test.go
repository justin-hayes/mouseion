package download

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("body broke") }
func (failingBody) Close() error             { return nil }

func requestFor(t *testing.T, target string) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	require.NoError(t, err)
	return request
}

func assertNoFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a failed download left a temporary file")
}

func TestFetchWritesTheBodyAndKeepsTheFile(t *testing.T) {
	body := []byte("downloaded bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	result, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
		Dir:         dir,
		Limit:       100,
		IdleTimeout: time.Second,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(dir), filepath.Dir(result.Path))
	assert.Equal(t, int64(len(body)), result.Size)
	assert.Equal(t, sha256.Sum256(body), result.Digest)
	stored, err := os.ReadFile(result.Path)
	require.NoError(t, err)
	assert.Equal(t, body, stored, "the caller keeps the completed download")
}

func TestFetchMatchesAnExpectedSizeAndDigest(t *testing.T) {
	body := []byte("pinned bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	defer server.Close()

	sum := sha256.Sum256(body)
	result, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
		Dir:         t.TempDir(),
		Limit:       int64(len(body)),
		Expected:    &Expectation{Size: int64(len(body)), Digest: resultHex(sum)},
		IdleTimeout: time.Second,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, sum, result.Digest)
}

func TestFetchRefusesAnOversizeBodyAndRemovesTheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, "more than the limit"); err != nil {
			return
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	_, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
		Dir:         dir,
		Limit:       8,
		IdleTimeout: time.Second,
		Timeout:     5 * time.Second,
	})
	require.ErrorIs(t, err, ErrTooLarge)
	assertNoFiles(t, dir)
}

func TestFetchFailsAStalledBodyAndRemovesTheFile(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, "chunk"); err != nil {
			return
		}
		testwrite.Flush(t, w)
		<-release
	}))
	defer server.Close()
	defer close(release)

	dir := t.TempDir()
	_, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
		Dir:         dir,
		Limit:       100,
		IdleTimeout: 20 * time.Millisecond,
		Timeout:     5 * time.Second,
	})
	require.ErrorIs(t, err, ErrStalled)
	assert.Contains(t, err.Error(), "no progress")
	assertNoFiles(t, dir)
}

func TestFetchHonoursTheOverallDeadlineAndRemovesTheFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for {
			if _, err := io.WriteString(w, "x"); err != nil {
				return
			}
			testwrite.Flush(t, w)
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	_, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
		Dir:         dir,
		Limit:       1 << 20,
		IdleTimeout: time.Second,
		Timeout:     40 * time.Millisecond,
	})
	require.ErrorIs(t, err, ErrDeadline)
	assert.Contains(t, err.Error(), "deadline exceeded")
	assertNoFiles(t, dir)
}

func TestFetchHonoursTheOverallDeadlineBeforeResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}

	_, err := Fetch(t.Context(), client, requestFor(t, "http://catalog.invalid/book"), Options{
		Dir:         t.TempDir(),
		Limit:       100,
		IdleTimeout: time.Second,
		Timeout:     20 * time.Millisecond,
	})
	require.ErrorIs(t, err, ErrDeadline)
}

func TestFetchRefusesAMismatchedSizeOrDigestAndRemovesTheFile(t *testing.T) {
	body := []byte("pinned bytes")
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(body); err != nil {
			return
		}
	}))
	defer server.Close()

	cases := map[string]Expectation{
		"size":   {Size: int64(len(body)) + 1, Digest: resultHex(sum)},
		"digest": {Size: int64(len(body)), Digest: strings.Repeat("0", sha256.Size*2)},
	}
	for name, expected := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
				Dir:         dir,
				Limit:       100,
				Expected:    &expected,
				IdleTimeout: time.Second,
				Timeout:     5 * time.Second,
			})
			require.ErrorIs(t, err, ErrMismatch)
			assertNoFiles(t, dir)
		})
	}
}

func TestFetchReportsAReadErrorAndRemovesTheFile(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
	})}

	dir := t.TempDir()
	_, err := Fetch(t.Context(), client, requestFor(t, "http://catalog.invalid/book"), Options{
		Dir:         dir,
		Limit:       100,
		IdleTimeout: time.Second,
		Timeout:     5 * time.Second,
	})
	require.ErrorIs(t, err, ErrRead)
	assert.Contains(t, err.Error(), "read failed")
	assertNoFiles(t, dir)
}

func TestFetchReportsARequestErrorAndLeavesNoFile(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}

	dir := t.TempDir()
	_, err := Fetch(t.Context(), client, requestFor(t, "http://catalog.invalid/book"), Options{
		Dir:         dir,
		Limit:       100,
		IdleTimeout: time.Second,
		Timeout:     5 * time.Second,
	})
	require.ErrorIs(t, err, ErrRequest)
	assert.Contains(t, err.Error(), "offline")
	assertNoFiles(t, dir)
}

func TestFetchReturnsACheckedResponseErrorUnchanged(t *testing.T) {
	checkErr := errors.New("incompatible media type")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	dir := t.TempDir()
	_, err := Fetch(t.Context(), server.Client(), requestFor(t, server.URL), Options{
		Dir:         dir,
		Limit:       100,
		IdleTimeout: time.Second,
		Timeout:     5 * time.Second,
		CheckResponse: func(*http.Response) error {
			return checkErr
		},
	})
	require.ErrorIs(t, err, checkErr)
	assertNoFiles(t, dir)
}

func resultHex(sum [sha256.Size]byte) string {
	return hex.EncodeToString(sum[:])
}
