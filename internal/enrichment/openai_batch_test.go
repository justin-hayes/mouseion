package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestOpenAIBatchClientFilesAndBatchOperations(t *testing.T) {
	var operations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization=%q", r.Header.Get("Authorization"))
		}
		operations = append(operations, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			if r.ContentLength != -1 {
				t.Errorf("multipart upload was buffered: content length=%d", r.ContentLength)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("purpose") != "batch" || r.FormValue("expires_after[anchor]") != "created_at" || r.FormValue("expires_after[seconds]") != "604800" {
				t.Errorf("multipart fields=%v", r.MultipartForm.Value)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			content, _ := io.ReadAll(file)
			if header.Filename != "run-opaque.jsonl" || string(content) != "{\"custom_id\":\"opaque\"}\n" {
				t.Errorf("filename=%q content=%q", header.Filename, content)
			}
			_, _ = io.WriteString(w, `{"id":"file-input","object":"file","bytes":25,"created_at":1,"expires_at":2,"filename":"run-opaque.jsonl","purpose":"batch"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/file-output/content":
			w.Header().Set("Content-Type", "application/jsonl")
			_, _ = io.WriteString(w, "result-line\n")
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/files/file-input":
			_, _ = io.WriteString(w, `{"id":"file-input","object":"file","deleted":true}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["input_file_id"] != "file-input" || request["endpoint"] != OpenAIChatCompletionsEndpoint || request["completion_window"] != "24h" {
				t.Errorf("create request=%v", request)
			}
			metadata := request["metadata"].(map[string]any)
			if metadata["chunk_id"] != "018f64b6-opaque" {
				t.Errorf("metadata=%v", metadata)
			}
			writeBatchStub(w, "batch-created", BatchStatusValidating)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches/batch-created":
			writeBatchStub(w, "batch-created", BatchStatusCompleted)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/batches":
			if r.URL.Query().Get("after") != "batch-before" || r.URL.Query().Get("limit") != "2" {
				t.Errorf("list query=%v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"object":"list","data":[`)
			writeBatchStub(w, "batch-listed", BatchStatusInProgress)
			_, _ = io.WriteString(w, `],"first_id":"batch-listed","last_id":"batch-listed","has_more":false}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/batches/batch-created/cancel":
			writeBatchStub(w, "batch-created", BatchStatusCancelling)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newStubbedBatchClient(t, server)

	file, err := client.UploadFile(context.Background(), "run-opaque.jsonl", strings.NewReader("{\"custom_id\":\"opaque\"}\n"))
	if err != nil || file.ID != "file-input" || file.Purpose != "batch" {
		t.Fatalf("file=%+v err=%v", file, err)
	}
	var content bytes.Buffer
	if err = client.FileContent(context.Background(), "file-output", &content); err != nil || content.String() != "result-line\n" {
		t.Fatalf("content=%q err=%v", content.String(), err)
	}
	deleted, err := client.DeleteFile(context.Background(), "file-input")
	if err != nil || !deleted.Deleted {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	created, err := client.CreateBatch(context.Background(), CreateBatchRequest{InputFileID: "file-input", Metadata: map[string]string{"chunk_id": "018f64b6-opaque"}})
	if err != nil || created.Status != BatchStatusValidating {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	got, err := client.GetBatch(context.Background(), "batch-created")
	if err != nil || got.Status != BatchStatusCompleted || got.RequestCounts != (BatchRequestCounts{Total: 3, Completed: 2, Failed: 1}) || got.Usage.TotalTokens != 30 || got.Errors[0].Class != ProviderErrorInvalidRequest {
		t.Fatalf("batch=%+v err=%v", got, err)
	}
	list, err := client.ListBatches(context.Background(), ListBatchesRequest{After: "batch-before", Limit: 2})
	if err != nil || len(list.Data) != 1 || list.Data[0].ID != "batch-listed" || list.HasMore {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	cancelled, err := client.CancelBatch(context.Background(), "batch-created")
	if err != nil || cancelled.Status != BatchStatusCancelling {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	if len(operations) != 7 {
		t.Fatalf("operations=%v", operations)
	}
}

func TestOpenAIBatchClientAcceptsRealisticValidatingFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/openai_batch_validating.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	client := newStubbedBatchClient(t, server)
	batch, err := client.GetBatch(context.Background(), "batch-validating")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != BatchStatusValidating || batch.RequestCounts != (BatchRequestCounts{}) || batch.OutputFileID != "" || batch.ErrorFileID != "" {
		t.Fatalf("validating Batch=%+v", batch)
	}
}

func TestOpenAIBatchClientClassifiesQueuedTokenValidationFailure(t *testing.T) {
	fixture, err := os.ReadFile("testdata/openai_batch_token_limit_failed.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	client := newStubbedBatchClient(t, server)
	batch, err := client.GetBatch(context.Background(), "batch-token-limit")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != BatchStatusFailed || len(batch.Errors) != 1 || batch.Errors[0].Class != ProviderErrorRateLimit || batch.Errors[0].Code != "token_limit_exceeded" {
		t.Fatalf("queued-token Batch=%+v", batch)
	}
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
		if !errors.As(err, &providerErr) || providerErr.Class != ProviderErrorIneligibleEndpoint || strings.Contains(err.Error(), baseURL) {
			t.Fatalf("baseURL=%q err=%v", baseURL, err)
		}
	}
	if _, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{}); err != nil {
		t.Fatalf("default official endpoint rejected: %v", err)
	}
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
				_, _ = io.WriteString(w, test.response)
			}))
			defer server.Close()
			client := newStubbedBatchClient(t, server)
			_, err := client.GetBatch(context.Background(), "batch-opaque")
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Class != test.wantClass {
				t.Fatalf("err=%v", err)
			}
			for _, forbidden := range []string{"private", "secret", "batch-opaque"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("error leaked %q: %v", forbidden, err)
				}
			}
		})
	}
}

func TestOpenAIBatchClientRedactsTransportErrors(t *testing.T) {
	client, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("private source and sk-secret")
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetBatch(context.Background(), "batch-opaque")
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Class != ProviderErrorTransport || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenAIBatchClientValidatesArguments(t *testing.T) {
	client, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
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
		if err := check(); !errors.As(err, &providerErr) || providerErr.Class != ProviderErrorInvalidRequest {
			t.Fatalf("check %d err=%v", i, err)
		}
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
		if _, err := wire.batch(); err == nil {
			t.Fatalf("inconsistent Batch accepted: %+v", wire)
		}
	}
}

func writeBatchStub(w io.Writer, id string, status BatchStatus) {
	_, _ = fmt.Fprintf(w, `{"id":%q,"object":"batch","endpoint":"/v1/chat/completions","input_file_id":"file-input","completion_window":"24h","status":%q,"output_file_id":"file-output","error_file_id":"file-error","created_at":1,"in_progress_at":2,"expires_at":3,"finalizing_at":4,"completed_at":5,"failed_at":0,"expired_at":0,"cancelling_at":0,"cancelled_at":0,"request_counts":{"total":3,"completed":2,"failed":1},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30},"errors":{"object":"list","data":[{"code":"invalid_request","line":7,"message":"private ignored"}]},"metadata":{"chunk_id":"018f64b6-opaque"}}`, id, status)
}

func newStubbedBatchClient(t *testing.T, server *httptest.Server) *OpenAIBatchClient {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewOpenAIBatchClient(LLMConfig{APIKey: "secret"}, &http.Client{Transport: &rewriteTransport{target: target, base: server.Client().Transport}})
	if err != nil {
		t.Fatal(err)
	}
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
