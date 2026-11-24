package webapp

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const catalogOpdsPageSize = 6

// catalogLanguageOption is one language the Catalog filter offers. The empty
// value is "all languages".
type catalogLanguageOption struct {
	Value    string
	Label    string
	Selected bool
}

// catalogFilter is the one query and language narrow shared by the Classical
// search and every OPDS section.
type catalogFilter struct {
	Query    string
	Language string
}

// browseURL is the Catalog location for this filter and the sections' pages.
func (f catalogFilter) browseURL(pages map[string]int) string {
	values := url.Values{}
	if f.Query != "" {
		values.Set("q", f.Query)
	}
	values.Set("language", f.Language)
	for connectionID, page := range pages {
		if page > 1 {
			values.Set("page_"+connectionID, strconv.Itoa(page))
		}
	}
	return "/catalogs?" + values.Encode()
}

// atPage changes one section's page while preserving the query, the language
// filter, and every other section's page.
func (f catalogFilter) atPage(pages map[string]int, connectionID string, page int) string {
	next := make(map[string]int, len(pages))
	maps.Copy(next, pages)
	next[connectionID] = page
	return f.browseURL(next)
}

func (f catalogFilter) withoutQuery() catalogFilter {
	f.Query = ""
	return f
}

func (f catalogFilter) allLanguages() catalogFilter {
	f.Language = ""
	return f
}

// catalogOpdsSection is one connection's matching entries and paging state.
type catalogOpdsSection struct {
	Connection  domain.OpdsConnection
	Page        int
	PageCount   int
	Total       int
	Entries     []domain.CatalogueEntry
	PreviousURL string
	NextURL     string
}

// catalogBrowseView is everything the Catalog results need: one search and
// language filter over the Classical catalog and every OPDS section, grouped
// by source.
type catalogBrowseView struct {
	Query            string
	Language         string
	LanguageLabel    string
	LanguageOptions  []catalogLanguageOption
	ClassicalAuthors []classical.CatalogAuthor
	ClassicalView    classicalCatalogView
	ClassicalTotal   int
	Sources          []catalogOpdsSection
	HasClassical     bool
	TotalResults     int
	ResultSummary    string
	EmptyTitle       string
	EmptyMessage     string
	EmptyActionLabel string
	EmptyActionURL   string
	ClearSearchURL   string
	ShowAllURL       string
	// AddedBookID is the Book an immediate Add named, after its alias was
	// verified owner-scoped. FocusAddedBook moves focus to that confirmation
	// after the ordinary POST redirect.
	AddedBookID    string
	FocusAddedBook bool
	// ReadBookID is the Book an immediate Previously read assertion named,
	// after its alias was verified owner-scoped. FocusReadBook moves focus to
	// that confirmation after the ordinary POST redirect.
	ReadBookID    string
	FocusReadBook bool
}

// loadCatalogBrowse applies one query and language filter to the Classical
// catalog and every OPDS connection. Only the local Catalog index is read; the
// OPDS server is never contacted.
func (h *Handler) loadCatalogBrowse(r *http.Request, owner string, connections []domain.OpdsConnection, classicalView classicalCatalogView) (catalogBrowseView, error) {
	filter := catalogBrowseQuery(r)
	hasClassical := h.services.Classical != nil && h.services.Classical.Counts().Works > 0
	view := catalogBrowseView{
		Query:           filter.Query,
		Language:        filter.Language,
		LanguageOptions: catalogLanguageOptions(r, filter.Language, hasClassical),
		ClassicalView:   classicalView,
		HasClassical:    hasClassical,
	}
	view.LanguageLabel = catalogLanguageLabel(view.LanguageOptions, filter.Language)
	view.ClearSearchURL = filter.withoutQuery().browseURL(nil)
	view.ShowAllURL = filter.allLanguages().browseURL(nil)
	if hasClassical && (filter.Language == "" || filter.Language == classical.Language) {
		view.ClassicalAuthors = filterClassicalCatalog(h.services.Classical.Authors(), filter.Query)
		view.ClassicalTotal = catalogAuthorsWorkCount(view.ClassicalAuthors)
	}
	sources, opdsTotal, err := h.loadCatalogOpdsSections(r, owner, connections, filter)
	if err != nil {
		return catalogBrowseView{}, err
	}
	view.Sources = sources
	view.TotalResults = view.ClassicalTotal + opdsTotal
	view.ResultSummary = catalogResultSummary(view.TotalResults, catalogSourceCount(view))
	view.EmptyTitle, view.EmptyMessage, view.EmptyActionLabel, view.EmptyActionURL = catalogEmptyState(view)
	return view, nil
}

func catalogBrowseQuery(r *http.Request) catalogFilter {
	filter := catalogFilter{Query: strings.TrimSpace(r.URL.Query().Get("q"))}
	activeLanguage, _ := activeStudyLanguageForContext(r.Context())
	filter.Language = canonicalization.NormalizeLanguage(activeLanguage)
	if r.URL.Query().Has("language") {
		filter.Language = canonicalization.NormalizeLanguage(r.URL.Query().Get("language"))
	}
	return filter
}

// loadCatalogOpdsSections reads each connection's local index page and returns
// the sections plus the total matches before paging.
func (h *Handler) loadCatalogOpdsSections(r *http.Request, owner string, connections []domain.OpdsConnection, filter catalogFilter) ([]catalogOpdsSection, int, error) {
	pages := make(map[string]int, len(connections))
	sources := make([]catalogOpdsSection, 0, len(connections))
	total := 0
	for _, connection := range connections {
		section, err := h.loadCatalogOpdsSection(r, owner, connection, filter)
		if err != nil {
			return nil, 0, err
		}
		pages[connection.ID] = section.Page
		sources = append(sources, section)
		total += section.Total
	}
	catalogSetSectionPagingURLs(sources, pages, filter)
	return sources, total, nil
}

func (h *Handler) loadCatalogOpdsSection(r *http.Request, owner string, connection domain.OpdsConnection, filter catalogFilter) (catalogOpdsSection, error) {
	page := catalogPageParam(r, connection.ID)
	result, err := h.browseCatalogueConnection(r, owner, connection.ID, filter, page)
	if err != nil {
		return catalogOpdsSection{}, err
	}
	pageCount := catalogPageCount(result.Total)
	if page > pageCount {
		page = pageCount
		result, err = h.browseCatalogueConnection(r, owner, connection.ID, filter, page)
		if err != nil {
			return catalogOpdsSection{}, err
		}
	}
	return catalogOpdsSection{
		Connection: connection,
		Page:       page,
		PageCount:  pageCount,
		Total:      result.Total,
		Entries:    result.Entries,
	}, nil
}

func catalogSetSectionPagingURLs(sources []catalogOpdsSection, pages map[string]int, filter catalogFilter) {
	for i := range sources {
		section := &sources[i]
		if section.Page > 1 {
			section.PreviousURL = filter.atPage(pages, section.Connection.ID, section.Page-1)
		}
		if section.Page < section.PageCount {
			section.NextURL = filter.atPage(pages, section.Connection.ID, section.Page+1)
		}
	}
}

func (h *Handler) browseCatalogueConnection(r *http.Request, owner, connectionID string, filter catalogFilter, page int) (domain.CataloguePage, error) {
	return h.services.Store.Catalogs.BrowseCatalogueEntries(r.Context(), owner, connectionID, domain.CatalogueQuery{
		Search:   filter.Query,
		Language: filter.Language,
		Offset:   (page - 1) * catalogOpdsPageSize,
		Limit:    catalogOpdsPageSize,
	})
}

// catalogLanguageOptions lists the learner's study languages plus the Classical
// catalog's language. The empty value is first so "All languages" is always
// available, and a requested language is kept visible even when no Book
// currently derives it.
func catalogLanguageOptions(r *http.Request, requested string, hasClassical bool) []catalogLanguageOption {
	names := map[string]string{}
	if shell := shellViewFromContext(r.Context()); shell != nil {
		for _, option := range shell.Options {
			if language := canonicalization.NormalizeLanguage(option.Language); language != "" {
				names[language] = option.DisplayName
			}
		}
	}
	if hasClassical {
		if _, ok := names[classical.Language]; !ok {
			names[classical.Language] = classical.LanguageDisplayName
		}
	}
	if requested != "" {
		if _, ok := names[requested]; !ok {
			names[requested] = requested
		}
	}
	languages := make([]string, 0, len(names))
	for language := range names {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	options := make([]catalogLanguageOption, 0, len(languages)+1)
	options = append(options, catalogLanguageOption{Value: "", Label: "All languages", Selected: requested == ""})
	for _, language := range languages {
		options = append(options, catalogLanguageOption{Value: language, Label: names[language], Selected: language == requested})
	}
	return options
}

func catalogLanguageLabel(options []catalogLanguageOption, language string) string {
	for _, option := range options {
		if option.Value == language {
			return option.Label
		}
	}
	return language
}

func catalogPageParam(r *http.Request, connectionID string) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page_" + connectionID))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func catalogPageCount(total int) int {
	if total <= 0 {
		return 1
	}
	return (total + catalogOpdsPageSize - 1) / catalogOpdsPageSize
}

func catalogCountLabel(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(count) + " " + plural
}

func catalogBookInMyBooksURL(bookID string) string {
	return "/catalogs/books/" + url.PathEscape(bookID) + "/in-my-books"
}

func catalogSourceCount(view catalogBrowseView) int {
	count := 0
	if view.ClassicalTotal > 0 {
		count++
	}
	for _, section := range view.Sources {
		if section.Total > 0 {
			count++
		}
	}
	return count
}

func catalogResultSummary(total, sources int) string {
	if total == 0 {
		return ""
	}
	if total == 1 {
		return "1 result from 1 source."
	}
	if sources == 1 {
		return fmt.Sprintf("%d results from 1 source.", total)
	}
	return fmt.Sprintf("%d results from %d sources.", total, sources)
}

func catalogEmptyState(view catalogBrowseView) (title, message, actionLabel, actionURL string) {
	if len(view.Sources) == 0 && !view.HasClassical {
		return "", "", "", ""
	}
	if view.TotalResults > 0 {
		return "", "", "", ""
	}
	if view.Query != "" {
		return "No matches", fmt.Sprintf("No entry in the Catalog matches “%s”. Try another search or clear it.", view.Query), "Clear search", view.ClearSearchURL
	}
	if view.Language != "" {
		return "Nothing in " + view.LanguageLabel, fmt.Sprintf("No Catalog entry is in %s.", view.LanguageLabel), "Show all languages", view.ShowAllURL
	}
	return "Nothing to browse yet", "Sync an OPDS source, or browse the Classical catalog.", "Manage sources", "/catalogs/sources"
}
