//go:build integration

package webapp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingKnownVocab accepts no import, so the handler's submit-failure path runs.
type failingKnownVocab struct{}

func (failingKnownVocab) Submit(context.Context, string, string, string) (knownvocab.Handle, error) {
	return knownvocab.Handle{}, errors.New("known vocabulary queue unavailable")
}

func (failingKnownVocab) Get(context.Context, string, int64) (knownvocab.Status, error) {
	return knownvocab.Status{}, errors.New("unused")
}

// knownVocabImportLearner signs in a learner. A German book makes German the
// active study language; without one the learner has no language to import into.
func knownVocabImportLearner(t *testing.T, withGermanBook bool, known KnownVocabulary) (http.Handler, []*http.Cookie, string) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "known-vocab-import-feedback-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	learner := createAccount(t, ctx, store, "vocab-import-learner", "vocab-import-password")
	if withGermanBook {
		_, err = store.CreateBook(ctx, domain.Book{OwnerID: learner.ID, Title: "German library book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, err)
	}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), KnownVocab: known, SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, "vocab-import-learner", "vocab-import-password")
	return h, cookies, csrf
}

// knownVocabUpload posts a vocabulary file with the given part Content-Type. An
// empty contentType omits the header, as some browsers and clients do.
func knownVocabUpload(t *testing.T, h http.Handler, cookies []*http.Cookie, csrf, contentType, content string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("csrf_token", csrf))
	header := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="vocabulary_file"; filename="known.txt"`}}
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = io.WriteString(part, content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/vocabulary/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if htmx {
		request.Header.Set("Hx-Request-Type", "partial")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestKnownVocabImportExplainsUnusableUploads(t *testing.T) {
	for _, test := range []struct {
		name           string
		withGermanBook bool
		contentType    string
		content        string
		want           string
	}{
		{name: "no language", withGermanBook: false, contentType: "text/plain", content: "Haus\n", want: "Choose a language before importing."},
		{name: "unsupported content type", withGermanBook: true, contentType: "image/png", content: "Haus\n", want: "Choose a UTF-8 plain text file to import."},
		{name: "empty file", withGermanBook: true, contentType: "text/plain", content: "", want: "Choose a non-empty UTF-8 text file to import."},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, cookies, csrf := knownVocabImportLearner(t, test.withGermanBook, nil)

			response := knownVocabUpload(t, h, cookies, csrf, test.contentType, test.content, false)

			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), test.want)
		})
	}
}

func TestKnownVocabImportReportsSubmitFailureInline(t *testing.T) {
	h, cookies, csrf := knownVocabImportLearner(t, true, failingKnownVocab{})

	response := knownVocabUpload(t, h, cookies, csrf, "text/plain", "Haus\n", false)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Import failed: known vocabulary queue unavailable")
}

func TestKnownVocabImportPartialRequestReturnsOnlyTheFeedback(t *testing.T) {
	h, cookies, csrf := knownVocabImportLearner(t, true, nil)

	response := knownVocabUpload(t, h, cookies, csrf, "image/png", "Haus\n", true)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Choose a UTF-8 plain text file to import.")
	assert.NotContains(t, response.Body.String(), "<html")
}
