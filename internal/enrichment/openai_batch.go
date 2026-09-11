package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	OpenAIChatCompletionsEndpoint = "/v1/chat/completions"
	openAIBatchCompletionWindow   = "24h"
	openAIBatchFileExpiration     = 7 * 24 * time.Hour
	maxOpenAIJSONResponseBytes    = 4 << 20
	maxOpenAIBatchFileBytes       = 200 << 20
	openAIFileReadyTimeout        = 30 * time.Second
	openAIFileInitialPollInterval = 250 * time.Millisecond
	openAIFileMaxPollInterval     = 2 * time.Second
)

var errBatchFileTooLarge = errors.New("Batch file exceeds provider limit")
var errOpenAIFileNotReady = errors.New("provider file is not ready")

// ProviderErrorClass is a bounded, privacy-safe classification for failures at
// the OpenAI Files and Batch boundary.
type ProviderErrorClass string

const (
	ProviderErrorNone               ProviderErrorClass = ""
	ProviderErrorInvalidRequest     ProviderErrorClass = "invalid_request"
	ProviderErrorIneligibleEndpoint ProviderErrorClass = "ineligible_endpoint"
	ProviderErrorAuthentication     ProviderErrorClass = "authentication"
	ProviderErrorPermission         ProviderErrorClass = "permission"
	ProviderErrorRateLimit          ProviderErrorClass = "rate_limit"
	ProviderErrorTimeout            ProviderErrorClass = "timeout"
	ProviderErrorUnavailable        ProviderErrorClass = "provider_unavailable"
	ProviderErrorTransport          ProviderErrorClass = "transport"
	ProviderErrorMalformedResponse  ProviderErrorClass = "malformed_response"
	ProviderErrorResponseTooLarge   ProviderErrorClass = "response_too_large"
	ProviderErrorInvalidResponse    ProviderErrorClass = "invalid_translation_response"
	ProviderErrorExpired            ProviderErrorClass = "expired"
	ProviderErrorCancelled          ProviderErrorClass = "cancelled"
	ProviderErrorRequestFailed      ProviderErrorClass = "request_failed"
	ProviderErrorFileProcessing     ProviderErrorClass = "file_processing_failed"
)

// ProviderError omits URLs, object IDs, provider bodies, credentials, prompts,
// and source text from Error so it is safe for ordinary structured logging.
type ProviderError struct {
	Operation  string
	Class      ProviderErrorClass
	StatusCode int
	cause      error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "OpenAI provider error"
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("OpenAI %s failed (%s, HTTP %d)", e.Operation, e.Class, e.StatusCode)
	}
	return fmt.Sprintf("OpenAI %s failed (%s)", e.Operation, e.Class)
}

func (e *ProviderError) Unwrap() error { return e.cause }

func (e *ProviderError) Temporary() bool {
	return e != nil && (e.Class == ProviderErrorRateLimit || e.Class == ProviderErrorTimeout || e.Class == ProviderErrorUnavailable || e.Class == ProviderErrorTransport)
}

// BatchStatus is the complete provider lifecycle status set.
type BatchStatus string

const (
	BatchStatusValidating BatchStatus = "validating"
	BatchStatusFailed     BatchStatus = "failed"
	BatchStatusInProgress BatchStatus = "in_progress"
	BatchStatusFinalizing BatchStatus = "finalizing"
	BatchStatusCompleted  BatchStatus = "completed"
	BatchStatusExpired    BatchStatus = "expired"
	BatchStatusCancelling BatchStatus = "cancelling"
	BatchStatusCancelled  BatchStatus = "cancelled"
)

func (s BatchStatus) valid() bool {
	switch s {
	case BatchStatusValidating, BatchStatusFailed, BatchStatusInProgress, BatchStatusFinalizing, BatchStatusCompleted, BatchStatusExpired, BatchStatusCancelling, BatchStatusCancelled:
		return true
	default:
		return false
	}
}

type OpenAIFile struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	Status    string `json:"status"`
}

type OpenAIFileDeletion struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

type BatchRequestCounts struct {
	Total, Completed, Failed int
}

type BatchUsage struct {
	InputTokens, OutputTokens, TotalTokens int64
}

type BatchIssue struct {
	Class ProviderErrorClass
	Code  string
	Line  int
}

type Batch struct {
	ID, InputFileID, OutputFileID, ErrorFileID string
	Endpoint, CompletionWindow                 string
	Status                                     BatchStatus
	RequestCounts                              BatchRequestCounts
	Usage                                      BatchUsage
	Metadata                                   map[string]string
	Errors                                     []BatchIssue
	CreatedAt, InProgressAt, ExpiresAt         int64
	FinalizingAt, CompletedAt, FailedAt        int64
	ExpiredAt, CancellingAt, CancelledAt       int64
}

type BatchList struct {
	Data            []Batch
	FirstID, LastID string
	HasMore         bool
}

type CreateBatchRequest struct {
	InputFileID string
	Metadata    map[string]string
}

type ListBatchesRequest struct {
	After string
	Limit int
}

// OpenAIBatchClient is a deliberately narrow adapter for the official OpenAI
// Files and Batch endpoints. Constructing it is the explicit Batch enablement
// boundary; custom OpenAI-compatible base URLs are rejected.
type OpenAIBatchClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenAIBatchClient(cfg LLMConfig, client *http.Client) (*OpenAIBatchClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, providerError("configure Batch client", ProviderErrorInvalidRequest, 0, nil)
	}
	base, parsed, err := parseLLMBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, providerError("configure Batch client", ProviderErrorIneligibleEndpoint, 0, err)
	}
	if !officialOpenAIBaseURL(parsed) {
		return nil, providerError("configure Batch client", ProviderErrorIneligibleEndpoint, 0, nil)
	}
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultLLMTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAIBatchClient{apiKey: strings.TrimSpace(cfg.APIKey), baseURL: base, httpClient: client}, nil
}

func officialOpenAIBaseURL(base *url.URL) bool {
	if base == nil || !strings.EqualFold(base.Scheme, "https") || strings.ToLower(strings.TrimSuffix(base.Hostname(), ".")) != "api.openai.com" {
		return false
	}
	if port := base.Port(); port != "" && port != "443" {
		return false
	}
	return strings.TrimRight(base.EscapedPath(), "/") == "/v1" && base.User == nil && base.RawQuery == "" && base.Fragment == "" && base.RawPath == ""
}

// UploadFile streams one JSONL input file as multipart form data. The provider
// is instructed to expire the temporary file seven days after creation.
func (c *OpenAIBatchClient) UploadFile(ctx context.Context, filename string, content io.Reader) (OpenAIFile, error) {
	if c == nil || content == nil || !privacySafeFilename(filename) {
		return OpenAIFile{}, providerError("upload file", ProviderErrorInvalidRequest, 0, nil)
	}
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	writeResult := make(chan error, 1)
	go func() {
		err := writeBatchMultipart(multipartWriter, filename, content)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
		writeResult <- err
	}()
	req, err := c.newRequest(ctx, http.MethodPost, "/files", reader)
	if err != nil {
		_ = reader.CloseWithError(err)
		<-writeResult
		return OpenAIFile{}, err
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	var file OpenAIFile
	err = c.doJSON(req, "upload file", &file)
	_ = reader.CloseWithError(err)
	if writeErr := <-writeResult; writeErr != nil && err == nil {
		class := ProviderErrorTransport
		if errors.Is(writeErr, errBatchFileTooLarge) {
			class = ProviderErrorResponseTooLarge
		}
		err = providerError("upload file", class, 0, writeErr)
	}
	if err != nil {
		return OpenAIFile{}, err
	}
	if !validOpenAIFile(file) {
		return OpenAIFile{}, providerError("upload file", ProviderErrorMalformedResponse, 0, nil)
	}
	return c.waitForFileReady(ctx, file)
}

// GetFile returns a Batch input file using the same authenticated client that
// uploaded it. File processing is asynchronous, so callers must not infer
// Batch readiness from the upload response alone.
func (c *OpenAIBatchClient) GetFile(ctx context.Context, fileID string) (OpenAIFile, error) {
	if c == nil || !validProviderObjectID(fileID) {
		return OpenAIFile{}, providerError("get file", ProviderErrorInvalidRequest, 0, nil)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/files/"+url.PathEscape(fileID), nil)
	if err != nil {
		return OpenAIFile{}, err
	}
	var file OpenAIFile
	if err = c.doJSON(req, "get file", &file); err != nil {
		return OpenAIFile{}, err
	}
	if !validOpenAIFile(file) {
		return OpenAIFile{}, providerError("get file", ProviderErrorMalformedResponse, 0, nil)
	}
	return file, nil
}

func validOpenAIFile(file OpenAIFile) bool {
	if file.ID == "" || file.Object != "file" || file.Purpose != "batch" || file.Bytes < 0 || file.Bytes > maxOpenAIBatchFileBytes {
		return false
	}
	switch file.Status {
	case "uploaded", "pending", "processed", "error":
		return true
	default:
		return false
	}
}

func (c *OpenAIBatchClient) waitForFileReady(ctx context.Context, uploaded OpenAIFile) (OpenAIFile, error) {
	pollCtx, cancel := context.WithTimeout(ctx, openAIFileReadyTimeout)
	defer cancel()

	backoffPolicy := backoff.NewExponentialBackOff(
		backoff.WithInitialInterval(openAIFileInitialPollInterval),
		backoff.WithMultiplier(2),
		backoff.WithRandomizationFactor(0),
		backoff.WithMaxInterval(openAIFileMaxPollInterval),
		backoff.WithMaxElapsedTime(0),
	)
	file, err := backoff.RetryWithData(func() (OpenAIFile, error) {
		file, err := c.GetFile(pollCtx, uploaded.ID)
		if err != nil {
			if errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
				return OpenAIFile{}, backoff.Permanent(fileReadinessTimeoutError())
			}
			if !temporaryOpenAIError(err) {
				return OpenAIFile{}, backoff.Permanent(err)
			}
			return OpenAIFile{}, err
		}
		switch file.Status {
		case "processed":
			return file, nil
		case "error":
			return OpenAIFile{}, backoff.Permanent(providerError("wait for file processing", ProviderErrorFileProcessing, 0, errors.New("provider reported terminal file processing failure")))
		default:
			return OpenAIFile{}, errOpenAIFileNotReady
		}
	}, backoff.WithContext(backoffPolicy, pollCtx))
	if err == nil {
		return file, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OpenAIFile{}, fileReadinessTimeoutError()
	}
	if errors.Is(err, context.Canceled) {
		return OpenAIFile{}, providerError("wait for file processing", ProviderErrorTransport, 0, err)
	}
	return OpenAIFile{}, err
}

func fileReadinessTimeoutError() error {
	return providerError("wait for file processing", ProviderErrorTimeout, 0, errors.New("file processing did not complete before the provider readiness deadline"))
}

func temporaryOpenAIError(err error) bool {
	var providerErr *ProviderError
	return errors.As(err, &providerErr) && providerErr.Temporary()
}

func writeBatchMultipart(writer *multipart.Writer, filename string, content io.Reader) error {
	for _, field := range []struct{ name, value string }{
		{"purpose", "batch"},
		{"expires_after[anchor]", "created_at"},
		{"expires_after[seconds]", strconv.FormatInt(int64(openAIBatchFileExpiration/time.Second), 10)},
	} {
		if err := writer.WriteField(field.name, field.value); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	written, err := io.Copy(part, io.LimitReader(content, maxOpenAIBatchFileBytes+1))
	if err != nil {
		return err
	}
	if written > maxOpenAIBatchFileBytes {
		return errBatchFileTooLarge
	}
	return nil
}

// FileContent streams a provider file to dst and rejects content larger than
// the Batch input/output safety ceiling.
func (c *OpenAIBatchClient) FileContent(ctx context.Context, fileID string, dst io.Writer) error {
	if c == nil || dst == nil || !validProviderObjectID(fileID) {
		return providerError("get file content", ProviderErrorInvalidRequest, 0, nil)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/files/"+url.PathEscape(fileID)+"/content", nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req, "get file content")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	written, copyErr := io.CopyN(dst, resp.Body, maxOpenAIBatchFileBytes+1)
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return providerError("get file content", ProviderErrorTransport, 0, copyErr)
	}
	if written > maxOpenAIBatchFileBytes {
		return providerError("get file content", ProviderErrorResponseTooLarge, 0, nil)
	}
	return nil
}

func (c *OpenAIBatchClient) DeleteFile(ctx context.Context, fileID string) (OpenAIFileDeletion, error) {
	if c == nil || !validProviderObjectID(fileID) {
		return OpenAIFileDeletion{}, providerError("delete file", ProviderErrorInvalidRequest, 0, nil)
	}
	req, err := c.newRequest(ctx, http.MethodDelete, "/files/"+url.PathEscape(fileID), nil)
	if err != nil {
		return OpenAIFileDeletion{}, err
	}
	var deleted OpenAIFileDeletion
	if err = c.doJSON(req, "delete file", &deleted); err != nil {
		return OpenAIFileDeletion{}, err
	}
	if deleted.ID == "" || deleted.Object != "file" || !deleted.Deleted {
		return OpenAIFileDeletion{}, providerError("delete file", ProviderErrorMalformedResponse, 0, nil)
	}
	return deleted, nil
}

func (c *OpenAIBatchClient) CreateBatch(ctx context.Context, input CreateBatchRequest) (Batch, error) {
	if c == nil || !validProviderObjectID(input.InputFileID) || !validBatchMetadata(input.Metadata) {
		return Batch{}, providerError("create batch", ProviderErrorInvalidRequest, 0, nil)
	}
	body, err := json.Marshal(struct {
		InputFileID      string            `json:"input_file_id"`
		Endpoint         string            `json:"endpoint"`
		CompletionWindow string            `json:"completion_window"`
		Metadata         map[string]string `json:"metadata,omitempty"`
	}{input.InputFileID, OpenAIChatCompletionsEndpoint, openAIBatchCompletionWindow, input.Metadata})
	if err != nil {
		return Batch{}, providerError("create batch", ProviderErrorInvalidRequest, 0, err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/batches", bytes.NewReader(body))
	if err != nil {
		return Batch{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.decodeBatch(req, "create batch")
}

func (c *OpenAIBatchClient) GetBatch(ctx context.Context, batchID string) (Batch, error) {
	if c == nil || !validProviderObjectID(batchID) {
		return Batch{}, providerError("get batch", ProviderErrorInvalidRequest, 0, nil)
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/batches/"+url.PathEscape(batchID), nil)
	if err != nil {
		return Batch{}, err
	}
	return c.decodeBatch(req, "get batch")
}

func (c *OpenAIBatchClient) ListBatches(ctx context.Context, input ListBatchesRequest) (BatchList, error) {
	if c == nil || input.Limit < 0 || input.Limit > 100 || (input.After != "" && !validProviderObjectID(input.After)) {
		return BatchList{}, providerError("list batches", ProviderErrorInvalidRequest, 0, nil)
	}
	query := url.Values{}
	if input.After != "" {
		query.Set("after", input.After)
	}
	if input.Limit != 0 {
		query.Set("limit", strconv.Itoa(input.Limit))
	}
	path := "/batches"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return BatchList{}, err
	}
	var wire struct {
		Object  string      `json:"object"`
		Data    []batchWire `json:"data"`
		FirstID string      `json:"first_id"`
		LastID  string      `json:"last_id"`
		HasMore bool        `json:"has_more"`
	}
	if err = c.doJSON(req, "list batches", &wire); err != nil {
		return BatchList{}, err
	}
	if wire.Object != "list" {
		return BatchList{}, providerError("list batches", ProviderErrorMalformedResponse, 0, nil)
	}
	result := BatchList{Data: make([]Batch, len(wire.Data)), FirstID: wire.FirstID, LastID: wire.LastID, HasMore: wire.HasMore}
	for i := range wire.Data {
		result.Data[i], err = wire.Data[i].batch()
		if err != nil {
			return BatchList{}, providerError("list batches", ProviderErrorMalformedResponse, 0, err)
		}
	}
	return result, nil
}

func (c *OpenAIBatchClient) CancelBatch(ctx context.Context, batchID string) (Batch, error) {
	if c == nil || !validProviderObjectID(batchID) {
		return Batch{}, providerError("cancel batch", ProviderErrorInvalidRequest, 0, nil)
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/batches/"+url.PathEscape(batchID)+"/cancel", nil)
	if err != nil {
		return Batch{}, err
	}
	return c.decodeBatch(req, "cancel batch")
}

func (c *OpenAIBatchClient) decodeBatch(req *http.Request, operation string) (Batch, error) {
	var wire batchWire
	if err := c.doJSON(req, operation, &wire); err != nil {
		return Batch{}, err
	}
	batch, err := wire.batch()
	if err != nil {
		return Batch{}, providerError(operation, ProviderErrorMalformedResponse, 0, err)
	}
	return batch, nil
}

type batchWire struct {
	ID               string            `json:"id"`
	Object           string            `json:"object"`
	InputFileID      string            `json:"input_file_id"`
	OutputFileID     string            `json:"output_file_id"`
	ErrorFileID      string            `json:"error_file_id"`
	Endpoint         string            `json:"endpoint"`
	CompletionWindow string            `json:"completion_window"`
	Status           BatchStatus       `json:"status"`
	Metadata         map[string]string `json:"metadata"`
	RequestCounts    struct {
		Total     int `json:"total"`
		Completed int `json:"completed"`
		Failed    int `json:"failed"`
	} `json:"request_counts"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
	Errors *struct {
		Data []struct {
			Code string `json:"code"`
			Line int    `json:"line"`
		} `json:"data"`
	} `json:"errors"`
	CreatedAt    int64 `json:"created_at"`
	InProgressAt int64 `json:"in_progress_at"`
	ExpiresAt    int64 `json:"expires_at"`
	FinalizingAt int64 `json:"finalizing_at"`
	CompletedAt  int64 `json:"completed_at"`
	FailedAt     int64 `json:"failed_at"`
	ExpiredAt    int64 `json:"expired_at"`
	CancellingAt int64 `json:"cancelling_at"`
	CancelledAt  int64 `json:"cancelled_at"`
}

func (w batchWire) batch() (Batch, error) {
	if w.ID == "" || w.Object != "batch" || !w.Status.valid() ||
		w.RequestCounts.Total < 0 || w.RequestCounts.Completed < 0 || w.RequestCounts.Failed < 0 ||
		w.RequestCounts.Completed > w.RequestCounts.Total || w.RequestCounts.Failed > w.RequestCounts.Total ||
		w.RequestCounts.Completed+w.RequestCounts.Failed > w.RequestCounts.Total ||
		w.Usage.InputTokens < 0 || w.Usage.OutputTokens < 0 || w.Usage.TotalTokens < 0 {
		return Batch{}, errors.New("invalid Batch object")
	}
	result := Batch{
		ID: w.ID, InputFileID: w.InputFileID, OutputFileID: w.OutputFileID, ErrorFileID: w.ErrorFileID,
		Endpoint: w.Endpoint, CompletionWindow: w.CompletionWindow, Status: w.Status, Metadata: w.Metadata,
		RequestCounts: BatchRequestCounts{Total: w.RequestCounts.Total, Completed: w.RequestCounts.Completed, Failed: w.RequestCounts.Failed},
		Usage:         BatchUsage{InputTokens: w.Usage.InputTokens, OutputTokens: w.Usage.OutputTokens, TotalTokens: w.Usage.TotalTokens},
		CreatedAt:     w.CreatedAt, InProgressAt: w.InProgressAt, ExpiresAt: w.ExpiresAt, FinalizingAt: w.FinalizingAt,
		CompletedAt: w.CompletedAt, FailedAt: w.FailedAt, ExpiredAt: w.ExpiredAt, CancellingAt: w.CancellingAt, CancelledAt: w.CancelledAt,
	}
	if w.Errors != nil {
		result.Errors = make([]BatchIssue, len(w.Errors.Data))
		for i, issue := range w.Errors.Data {
			result.Errors[i] = BatchIssue{Class: classifyProviderCode(issue.Code), Code: boundedProviderIssueCode(issue.Code), Line: issue.Line}
		}
	}
	return result, nil
}

func (c *OpenAIBatchClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if c == nil || c.httpClient == nil {
		return nil, providerError("create request", ProviderErrorInvalidRequest, 0, nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, providerError("create request", ProviderErrorInvalidRequest, 0, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c *OpenAIBatchClient) doJSON(req *http.Request, operation string, dst any) error {
	resp, err := c.do(req, operation)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAIJSONResponseBytes+1))
	if err != nil {
		return providerError(operation, ProviderErrorTransport, 0, err)
	}
	if len(body) > maxOpenAIJSONResponseBytes {
		return providerError(operation, ProviderErrorResponseTooLarge, 0, nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err = decoder.Decode(dst); err != nil {
		return providerError(operation, ProviderErrorMalformedResponse, 0, err)
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return providerError(operation, ProviderErrorMalformedResponse, 0, nil)
	}
	return nil
}

func (c *OpenAIBatchClient) do(req *http.Request, operation string) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, providerError(operation, ProviderErrorTransport, 0, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
		return nil, providerError(operation, classifyHTTPStatus(resp.StatusCode), resp.StatusCode, nil)
	}
	return resp, nil
}

func providerError(operation string, class ProviderErrorClass, status int, cause error) *ProviderError {
	return &ProviderError{Operation: operation, Class: class, StatusCode: status, cause: cause}
}

func classifyHTTPStatus(status int) ProviderErrorClass {
	switch {
	case status == http.StatusUnauthorized:
		return ProviderErrorAuthentication
	case status == http.StatusForbidden:
		return ProviderErrorPermission
	case status == http.StatusRequestTimeout:
		return ProviderErrorTimeout
	case status == http.StatusTooManyRequests:
		return ProviderErrorRateLimit
	case status >= 500:
		return ProviderErrorUnavailable
	case status >= 400:
		return ProviderErrorInvalidRequest
	default:
		return ProviderErrorRequestFailed
	}
}

func classifyProviderCode(code string) ProviderErrorClass {
	normalized := strings.ToLower(strings.TrimSpace(code))
	switch {
	case normalized == "batch_expired":
		return ProviderErrorExpired
	case normalized == "batch_cancelled":
		return ProviderErrorCancelled
	case normalized == "token_limit_exceeded":
		return ProviderErrorRateLimit
	case normalized == "request_timeout" || strings.Contains(normalized, "timeout"):
		return ProviderErrorTimeout
	case strings.Contains(normalized, "rate_limit"):
		return ProviderErrorRateLimit
	case strings.Contains(normalized, "server") || strings.Contains(normalized, "unavailable"):
		return ProviderErrorUnavailable
	case strings.Contains(normalized, "invalid"):
		return ProviderErrorInvalidRequest
	default:
		return ProviderErrorRequestFailed
	}
}

func boundedProviderIssueCode(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))
	if normalized == "token_limit_exceeded" {
		return normalized
	}
	return string(classifyProviderCode(normalized))
}

func privacySafeFilename(filename string) bool {
	if filename == "" || len(filename) > 128 || filepath.Base(filename) != filename || !strings.HasSuffix(filename, ".jsonl") {
		return false
	}
	for _, r := range filename {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validProviderObjectID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validBatchMetadata(metadata map[string]string) bool {
	if len(metadata) > 16 {
		return false
	}
	for key, value := range metadata {
		if !validOpaqueMetadataPart(key, 64) || !validOpaqueMetadataPart(value, 512) {
			return false
		}
	}
	return true
}

func validOpaqueMetadataPart(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' && r != ':' {
			return false
		}
	}
	return true
}
