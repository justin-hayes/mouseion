package webapp

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassicalCatalogCardsMatchTitlesAndAuthorsInTitleOrder(t *testing.T) {
	authors := catalogFixture(t).Authors()
	view := classicalCatalogView{AddAvailable: true}
	titles := func(cards []catalogCard) []string {
		out := make([]string, 0, len(cards))
		for _, card := range cards {
			out = append(out, card.Title)
		}
		return out
	}

	t.Run("matches an author name", func(t *testing.T) {
		require.Equal(t, []string{"Iliad"}, titles(classicalCatalogCardsIn(authors, "HOMER", "", view)))
	})
	t.Run("matches a folded Greek title", func(t *testing.T) {
		require.Equal(t, []string{"Anabasis"}, titles(classicalCatalogCardsIn(authors, "αναβασισ", "", view)))
	})
	t.Run("treats a trailing Book number as ordinary search text", func(t *testing.T) {
		require.Empty(t, classicalCatalogCardsIn(authors, "iliad 2", "", view), "a Work has no constituent Book to narrow to")
	})
	t.Run("orders cards by title", func(t *testing.T) {
		all := titles(classicalCatalogCardsIn(authors, "", "", view))
		require.NotEmpty(t, all)
		require.True(t, sort.StringsAreSorted(all), "cards are ordered by title: %v", all)
	})
}

func TestCatalogEmptyStateChoosesOneDistinctReason(t *testing.T) {
	tests := []struct {
		name       string
		view       catalogBrowseView
		wantTitle  string
		wantAction string
	}{
		{name: "no match", view: catalogBrowseView{Query: "missing", ClearSearchURL: "/catalogs?language=", HasClassical: true}, wantTitle: "No matches", wantAction: "Clear search"},
		{name: "nothing in this language", view: catalogBrowseView{Language: "fr", LanguageLabel: "French", ShowAllURL: "/catalogs?language=", HasClassical: true}, wantTitle: "Nothing in French", wantAction: "Show all languages"},
		{name: "nothing to browse", view: catalogBrowseView{HasClassical: true}, wantTitle: "Nothing to browse yet", wantAction: "Manage sources"},
		{name: "results stay quiet", view: catalogBrowseView{TotalResults: 2, HasClassical: true}, wantTitle: ""},
		{name: "no sources", view: catalogBrowseView{}, wantTitle: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			title, _, action, _ := catalogEmptyState(test.view)
			assert.Equal(t, test.wantTitle, title)
			assert.Equal(t, test.wantAction, action)
		})
	}
}

func TestCatalogPagingAndSummaryHelpers(t *testing.T) {
	t.Run("an empty result still has one page", func(t *testing.T) {
		require.Equal(t, 1, catalogPageCount(0))
	})
	t.Run("a full page is one page", func(t *testing.T) {
		require.Equal(t, 1, catalogPageCount(12))
	})
	t.Run("one past a page adds a page", func(t *testing.T) {
		require.Equal(t, 2, catalogPageCount(13))
	})
	t.Run("summary pluralizes results and sources", func(t *testing.T) {
		require.Equal(t, "1 result from 1 source.", catalogResultSummary(1, 1))
		require.Equal(t, "4 results from 1 source.", catalogResultSummary(4, 1))
		require.Equal(t, "4 results from 2 sources.", catalogResultSummary(4, 2))
	})
	t.Run("count label pluralizes", func(t *testing.T) {
		require.Equal(t, "1 entry", catalogCountLabel(1, "entry", "entries"))
		require.Equal(t, "3 entries", catalogCountLabel(3, "entry", "entries"))
	})
}
