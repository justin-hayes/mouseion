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
	Disposition        domain.BookDisposition
	Language           string
	LanguageLabel      string
	NeedsLanguage      bool
	NeedsLanguageCount int
	AllCount           int
	ScopeTotal         int
	Total              int
	Page               int
	PageCount          int
	TextNoMatch        bool
	InboxCount         int
	ToReadCount        int
	SetAsideCount      int
	RefreshableBookIDs map[string]bool
}

func myBooksURL(query string, page int, needsLanguage bool) string {
	return myBooksFilteredURL(query, page, needsLanguage, "")
}

func myBooksFilteredURL(query string, page int, needsLanguage bool, disposition domain.BookDisposition) string {
	values := url.Values{}
	if needsLanguage {
		values.Set("needs-language", "")
	}
	if query != "" {
		values.Set("q", query)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if disposition != "" {
		values.Set("disposition", string(disposition))
	}
	if encoded := values.Encode(); encoded != "" {
		if needsLanguage {
			encoded = strings.Replace(encoded, "needs-language=", "needs-language", 1)
		}
		return "/library?" + encoded
	}
	return "/library"
}

func parseMyBooksBrowseRequest(rURL *url.URL) (query string, page int, needsLanguage bool) {
	query, page, needsLanguage, _ = parseMyBooksBrowseRequestWithDisposition(rURL)
	return query, page, needsLanguage
}

func parseMyBooksBrowseRequestWithDisposition(rURL *url.URL) (query string, page int, needsLanguage bool, disposition domain.BookDisposition) {
	query = strings.TrimSpace(rURL.Query().Get("q"))
	needsLanguage = rURL.Query().Has("needs-language")
	disposition = domain.BookDisposition(strings.TrimSpace(rURL.Query().Get("disposition")))
	if disposition.Validate() != nil {
		disposition = ""
	}
	page = 1
	if parsed, err := strconv.Atoi(rURL.Query().Get("page")); err == nil && parsed >= 1 {
		page = parsed
	}
	return query, page, needsLanguage, disposition
}

func myBooksResultsURL(browse MyBooksBrowseState, page int) string {
	return myBooksFilteredURL(browse.Query, page, browse.NeedsLanguage, browse.Disposition)
}

func myBooksDispositionURL(browse MyBooksBrowseState, disposition domain.BookDisposition) string {
	return myBooksFilteredURL(browse.Query, 1, browse.NeedsLanguage, disposition)
}

func myBooksDispositionCount(browse MyBooksBrowseState, disposition domain.BookDisposition) int {
	switch disposition {
	case domain.BookDispositionInbox:
		return browse.InboxCount
	case domain.BookDispositionToRead:
		return browse.ToReadCount
	case domain.BookDispositionSetAside:
		return browse.SetAsideCount
	default:
		return browse.ScopeTotal
	}
}

func myBooksDispositionLabel(disposition domain.BookDisposition) string {
	switch disposition {
	case domain.BookDispositionInbox:
		return "Inbox"
	case domain.BookDispositionToRead:
		return "To Read"
	case domain.BookDispositionSetAside:
		return "Set Aside"
	default:
		return "All"
	}
}

func myBookDisposition(book domain.MyBook) domain.BookDisposition {
	if book.Disposition != "" {
		return book.Disposition
	}
	if myBookInJourney(book) {
		return domain.BookDispositionToRead
	}
	return domain.BookDispositionInbox
}

func myBookSetAsideConfirmationText(book domain.MyBook) string {
	retained := " Acquired content, analysis, provenance, and reading history remain."
	if myBookDisposition(book) == domain.BookDispositionToRead {
		return "This removes the Book from Reading Journey." + retained
	}
	return "This sets aside the Book without adding it to Reading Journey." + retained
}

func myBooksAllDispositionCount(browse MyBooksBrowseState) int {
	if browse.Enabled {
		return browse.ScopeTotal
	}
	return browse.AllCount
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

func myBooksResultAnnouncementAttributes() templ.Attributes {
	return templ.Attributes{"role": "status", "aria-live": "polite", "aria-atomic": "true"}
}

func myBooksResultsHeading(browse MyBooksBrowseState) string {
	if browse.NeedsLanguage {
		return "Books awaiting a language"
	}
	workflow := ""
	if browse.Disposition != "" {
		workflow = myBooksDispositionLabel(browse.Disposition) + " in "
	}
	if browse.Language != "" {
		language := myBooksLanguageName(browse)
		if browse.Query != "" {
			return fmt.Sprintf("%sMy Books in %s matching “%s”", workflow, language, browse.Query)
		}
		return workflow + "My Books in " + language
	}
	if browse.Query != "" {
		return fmt.Sprintf("%sMy Books matching “%s”", workflow, browse.Query)
	}
	if workflow != "" {
		return strings.TrimSpace(workflow) + "My Books"
	}
	return "Books in My Books"
}

func myBooksPageHeading(browse MyBooksBrowseState) string {
	if browse.NeedsLanguage {
		return "Books awaiting a language"
	}
	if browse.Language == "" {
		return "My Books"
	}
	return "My Books in " + myBooksLanguageName(browse)
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
