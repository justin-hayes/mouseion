package webapp

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// bookDetailView is the Book detail page: the Book, the analysis actions it
// hosts, and the read-only list of its words by Study state.
type bookDetailView struct {
	Book          domain.MyBook
	Language      string
	LanguageLabel string
	// Words is nil when the Book has no chosen language, so no word list exists.
	Words *bookWordsView
}

// bookDetailPage serves a Book's detail page (ADR 0136 §5). It replaces the
// Reading working desk: the Book's analysis actions are the only writes on the
// page, and the word list edits nothing.
func (h *Handler) bookDetailPage(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	detail, ok := h.bookDetail(w, r, owner.ID, r.PathValue("id"))
	if !ok {
		return
	}
	adoptDetailLanguage(r.Context(), detail)
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	if bookLanguage := bookStudyLanguage(detail); detail.Book.LanguageState == domain.LanguageChosen && bookLanguage != "" {
		language = bookLanguage
	}
	if err := h.annotateBookToReadLanguage(r.Context(), owner.ID, language, &detail); err != nil {
		fail(w, err)
		return
	}
	view := bookDetailView{Book: detail, Language: language, LanguageLabel: languageLabel}
	status := http.StatusOK
	if detail.Book.LanguageState == domain.LanguageChosen && language != "" {
		words, wordsStatus := h.loadBookWords(r.Context(), r, owner.ID, language, detail.Book.ID)
		view.Words, status = &words, wordsStatus
	}
	query := r.URL.Query()
	renderStatus(w, r, status, BookDetailPage(owner, h.csrf(w, r), view, query.Get("message"), query.Get("error")))
}

// bookDetailWordKeys are the word-list state a retired working-desk address
// carries over to the Book detail page.
var bookDetailWordKeys = []string{"q", "rev", "row", "page"}

// redirectReadingToBookDetail moves the retired working-desk address
// (/reading?reading={id}) to the named Book's detail page, keeping the prefix,
// revision, row and page the word list still understands. It reports whether
// the request named a Book; a request that does not belongs to Reading.
func redirectReadingToBookDetail(w http.ResponseWriter, r *http.Request) bool {
	query := r.URL.Query()
	bookID := strings.TrimSpace(query.Get("reading"))
	if bookID == "" {
		return false
	}
	location := bookDetailURL(bookID)
	values := url.Values{}
	for _, key := range bookDetailWordKeys {
		// Page 1 is the default, so it adds nothing to the address.
		if value := query.Get(key); value != "" && (key != "page" || value != "1") {
			values.Set(key, value)
		}
	}
	if encoded := values.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
	return true
}
