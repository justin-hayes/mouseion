package webapp

import (
	"context"
	"errors"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func bookStudyLanguage(detail domain.MyBook) string {
	language := strings.TrimSpace(detail.Book.LanguageTag)
	if language == "" && detail.Acquired != nil {
		language = strings.TrimSpace(detail.Acquired.Source.Language)
	}
	return language
}

// readingBookURLForSource returns a link only when the owner's current Book
// evidence can be served in Reading Journey. Source material IDs are accepted
// because Jobs and deck history retain acquisition identities.
func (h *Handler) readingBookURLForSource(ctx context.Context, owner, sourceID string) (string, error) {
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
	if detail.Disposition == domain.BookDispositionToRead {
		return readingBookOrLanguageHandoffURL(ctx, detail), nil
	}
	return "", nil
}

func readingBookOrLanguageHandoffURL(ctx context.Context, detail domain.MyBook) string {
	if shellViewFromContext(ctx) != nil {
		bookLanguage := canonicalization.NormalizeLanguage(bookStudyLanguage(detail))
		activeLanguage, _ := activeStudyLanguageForContext(ctx)
		if bookLanguage != "" && canonicalization.NormalizeLanguage(activeLanguage) != bookLanguage {
			return journeyLanguageHandoffURL(detail.Book.ID, bookLanguage)
		}
	}
	return readingBookURL(detail.Book.ID)
}

func (h *Handler) readingBookURLs(ctx context.Context, owner string, sourceIDs []string) (map[string]string, error) {
	urls := make(map[string]string, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		if _, seen := urls[sourceID]; seen {
			continue
		}
		url, err := h.readingBookURLForSource(ctx, owner, sourceID)
		if err != nil {
			return nil, err
		}
		urls[sourceID] = url
	}
	return urls, nil
}
