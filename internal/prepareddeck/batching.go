package prepareddeck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const (
	ProviderBatchMaxRequests = 50000
	ProviderBatchMaxBytes    = 200 << 20
	// Four bytes per JSON byte is deliberately conservative for the queued
	// prompt budget. The actual provider tokenizer is not part of this layer.
	DefaultBatchPromptTokenBudget int64 = 1_000_000
)

// BatchChunkLimits contains local ceilings. MaxRequests and MaxBytes are
// clamped to the provider limits; MaxPromptTokens is a conservative account
// budget selected by deployment policy.
type BatchChunkLimits struct {
	MaxRequests     int
	MaxBytes        int64
	MaxPromptTokens int64
}

// PlanBatchChunks serializes each request once, then packs consecutive frozen
// manifest items without crossing any configured/provider ceiling. The
// resulting digest and byte count describe the exact JSONL sent to OpenAI.
func PlanBatchChunks(codec *enrichment.TranslationCodec, runID string, generation int, model, endpoint string, items []enrichment.BatchTranslationItem, limits BatchChunkLimits) ([]persistence.PreparedDeckBatchChunkPlan, error) {
	if codec == nil || len(items) == 0 || generation < 1 || model == "" || endpoint != enrichment.OpenAIChatCompletionsEndpoint {
		return nil, errors.New("prepareddeck: invalid Batch chunk planning input")
	}
	if codec.Model() != model {
		return nil, errors.New("prepareddeck: Batch model does not match translation codec")
	}
	limits = normalizeBatchChunkLimits(limits)
	if limits.MaxRequests < 1 || limits.MaxBytes < 1 || limits.MaxPromptTokens < 1 {
		return nil, errors.New("prepareddeck: invalid Batch chunk limits")
	}

	type encodedItem struct {
		item  enrichment.BatchTranslationItem
		line  []byte
		token int64
	}
	encoded := make([]encodedItem, len(items))
	for i, item := range items {
		if i > 0 && item.Ordinal <= items[i-1].Ordinal {
			return nil, errors.New("prepareddeck: Batch items are not in frozen manifest order")
		}
		var line bytes.Buffer
		if _, err := codec.WriteBatchJSONL(&line, runID, generation, []enrichment.BatchTranslationItem{item}); err != nil {
			return nil, fmt.Errorf("encode Batch item %d: %w", item.Ordinal, err)
		}
		body, err := codec.EncodeRequest(item.Request)
		if err != nil {
			return nil, err
		}
		token := conservativePromptTokens(body)
		if int64(line.Len()) > limits.MaxBytes || token > limits.MaxPromptTokens {
			return nil, fmt.Errorf("prepareddeck: Batch item %d exceeds a configured limit", item.Ordinal)
		}
		encoded[i] = encodedItem{item: item, line: append([]byte(nil), line.Bytes()...), token: token}
	}

	plans := make([]persistence.PreparedDeckBatchChunkPlan, 0, (len(encoded)+limits.MaxRequests-1)/limits.MaxRequests)
	nextSplitReason := "run"
	for position := 0; position < len(encoded); {
		start := position
		var size, tokens int64
		reason := nextSplitReason
		nextSplitReason = "run"
		for position < len(encoded) {
			next := encoded[position]
			if position > start {
				switch {
				case position-start >= limits.MaxRequests:
					nextSplitReason = "request_limit"
				case size+int64(len(next.line)) > limits.MaxBytes:
					nextSplitReason = "byte_limit"
				case tokens+next.token > limits.MaxPromptTokens:
					nextSplitReason = "token_limit"
				}
				if position-start >= limits.MaxRequests || size+int64(len(next.line)) > limits.MaxBytes || tokens+next.token > limits.MaxPromptTokens {
					break
				}
			}
			size += int64(len(next.line))
			tokens += next.token
			position++
		}
		if position == start {
			return nil, errors.New("prepareddeck: Batch chunk planner made no progress")
		}
		var input bytes.Buffer
		ordinals := make([]int, 0, position-start)
		for _, item := range encoded[start:position] {
			_, _ = input.Write(item.line)
			ordinals = append(ordinals, item.item.Ordinal)
		}
		digest := sha256.Sum256(input.Bytes())
		if generation > 1 && len(plans) == 0 {
			reason = "retry"
		}
		plans = append(plans, persistence.PreparedDeckBatchChunkPlan{
			ChunkIndex: len(plans), Generation: generation, Model: model, Endpoint: endpoint,
			SplitReason: reason, InputDigest: hex.EncodeToString(digest[:]), InputBytes: int64(input.Len()),
			EstimatedPromptTokens: tokens, Ordinals: ordinals,
		})
	}
	return plans, nil
}

func normalizeBatchChunkLimits(limits BatchChunkLimits) BatchChunkLimits {
	if limits.MaxRequests == 0 {
		limits.MaxRequests = DefaultBatchMaxRequests
	}
	if limits.MaxBytes == 0 {
		limits.MaxBytes = ProviderBatchMaxBytes
	}
	if limits.MaxPromptTokens == 0 {
		limits.MaxPromptTokens = DefaultBatchPromptTokenBudget
	}
	if limits.MaxRequests > ProviderBatchMaxRequests {
		limits.MaxRequests = ProviderBatchMaxRequests
	}
	if limits.MaxBytes > ProviderBatchMaxBytes {
		limits.MaxBytes = ProviderBatchMaxBytes
	}
	return limits
}

func conservativePromptTokens(body []byte) int64 {
	if len(body) == 0 {
		return 0
	}
	return int64((len(body) + 3) / 4)
}

// writeChunkJSONL regenerates the exact planned stream and checks its frozen
// identity before it crosses the provider boundary.
func writeChunkJSONL(codec *enrichment.TranslationCodec, runID string, generation int, items []enrichment.BatchTranslationItem, chunk domain.PreparedDeckBatchChunk) (io.Reader, error) {
	var content bytes.Buffer
	if _, err := codec.WriteBatchJSONL(&content, runID, generation, items); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(content.Bytes())
	if int64(content.Len()) != chunk.InputBytes || hex.EncodeToString(sum[:]) != chunk.InputDigest || int64(len(items)) != int64(chunk.RequestCount) {
		return nil, errors.New("prepareddeck: durable Batch chunk identity mismatch")
	}
	return bytes.NewReader(content.Bytes()), nil
}
