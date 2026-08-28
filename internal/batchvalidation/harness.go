// Package batchvalidation contains the operator-only comparison harness for
// prepared-deck translation. It deliberately has no application wiring: the
// deployed worker cannot select a transport through this package.
package batchvalidation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

const (
	FixtureSchemaVersion = 1
	DefaultRunID         = "018f64b6-5f2f-7e12-a7a7-832a50f68b7c"
	DefaultGeneration    = 1
)

// Fixture is a frozen manifest plus the fixed response set used by the local
// replay. Request bodies are regenerated from Manifest and Codec, then checked
// against RequestsFile; they are never independently selected by a transport.
type Fixture struct {
	SchemaVersion   int               `json:"schema_version"`
	Name            string            `json:"name"`
	Provider        string            `json:"provider"`
	ProviderVersion string            `json:"provider_version"`
	Model           string            `json:"model"`
	PromptVersion   string            `json:"prompt_version"`
	Owner           string            `json:"owner"`
	DeckName        string            `json:"deck_name"`
	Items           []FixtureItem     `json:"items"`
	Responses       []FixtureResponse `json:"responses"`
}

type FixtureItem struct {
	Ordinal     int                            `json:"ordinal"`
	Disposition cardexport.ManifestDisposition `json:"disposition"`
	Language    string                         `json:"language"`
	Lemma       string                         `json:"canonical_lemma"`
	UPOS        string                         `json:"upos"`
	Sentence    string                         `json:"sentence"`
	Target      string                         `json:"target_word"`
	Source      string                         `json:"source_document"`
	FirstSeen   int64                          `json:"first_encounter"`
	Quality     cardexport.SentenceQuality     `json:"quality"`
	CacheKey    enrichment.CacheKey            `json:"cache_key"`
	CacheHit    bool                           `json:"cache_hit"`
}

type FixtureResponse struct {
	Ordinal                   int    `json:"ordinal"`
	Translation               string `json:"translation"`
	Gloss                     string `json:"gloss"`
	SentenceTranslation       string `json:"sentence_translation"`
	SentenceTranslationTarget string `json:"sentence_translation_target"`
}

// RequestFixture is the canonical request body and its stable Batch identity.
// Body is intentionally raw JSON so byte equality can be asserted.
type RequestFixture struct {
	Ordinal  int             `json:"-"`
	CustomID string          `json:"custom_id"`
	Method   string          `json:"method"`
	URL      string          `json:"url"`
	Body     json.RawMessage `json:"body"`
}

type FixtureFiles struct {
	Manifest  string
	Requests  string
	Responses string
}

func LoadFixture(files FixtureFiles) (Fixture, []byte, error) {
	manifestBytes, err := os.ReadFile(files.Manifest)
	if err != nil {
		return Fixture{}, nil, fmt.Errorf("read frozen manifest: %w", err)
	}
	var fixture Fixture
	if err = json.Unmarshal(manifestBytes, &fixture); err != nil {
		return Fixture{}, nil, fmt.Errorf("decode frozen manifest: %w", err)
	}
	if err = fixture.Validate(); err != nil {
		return Fixture{}, nil, err
	}
	responseBytes, err := os.ReadFile(files.Responses)
	if err != nil {
		return Fixture{}, nil, fmt.Errorf("read fixed responses: %w", err)
	}
	if err = json.Unmarshal(responseBytes, &fixture.Responses); err != nil {
		return Fixture{}, nil, fmt.Errorf("decode fixed responses: %w", err)
	}
	if err = fixture.ValidateResponses(); err != nil {
		return Fixture{}, nil, err
	}
	requestBytes, err := os.ReadFile(files.Requests)
	if err != nil {
		return Fixture{}, nil, fmt.Errorf("read frozen requests: %w", err)
	}
	return fixture, requestBytes, nil
}

func (f Fixture) Validate() error {
	if f.SchemaVersion != FixtureSchemaVersion || strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.Provider) == "" || strings.TrimSpace(f.ProviderVersion) == "" || strings.TrimSpace(f.Model) == "" || strings.TrimSpace(f.PromptVersion) == "" || strings.TrimSpace(f.Owner) == "" || strings.TrimSpace(f.DeckName) == "" || len(f.Items) == 0 {
		return errors.New("batchvalidation: invalid frozen fixture header")
	}
	for i, item := range f.Items {
		if item.Ordinal != i || strings.TrimSpace(item.Language) == "" || strings.TrimSpace(item.Lemma) == "" || item.UPOS == "" || item.UPOS != strings.ToUpper(item.UPOS) {
			return fmt.Errorf("batchvalidation: invalid frozen item %d", i)
		}
		if item.Disposition == cardexport.ManifestAccepted && strings.TrimSpace(item.CacheKey.Provider) == "" {
			return fmt.Errorf("batchvalidation: accepted item %d has no cache identity", i)
		}
		if item.Disposition != cardexport.ManifestAccepted && item.Disposition != cardexport.ManifestQualityOmitted {
			return fmt.Errorf("batchvalidation: invalid disposition at item %d", i)
		}
	}
	return nil
}

func (f Fixture) ValidateResponses() error {
	accepted := make(map[int]struct{}, len(f.Items))
	for _, item := range f.Items {
		if item.Disposition == cardexport.ManifestAccepted {
			accepted[item.Ordinal] = struct{}{}
		}
	}
	seen := make(map[int]bool, len(f.Responses))
	for _, response := range f.Responses {
		if response.Ordinal < 0 || seen[response.Ordinal] {
			return fmt.Errorf("batchvalidation: duplicate response ordinal %d", response.Ordinal)
		}
		if _, ok := accepted[response.Ordinal]; !ok {
			return fmt.Errorf("batchvalidation: response ordinal %d is not an accepted manifest item", response.Ordinal)
		}
		seen[response.Ordinal] = true
	}
	if len(f.Responses) != len(accepted) {
		return fmt.Errorf("batchvalidation: fixed response count %d does not match accepted item count %d", len(f.Responses), len(accepted))
	}
	return nil
}

func (f Fixture) Snapshot() cardexport.ManifestSnapshot {
	items := make([]cardexport.ManifestItem, len(f.Items))
	for i, item := range f.Items {
		entry := cardexport.Entry{Language: item.Language, CanonicalLemma: item.Lemma, UPOS: item.UPOS, Sentence: item.Sentence, TargetWord: item.Target, SourceDocument: item.Source, FirstEncounter: item.FirstSeen}
		var key *enrichment.CacheKey
		if item.Disposition == cardexport.ManifestAccepted {
			copy := item.CacheKey
			key = &copy
		}
		items[i] = cardexport.ManifestItem{Ordinal: item.Ordinal, Disposition: item.Disposition, Entry: entry, Quality: item.Quality, CacheKey: key}
	}
	return cardexport.ManifestSnapshot{SchemaVersion: cardexport.ManifestSchemaVersion, Owner: f.Owner, DeckName: f.DeckName, Filename: cardexport.DownloadFilename(f.DeckName), Items: items}
}

func (f Fixture) Manifest() (cardexport.Manifest, error) {
	return cardexport.ManifestFromSnapshot(f.Snapshot())
}

func (f Fixture) ManifestDigest() (string, error) { return f.Snapshot().Digest() }

func (f Fixture) Response(ordinal int) (FixtureResponse, bool) {
	for _, response := range f.Responses {
		if response.Ordinal == ordinal {
			return response, true
		}
	}
	return FixtureResponse{}, false
}

func FrozenRequests(codec *enrichment.TranslationCodec, f Fixture, runID string, generation int) ([]RequestFixture, []enrichment.BatchTranslationItem, error) {
	if err := f.Validate(); err != nil {
		return nil, nil, err
	}
	if codec == nil || codec.Model() != f.Model || codec.PromptVersion() != f.PromptVersion {
		return nil, nil, errors.New("batchvalidation: codec does not match frozen model or prompt version")
	}
	snapshot := f.Snapshot()
	items := make([]enrichment.BatchTranslationItem, 0, len(f.Items))
	for _, item := range snapshot.Items {
		if item.Disposition != cardexport.ManifestAccepted {
			continue
		}
		if fItem, _ := f.item(item.Ordinal); fItem.CacheHit {
			continue
		}
		items = append(items, enrichment.BatchTranslationItem{Ordinal: item.Ordinal, Request: enrichment.TranslationRequest{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence}})
	}
	requests := make([]RequestFixture, 0, len(items))
	for _, item := range items {
		body, err := codec.EncodeRequest(item.Request)
		if err != nil {
			return nil, nil, err
		}
		customID, err := enrichment.BatchCustomID(runID, item.Ordinal, generation)
		if err != nil {
			return nil, nil, err
		}
		requests = append(requests, RequestFixture{Ordinal: item.Ordinal, CustomID: customID, Method: "POST", URL: enrichment.OpenAIChatCompletionsEndpoint, Body: body})
	}
	return requests, items, nil
}

func (f Fixture) item(ordinal int) (FixtureItem, bool) {
	if ordinal >= 0 && ordinal < len(f.Items) && f.Items[ordinal].Ordinal == ordinal {
		return f.Items[ordinal], true
	}
	return FixtureItem{}, false
}

type Metrics struct {
	QueueLatency          time.Duration `json:"queue_latency"`
	CompletionLatency     time.Duration `json:"completion_latency"`
	TotalLatency          time.Duration `json:"total_latency"`
	CacheHits             int           `json:"cache_hits"`
	CacheMisses           int           `json:"cache_misses"`
	ProviderCalls         int           `json:"provider_calls"`
	Retries               int           `json:"retries"`
	ProviderErrors        int           `json:"provider_errors"`
	ExpiryCount           int           `json:"expiry_count"`
	ParseFailures         int           `json:"parse_failures"`
	ValidationFailures    int           `json:"validation_failures"`
	Completed             int           `json:"completed"`
	Failed                int           `json:"failed"`
	Omissions             int           `json:"omissions"`
	DuplicateOutcomes     int           `json:"duplicate_outcomes"`
	MiscorrelatedOutcomes int           `json:"miscorrelated_outcomes"`
	InputTokens           int64         `json:"input_tokens"`
	OutputTokens          int64         `json:"output_tokens"`
	EstimatedCostUSD      float64       `json:"estimated_cost_usd"`
}

type ArtifactDigests struct {
	APKG string `json:"apkg_sha256"`
	TSV  string `json:"tsv_sha256"`
}

type Quality struct {
	Translations         int    `json:"translations"`
	SentenceTranslations int    `json:"sentence_translations"`
	TargetAligned        int    `json:"target_aligned"`
	ReviewStatus         string `json:"review_status"`
}

type Result struct {
	Transport      string                                 `json:"transport"`
	Requests       []RequestFixture                       `json:"requests"`
	Metrics        Metrics                                `json:"metrics"`
	Quality        Quality                                `json:"quality"`
	Completeness   cardexport.Completeness                `json:"completeness"`
	Artifact       ArtifactDigests                        `json:"artifact"`
	ArtifactStable bool                                   `json:"artifact_byte_stable"`
	Responses      map[int]enrichment.TranslationResponse `json:"-"`
}

// ReplayFixedResponses runs the same fixed response set through both the
// synchronous decoder and Batch decoder. It is deterministic and never makes
// a network call.
func ReplayFixedResponses(ctx context.Context, codec *enrichment.TranslationCodec, f Fixture, requestFixture []byte, runID string, generation int) (Result, Result, error) {
	if codec == nil {
		return Result{}, Result{}, errors.New("batchvalidation: nil codec")
	}
	if err := f.ValidateResponses(); err != nil {
		return Result{}, Result{}, err
	}
	manifest, err := f.Manifest()
	if err != nil {
		return Result{}, Result{}, err
	}
	requests, items, err := FrozenRequests(codec, f, runID, generation)
	if err != nil {
		return Result{}, Result{}, err
	}
	var regenerated bytes.Buffer
	if _, err = codec.WriteBatchJSONL(&regenerated, runID, generation, items); err != nil {
		return Result{}, Result{}, err
	}
	if !bytes.Equal(canonicalRequestFile(requests), regenerated.Bytes()) {
		return Result{}, Result{}, errors.New("batchvalidation: request metadata does not match generated Batch JSONL")
	}
	if len(requestFixture) > 0 && !bytes.Equal(regenerated.Bytes(), requestFixture) {
		return Result{}, Result{}, fmt.Errorf("batchvalidation: regenerated requests differ from frozen request fixture (got %s)", regenerated.Bytes())
	}
	syncOutcomes, syncMetrics, err := decodeFixedSync(ctx, codec, f, manifest, items)
	if err != nil {
		return Result{}, Result{}, err
	}
	batchOutput, err := fixedBatchOutput(codec, f, items, runID, generation)
	if err != nil {
		return Result{}, Result{}, err
	}
	batchOutcomes, missing, err := codec.DecodeBatchResultsPartial(runID, generation, items, bytes.NewReader(batchOutput), nil)
	if err != nil || len(missing) != 0 {
		return Result{}, Result{}, fmt.Errorf("decode fixed Batch output: %w (missing=%v)", err, missing)
	}
	if !sameResponses(syncOutcomes, batchOutcomes, f) {
		return Result{}, Result{}, errors.New("batchvalidation: sync and Batch fixed responses differ")
	}
	batchResponses := make(map[int]enrichment.TranslationResponse, len(batchOutcomes))
	for ordinal, outcome := range batchOutcomes {
		batchResponses[ordinal] = outcome.Response
	}
	for ordinal, response := range syncOutcomes {
		item, _ := f.item(ordinal)
		if item.CacheHit {
			batchResponses[ordinal] = response
		}
	}
	syncResult, err := buildResult(ctx, "synchronous", codec, f, manifest, requests, syncOutcomes, syncMetrics)
	if err != nil {
		return Result{}, Result{}, err
	}
	batchResult, err := buildResult(ctx, "batch", codec, f, manifest, requests, batchResponses, Metrics{CacheHits: syncMetrics.CacheHits, CacheMisses: syncMetrics.CacheMisses, ProviderCalls: 1, Completed: len(items)})
	if err != nil {
		return Result{}, Result{}, err
	}
	syncResult.Quality.ReviewStatus = "not_reviewed_synthetic"
	batchResult.Quality.ReviewStatus = "not_reviewed_synthetic"
	return syncResult, batchResult, nil
}

func decodeFixedSync(ctx context.Context, codec *enrichment.TranslationCodec, f Fixture, manifest cardexport.Manifest, items []enrichment.BatchTranslationItem) (map[int]enrichment.TranslationResponse, Metrics, error) {
	_ = ctx
	responses := make(map[int]enrichment.TranslationResponse, len(f.Items))
	metrics := Metrics{}
	for _, item := range manifest.Snapshot().Items {
		if item.Disposition != cardexport.ManifestAccepted {
			metrics.Omissions++
			continue
		}
		fixtureItem, _ := f.item(item.Ordinal)
		if fixtureItem.CacheHit {
			metrics.CacheHits++
		} else {
			metrics.CacheMisses++
			metrics.ProviderCalls++
		}
		response, ok := f.Response(item.Ordinal)
		if !ok {
			return nil, Metrics{}, fmt.Errorf("missing fixed response for ordinal %d", item.Ordinal)
		}
		body, err := responseBody(response)
		if err != nil {
			return nil, Metrics{}, err
		}
		decoded, err := codec.DecodeResponse(enrichment.TranslationRequest{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence}, body)
		if err != nil {
			metrics.ValidationFailures++
			return nil, Metrics{}, err
		}
		responses[item.Ordinal] = decoded
	}
	metrics.Completed = len(responses)
	_ = items
	return responses, metrics, nil
}

func fixedBatchOutput(codec *enrichment.TranslationCodec, f Fixture, items []enrichment.BatchTranslationItem, runID string, generation int) ([]byte, error) {
	lines := make([][]byte, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- { // exercise unordered provider output.
		item := items[i]
		response, ok := f.Response(item.Ordinal)
		if !ok {
			return nil, fmt.Errorf("missing fixed response for ordinal %d", item.Ordinal)
		}
		body, err := responseBody(response)
		if err != nil {
			return nil, err
		}
		customID, err := enrichment.BatchCustomID(runID, item.Ordinal, generation)
		if err != nil {
			return nil, err
		}
		line, err := json.Marshal(struct {
			CustomID string `json:"custom_id"`
			Response struct {
				StatusCode int             `json:"status_code"`
				Body       json.RawMessage `json:"body"`
			} `json:"response"`
		}{CustomID: customID, Response: struct {
			StatusCode int             `json:"status_code"`
			Body       json.RawMessage `json:"body"`
		}{200, body}})
		if err != nil {
			return nil, err
		}
		lines = append(lines, []byte(line))
	}
	return bytes.Join(lines, []byte{'\n'}), nil
}

func responseBody(response FixtureResponse) ([]byte, error) {
	payload := struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}{Choices: []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{Content: mustJSON(response)}}}}
	return json.Marshal(payload)
}

func mustJSON(response FixtureResponse) string {
	payload, _ := json.Marshal(enrichment.TranslationResponse{Translation: response.Translation, Gloss: response.Gloss, SentenceTranslation: response.SentenceTranslation, SentenceTranslationTarget: response.SentenceTranslationTarget})
	return string(payload)
}

func sameResponses(a map[int]enrichment.TranslationResponse, b map[int]enrichment.BatchTranslationOutcome, fixture Fixture) bool {
	for ordinal, response := range a {
		item, _ := fixture.item(ordinal)
		if item.CacheHit {
			continue
		}
		if !b[ordinal].Successful() || b[ordinal].Response != response {
			return false
		}
	}
	for ordinal := range b {
		item, _ := fixture.item(ordinal)
		if item.CacheHit {
			return false
		}
	}
	return true
}

func buildResult(ctx context.Context, transport string, codec *enrichment.TranslationCodec, f Fixture, manifest cardexport.Manifest, requests []RequestFixture, responses map[int]enrichment.TranslationResponse, metrics Metrics) (Result, error) {
	_ = codec
	exact := make([]cardexport.ExactEnrichment, 0, len(manifest.EnrichmentCandidates()))
	keys := manifest.Snapshot().Items
	for _, item := range keys {
		if item.Disposition != cardexport.ManifestAccepted || item.CacheKey == nil {
			continue
		}
		response := responses[item.Ordinal]
		candidate := enrichment.Candidate{Identity: enrichment.Identity{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS}, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence}
		provenance := enrichment.Provenance{Provider: item.CacheKey.Provider, ProviderVersion: item.CacheKey.ProviderVersion, External: true}
		exact = append(exact, cardexport.ExactEnrichment{CacheKey: *item.CacheKey, Result: enrichment.Result{Candidate: candidate, Translation: available(response.Translation, provenance), Gloss: available(response.Gloss, provenance), SentenceTranslation: available(response.SentenceTranslation, provenance), SentenceTranslationTarget: available(response.SentenceTranslationTarget, provenance)}})
	}
	artifact, err := (&cardexport.Service{}).RenderManifest(ctx, manifest, exact)
	if err != nil {
		return Result{}, err
	}
	again, err := (&cardexport.Service{}).RenderManifest(ctx, manifest, exact)
	if err != nil {
		return Result{}, err
	}
	metrics.Omissions = artifact.Completeness.QualityOmitted
	if metrics.InputTokens == 0 && metrics.OutputTokens == 0 {
		for _, request := range requests {
			metrics.InputTokens += int64((len(request.Body) + 3) / 4)
			if response, ok := responses[request.Ordinal]; ok {
				encoded := mustJSON(FixtureResponse{Translation: response.Translation, Gloss: response.Gloss, SentenceTranslation: response.SentenceTranslation, SentenceTranslationTarget: response.SentenceTranslationTarget})
				metrics.OutputTokens += int64((len(encoded) + 3) / 4)
			}
		}
	}
	quality := Quality{ReviewStatus: "pending_operator_review"}
	for _, response := range responses {
		if strings.TrimSpace(response.Translation) != "" {
			quality.Translations++
		}
		if strings.TrimSpace(response.SentenceTranslation) != "" {
			quality.SentenceTranslations++
		}
	}
	for _, item := range f.Items {
		if item.Disposition == cardexport.ManifestAccepted {
			if response, ok := responses[item.Ordinal]; ok && strings.Contains(cardexport.HighlightEnglishTarget(response.SentenceTranslation, response.SentenceTranslationTarget), "<b>") {
				quality.TargetAligned++
			}
		}
	}
	return Result{Transport: transport, Requests: requests, Metrics: metrics, Quality: quality, Completeness: artifact.Completeness, Artifact: ArtifactDigests{APKG: digest(artifact.APKG), TSV: digest([]byte(artifact.TSV))}, ArtifactStable: bytes.Equal(artifact.APKG, again.APKG) && artifact.TSV == again.TSV}, nil
}

// BuildResult renders a provider run against the frozen manifest. It is used
// by the explicit operator command after real responses have been decoded.
// The caller owns the measurement values; this function only applies the
// frozen render contract and derives artifact/quality facts.
func BuildResult(ctx context.Context, transport string, codec *enrichment.TranslationCodec, f Fixture, requests []RequestFixture, responses map[int]enrichment.TranslationResponse, metrics Metrics) (Result, error) {
	manifest, err := f.Manifest()
	if err != nil {
		return Result{}, err
	}
	return buildResult(ctx, transport, codec, f, manifest, requests, responses, metrics)
}

func available(value string, provenance enrichment.Provenance) enrichment.Field[string] {
	return enrichment.Field[string]{Value: value, Available: strings.TrimSpace(value) != "", Provenance: provenance}
}

func digest(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

func canonicalRequestFile(requests []RequestFixture) []byte {
	var buffer bytes.Buffer
	for _, request := range requests {
		line, _ := json.Marshal(request)
		buffer.Write(line)
		buffer.WriteByte('\n')
	}
	return buffer.Bytes()
}

func WriteRequestFixture(w io.Writer, requests []RequestFixture) error {
	_, err := w.Write(canonicalRequestFile(requests))
	return err
}

// Report is intentionally split by evidence class. Synthetic results cannot
// satisfy the real-provider cutover gates.
type Report struct {
	FixtureName     string      `json:"fixture_name"`
	ManifestDigest  string      `json:"manifest_digest"`
	Provider        string      `json:"provider"`
	ProviderVersion string      `json:"provider_version"`
	Model           string      `json:"model"`
	Prompt          string      `json:"prompt_version"`
	Synthetic       Comparison  `json:"synthetic"`
	Real            *Comparison `json:"real_provider,omitempty"`
}

type Comparison struct {
	Synchronous Result `json:"synchronous"`
	Batch       Result `json:"batch"`
}

func WriteMarkdownReport(w io.Writer, report Report) error {
	if w == nil {
		return errors.New("batchvalidation: nil report writer")
	}
	_, err := fmt.Fprintf(w, "# OpenAI Batch prepared-deck validation\n\nFixture: `%s`  \nManifest digest: `%s`  \nProvider: `%s`  \nProvider version: `%s`  \nModel: `%s`  \nPrompt version: `%s`\n\n", report.FixtureName, report.ManifestDigest, report.Provider, report.ProviderVersion, report.Model, report.Prompt)
	if err != nil {
		return err
	}
	fmt.Fprint(w, "## Evidence classification\n\nSynthetic replay is deterministic harness evidence only; it is not a provider, production, quality, or cost claim. The real-provider section is measured evidence and remains `not run` until an operator runs the explicit command. Hypotheses and decisions belong in the review fields below.\n\n")
	writeComparison := func(title string, comparison Comparison) error {
		if _, err := fmt.Fprintf(w, "## %s\n\n| Measure | Synchronous | Batch |\n|---|---:|---:|\n| queue latency | %s | %s |\n| completion latency | %s | %s |\n| total latency | %s | %s |\n| cache-hit ratio | %.2f%% | %.2f%% |\n| input/output tokens | %d / %d | %d / %d |\n| estimated cost (USD) | %.6f | %.6f |\n| provider calls / retries / errors | %d / %d / %d | %d / %d / %d |\n| expiry / parse failures / validation failures | %d / %d / %d | %d / %d / %d |\n| omissions | %d | %d |\n| duplicate / miscorrelated outcomes | %d / %d | %d / %d |\n| target-aligned / translation review | %d / %s | %d / %s |\n| artifact byte stable | %t | %t |\n\n", title, comparison.Synchronous.Metrics.QueueLatency, comparison.Batch.Metrics.QueueLatency, comparison.Synchronous.Metrics.CompletionLatency, comparison.Batch.Metrics.CompletionLatency, comparison.Synchronous.Metrics.TotalLatency, comparison.Batch.Metrics.TotalLatency, ratio(comparison.Synchronous.Metrics), ratio(comparison.Batch.Metrics), comparison.Synchronous.Metrics.InputTokens, comparison.Synchronous.Metrics.OutputTokens, comparison.Batch.Metrics.InputTokens, comparison.Batch.Metrics.OutputTokens, comparison.Synchronous.Metrics.EstimatedCostUSD, comparison.Batch.Metrics.EstimatedCostUSD, comparison.Synchronous.Metrics.ProviderCalls, comparison.Synchronous.Metrics.Retries, comparison.Synchronous.Metrics.ProviderErrors, comparison.Batch.Metrics.ProviderCalls, comparison.Batch.Metrics.Retries, comparison.Batch.Metrics.ProviderErrors, comparison.Synchronous.Metrics.ExpiryCount, comparison.Synchronous.Metrics.ParseFailures, comparison.Synchronous.Metrics.ValidationFailures, comparison.Batch.Metrics.ExpiryCount, comparison.Batch.Metrics.ParseFailures, comparison.Batch.Metrics.ValidationFailures, comparison.Synchronous.Metrics.Omissions, comparison.Batch.Metrics.Omissions, comparison.Synchronous.Metrics.DuplicateOutcomes, comparison.Synchronous.Metrics.MiscorrelatedOutcomes, comparison.Batch.Metrics.DuplicateOutcomes, comparison.Batch.Metrics.MiscorrelatedOutcomes, comparison.Synchronous.Quality.TargetAligned, comparison.Synchronous.Quality.ReviewStatus, comparison.Batch.Quality.TargetAligned, comparison.Batch.Quality.ReviewStatus, comparison.Synchronous.ArtifactStable, comparison.Batch.ArtifactStable); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "Artifact digests: APKG `%s` / `%s`; TSV `%s` / `%s`.\n\n", comparison.Synchronous.Artifact.APKG, comparison.Batch.Artifact.APKG, comparison.Synchronous.Artifact.TSV, comparison.Batch.Artifact.TSV); err != nil {
			return err
		}
		return nil
	}
	if err := writeComparison("Synthetic replay (not cutover evidence)", report.Synthetic); err != nil {
		return err
	}
	if report.Real == nil {
		_, err = io.WriteString(w, "## Real-provider measurement\n\n`not run` — requires the explicit operator command and paid-provider acknowledgement.\n")
	} else {
		err = writeComparison("Real-provider measurement", *report.Real)
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, `
## Cutover gates — operator decision record

Record measured evidence, not hypotheses, for each gate. A gate is **pass** only when the real-provider comparison is complete on the agreed frozen manifests and the evidence is attached.

| Gate | Pass condition | Evidence / result | Decision |
|---|---|---|---|
| quality | translation-quality review and target alignment are no worse than synchronous, with explicit reviewer/sign-off |  |  |
| correctness | request parity, ID correlation, duplicate/miscorrelation, parse, and validation failures meet zero-tolerance policy |  |  |
| privacy | no owner/source metadata or raw prompts/responses in durable/log artifacts |  |  |
| durability | partial failure, expiry, cancellation, restart, and ambiguous submission recover as documented |  |  |
| cost | measured cost meets the approved budget after retries and cache effects |  |  |
| latency | queue, completion, and total latency are acceptable for the offline workflow |  |  |
| operational recovery | observation, cancellation, and fail-closed manual recovery are rehearsed |  |  |

### Review questions

- ADR-0031 accepted: yes / no; endpoint retirement for custom OpenAI-compatible providers: yes / no.
- Operational defaults approved: two Batch generations, 5,000-request ceiling, 30-second polling, and seven-day provider-file expiry: yes / no.
- Model, prompt, provider/API version, fixture digest, operator, date, and report artifact location: record here.
- Hypotheses needing follow-up (never counted as evidence):

### Rollback

Rollback for the eventual cutover is a code revert before synchronous support is removed. This harness introduces no deployed transport selector or feature flag.
`)
	return err
}

func ratio(metrics Metrics) float64 {
	total := metrics.CacheHits + metrics.CacheMisses
	if total == 0 {
		return 0
	}
	return 100 * float64(metrics.CacheHits) / float64(total)
}

func ReadDefaultFiles(dir string) FixtureFiles {
	return FixtureFiles{Manifest: filepath.Join(dir, "manifest.json"), Requests: filepath.Join(dir, "requests.jsonl"), Responses: filepath.Join(dir, "responses.json")}
}

// SortResponseOrdinals is useful to make operator-provided response fixtures
// stable before committing them.
func SortResponseOrdinals(responses []FixtureResponse) {
	sort.Slice(responses, func(i, j int) bool { return responses[i].Ordinal < responses[j].Ordinal })
}
