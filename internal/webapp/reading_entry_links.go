package webapp

import (
	"context"
	"errors"
	"strings"

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
		book.ReadingURL = readingBookURLForBook(detail)
	}
	return book, nil
}

// readingBookURLForBook links to a Book's Reading entry in the Book's
// own study language (ADR 0131): a Book-addressed link returns to its Book's
// language, never the Last study language.
func readingBookURLForBook(detail domain.MyBook) string {
	return readingBookURL(bookStudyLanguage(detail), detail.Book.ID)
}
