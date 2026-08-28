// Command batch-validation is an explicit operator tool. It is not imported
// by the server and has no environment-controlled transport selection.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/batchvalidation"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

const paidCallAcknowledgement = "I understand this makes paid OpenAI calls"

func main() {
	var fixtureDir, reportPath, runID, acknowledgement string
	var realProvider bool
	var pollInterval time.Duration
	var inputCostPerMillion, outputCostPerMillion, batchDiscount float64
	flag.StringVar(&fixtureDir, "fixture-dir", "internal/batchvalidation/testdata", "directory containing the frozen manifest, request, and response fixtures")
	flag.StringVar(&reportPath, "report", "-", "report path, or - for stdout")
	flag.StringVar(&runID, "run-id", batchvalidation.DefaultRunID, "opaque run UUID used for Batch custom IDs")
	flag.BoolVar(&realProvider, "real-provider", false, "run the explicitly requested synchronous and OpenAI Batch provider comparison")
	flag.StringVar(&acknowledgement, "acknowledge-paid-provider-calls", "", "must exactly acknowledge paid calls when -real-provider is set")
	flag.DurationVar(&pollInterval, "poll-interval", 30*time.Second, "real-provider Batch observation interval")
	flag.Float64Var(&inputCostPerMillion, "input-cost-per-million", 0, "provider input-token price in USD per million tokens (required for real-provider evidence)")
	flag.Float64Var(&outputCostPerMillion, "output-cost-per-million", 0, "provider output-token price in USD per million tokens (required for real-provider evidence)")
	flag.Float64Var(&batchDiscount, "batch-discount", 0.5, "Batch price multiplier used for the estimate")
	flag.Parse()

	if realProvider && acknowledgement != paidCallAcknowledgement {
		fatalf("-real-provider requires -acknowledge-paid-provider-calls=%q", paidCallAcknowledgement)
	}
	if pollInterval <= 0 || pollInterval > 24*time.Hour {
		fatalf("-poll-interval must be positive and no greater than 24h")
	}
	if inputCostPerMillion < 0 || outputCostPerMillion < 0 || batchDiscount < 0 || batchDiscount > 1 {
		fatalf("cost rates must be nonnegative and -batch-discount must be between 0 and 1")
	}
	if realProvider && (inputCostPerMillion == 0 || outputCostPerMillion == 0) {
		fatalf("real-provider evidence requires nonzero -input-cost-per-million and -output-cost-per-million")
	}

	fixture, frozenRequests, err := batchvalidation.LoadFixture(batchvalidation.ReadDefaultFiles(fixtureDir))
	if err != nil {
		fatalf("load fixture: %v", err)
	}
	manifestDigest, err := fixture.ManifestDigest()
	if err != nil {
		fatalf("digest frozen manifest: %v", err)
	}
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: fixture.Model})
	if err != nil {
		fatalf("configure fixture codec: %v", err)
	}
	syntheticSync, syntheticBatch, err := batchvalidation.ReplayFixedResponses(context.Background(), codec, fixture, frozenRequests, runID, batchvalidation.DefaultGeneration)
	if err != nil {
		fatalf("synthetic replay: %v", err)
	}
	report := batchvalidation.Report{FixtureName: fixture.Name, ManifestDigest: manifestDigest, Provider: fixture.Provider, ProviderVersion: fixture.ProviderVersion, Model: fixture.Model, Prompt: codec.PromptVersion(), Synthetic: batchvalidation.Comparison{Synchronous: syntheticSync, Batch: syntheticBatch}}
	if realProvider {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		realComparison, runErr := runReal(ctx, fixture, frozenRequests, codec, runID, pollInterval, inputCostPerMillion, outputCostPerMillion, batchDiscount)
		if runErr != nil {
			fatalf("real-provider validation: %v", runErr)
		}
		report.Real = &realComparison
	}
	if err := writeReport(reportPath, report); err != nil {
		fatalf("write report: %v", err)
	}
}

func runReal(ctx context.Context, fixture batchvalidation.Fixture, frozenRequests []byte, codec *enrichment.TranslationCodec, runID string, pollInterval time.Duration, inputCostPerMillion, outputCostPerMillion, batchDiscount float64) (batchvalidation.Comparison, error) {
	cfg := enrichment.LLMConfig{APIKey: strings.TrimSpace(os.Getenv("MOUSEION_LLM_API_KEY")), Model: strings.TrimSpace(os.Getenv("MOUSEION_LLM_MODEL")), BaseURL: strings.TrimSpace(os.Getenv("MOUSEION_LLM_BASE_URL")), Timeout: 30 * time.Second}
	if cfg.APIKey == "" || cfg.Model == "" {
		return batchvalidation.Comparison{}, errors.New("MOUSEION_LLM_API_KEY and MOUSEION_LLM_MODEL are required; MOUSEION_LLM_ENABLED is not consulted")
	}
	if cfg.Model != fixture.Model {
		return batchvalidation.Comparison{}, fmt.Errorf("configured model %q does not match frozen fixture model %q", cfg.Model, fixture.Model)
	}
	if fixture.Provider != "openai" || fixture.PromptVersion != codec.PromptVersion() {
		return batchvalidation.Comparison{}, errors.New("frozen fixture provider or prompt version does not match the validation contract")
	}
	cfg.ReasoningEffort = strings.TrimSpace(os.Getenv("MOUSEION_LLM_REASONING_EFFORT"))
	if value := strings.TrimSpace(os.Getenv("MOUSEION_LLM_SUPPORTS_REASONING_EFFORT")); value != "" {
		supports, parseErr := strconv.ParseBool(value)
		if parseErr != nil {
			return batchvalidation.Comparison{}, errors.New("MOUSEION_LLM_SUPPORTS_REASONING_EFFORT must be a boolean")
		}
		cfg.SupportsReasoningEffort = supports
	}
	providerCodec, err := enrichment.NewTranslationCodec(cfg)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	if providerCodec.PromptVersion() != codec.PromptVersion() || providerCodec.Model() != fixture.Model {
		return batchvalidation.Comparison{}, errors.New("provider codec does not match frozen request contract")
	}
	if _, _, err = batchvalidation.ReplayFixedResponses(context.Background(), providerCodec, fixture, frozenRequests, runID, batchvalidation.DefaultGeneration); err != nil {
		return batchvalidation.Comparison{}, fmt.Errorf("provider codec changed frozen request/response semantics: %w", err)
	}
	syncClient, err := enrichment.NewOpenAITranslationClient(cfg, nil)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	batchClient, err := enrichment.NewOpenAIBatchClient(cfg, nil)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	requests, items, err := batchvalidation.FrozenRequests(providerCodec, fixture, runID, batchvalidation.DefaultGeneration)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	start := time.Now()
	syncResponses := make(map[int]enrichment.TranslationResponse)
	var syncMetrics batchvalidation.Metrics
	for _, fixtureItem := range fixture.Items {
		if fixtureItem.Disposition != "accepted" {
			syncMetrics.Omissions++
			continue
		}
		if fixtureItem.CacheHit {
			fixed, ok := fixture.Response(fixtureItem.Ordinal)
			if !ok {
				return batchvalidation.Comparison{}, fmt.Errorf("missing fixed cache response for %d", fixtureItem.Ordinal)
			}
			syncResponses[fixtureItem.Ordinal] = enrichment.TranslationResponse{Translation: fixed.Translation, Gloss: fixed.Gloss, SentenceTranslation: fixed.SentenceTranslation, SentenceTranslationTarget: fixed.SentenceTranslationTarget}
			syncMetrics.CacheHits++
			continue
		}
		syncMetrics.CacheMisses++
		syncMetrics.ProviderCalls++
		item := items[itemIndex(items, fixtureItem.Ordinal)]
		response, usage, callErr := syncClient.TranslateWithUsage(ctx, item.Request)
		if callErr != nil {
			syncMetrics.ProviderErrors++
			if ctx.Err() != nil {
				return batchvalidation.Comparison{}, fmt.Errorf("synchronous call cancelled: %w", ctx.Err())
			}
			continue
		}
		syncResponses[fixtureItem.Ordinal] = response
		syncMetrics.InputTokens += usage.PromptTokens
		syncMetrics.OutputTokens += usage.CompletionTokens
	}
	syncMetrics.Completed = len(syncResponses)
	syncMetrics.EstimatedCostUSD = estimateCost(syncMetrics.InputTokens, syncMetrics.OutputTokens, inputCostPerMillion, outputCostPerMillion, 1)
	syncMetrics.TotalLatency = time.Since(start)
	syncMetrics.CompletionLatency = syncMetrics.TotalLatency
	var input bytes.Buffer
	if _, err = providerCodec.WriteBatchJSONL(&input, runID, batchvalidation.DefaultGeneration, items); err != nil {
		return batchvalidation.Comparison{}, err
	}
	batchStart := time.Now()
	inputFile, err := batchClient.UploadFile(ctx, "mouseion-validation.jsonl", bytes.NewReader(input.Bytes()))
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	created, err := batchClient.CreateBatch(ctx, enrichment.CreateBatchRequest{InputFileID: inputFile.ID, Metadata: map[string]string{"mouseion_validation": fixture.Name, "mouseion_run": runID}})
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	var terminal enrichment.Batch
	defer func() {
		for _, fileID := range []string{inputFile.ID, terminal.OutputFileID, terminal.ErrorFileID} {
			if fileID != "" {
				_, _ = batchClient.DeleteFile(context.Background(), fileID)
			}
		}
	}()
	terminal, err = observeBatch(ctx, batchClient, created, pollInterval)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	if ctx.Err() != nil {
		return batchvalidation.Comparison{}, errors.New("operator cancelled observation; provider Batch cancellation was requested")
	}
	var output, providerErrors bytes.Buffer
	if terminal.OutputFileID != "" {
		if err = batchClient.FileContent(ctx, terminal.OutputFileID, &output); err != nil {
			return batchvalidation.Comparison{}, err
		}
	}
	if terminal.ErrorFileID != "" {
		if err = batchClient.FileContent(ctx, terminal.ErrorFileID, &providerErrors); err != nil {
			return batchvalidation.Comparison{}, err
		}
	}
	batchOutcomes, missing, decodeErr := providerCodec.DecodeBatchResultsPartial(runID, batchvalidation.DefaultGeneration, items, bytes.NewReader(output.Bytes()), bytes.NewReader(providerErrors.Bytes()))
	batchMetrics := batchvalidation.Metrics{CacheHits: syncMetrics.CacheHits, CacheMisses: syncMetrics.CacheMisses, ProviderCalls: 1, Completed: terminal.RequestCounts.Completed, Failed: terminal.RequestCounts.Failed, InputTokens: terminal.Usage.InputTokens, OutputTokens: terminal.Usage.OutputTokens, TotalLatency: time.Since(batchStart), CompletionLatency: durationBetween(created.CreatedAt, terminalTime(terminal)), QueueLatency: durationBetween(created.CreatedAt, terminal.InProgressAt), ExpiryCount: len(missing)}
	if terminal.Status == enrichment.BatchStatusExpired {
		batchMetrics.ExpiryCount += len(missing)
	}
	if decodeErr != nil {
		batchMetrics.ValidationFailures++
	}
	batchResponses := make(map[int]enrichment.TranslationResponse, len(batchOutcomes))
	for ordinal, outcome := range batchOutcomes {
		if outcome.Successful() {
			batchResponses[ordinal] = outcome.Response
		} else {
			batchMetrics.ProviderErrors++
		}
	}
	for ordinal, response := range syncResponses {
		if fixtureItem, _ := fixtureItemByOrdinal(fixture, ordinal); fixtureItem.CacheHit {
			batchResponses[ordinal] = response
		}
	}
	if decodeErr != nil && len(batchOutcomes) == 0 {
		batchMetrics.ParseFailures++
	}
	if len(missing) > 0 {
		batchMetrics.Failed += len(missing)
	}
	batchMetrics.EstimatedCostUSD = estimateCost(batchMetrics.InputTokens, batchMetrics.OutputTokens, inputCostPerMillion, outputCostPerMillion, batchDiscount)
	syncResult, err := batchvalidation.BuildResult(ctx, "synchronous", providerCodec, fixture, requests, syncResponses, syncMetrics)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	batchResult, err := batchvalidation.BuildResult(ctx, "batch", providerCodec, fixture, requests, batchResponses, batchMetrics)
	if err != nil {
		return batchvalidation.Comparison{}, err
	}
	return batchvalidation.Comparison{Synchronous: syncResult, Batch: batchResult}, nil
}

func estimateCost(inputTokens, outputTokens int64, inputCostPerMillion, outputCostPerMillion, multiplier float64) float64 {
	return multiplier * (float64(inputTokens)/1_000_000*inputCostPerMillion + float64(outputTokens)/1_000_000*outputCostPerMillion)
}

func observeBatch(ctx context.Context, client *enrichment.OpenAIBatchClient, batch enrichment.Batch, interval time.Duration) (enrichment.Batch, error) {
	for {
		if batch.Status == enrichment.BatchStatusCompleted || batch.Status == enrichment.BatchStatusFailed || batch.Status == enrichment.BatchStatusExpired || batch.Status == enrichment.BatchStatusCancelled {
			return batch, nil
		}
		select {
		case <-ctx.Done():
			_, cancelErr := client.CancelBatch(context.Background(), batch.ID)
			if cancelErr != nil {
				return batch, fmt.Errorf("cancel Batch after operator interrupt: %w", cancelErr)
			}
			return batch, ctx.Err()
		case <-time.After(interval):
		}
		var err error
		batch, err = client.GetBatch(ctx, batch.ID)
		if err != nil {
			return batch, err
		}
		fmt.Fprintf(os.Stderr, "Batch observation: status=%s completed=%d failed=%d\n", batch.Status, batch.RequestCounts.Completed, batch.RequestCounts.Failed)
	}
}

func terminalTime(batch enrichment.Batch) int64 {
	switch batch.Status {
	case enrichment.BatchStatusCompleted:
		return batch.CompletedAt
	case enrichment.BatchStatusFailed:
		return batch.FailedAt
	case enrichment.BatchStatusExpired:
		return batch.ExpiredAt
	case enrichment.BatchStatusCancelled:
		return batch.CancelledAt
	}
	return 0
}
func durationBetween(start, end int64) time.Duration {
	if start <= 0 || end <= 0 || end < start {
		return 0
	}
	return time.Duration(end-start) * time.Second
}
func itemIndex(items []enrichment.BatchTranslationItem, ordinal int) int {
	for i := range items {
		if items[i].Ordinal == ordinal {
			return i
		}
	}
	return 0
}
func fixtureItemByOrdinal(fixture batchvalidation.Fixture, ordinal int) (batchvalidation.FixtureItem, bool) {
	for _, item := range fixture.Items {
		if item.Ordinal == ordinal {
			return item, true
		}
	}
	return batchvalidation.FixtureItem{}, false
}

func writeReport(path string, report batchvalidation.Report) error {
	if path == "-" {
		return batchvalidation.WriteMarkdownReport(os.Stdout, report)
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return batchvalidation.WriteMarkdownReport(file, report)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "batch-validation: "+format+"\n", args...)
	os.Exit(1)
}
