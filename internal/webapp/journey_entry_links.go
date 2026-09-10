package webapp

import (
	"context"
	"errors"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// journeyEntryURLForSource returns a link only when the owner's current Book
// evidence can be served by the Journey entry route. Source material IDs are
// accepted because Jobs and deck history retain acquisition identities.
func (h *Handler) journeyEntryURLForSource(ctx context.Context, owner, sourceID string) (string, error) {
	if sourceID == "" {
		return "", nil
	}
	detail, err := h.services.Store.GetBookDetail(ctx, owner, sourceID)
	if errors.Is(err, persistence.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if detail.Acquired == nil || detail.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*detail.Acquired) {
		return "", nil
	}
	language := detail.Book.LanguageTag
	if language == "" {
		language = detail.Acquired.Source.Language
	}
	journey, err := h.services.Store.GetReadingJourney(ctx, owner, language)
	if err != nil {
		return "", err
	}
	for _, entry := range journey.Entries {
		if entry.BookID == detail.Book.ID {
			return journeyEntryURL(detail.Book.ID), nil
		}
	}
	return "", nil
}

func (h *Handler) journeyEntryURLs(ctx context.Context, owner string, sourceIDs []string) (map[string]string, error) {
	urls := make(map[string]string, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		if _, seen := urls[sourceID]; seen {
			continue
		}
		url, err := h.journeyEntryURLForSource(ctx, owner, sourceID)
		if err != nil {
			return nil, err
		}
		urls[sourceID] = url
	}
	return urls, nil
}
