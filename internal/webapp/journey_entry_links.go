package webapp

import (
	"context"
	"errors"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type jobBookContext struct {
	Title, ReadingURL string
}

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
	book, err := h.jobBookContextForSource(ctx, owner, sourceID)
	return book.ReadingURL, err
}

func (h *Handler) jobBookContextForSource(ctx context.Context, owner, sourceID string) (jobBookContext, error) {
	if sourceID == "" {
		return jobBookContext{}, nil
	}
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, sourceID)
	if errors.Is(err, persistence.ErrNotFound) {
		return jobBookContext{}, nil
	}
	if err != nil {
		return jobBookContext{}, err
	}
	book := jobBookContext{Title: strings.TrimSpace(detail.Book.Title)}
	if book.Title == "" && detail.Acquired != nil {
		book.Title = canonicalBookTitle(*detail.Acquired)
	}
	if detail.Acquired == nil || detail.Acquired.EvidenceState() != domain.BookAnalyzed || !bookHasCompletedAnalysis(*detail.Acquired) {
		return book, nil
	}
	if detail.Disposition == domain.BookDispositionToRead {
		book.ReadingURL = readingBookURLInActiveLanguage(ctx, detail)
	}
	return book, nil
}

// readingBookURLInActiveLanguage links to a Book's Reading entry only while the
// Book belongs to the active study language. Another language's Book has no
// reachable Reading entry; the learner changes language deliberately instead.
func readingBookURLInActiveLanguage(ctx context.Context, detail domain.MyBook) string {
	if shellViewFromContext(ctx) != nil {
		bookLanguage := canonicalization.NormalizeLanguage(bookStudyLanguage(detail))
		activeLanguage, _ := activeStudyLanguageForContext(ctx)
		if bookLanguage != "" && canonicalization.NormalizeLanguage(activeLanguage) != bookLanguage {
			return ""
		}
	}
	return readingBookURL(detail.Book.ID)
}

func (h *Handler) jobBookContexts(ctx context.Context, owner string, sourceIDs []string) (map[string]jobBookContext, error) {
	contexts := make(map[string]jobBookContext, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		if _, seen := contexts[sourceID]; seen {
			continue
		}
		book, err := h.jobBookContextForSource(ctx, owner, sourceID)
		if err != nil {
			return nil, err
		}
		contexts[sourceID] = book
	}
	return contexts, nil
}
