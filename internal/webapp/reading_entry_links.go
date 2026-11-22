package webapp

import (
	"context"
	"errors"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// jobBookContext carries the Book identity one operational detail page names.
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

func (h *Handler) jobBookContextForSource(ctx context.Context, owner, sourceID string) (jobBookContext, error) {
	if sourceID == "" {
		return jobBookContext{}, nil
	}
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, sourceID)
	if errors.Is(err, domain.ErrNotFound) {
		return jobBookContext{}, nil
	}
	if err != nil {
		return jobBookContext{}, err
	}
	book := jobBookContext{Title: strings.TrimSpace(detail.Book.Title)}
	if book.Title == "" && detail.Acquired != nil {
		book.Title = canonicalBookTitle(*detail.Acquired)
	}
	if detail.Acquired == nil || !analysisReadyForReading(*detail.Acquired) {
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
