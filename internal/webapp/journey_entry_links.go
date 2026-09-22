package webapp

import (
	"context"
	"errors"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func journeyBookLanguage(detail domain.MyBook) string {
	language := strings.TrimSpace(detail.Book.LanguageTag)
	if language == "" && detail.Acquired != nil {
		language = strings.TrimSpace(detail.Acquired.Source.Language)
	}
	return language
}

// journeyEntryURLForSource returns a link only when the owner's current Book
// evidence can be served in Reading Journey. Source material IDs are accepted
// because Jobs and deck history retain acquisition identities.
func (h *Handler) journeyEntryURLForSource(ctx context.Context, owner, sourceID string) (string, error) {
	if sourceID == "" {
		return "", nil
	}
	detail, err := h.services.Store.Books.GetBookDetail(ctx, owner, sourceID)
	if errors.Is(err, persistence.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if detail.Acquired == nil || detail.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*detail.Acquired) {
		return "", nil
	}
	journey, err := h.services.Store.Journey.GetReadingJourney(ctx, owner, journeyBookLanguage(detail))
	if err != nil {
		return "", err
	}
	for _, entry := range journey.Entries {
		if entry.BookID == detail.Book.ID {
			return journeyCompatibilityURL(ctx, detail), nil
		}
	}
	return "", nil
}

func journeyCompatibilityURL(ctx context.Context, detail domain.MyBook) string {
	if shellViewFromContext(ctx) != nil {
		bookLanguage := canonicalization.NormalizeLanguage(journeyBookLanguage(detail))
		activeLanguage, _ := activeStudyLanguageForContext(ctx)
		if bookLanguage != "" && canonicalization.NormalizeLanguage(activeLanguage) != bookLanguage {
			return journeyLanguageHandoffURL(detail.Book.ID, bookLanguage)
		}
	}
	return journeyEntryURL(detail.Book.ID)
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
