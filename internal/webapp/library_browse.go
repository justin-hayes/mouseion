package webapp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const myBooksPageSize = 25

// MyBooksBrowseState carries the server-rendered collection controls and
// result state. Enabled deliberately distinguishes the new read model from
// the zero-value legacy rendering used by lightweight stores.
type MyBooksBrowseState struct {
	Enabled            bool
	Query              string
	Language           string
	LanguageLabel      string
	LanguageCorpus     *languageCorpusPanelView
	NeedsLanguageCount int
	AllCount           int
	Total              int
	Page               int
	PageCount          int
	TextNoMatch        bool
}

func myBooksBrowseURL(query string, page int) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if encoded := values.Encode(); encoded != "" {
		return "/library?" + encoded
	}
	return "/library"
}

func parseMyBooksBrowseRequest(rURL *url.URL) (query string, page int) {
	query = strings.TrimSpace(rURL.Query().Get("q"))
	page = 1
	if parsed, err := strconv.Atoi(rURL.Query().Get("page")); err == nil && parsed >= 1 {
		page = parsed
	}
	return query, page
}

func myBooksPageCount(total int) int {
	if total <= 0 {
		return 0
	}
	return (total + myBooksPageSize - 1) / myBooksPageSize
}

func myBooksSelectedAttributes(selected bool) templ.Attributes {
	if selected {
		return templ.Attributes{"aria-current": "page"}
	}
	return nil
}

func myBooksResultCount(total int) string {
	if total == 1 {
		return "1 book in this result"
	}
	return fmt.Sprintf("%d books in this result", total)
}

func myBooksResultAnnouncementAttributes(panel *languageCorpusPanelView) templ.Attributes {
	if panel != nil {
		return nil
	}
	return templ.Attributes{"role": "status", "aria-live": "polite", "aria-atomic": "true"}
}

func myBooksResultsHeading(browse MyBooksBrowseState) string {
	if browse.Language != "" {
		language := myBooksLanguageName(browse)
		if browse.Query != "" {
			return fmt.Sprintf("My Books in %s matching “%s”", language, browse.Query)
		}
		return fmt.Sprintf("My Books in %s", language)
	}
	if browse.Query != "" {
		return fmt.Sprintf("My Books matching “%s”", browse.Query)
	}
	return "Books in My Books"
}

func myBooksPageHeading(browse MyBooksBrowseState) string {
	if browse.Language == "" {
		return "My Books"
	}
	return fmt.Sprintf("My Books in %s", myBooksLanguageName(browse))
}

func myBooksLanguageName(browse MyBooksBrowseState) string {
	if browse.LanguageLabel != "" {
		return browse.LanguageLabel
	}
	return browse.Language
}

func myBooksBookInLanguage(browse MyBooksBrowseState, book domain.MyBook) bool {
	if browse.Language == "" {
		return true
	}
	return book.Book.LanguageState == domain.LanguageChosen && strings.EqualFold(strings.TrimSpace(book.Book.LanguageTag), browse.Language)
}

func myBooksPageOffset(page int) int {
	if page <= 1 {
		return 0
	}
	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/myBooksPageSize {
		return maxInt
	}
	return (page - 1) * myBooksPageSize
}
