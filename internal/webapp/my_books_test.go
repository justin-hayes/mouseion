package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestMyBooksMetadataOnlyRowExposesOnlySupportedActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "A book without an EPUB", LanguageState: domain.LanguageUnknown}, EvidenceState: domain.MyBookNotAcquired}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", "", false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"A book without an EPUB", "Not acquired", "Acquire this book", "/connections?book_id=metadata-book", "Remove from My Books", `action="/library/books/metadata-book/remove"`, `name="language_state"`, "language not chosen"} {
		if !strings.Contains(html, want) {
			t.Errorf("metadata-only My Books row missing %q: %s", want, html)
		}
	}
	row := html
	if start := strings.Index(row, `aria-labelledby="book-title-metadata-book"`); start >= 0 {
		row = row[start:]
		if end := strings.Index(row, "</article>"); end >= 0 {
			row = row[:end+len("</article>")]
		}
	}
	for _, forbidden := range []string{"Review scope", "Start analysis", "Prepare deck", "View analysis result", "coverage"} {
		if strings.Contains(row, forbidden) {
			t.Errorf("metadata-only My Books row exposed unsupported action %q: %s", forbidden, row)
		}
	}
}

func TestMyBooksEvidenceStatesRemainDistinct(t *testing.T) {
	states := []domain.MyBookEvidenceState{
		domain.MyBookUnavailable,
		domain.MyBookNotAcquired,
		domain.MyBookAcquiredUnassessed,
		domain.MyBookAnalyzed,
		domain.MyBookStale,
	}
	books := make([]domain.MyBook, 0, len(states))
	for i, state := range states {
		book := domain.MyBook{Book: domain.Book{ID: "book-" + string(rune('a'+i)), OwnerID: "owner", Title: "Book " + string(rune('A'+i)), LanguageState: domain.LanguageChosen, LanguageTag: "de"}, EvidenceState: state}
		if state != domain.MyBookNotAcquired {
			book.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: book.Book.ID, OwnerID: "owner", Title: book.Book.Title, Language: "de", MediaType: "application/epub+zip", ContentRevisionID: "revision"}, AnalysisStatus: "not analyzed"}
		}
		books = append(books, book)
	}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "", false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, label := range []string{"Unavailable", "Not acquired", "Acquired — unassessed", "Analyzed", "Stale analysis"} {
		if strings.Count(html, label) != 1 {
			t.Errorf("evidence label %q count=%d", label, strings.Count(html, label))
		}
	}
}

func TestMyBooksEmptyOnboardingDependsOnCatalogueConnections(t *testing.T) {
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"learner-owned catalogue connection", "ready-language bibliographic metadata", `href="/connections"`, "Add catalogue connection"} {
		if !strings.Contains(html, want) {
			t.Errorf("empty My Books onboarding missing %q: %s", want, html)
		}
	}

	output.Reset()
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", "", true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No books in My Books yet") || !strings.Contains(output.String(), "Acquire an EPUB") {
		t.Fatalf("existing connected empty state changed: %s", output.String())
	}
}

func TestMetadataOnlyBookPageDoesNotExposeContentActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "Catalogue metadata", LanguageState: domain.LanguageUnknown}, EvidenceState: domain.MyBookNotAcquired}
	var output bytes.Buffer
	if err := MetadataOnlyBookPage(domain.User{Username: "learner"}, "csrf", book, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"Catalogue metadata", "Metadata only", "Content not acquired", "Analysis, scope review, and deck preparation are unavailable"} {
		if !strings.Contains(html, want) {
			t.Errorf("metadata-only page missing %q: %s", want, html)
		}
	}
	for _, forbidden := range []string{"Review scope", "Start analysis", "Prepare deck", `action=\"/books/metadata-book/analyze\"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("metadata-only page exposed unsupported action %q: %s", forbidden, html)
		}
	}
}

func TestBookIDFromReturnPathRejectsExternalTargets(t *testing.T) {
	if got := bookIDFromReturnPath("/opds/language?book_id=book-1&language=de"); got != "book-1" {
		t.Fatalf("book id=%q", got)
	}
	if got := bookIDFromReturnPath("https://evil.example/?book_id=other"); got != "" {
		t.Fatalf("external return path yielded book id=%q", got)
	}
}
