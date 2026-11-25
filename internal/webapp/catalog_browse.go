package webapp

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// catalogSectionPageSize is the one page size every Catalog section shares.
const catalogSectionPageSize = 12

// catalogClassicalSectionID names the Classical section in paging parameters.
// Connection IDs are UUIDs, so it cannot collide with an OPDS section.
const catalogClassicalSectionID = "classical"

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

// catalogSection is one source's matching cards and paging state. Every source
// renders through it: an OPDS connection or the Classical catalog.
type catalogSection struct {
	ID          string
	Title       string
	Page        int
	PageCount   int
	Total       int
	Cards       []catalogCard
	PreviousURL string
	NextURL     string
	// Attribution is the licence credit rendered as the section's footer.
	Attribution []classical.SourceAttribution
	// Notice explains why the section cannot offer Add.
	Notice string
}

// catalogCard is one Catalog text from any source: an OPDS entry or one
// Classical Reading unit. Plate is the image slot; a retained cover replaces it
// when the source has one.
type catalogCard struct {
	ID         string
	Title      string
	GreekTitle string
	Author     string
	// Detail is the one detail line, the Edition label for a Classical unit.
	Detail   string
	TitleURL string
	Plate    titlePlate
	// CoverURL is the local owner-scoped address of a retained cover with its
	// intrinsic size; empty while the card shows its Plate. It is never an
	// upstream URL.
	CoverURL                string
	CoverWidth, CoverHeight int
	// OwnedURL opens the owned Book. When empty the card offers Add and I've
	// read this at AddURL and ReadURL, both unavailable when AddURL is empty.
	OwnedURL string
	AddURL   string
	ReadURL  string
	// EntryIdentifier is posted by an OPDS card; ReturnTo by a Classical card
	// so the Add returns to the Catalog.
	EntryIdentifier string
	ReturnTo        string
}

// catalogBrowseView is everything the Catalog results need: one search and
// language filter over the Classical catalog and every OPDS section, grouped
// by source.
type catalogBrowseView struct {
	Query            string
	Language         string
	LanguageLabel    string
	LanguageOptions  []catalogLanguageOption
	Classical        catalogSection
	Sources          []catalogSection
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
	languages, err := h.catalogIndexLanguages(r, owner, connections)
	if err != nil {
		return catalogBrowseView{}, err
	}
	view := catalogBrowseView{
		Query:           filter.Query,
		Language:        filter.Language,
		LanguageOptions: catalogLanguageOptions(r, filter.Language, hasClassical, languages),
		HasClassical:    hasClassical,
	}
	view.LanguageLabel = catalogLanguageLabel(view.LanguageOptions, filter.Language)
	view.ClearSearchURL = filter.withoutQuery().browseURL(nil)
	view.ShowAllURL = filter.allLanguages().browseURL(nil)
	pages := make(map[string]int, len(connections)+1)
	sections := make([]*catalogSection, 0, len(connections)+1)
	if hasClassical && (filter.Language == "" || filter.Language == classical.Language) {
		view.Classical = h.loadCatalogClassicalSection(r, filter, classicalView)
		pages[catalogClassicalSectionID] = view.Classical.Page
		sections = append(sections, &view.Classical)
	}
	sources, err := h.loadCatalogOpdsSections(r, owner, connections, filter)
	if err != nil {
		return catalogBrowseView{}, err
	}
	view.Sources = sources
	total := view.Classical.Total
	for i := range view.Sources {
		pages[view.Sources[i].ID] = view.Sources[i].Page
		sections = append(sections, &view.Sources[i])
		total += view.Sources[i].Total
	}
	catalogSetSectionPagingURLs(sections, pages, filter)
	view.TotalResults = total
	view.ResultSummary = catalogResultSummary(view.TotalResults, catalogSourceCount(view))
	view.EmptyTitle, view.EmptyMessage, view.EmptyActionLabel, view.EmptyActionURL = catalogEmptyState(view)
	return view, nil
}

// loadCatalogClassicalSection builds the Classical section from the release:
// one card per current Reading unit, ordered and paged like every other
// section.
func (h *Handler) loadCatalogClassicalSection(r *http.Request, filter catalogFilter, view classicalCatalogView) catalogSection {
	cards := classicalCatalogCards(h.services.Classical.Authors(), filter.Query, view)
	page, pageCount := catalogClampPage(catalogPageParam(r, catalogClassicalSectionID), len(cards))
	offset := (page - 1) * catalogSectionPageSize
	end := min(offset+catalogSectionPageSize, len(cards))
	section := catalogSection{
		ID:          catalogClassicalSectionID,
		Title:       "Classical catalog",
		Page:        page,
		PageCount:   pageCount,
		Total:       len(cards),
		Attribution: h.services.Classical.SourceAttributions(),
	}
	if offset < end {
		section.Cards = cards[offset:end]
	}
	if !view.AddAvailable {
		section.Notice = view.AddUnavailableReason
	}
	return section
}

// catalogClampPage keeps a requested page inside the section's page range.
func catalogClampPage(page, total int) (int, int) {
	pageCount := catalogPageCount(total)
	return min(max(page, 1), pageCount), pageCount
}

// catalogIndexLanguages lists every language tag the learner's Catalog index
// holds, so the filter offers languages the learner does not study yet.
func (h *Handler) catalogIndexLanguages(r *http.Request, owner string, connections []domain.OpdsConnection) ([]string, error) {
	seen := map[string]struct{}{}
	for _, connection := range connections {
		result, err := h.services.Store.Catalogs.BrowseCatalogueEntries(r.Context(), owner, connection.ID, domain.CatalogueQuery{})
		if err != nil {
			return nil, err
		}
		for _, entry := range result.Entries {
			if language := canonicalization.NormalizeLanguage(entry.Index.LanguageTag); language != "" {
				seen[language] = struct{}{}
			}
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

func catalogBrowseQuery(r *http.Request) catalogFilter {
	filter := catalogFilter{Query: strings.TrimSpace(r.URL.Query().Get("q"))}
	// The Catalog has no page language, so the filter defaults to the Last
	// study language and is never stored (ADR 0129, ADR 0131).
	filter.Language = canonicalization.NormalizeLanguage(shellViewFromContext(r.Context()).lastLanguage())
	if r.URL.Query().Has("language") {
		filter.Language = canonicalization.NormalizeLanguage(r.URL.Query().Get("language"))
	}
	return filter
}

// loadCatalogOpdsSections reads each connection's local index page and returns
// the sections.
func (h *Handler) loadCatalogOpdsSections(r *http.Request, owner string, connections []domain.OpdsConnection, filter catalogFilter) ([]catalogSection, error) {
	sources := make([]catalogSection, 0, len(connections))
	for _, connection := range connections {
		section, err := h.loadCatalogOpdsSection(r, owner, connection, filter)
		if err != nil {
			return nil, err
		}
		sources = append(sources, section)
	}
	return sources, nil
}

func (h *Handler) loadCatalogOpdsSection(r *http.Request, owner string, connection domain.OpdsConnection, filter catalogFilter) (catalogSection, error) {
	page := catalogPageParam(r, connection.ID)
	result, err := h.browseCatalogueConnection(r, owner, connection.ID, filter, page)
	if err != nil {
		return catalogSection{}, err
	}
	pageCount := catalogPageCount(result.Total)
	if page > pageCount {
		page = pageCount
		result, err = h.browseCatalogueConnection(r, owner, connection.ID, filter, page)
		if err != nil {
			return catalogSection{}, err
		}
	}
	cards := make([]catalogCard, 0, len(result.Entries))
	for _, entry := range result.Entries {
		cards = append(cards, opdsCatalogCard(entry))
	}
	return catalogSection{
		ID:        connection.ID,
		Title:     connection.Name,
		Page:      page,
		PageCount: pageCount,
		Total:     result.Total,
		Cards:     cards,
	}, nil
}

// opdsCatalogCard is one OPDS entry as a Catalog card.
func opdsCatalogCard(entry domain.CatalogueEntry) catalogCard {
	index := entry.Index
	card := catalogCard{
		ID:              index.EntryIdentifier,
		Title:           index.Title,
		Author:          index.Author,
		Plate:           newTitlePlate(index.Title, "", index.Author),
		EntryIdentifier: index.EntryIdentifier,
	}
	if index.CoverRetained && index.CoverWidth > 0 && index.CoverHeight > 0 {
		card.CoverURL = catalogEntryCoverURL(index.ConnectionID, index.EntryIdentifier)
		card.CoverWidth, card.CoverHeight = index.CoverWidth, index.CoverHeight
	}
	if entry.BookID != "" {
		card.OwnedURL = catalogBookInMyBooksURL(entry.BookID)
		return card
	}
	card.AddURL = "/catalogs/sources/" + url.PathEscape(index.ConnectionID) + "/add"
	card.ReadURL = "/catalogs/sources/" + url.PathEscape(index.ConnectionID) + "/previously-read"
	return card
}

func catalogSetSectionPagingURLs(sections []*catalogSection, pages map[string]int, filter catalogFilter) {
	for _, section := range sections {
		if section.Page > 1 {
			section.PreviousURL = filter.atPage(pages, section.ID, section.Page-1)
		}
		if section.Page < section.PageCount {
			section.NextURL = filter.atPage(pages, section.ID, section.Page+1)
		}
	}
}

func (h *Handler) browseCatalogueConnection(r *http.Request, owner, connectionID string, filter catalogFilter, page int) (domain.CataloguePage, error) {
	return h.services.Store.Catalogs.BrowseCatalogueEntries(r.Context(), owner, connectionID, domain.CatalogueQuery{
		Search:   filter.Query,
		Language: filter.Language,
		Offset:   (page - 1) * catalogSectionPageSize,
		Limit:    catalogSectionPageSize,
	})
}

// catalogLanguageOptions lists the learner's study languages, every language the
// Catalog index holds, and the Classical catalog's language. The empty value is
// first so "All languages" is always available, and a requested language is
// kept visible even when no Book currently derives it.
func catalogLanguageOptions(r *http.Request, requested string, hasClassical bool, indexLanguages []string) []catalogLanguageOption {
	names := languageNames{}
	if shell := shellViewFromContext(r.Context()); shell != nil {
		for _, option := range shell.Options {
			if language := canonicalization.NormalizeLanguage(option.Language); language != "" {
				names[language] = option.DisplayName
			}
		}
	}
	if hasClassical {
		names.addIfAbsent(classical.Language, classical.LanguageDisplayName)
	}
	for _, language := range indexLanguages {
		names.addIfAbsent(language, language)
	}
	if requested != "" {
		names.addIfAbsent(requested, requested)
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

// languageNames maps a language tag to the label the filter shows for it.
type languageNames map[string]string

// addIfAbsent keeps the first label a language was given.
func (n languageNames) addIfAbsent(language, label string) {
	if _, ok := n[language]; !ok {
		n[language] = label
	}
}

func catalogLanguageLabel(options []catalogLanguageOption, language string) string {
	for _, option := range options {
		if option.Value == language {
			return option.Label
		}
	}
	return language
}

func catalogPageParam(r *http.Request, sectionID string) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page_" + sectionID))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

func catalogPageCount(total int) int {
	if total <= 0 {
		return 1
	}
	return (total + catalogSectionPageSize - 1) / catalogSectionPageSize
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
	if view.Classical.Total > 0 {
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
	return "Nothing to browse yet", "Sync an OPDS source, or choose Ancient Greek for the Classical catalog.", "Manage sources", "/catalogs/sources"
}
