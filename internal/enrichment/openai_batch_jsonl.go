package enrichment

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	batchCustomIDPrefix     = "prepared-deck"
	maxBatchResultLineBytes = (1 << 20) + (64 << 10)
)

var ErrInvalidBatchCustomID = errors.New("enrichment: invalid Batch custom ID")

type BatchTranslationItem struct {
	Ordinal int
	Request TranslationRequest
}

type BatchItemIdentity struct {
	RunID      string
	Ordinal    int
	Generation int
}

type BatchTranslationOutcome struct {
	CustomID   string
	Ordinal    int
	Response   TranslationResponse
	ErrorClass ProviderErrorClass
	StatusCode int
}

func (o BatchTranslationOutcome) Successful() bool { return o.ErrorClass == ProviderErrorNone }

// BatchCustomID returns the stable correlation key for one run item and retry
// generation. Only an opaque UUID and numeric orchestration identities are
// accepted, preventing source or learner metadata from entering the ID.
func BatchCustomID(runID string, ordinal, generation int) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil || ordinal < 0 || generation < 1 {
		return "", ErrInvalidBatchCustomID
	}
	return batchCustomIDPrefix + ":" + parsed.String() + ":" + strconv.Itoa(ordinal) + ":" + strconv.Itoa(generation), nil
}

func ParseBatchCustomID(customID string) (BatchItemIdentity, error) {
	parts := strings.Split(customID, ":")
	if len(parts) != 4 || parts[0] != batchCustomIDPrefix {
		return BatchItemIdentity{}, ErrInvalidBatchCustomID
	}
	parsed, err := uuid.Parse(parts[1])
	if err != nil || parsed.String() != parts[1] {
		return BatchItemIdentity{}, ErrInvalidBatchCustomID
	}
	ordinal, ordinalErr := strconv.Atoi(parts[2])
	generation, generationErr := strconv.Atoi(parts[3])
	if ordinalErr != nil || generationErr != nil || ordinal < 0 || generation < 1 || strconv.Itoa(ordinal) != parts[2] || strconv.Itoa(generation) != parts[3] {
		return BatchItemIdentity{}, ErrInvalidBatchCustomID
	}
	return BatchItemIdentity{RunID: parsed.String(), Ordinal: ordinal, Generation: generation}, nil
}

// WriteBatchJSONL writes one canonical Chat Completions request per item in
// manifest ordinal order and returns the exact serialized byte count.
func (c *TranslationCodec) WriteBatchJSONL(dst io.Writer, runID string, generation int, items []BatchTranslationItem) (int64, error) {
	if c == nil || dst == nil || len(items) == 0 {
		return 0, providerError("encode Batch JSONL", ProviderErrorInvalidRequest, 0, nil)
	}
	canonicalRunID, err := canonicalBatchRunID(runID)
	if err != nil || generation < 1 {
		return 0, providerError("encode Batch JSONL", ProviderErrorInvalidRequest, 0, err)
	}
	ordered := append([]BatchTranslationItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Ordinal < ordered[j].Ordinal })
	for i, item := range ordered {
		if item.Ordinal < 0 || (i > 0 && item.Ordinal == ordered[i-1].Ordinal) {
			return 0, providerError("encode Batch JSONL", ProviderErrorInvalidRequest, 0, nil)
		}
	}
	counter := &countingWriter{writer: dst}
	encoder := json.NewEncoder(counter)
	encoder.SetEscapeHTML(false)
	for _, item := range ordered {
		customID, customIDErr := BatchCustomID(canonicalRunID, item.Ordinal, generation)
		if customIDErr != nil {
			return 0, providerError("encode Batch JSONL", ProviderErrorInvalidRequest, 0, customIDErr)
		}
		body, encodeErr := c.EncodeRequest(item.Request)
		if encodeErr != nil {
			return 0, providerError("encode Batch JSONL", ProviderErrorInvalidRequest, 0, encodeErr)
		}
		line := struct {
			CustomID string          `json:"custom_id"`
			Method   string          `json:"method"`
			URL      string          `json:"url"`
			Body     json.RawMessage `json:"body"`
		}{customID, "POST", OpenAIChatCompletionsEndpoint, body}
		if encodeErr = encoder.Encode(line); encodeErr != nil {
			return 0, providerError("encode Batch JSONL", ProviderErrorTransport, 0, encodeErr)
		}
	}
	return counter.count, nil
}

// DecodeBatchResults correlates unordered output and error JSONL by custom ID.
// Duplicate, unknown, or malformed identities fail the whole decode because
// their outcomes cannot be trusted. Request and translation failures remain
// typed per-item outcomes.
func (c *TranslationCodec) DecodeBatchResults(runID string, generation int, items []BatchTranslationItem, output, errorOutput io.Reader) (map[int]BatchTranslationOutcome, error) {
	if c == nil || len(items) == 0 {
		return nil, providerError("decode Batch results", ProviderErrorInvalidRequest, 0, nil)
	}
	canonicalRunID, err := canonicalBatchRunID(runID)
	if err != nil || generation < 1 {
		return nil, providerError("decode Batch results", ProviderErrorInvalidRequest, 0, err)
	}
	expected := make(map[int]TranslationRequest, len(items))
	for _, item := range items {
		if item.Ordinal < 0 {
			return nil, providerError("decode Batch results", ProviderErrorInvalidRequest, 0, nil)
		}
		if _, duplicate := expected[item.Ordinal]; duplicate {
			return nil, providerError("decode Batch results", ProviderErrorInvalidRequest, 0, nil)
		}
		expected[item.Ordinal] = item.Request
	}
	outcomes := make(map[int]BatchTranslationOutcome, len(items))
	for _, source := range []io.Reader{output, errorOutput} {
		if source == nil {
			continue
		}
		if err = c.decodeBatchResultReader(source, canonicalRunID, generation, expected, outcomes); err != nil {
			return nil, err
		}
	}
	return outcomes, nil
}

func (c *TranslationCodec) decodeBatchResultReader(source io.Reader, runID string, generation int, expected map[int]TranslationRequest, outcomes map[int]BatchTranslationOutcome) error {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64<<10), maxBatchResultLineBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			return providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
		}
		var wire batchResultWire
		decoder := json.NewDecoder(bytes.NewReader(line))
		if err := decoder.Decode(&wire); err != nil {
			return providerError("decode Batch results", ProviderErrorMalformedResponse, 0, err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
		}
		identity, err := ParseBatchCustomID(wire.CustomID)
		if err != nil || identity.RunID != runID || identity.Generation != generation {
			return providerError("decode Batch results", ProviderErrorMalformedResponse, 0, err)
		}
		request, known := expected[identity.Ordinal]
		if !known {
			return providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
		}
		if _, duplicate := outcomes[identity.Ordinal]; duplicate {
			return providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
		}
		outcome, err := c.decodeBatchResultLine(wire, identity.Ordinal, request)
		if err != nil {
			return err
		}
		outcomes[identity.Ordinal] = outcome
	}
	if err := scanner.Err(); err != nil {
		return providerError("decode Batch results", ProviderErrorResponseTooLarge, 0, err)
	}
	return nil
}

type batchResultWire struct {
	CustomID string `json:"custom_id"`
	Response *struct {
		StatusCode int             `json:"status_code"`
		Body       json.RawMessage `json:"body"`
	} `json:"response"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (c *TranslationCodec) decodeBatchResultLine(wire batchResultWire, ordinal int, request TranslationRequest) (BatchTranslationOutcome, error) {
	if (wire.Response == nil) == (wire.Error == nil) {
		return BatchTranslationOutcome{}, providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
	}
	outcome := BatchTranslationOutcome{CustomID: wire.CustomID, Ordinal: ordinal}
	if wire.Error != nil {
		if strings.TrimSpace(wire.Error.Code) == "" {
			return BatchTranslationOutcome{}, providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
		}
		outcome.ErrorClass = classifyProviderCode(wire.Error.Code)
		return outcome, nil
	}
	outcome.StatusCode = wire.Response.StatusCode
	if wire.Response.StatusCode < 100 || wire.Response.StatusCode > 599 {
		return BatchTranslationOutcome{}, providerError("decode Batch results", ProviderErrorMalformedResponse, 0, nil)
	}
	if wire.Response.StatusCode < 200 || wire.Response.StatusCode >= 300 {
		outcome.ErrorClass = classifyHTTPStatus(wire.Response.StatusCode)
		return outcome, nil
	}
	response, err := c.DecodeResponse(request, wire.Response.Body)
	if err != nil {
		outcome.ErrorClass = ProviderErrorInvalidResponse
		return outcome, nil
	}
	outcome.Response = response
	return outcome, nil
}

func canonicalBatchRunID(runID string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(runID))
	if err != nil {
		return "", ErrInvalidBatchCustomID
	}
	return parsed.String(), nil
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.count += int64(n)
	return n, err
}

func (i BatchItemIdentity) String() string {
	customID, err := BatchCustomID(i.RunID, i.Ordinal, i.Generation)
	if err != nil {
		return ""
	}
	return customID
}

func (o BatchTranslationOutcome) String() string {
	return fmt.Sprintf("Batch translation outcome ordinal=%d class=%s status=%d", o.Ordinal, o.ErrorClass, o.StatusCode)
}
