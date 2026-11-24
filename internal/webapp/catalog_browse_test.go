package webapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterClassicalCatalogMatchesTitlesAndAuthors(t *testing.T) {
	catalog := catalogFixture(t)
	authors := catalog.Authors()

	byAuthor := filterClassicalCatalog(authors, "HOMER")
	require.Len(t, byAuthor, 1)
	assert.Equal(t, "Homer", byAuthor[0].Name)
	assert.Len(t, byAuthor[0].Works, 1)
	assert.Equal(t, "Iliad", byAuthor[0].Works[0].Title)

	byGreekTitle := filterClassicalCatalog(authors, "αναβασισ")
	require.Len(t, byGreekTitle, 1)
	assert.Equal(t, "Xenophon", byGreekTitle[0].Name)
	assert.Equal(t, "Anabasis", byGreekTitle[0].Works[0].Title)

	assert.Empty(t, filterClassicalCatalog(authors, "iliad 2"), "a trailing Book number is ordinary search text: a Work has no constituent Book to narrow to")
	assert.Equal(t, authors, filterClassicalCatalog(authors, ""))
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
	assert.Equal(t, 1, catalogPageCount(0))
	assert.Equal(t, 1, catalogPageCount(6))
	assert.Equal(t, 2, catalogPageCount(7))
	assert.Equal(t, "1 result from 1 source.", catalogResultSummary(1, 1))
	assert.Equal(t, "4 results from 1 source.", catalogResultSummary(4, 1))
	assert.Equal(t, "4 results from 2 sources.", catalogResultSummary(4, 2))
	assert.Equal(t, "1 entry", catalogCountLabel(1, "entry", "entries"))
	assert.Equal(t, "3 entries", catalogCountLabel(3, "entry", "entries"))
}
