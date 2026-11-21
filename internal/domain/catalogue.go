package domain

import (
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

// CatalogueIndexEntry is one entry an OPDS connection offers within the
// catalog sync scope. It mirrors upstream metadata and holds no learner state:
// the entry's ownership comes through ConnectionID. An empty CoverHref/MediaType
// means the entry advertises no cover.
type CatalogueIndexEntry struct {
	ConnectionID, EntryIdentifier, Title, Author, LanguageTag string
	CoverHref, CoverMediaType, EPUBHref                       string
	LastSeenSync                                              time.Time
}

// Normalized trims the whitespace-bearing metadata a catalog advertises so the
// index compares and stores one spelling of it.
func (e CatalogueIndexEntry) Normalized() CatalogueIndexEntry {
	e.EntryIdentifier = strings.TrimSpace(e.EntryIdentifier)
	e.Title = strings.TrimSpace(e.Title)
	e.Author = strings.TrimSpace(e.Author)
	e.LanguageTag = strings.TrimSpace(e.LanguageTag)
	e.CoverHref = strings.TrimSpace(e.CoverHref)
	e.CoverMediaType = strings.TrimSpace(e.CoverMediaType)
	e.EPUBHref = strings.TrimSpace(e.EPUBHref)
	return e
}

// CatalogueQuery narrows one connection's Catalog index for browsing. Search is
// compared through the Go search key, never a persistence lower(); Language
// uses the same normalized tag as Book languages. An empty field does not
// narrow, and a non-positive Limit returns the whole matching collection.
type CatalogueQuery struct {
	Search   string
	Language string
	Offset   int
	Limit    int
}

// CatalogueEntry is one browsable index entry. BookID is the learner's active
// Book linked to the entry, or empty when the learner does not own it.
type CatalogueEntry struct {
	Index  CatalogueIndexEntry
	BookID string
}

// CataloguePage is one source's matching entries and the total before paging.
type CataloguePage struct {
	Entries []CatalogueEntry
	Total   int
}

// BrowseCataloguePage filters one connection's index and pages the matches in
// title, then author, then identity order. Filtering happens here so every
// adapter reaches the same accent- and case-insensitive result; the index
// itself never stores a lowercased or folded search column.
func BrowseCataloguePage(entries []CatalogueIndexEntry, ownedByEntry map[string]string, query CatalogueQuery) CataloguePage {
	search := canonicalization.SearchKey(query.Search)
	language := canonicalization.NormalizeLanguage(query.Language)
	matched := make([]CatalogueIndexEntry, 0, len(entries))
	for _, entry := range entries {
		if language != "" && canonicalization.NormalizeLanguage(entry.LanguageTag) != language {
			continue
		}
		if search != "" && !CatalogueTextMatches(entry.Title, search) && !CatalogueTextMatches(entry.Author, search) {
			continue
		}
		matched = append(matched, entry)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if left, right := canonicalization.SearchKey(matched[i].Title), canonicalization.SearchKey(matched[j].Title); left != right {
			return left < right
		}
		if left, right := canonicalization.SearchKey(matched[i].Author), canonicalization.SearchKey(matched[j].Author); left != right {
			return left < right
		}
		return matched[i].EntryIdentifier < matched[j].EntryIdentifier
	})
	total := len(matched)
	offset := max(query.Offset, 0)
	if offset >= total {
		return CataloguePage{Total: total}
	}
	end := total
	if query.Limit > 0 {
		end = min(offset+query.Limit, total)
	}
	page := CataloguePage{Entries: make([]CatalogueEntry, 0, end-offset), Total: total}
	for _, entry := range matched[offset:end] {
		page.Entries = append(page.Entries, CatalogueEntry{Index: entry, BookID: ownedByEntry[entry.EntryIdentifier]})
	}
	return page
}

// CatalogueTextMatches compares one display field to an already folded search
// key, so "odyssee" finds "Odyssée" and "HOMER" finds "Hómēr". It is the one
// comparison both the OPDS index read and the Classical catalog search use.
func CatalogueTextMatches(value, searchKey string) bool {
	return searchKey != "" && strings.Contains(canonicalization.SearchKey(value), searchKey)
}
