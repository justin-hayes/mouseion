package webapp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

const myBooksPageSize = 25

// MyBooksLanguageCount is the count shown by one language filter pill.
type MyBooksLanguageCount struct {
	Tag   string
	Count int
}

// MyBooksBrowseState carries the server-rendered collection controls and
// result state. Enabled deliberately distinguishes the new read model from
// the zero-value legacy rendering used by lightweight stores.
type MyBooksBrowseState struct {
	Enabled         bool
	Query           string
	Language        string
	LanguageCorpus  *languageCorpusPanelView
	Counts          []MyBooksLanguageCount
	AllCount        int
	Total           int
	Page            int
	PageCount       int
	TextNoMatch     bool
	CombinedNoMatch bool
}

func myBooksBrowseURL(query, language string, page int) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if language != "" {
		values.Set("language", language)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if encoded := values.Encode(); encoded != "" {
		return "/library?" + encoded
	}
	return "/library"
}

func parseMyBooksBrowseRequest(rURL *url.URL) (query, language string, page int) {
	query = strings.TrimSpace(rURL.Query().Get("q"))
	language = strings.TrimSpace(rURL.Query().Get("language"))
	page = 1
	if parsed, err := strconv.Atoi(rURL.Query().Get("page")); err == nil && parsed >= 1 {
		page = parsed
	}
	return query, language, page
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

func myBooksLanguageSelected(selected, tag string) bool {
	return strings.EqualFold(strings.TrimSpace(selected), tag)
}

func myBooksSelectionSuffix(selected bool) string {
	if selected {
		return " (selected)"
	}
	return ""
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
	if browse.Query != "" {
		return fmt.Sprintf("My Books matching “%s”", browse.Query)
	}
	if browse.Language != "" {
		return fmt.Sprintf("My Books in %s", browse.Language)
	}
	return "Books in My Books"
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
