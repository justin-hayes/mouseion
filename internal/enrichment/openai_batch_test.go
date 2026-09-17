package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIBatchClientFilesAndBatchOperations(t *testing.T) {
	var operations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		operations = append(operations, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			assert.Equal(t, int64(-1), r.ContentLength, "multipart upload was buffered: content length=%d", r.ContentLength)
			r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
			parseErr := r.ParseMultipartForm(1 << 20) //nolint:gosec // MaxBytesReader above bounds the complete test request.
			require.NoError(t, parseErr)
			assert.Equal(t, "batch", r.FormValue("purpose"))
			assert.Equal(t, "created_at", r.FormValue("expires_after[anchor]"))
			assert.Equal(t, "604800", r.FormValue("expires_after[seconds]"))
			file, header, err := r.FormFile("file")
			require.NoError(t, err)
			// The multipart file is owned by this request and must close before
			// the handler returns; t.Cleanup would run after that ownership ends.
			defer func() {
				if err := file.Close(); err != nil {
					t.Errorf("multipart file cleanup failed: %v", err)
				}
			}()
			content, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, "run-opaque.jsonl", header.Filename)
			assert.Equal(t, "{\"custom_id\":\"opaque\"}\n", string(content))
			testwrite.String(t, w, `{"id":"file-input","object":"file","bytes":25,"created_at":1,"expires_at":2,"filename":"run-opaque.jsonl","purpose":"batch","status":"uploaded"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-input":
			testwrite.String(t, w, `{"id":"file-input","object":"file","bytes":25,"created_at":1,"expires_at":2,"filename":"run-opaque.jsonl","purpose":"batch","status":"processed"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-output/content":
			w.Header().Set("Content-Type", "application/jsonl")
			testwrite.String(t, w, "result-line\n")
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/file-input":
			testwrite.String(t, w, `{"id":"file-input","object":"file","deleted":true}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches":
			var request map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.Equal(t, "file-input", request["input_file_id"])
			assert.Equal(t, OpenAIChatCompletionsEndpoint, request["endpoint"])
			assert.Equal(t, "24h", request["completion_window"])
			metadata, ok := request["metadata"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "018f64b6-opaque", metadata["chunk_id"])
			writeBatchStub(t, w, "batch-created", BatchStatusValidating)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches/batch-created":
			writeBatchStub(t, w, "batch-created", BatchStatusCompleted)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches":
			assert.Equal(t, "batch-before", r.URL.Query().Get("after"))
			assert.Equal(t, "2", r.URL.Query().Get("limit"))
			testwrite.String(t, w, `{"object":"list","data":[`)
			writeBatchStub(t, w, "batch-listed", BatchStatusInProgress)
			testwrite.String(t, w, `],"first_id":"batch-listed","last_id":"batch-listed","has_more":false}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches/batch-created/cancel":
			writeBatchStub(t, w, "batch-created", BatchStatusCancelling)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newStubbedBatchClient(t, server)

	file, err := client.UploadFile(context.Background(), "run-opaque.jsonl", strings.NewReader("{\"custom_id\":\"opaque\"}\n"))
	require.NoError(t, err)
	assert.Equal(t, "file-input", file.ID)
	assert.Equal(t, "batch", file.Purpose)
	var content bytes.Buffer
	require.NoError(t, client.FileContent(context.Background(), "file-output", &content))
	assert.Equal(t, "result-line\n", content.String())
	deleted, err := client.DeleteFile(context.Background(), "file-input")
	require.NoError(t, err)
	assert.True(t, deleted.Deleted)
	created, err := client.CreateBatch(context.Background(), CreateBatchRequest{InputFileID: "file-input", Metadata: map[string]string{"chunk_id": "018f64b6-opaque"}})
	require.NoError(t, err)
	assert.Equal(t, BatchStatusValidating, created.Status)
	got, err := client.GetBatch(context.Background(), "batch-created")
	require.NoError(t, err)
	assert.Equal(t, BatchStatusCompleted, got.Status)
	assert.Equal(t, BatchRequestCounts{Total: 3, Completed: 2, Failed: 1}, got.RequestCounts)
	assert.Equal(t, int64(30), got.Usage.TotalTokens)
	assert.Equal(t, ProviderErrorInvalidRequest, got.Errors[0].Class)
	list, err := client.ListBatches(context.Background(), ListBatchesRequest{After: "batch-before", Limit: 2})
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "batch-listed", list.Data[0].ID)
	assert.False(t, list.HasMore)
	cancelled, err := client.CancelBatch(context.Background(), "batch-created")
	require.NoError(t, err)
	assert.Equal(t, BatchStatusCancelling, cancelled.Status)
	assert.Len(t, operations, 8)
}

func TestOpenAIBatchClientWaitsForFileProcessing(t *testing.T) {
	var fileGets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			testwrite.String(t, w, `{"id":"file-pending","object":"file","bytes":25,"created_at":1,"filename":"run.jsonl","purpose":"batch","status":"uploaded"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-pending":
			fileGets++
			status := "pending"
			if fileGets == 2 {
				status = "processed"
			}
			testwrite.Fprintf(t, w, `{"id":"file-pending","object":"file","bytes":25,"created_at":1,"filename":"run.jsonl","purpose":"batch","status":%q}`, status)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newStubbedBatchClient(t, server)
	file, err := client.UploadFile(context.Background(), "run.jsonl", strings.NewReader("{\"custom_id\":\"opaque\"}\n"))
	require.NoError(t, err)
	assert.Equal(t, "processed", file.Status)
	assert.Equal(t, 2, fileGets)
}

func TestOpenAIBatchClientReportsFileProcessingFailureWithoutProviderDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			testwrite.String(t, w, `{"id":"file-error","object":"file","bytes":25,"created_at":1,"filename":"run.jsonl","purpose":"batch","status":"uploaded"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-error":
			testwrite.String(t, w, `{"id":"file-error","object":"file","bytes":25,"created_at":1,"filename":"run.jsonl","purpose":"batch","status":"error","status_details":"private source text"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newStubbedBatchClient(t, server)
	_, err := client.UploadFile(context.Background(), "run.jsonl", strings.NewReader("x"))
	var providerErr *ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, ProviderErrorFileProcessing, providerErr.Class)
	assert.Equal(t, "wait for file processing", providerErr.Operation)
	for _, forbidden := range []string{"file-error", "private", "source"} {
		assert.NotContains(t, err.Error(), forbidden, "error leaked %q: %v", forbidden, err)
	}
}

func TestOpenAIBatchClientTimesOutWaitingForFileProcessing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			testwrite.String(t, w, `{"id":"file-timeout","object":"file","bytes":25,"created_at":1,"filename":"run.jsonl","purpose":"batch","status":"uploaded"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-timeout":
			testwrite.String(t, w, `{"id":"file-timeout","object":"file","bytes":25,"created_at":1,"filename":"run.jsonl","purpose":"batch","status":"pending"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newStubbedBatchClient(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.UploadFile(ctx, "run.jsonl", strings.NewReader("x"))
	var providerErr *ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, ProviderErrorTimeout, providerErr.Class)
	assert.Equal(t, "wait for file processing", providerErr.Operation)
}

func TestOpenAIBatchClientAcceptsRealisticValidatingFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/openai_batch_validating.json")
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		testwrite.Bytes(t, w, fixture)
	}))
	defer server.Close()
	client := newStubbedBatchClient(t, server)
	batch, err := client.GetBatch(context.Background(), "batch-validating")
	require.NoError(t, err)
	assert.Equal(t, BatchStatusValidating, batch.Status)
	assert.Equal(t, BatchRequestCounts{}, batch.RequestCounts)
	assert.Empty(t, batch.OutputFileID)
	assert.Empty(t, batch.ErrorFileID)
}

func TestOpenAIBatchClientClassifiesQueuedTokenValidationFailure(t *testing.T) {
	fixture, err := os.ReadFile("testdata/openai_batch_token_limit_failed.json")
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		testwrite.Bytes(t, w, fixture)
	}))
	defer server.Close()
	client := newStubbedBatchClient(t, server)
	batch, err := client.GetBatch(context.Background(), "batch-token-limit")
	require.NoError(t, err)
	assert.Equal(t, BatchStatusFailed, batch.Status)
	require.Len(t, batch.Errors, 1)
	assert.Equal(t, ProviderErrorRateLimit, batch.Errors[0].Class)
	assert.Equal(t, "token_limit_exceeded", batch.Errors[0].Code)
}

func TestOpenAIBatchClientRejectsIneligibleEndpoints(t *testing.T) {
	for _, baseURL := range []string{
		"http://api.openai.com/v1",
		"https://api.openai.com/v2",
		"https://api.openai.com.evil.example/v1",
		"https://llm.example/v1",
		"https://api.openai.com/v1?tenant=private",
	} {
		_, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret", BaseURL: baseURL}, nil)
		var providerErr *ProviderError
		require.ErrorAs(t, err, &providerErr)
		assert.Equal(t, ProviderErrorIneligibleEndpoint, providerErr.Class, "baseURL=%q", baseURL)
		assert.NotContains(t, err.Error(), baseURL, "baseURL=%q", baseURL)
	}
	_, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{})
	assert.NoError(t, err, "default official endpoint rejected")
}

func TestOpenAIBatchClientRedactsHTTPAndMalformedResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		status    int
		wantClass ProviderErrorClass
	}{
		{"non-2xx", `{"error":{"message":"private source sentence and sk-secret"}}`, http.StatusTooManyRequests, ProviderErrorRateLimit},
		{"malformed JSON", `{"id":"batch-private"`, http.StatusOK, ProviderErrorMalformedResponse},
		{"unknown status", `{"id":"batch-private","object":"batch","status":"mysterious"}`, http.StatusOK, ProviderErrorMalformedResponse},
		{"bounded JSON", strings.Repeat("x", maxOpenAIJSONResponseBytes+1), http.StatusOK, ProviderErrorResponseTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				testwrite.String(t, w, test.response)
			}))
			defer server.Close()
			client := newStubbedBatchClient(t, server)
			_, err := client.GetBatch(context.Background(), "batch-opaque")
			var providerErr *ProviderError
			require.ErrorAs(t, err, &providerErr)
			assert.Equal(t, test.wantClass, providerErr.Class, "err=%v", err)
			for _, forbidden := range []string{"private", "secret", "batch-opaque"} {
				assert.NotContains(t, err.Error(), forbidden, "error leaked %q: %v", forbidden, err)
			}
		})
	}
}

func TestOpenAIBatchClientRedactsTransportErrors(t *testing.T) {
	client, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private source and sk-secret")
	})})
	require.NoError(t, err)
	_, err = client.GetBatch(context.Background(), "batch-opaque")
	var providerErr *ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, ProviderErrorTransport, providerErr.Class)
	assert.NotContains(t, err.Error(), "private")
	assert.NotContains(t, err.Error(), "secret")
}

func TestOpenAIBatchClientValidatesArguments(t *testing.T) {
	client, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{})
	require.NoError(t, err)
	checks := []func() error{
		func() error {
			_, err := client.UploadFile(context.Background(), "owner/private.jsonl", strings.NewReader("x"))
			return err
		},
		func() error { return client.FileContent(context.Background(), "../file", io.Discard) },
		func() error { _, err := client.DeleteFile(context.Background(), ""); return err },
		func() error {
			_, err := client.CreateBatch(context.Background(), CreateBatchRequest{InputFileID: "file-ok", Metadata: map[string]string{"owner": "private email"}})
			return err
		},
		func() error {
			_, err := client.ListBatches(context.Background(), ListBatchesRequest{Limit: 101})
			return err
		},
		func() error { _, err := client.CancelBatch(context.Background(), "batch/id"); return err },
	}
	for i, check := range checks {
		var providerErr *ProviderError
		err := check()
		require.ErrorAs(t, err, &providerErr)
		assert.Equal(t, ProviderErrorInvalidRequest, providerErr.Class, "check %d", i)
	}
}

func TestBatchWireRejectsInconsistentCountsAndUsage(t *testing.T) {
	tests := []batchWire{
		{ID: "batch", Object: "batch", Status: BatchStatusCompleted, RequestCounts: struct {
			Total     int `json:"total"`
			Completed int `json:"completed"`
			Failed    int `json:"failed"`
		}{Total: 1, Completed: 1, Failed: 1}},
		{ID: "batch", Object: "batch", Status: BatchStatusCompleted, RequestCounts: struct {
			Total     int `json:"total"`
			Completed int `json:"completed"`
			Failed    int `json:"failed"`
		}{Total: 1, Completed: 0, Failed: 0}, Usage: struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		}{InputTokens: -1}},
	}
	for _, wire := range tests {
		_, err := wire.batch()
		assert.Error(t, err, "inconsistent Batch accepted: %+v", wire)
	}
}

func writeBatchStub(t *testing.T, w io.Writer, id string, status BatchStatus) {
	testwrite.Fprintf(t, w, `{"id":%q,"object":"batch","endpoint":"/v1/chat/completions","input_file_id":"file-input","completion_window":"24h","status":%q,"output_file_id":"file-output","error_file_id":"file-error","created_at":1,"in_progress_at":2,"expires_at":3,"finalizing_at":4,"completed_at":5,"failed_at":0,"expired_at":0,"cancelling_at":0,"cancelled_at":0,"request_counts":{"total":3,"completed":2,"failed":1},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30},"errors":{"object":"list","data":[{"code":"invalid_request","line":7,"message":"private ignored"}]},"metadata":{"chunk_id":"018f64b6-opaque"}}`, id, status)
}

func newStubbedBatchClient(t *testing.T, server *httptest.Server) *OpenAIBatchClient {
	t.Helper()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{Transport: &rewriteTransport{target: target, base: server.Client().Transport}})
	require.NoError(t, err)
	return client
}

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
