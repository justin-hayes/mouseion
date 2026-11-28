package webapp

import (
	"bytes"
	"context"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksMostReadableSortIsKeptInTheAddress(t *testing.T) {
	request, ok := parseMyBooksBrowseRequest(&url.URL{RawQuery: "sort=readable&q=faust"})
	assert.True(t, ok)
	assert.True(t, request.mostReadable)

	request, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: "q=faust"})
	assert.True(t, ok)
	assert.False(t, request.mostReadable)

	_, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: "sort=bogus"})
	assert.False(t, ok, "an unknown sort value is not Reading")

	browse := MyBooksBrowseState{Language: "de", Disposition: domain.BookDispositionToRead, MostReadable: true, Query: "faust"}
	assert.Equal(t, "/de/reading?disposition=to_read&q=faust&sort=readable", myBooksResultsURL(browse, 1))
	assert.Equal(t, "/de/reading?disposition=to_read&sort=readable", myBooksClearSearchURL(browse))
	assert.Equal(t, "/de/reading?history=read&q=faust", myBooksReadTabURL(browse), "the Read tab keeps its own order")
	assert.Equal(t, "/de/reading?disposition=to_read&q=faust", myBooksSortToggleURL(browse), "the toggle returns to the default title order")
	assert.Equal(t, "/de/reading?disposition=to_read&q=faust&sort=readable", myBooksSortToggleURL(MyBooksBrowseState{Language: "de", Disposition: domain.BookDispositionToRead, Query: "faust"}))

	_, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: "history=read&sort=readable"})
	assert.False(t, ok, "the most-readable sort is a To Read control")

	// The sort composes with the needs-action filter in one address.
	filtered := MyBooksBrowseState{Language: "de", Disposition: domain.BookDispositionToRead, NeedsAction: true, MostReadable: true, Query: "faust"}
	assert.Equal(t, "/de/reading?disposition=to_read&needs=action&page=2&q=faust&sort=readable", myBooksResultsURL(filtered, 2))
	assert.Equal(t, "/de/reading?disposition=to_read&q=faust&sort=readable", myBooksNeedsActionToggleURL(filtered), "the needs toggle keeps the sort")

	returned := myBooksReturnURL("/de/reading?sort=readable&disposition=to_read", "de")
	assert.Equal(t, "/de/reading?disposition=to_read&sort=readable", returned)
}

func TestMyBooksSortControlOffersMostReadableOnTheToReadTab(t *testing.T) {
	render := func(state MyBooksBrowseState) string {
		var output bytes.Buffer
		books := []domain.MyBook{{Book: domain.Book{ID: "book", OwnerID: "owner", Title: "A Book", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Disposition: domain.BookDispositionToRead}}
		require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", state).Render(context.Background(), &output))
		return output.String()
	}

	toRead := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 1, ScopeTotal: 1, Total: 1, Disposition: domain.BookDispositionToRead}
	body := render(toRead)
	assert.Contains(t, body, "Most readable first")
	assert.Contains(t, body, "/de/reading?disposition=to_read&amp;sort=readable", "the sort control is an address")
	assert.NotContains(t, body, `name="sort"`, "the default order carries no sort field")

	sorted := render(MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 1, ScopeTotal: 1, Total: 1, Disposition: domain.BookDispositionToRead, MostReadable: true})
	assert.Contains(t, sorted, "Title order")
	assert.Contains(t, sorted, `name="sort" value="readable"`, "a search keeps the sort")

	read := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 1, ScopeTotal: 1, Total: 1, History: true}
	assert.NotContains(t, render(read), "Most readable first", "the Read tab keeps its own order")
}
