package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// GetLSJSenseSummary reads one cached provider-written LSJ sense summary by its
// (pin, entry ID, prompt version) identity.
func (s *PostgresStore) GetLSJSenseSummary(ctx context.Context, key enrichment.LSJSenseSummaryKey) (enrichment.LSJSenseSummary, bool, error) {
	payload, err := s.queries().GetLSJSenseSummary(ctx, sqlcgen.GetLSJSenseSummaryParams{LsjPin: key.Pin, LsjEntryID: key.EntryID, PromptVersion: key.PromptVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return enrichment.LSJSenseSummary{}, false, nil
	}
	if err != nil {
		return enrichment.LSJSenseSummary{}, false, fmt.Errorf("get LSJ sense summary: %w", err)
	}
	summary, err := decodeLSJSenseSummaryLines(key, payload)
	if err != nil {
		return enrichment.LSJSenseSummary{}, false, err
	}
	return summary, true, nil
}

// PutLSJSenseSummary stores one summary under its per-entry identity, keeping the
// first stored value: a summary is immutable for a given (pin, entry ID, prompt
// version), so a concurrent writer never overwrites an existing summary.
func (s *PostgresStore) PutLSJSenseSummary(ctx context.Context, key enrichment.LSJSenseSummaryKey, lines []enrichment.LSJSenseSummaryLine) error {
	payload, err := json.Marshal(lines)
	if err != nil {
		return fmt.Errorf("encode LSJ sense summary: %w", err)
	}
	if err := s.queries().UpsertLSJSenseSummary(ctx, sqlcgen.UpsertLSJSenseSummaryParams{LsjPin: key.Pin, LsjEntryID: key.EntryID, PromptVersion: key.PromptVersion, SummaryLines: payload, CreatedAt: time.Now().UTC()}); err != nil {
		return fmt.Errorf("put LSJ sense summary: %w", err)
	}
	return nil
}

// LoadLSJSenseSummaries reads every cached summary for the given identities in
// one query, keyed by identity. An identity with no stored row is simply absent.
func (s *PostgresStore) LoadLSJSenseSummaries(ctx context.Context, keys []enrichment.LSJSenseSummaryKey) (map[enrichment.LSJSenseSummaryKey]enrichment.LSJSenseSummary, error) {
	result := make(map[enrichment.LSJSenseSummaryKey]enrichment.LSJSenseSummary)
	if len(keys) == 0 {
		return result, nil
	}
	rows, err := s.queries().ListLSJSenseSummariesByPins(ctx, distinctLSJSummaryPins(keys))
	if err != nil {
		return nil, fmt.Errorf("list LSJ sense summaries: %w", err)
	}
	wanted := make(map[enrichment.LSJSenseSummaryKey]struct{}, len(keys))
	for _, key := range keys {
		wanted[key] = struct{}{}
	}
	for _, row := range rows {
		key := enrichment.LSJSenseSummaryKey{Pin: row.LsjPin, EntryID: row.LsjEntryID, PromptVersion: row.PromptVersion}
		if _, ok := wanted[key]; !ok {
			continue
		}
		summary, err := decodeLSJSenseSummaryLines(key, row.SummaryLines)
		if err != nil {
			return nil, err
		}
		result[key] = summary
	}
	return result, nil
}

// decodeLSJSenseSummaryLines rebuilds one summary from its stored line payload,
// naming the identity so a corrupt row is diagnosable without exposing Greek
// text.
func decodeLSJSenseSummaryLines(key enrichment.LSJSenseSummaryKey, payload []byte) (enrichment.LSJSenseSummary, error) {
	var lines []enrichment.LSJSenseSummaryLine
	if err := json.Unmarshal(payload, &lines); err != nil {
		return enrichment.LSJSenseSummary{}, fmt.Errorf("decode LSJ sense summary %s/%s: %w", key.Pin, key.EntryID, err)
	}
	return enrichment.LSJSenseSummary{EntryID: key.EntryID, Pin: key.Pin, Lines: lines}, nil
}

// distinctLSJSummaryPins lists each pin once, so one query covers every identity
// the caller wants.
func distinctLSJSummaryPins(keys []enrichment.LSJSenseSummaryKey) []string {
	seen := make(map[string]struct{}, len(keys))
	pins := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[key.Pin]; ok {
			continue
		}
		seen[key.Pin] = struct{}{}
		pins = append(pins, key.Pin)
	}
	return pins
}
