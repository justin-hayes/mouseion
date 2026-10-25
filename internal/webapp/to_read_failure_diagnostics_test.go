package webapp

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// metadataOnlyBookID is the fixture Book that has a catalogue entry but no
// acquired source, so To Read and Reanalyze both attempt acquisition.
const metadataOnlyBookID = "fixture-metadata-only"

type failingToReadOPDS struct {
	fixtures.OPDS
	err error
}

func (o failingToReadOPDS) AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
	return epub.ImportResult{}, o.err
}

type failingToReadAnalysis struct {
	fixtures.Analysis
	err error
}

func (a failingToReadAnalysis) SubmitToReadBookAnalysis(context.Context, string, string, string) (analysis.Handle, error) {
	return analysis.Handle{}, a.err
}

// captureLog redirects the standard logger for one test. Tests in this package
// that run in parallel start only after every sequential test has finished.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &output
}

func redirectMessage(t *testing.T, location, key string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	return parsed.Query().Get(key)
}

func TestReanalyzeReportsAndLogsAcquisitionFailure(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	require.NoError(t, store.SetBookDisposition(context.Background(), fixtures.OwnerID, metadataOnlyBookID, domain.BookDispositionToRead))
	requireHandler(t, h).services.OPDS = failingToReadOPDS{err: opds.ErrNoEPUB}
	logs := captureLog(t)

	response := readingTestRequest(t, h, "/reading/books/"+metadataOnlyBookID+"/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)

	require.Equal(t, http.StatusSeeOther, response.Code)
	message := redirectMessage(t, response.Header().Get("Location"), "error")
	assert.Contains(t, message, "Could not acquire")
	assert.Contains(t, message, "This book is not available as an EPUB.")
	assert.Contains(t, message, "EPUB. Choose another edition or format. The To Read choice is retained", "one sentence break, not a doubled period")
	assert.Contains(t, logs.String(), "mouseion: To Read acquisition failed for owner "+fixtures.OwnerID+" book "+strconv.Quote(metadataOnlyBookID)+": "+strconv.Quote(opds.ErrNoEPUB.Error()))
}

func TestReanalyzeReportsAndLogsAnalysisSubmissionFailure(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler := requireHandler(t, h)
	handler.services.Analysis = failingToReadAnalysis{err: analysis.ErrEPUBRequired}
	logs := captureLog(t)

	response := readingTestRequest(t, h, "/reading/books/fixture-failed/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "The current source has no usable EPUB units. The To Read choice is retained; analysis is unavailable.", redirectMessage(t, response.Header().Get("Location"), "error"))
	assert.Contains(t, logs.String(), "mouseion: To Read analysis submission failed for owner "+fixtures.OwnerID+` book "fixture-failed": `+strconv.Quote(analysis.ErrEPUBRequired.Error()))
}

func TestReanalyzeKeepsGenericMessageForUnclassifiedFailure(t *testing.T) {
	assert.Equal(t, "Analysis could not be started. The To Read choice is retained. Review current book content in My Books and try again.",
		reanalysisError(context.Background(), nil, fixtures.OwnerID, "fixture-failed", "Title", cataloguesync.AcquisitionTarget{}, false, assert.AnError))
}

func TestCurrentBookRefreshRejectionIsNotLoggedAsFailure(t *testing.T) {
	logs := captureLog(t)
	logToReadAnalysisFailure(fixtures.OwnerID, fixtures.BookID, false, analysis.ErrCurrentBook)
	assert.Empty(t, logs.String())
}

func TestToReadFailureLogQuotesRequestPathBookID(t *testing.T) {
	logs := captureLog(t)
	logToReadAnalysisFailure(fixtures.OwnerID, "forged\nmouseion: injected", true, errors.New("lookup forged\nmouseion: injected"))
	assert.Equal(t, 1, strings.Count(logs.String(), "\n"), "a newline in the book id or error must not start a new log line")
	assert.Contains(t, logs.String(), `book "forged\nmouseion: injected": "lookup forged\nmouseion: injected"`)
}

func TestMoveToReadLogsAcquisitionFailure(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	requireHandler(t, h).services.OPDS = failingToReadOPDS{err: opds.ErrNoEPUB}
	detail, err := store.GetBookDetail(context.Background(), fixtures.OwnerID, metadataOnlyBookID)
	require.NoError(t, err)
	require.Nil(t, detail.Acquired, "the metadata-only Book has no acquired source")
	require.NotEqual(t, domain.BookDispositionToRead, detail.Disposition)
	logs := captureLog(t)

	response := readingTestRequest(t, h, "/library/books/"+metadataOnlyBookID+"/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, redirectMessage(t, response.Header().Get("Location"), "message"), "EPUB. Choose another edition or format. The To Read status is retained", "one sentence break, not a doubled period")
	assert.Contains(t, logs.String(), "mouseion: To Read acquisition failed for owner "+fixtures.OwnerID+" book "+strconv.Quote(metadataOnlyBookID)+": "+strconv.Quote(opds.ErrNoEPUB.Error()))
}
