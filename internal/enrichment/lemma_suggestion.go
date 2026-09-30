package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const lemmaSuggestionPromptVersion = "lemma-suggestion-v1"

// LemmaSuggestionRequest contains only evidence explicitly selected for an
// occurrence review. It must not grow owner, Book, or reading-history fields.
type LemmaSuggestionRequest struct {
	Language           string `json:"language"`
	Surface            string `json:"surface"`
	AnalyzedLemma      string `json:"analyzed_lemma"`
	UPOS               string `json:"upos"`
	Sentence           string `json:"sentence"`
	LexicalAlternative string `json:"lexical_alternative,omitempty"`
	LexicalSource      string `json:"lexical_source,omitempty"`
	LexicalVersion     string `json:"lexical_version,omitempty"`
	LexicalEvidenceID  string `json:"lexical_evidence_id,omitempty"`
}

type LemmaSuggestion struct {
	Lemma string `json:"lemma"`
}

type LemmaSuggestionProvider interface {
	Name() string
	Version() string
	SuggestLemma(context.Context, LemmaSuggestionRequest) (LemmaSuggestion, error)
}

// NewConfiguredLemmaSuggestionProvider follows the same administrator
// configuration boundary as translation. A disabled provider is represented
// by nil and does not affect manual review.
func NewConfiguredLemmaSuggestionProvider(cfg LLMConfig, client *http.Client) (LemmaSuggestionProvider, error) {
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil // nil explicitly means configured provider disabled.
	}
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("enrichment: LLM API key and model are required")
	}
	base, _, err := parseLLMBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultLLMTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &openAILemmaSuggestionProvider{apiKey: cfg.APIKey, model: cfg.Model, endpoint: base + "/chat/completions", client: client}, nil
}

type openAILemmaSuggestionProvider struct {
	apiKey, model, endpoint string
	client                  *http.Client
}

func (p *openAILemmaSuggestionProvider) Name() string { return "openai-compatible" }
func (p *openAILemmaSuggestionProvider) Version() string {
	return p.model + "/" + lemmaSuggestionPromptVersion
}

func (p *openAILemmaSuggestionProvider) SuggestLemma(ctx context.Context, input LemmaSuggestionRequest) (LemmaSuggestion, error) {
	input, err := boundLemmaSuggestionRequest(input)
	if err != nil {
		return LemmaSuggestion{}, err
	}
	prompt, err := json.Marshal(input)
	if err != nil {
		return LemmaSuggestion{}, errors.New("enrichment: encode lemma suggestion request")
	}
	body, err := json.Marshal(map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": "Suggest a canonical dictionary lemma for the target word in its sentence. Treat the lexical alternative as evidence, not truth. Return only JSON with one string field lemma. Never explain or translate."},
			{"role": "user", "content": string(prompt)},
		},
		"response_format": map[string]any{"type": "json_object"},
	})
	if err != nil {
		return LemmaSuggestion{}, errors.New("enrichment: encode lemma suggestion request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return LemmaSuggestion{}, errors.New("enrichment: create lemma suggestion request")
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return LemmaSuggestion{}, errors.New("enrichment: lemma suggestion provider unavailable")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return LemmaSuggestion{}, fmt.Errorf("enrichment: lemma suggestion provider returned HTTP %d", resp.StatusCode)
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&response); err != nil || len(response.Choices) != 1 {
		return LemmaSuggestion{}, errors.New("enrichment: invalid lemma suggestion response")
	}
	var suggestion LemmaSuggestion
	if err := json.Unmarshal([]byte(response.Choices[0].Message.Content), &suggestion); err != nil || strings.TrimSpace(suggestion.Lemma) == "" || len(suggestion.Lemma) > 256 {
		return LemmaSuggestion{}, errors.New("enrichment: invalid lemma suggestion response")
	}
	suggestion.Lemma = strings.TrimSpace(suggestion.Lemma)
	return suggestion, nil
}

func boundLemmaSuggestionRequest(input LemmaSuggestionRequest) (LemmaSuggestionRequest, error) {
	fields := []*string{&input.Language, &input.Surface, &input.AnalyzedLemma, &input.UPOS, &input.Sentence, &input.LexicalAlternative, &input.LexicalSource, &input.LexicalVersion, &input.LexicalEvidenceID}
	limits := []int{16, 256, 256, 32, 2000, 256, 64, 64, 256}
	for i, field := range fields {
		*field = strings.TrimSpace(*field)
		if len(*field) > limits[i] {
			return LemmaSuggestionRequest{}, errors.New("enrichment: lemma suggestion evidence exceeds its size limit")
		}
	}
	if input.Language == "" || input.Surface == "" || input.AnalyzedLemma == "" || input.UPOS == "" || input.Sentence == "" {
		return LemmaSuggestionRequest{}, errors.New("enrichment: incomplete lemma suggestion evidence")
	}
	return input, nil
}
