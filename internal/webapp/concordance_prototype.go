package webapp

// THROWAWAY: layout evidence for "Explore a simpler Concordance layout with
// compact book groups". Enabled only in cmd/fixtureserver by
// `make prototype-concordance`. Three source arrangements share the real shell
// and stylesheet at /vocabulary/concordance?variant=margin|inline|original.
// Sentences, interpretation, states, paging, and Study are synthetic, read-only
// demonstrations, not production search or source/analyzer evidence.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type concordancePrototypeRow struct {
	ID, Book, Author, Before, Target, After string
	First, Continued, Priority              bool
	Count                                   int
}

func (row concordancePrototypeRow) countText() string {
	if row.Count == 1 {
		return "1 occurrence on this page"
	}
	return fmt.Sprintf("%d occurrences on this page", row.Count)
}

type concordancePrototypeView struct {
	Variant, Scenario, Term, Applied, Notice, Action, Study, Focus string
	Rows                                                           []concordancePrototypeRow
	Page, Start, End                                               int
	Previous, Next                                                 bool
}

const prototypePriorityTitle = "Die Leiden des jungen Werthers — Briefe und ergänzende Aufzeichnungen aus einer kommentierten Ausgabe"

// ConcordancePrototypeHandler is deliberately wired only by the fixture server.
func ConcordancePrototypeHandler(w http.ResponseWriter, r *http.Request) {
	view := concordancePrototypeData(r.URL.Query())
	ctx := context.WithValue(r.Context(), shellViewContextKey{}, &shellView{
		ActiveLanguage: "de", Options: []activeStudyLanguageOption{{StudyLanguage: domain.StudyLanguage{Language: "de", DisplayName: "Deutsch"}, HasBooks: true}},
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ConcordancePrototypePage(view).Render(ctx, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func concordancePrototypeData(q url.Values) concordancePrototypeView {
	v := concordancePrototypeView{Variant: q.Get("variant"), Scenario: q.Get("case"), Term: q.Get("term"), Study: q.Get("study"), Focus: q.Get("focus"), Page: 1}
	if v.Variant != "inline" && v.Variant != "original" {
		v.Variant = "margin"
	}
	if v.Scenario == "" {
		v.Scenario = "mixed"
	}
	if !q.Has("term") {
		v.Term = "gehen"
	}
	v.Term = strings.TrimSpace(v.Term)
	if page, err := strconv.Atoi(q.Get("page")); err == nil && page > 0 {
		v.Page = page
	}
	v.Applied = "Forms of " + v.Term
	if strings.EqualFold(v.Term, "ging") {
		v.Applied = "Word form " + v.Term
	}
	if v.Scenario == "browse" {
		v.Applied += " · verb"
	}
	v.Previous = v.Page > 1
	if v.Term == "" {
		v.Notice = "Enter a lemma or word form to find examples in your currently analyzed books."
		return v
	}
	if len(strings.Fields(v.Term)) > 1 {
		v.Notice = "Enter one lemma or word form. Phrase search is not supported."
		return v
	}
	lookupTerm := v.Term
	switch v.Scenario {
	case "empty":
		v.Notice = "No matching occurrences in your currently analyzed books."
		return v
	case "error":
		v.Notice, v.Action = "Could not search for this term. No new results were applied.", "Retry"
		return v
	case "revision":
		v.Notice, v.Action = "Current evidence changed. Refresh results to search the current analysis and restart at page 1.", "Refresh results"
		return v
	case "language":
		v.Notice, v.Action = "This lookup belongs to Italiano, but your active study language is Deutsch. Choose deliberately which language to search.", "Start a fresh lookup in Deutsch"
		return v
	case "legacy":
		v.Notice, v.Action = "Book filtering has been retired. These results have not been silently broadened.", "Search all currently analyzed books"
		return v
	case "loading":
		v.Notice = "Searching for Haus… Previous results remain below under their applied query: Forms of gehen."
		v.Applied = "Forms of gehen"
		v.Term, lookupTerm = "Haus", "gehen"
	case "changed":
		v.Notice, v.Action = "Current reading changed to Der Prozess. These results retain the previously applied book order.", "Refresh results"
	case "no-current-matches":
		v.Notice = "No matching occurrences in your current reading. Other currently analyzed books are shown below."
	case "stale":
		v.Notice = "Current-reading analysis is out of date. Results include the other currently analyzed books, not the frozen reading snapshot."
	case "no-current":
		v.Notice = "No current reading. Books are in title order."
	}
	books := []struct {
		title, author string
		hits          int
	}{
		{prototypePriorityTitle, "Johann Wolfgang von Goethe", 6},
		{"Aus dem Leben eines Taugenichts — mit einem Anhang über Reisen, Begegnungen und die Kunst des Müßiggangs", "Joseph von Eichendorff", 1},
		{"Buddenbrooks: Verfall einer Familie", "Thomas Mann", 5},
		{"Der Prozess", "Franz Kafka", 1},
		{"Die Aufzeichnungen des Malte Laurids Brigge — mit Materialien und einem ausführlichen Nachwort zur Ausgabe", "Rainer Maria Rilke", 1},
		{"Effi Briest", "Theodor Fontane", 6},
		{"Über die allmähliche Verfertigung der Gedanken beim Reden", "Heinrich von Kleist", 7},
	}
	if v.Scenario == "paging" {
		books[0].hits = 60
	}
	if v.Scenario == "no-current" {
		slices.SortStableFunc(books, func(a, b struct {
			title, author string
			hits          int
		}) int {
			return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
		})
	}
	var all []concordancePrototypeRow
	for bookIndex, book := range books {
		if bookIndex == 0 && (v.Scenario == "no-current-matches" || v.Scenario == "stale") {
			continue
		}
		for hit := range book.hits {
			target := []string{"geht", "ging", "gegangen", "gehen", "Ging", "gehst"}[hit%6]
			if strings.EqualFold(lookupTerm, "ging") {
				target = "ging"
			} else if strings.EqualFold(lookupTerm, "aufstehen") {
				target = "stehe"
			} else if lookupTerm != "gehen" {
				target = lookupTerm
			}
			before := []string{"Am frühen Morgen ", "Nachdem der Regen endlich aufgehört hatte, ", "Er erinnerte sich daran, wie sie gemeinsam ", "Vor dem alten Haus, in dessen Fenstern noch das Licht der vergangenen Nacht brannte, "}[hit%4]
			after := []string{" er langsam durch den Garten.", " sie allein bis zum Fluss, wo die anderen bereits warteten.", " waren, ohne noch einmal zurückzublicken.", " er weiter, während hinter ihm die Stimmen leiser wurden und der Weg zwischen den Bäumen verschwand."}[hit%4]
			if hit%6 == 3 {
				after = " die Kinder weiter, während hinter ihnen die Stimmen leiser wurden und der Weg zwischen den Bäumen verschwand."
			} else if hit%6 == 4 {
				before, after = "»", " er wirklich allein bis zum Fluss?«, fragte sie."
			} else if hit%6 == 5 {
				after = " du allein bis zum Fluss, wo die anderen bereits warten."
			}
			if strings.EqualFold(lookupTerm, "aufstehen") {
				before, after = "Ich ", " jeden Morgen früh auf, bevor die Stadt erwacht."
			}
			if v.Scenario == "long-target" {
				target = "Donaudampfschifffahrtsgesellschaftskapitän"
			}
			all = append(all, concordancePrototypeRow{ID: fmt.Sprintf("example-%d-%d", bookIndex, hit), Book: book.title, Author: book.author, Before: before, Target: target, After: after, Priority: bookIndex == 0 && v.Scenario != "no-current"})
		}
	}
	start := (v.Page - 1) * 25
	if start >= len(all) {
		v.Notice, v.Action = "No results on this page. This does not mean the lookup has no matches.", "Return to page 1"
		return v
	}
	end := min(start+25, len(all))
	v.Start, v.End, v.Next = start+1, end, end < len(all)
	v.Rows = all[start:end]
	for i := range v.Rows {
		row := &v.Rows[i]
		row.First = i == 0 || v.Rows[i-1].Book != row.Book
		row.Continued = i == 0 && start > 0 && all[start-1].Book == row.Book
		for _, other := range v.Rows {
			if other.Book == row.Book {
				row.Count++
			}
		}
	}
	return v
}

func (v concordancePrototypeView) link(variant, scenario string, page int, study string) string {
	q := url.Values{"variant": {variant}, "case": {scenario}, "page": {strconv.Itoa(page)}, "term": {v.Term}}
	if study != "" {
		q.Set("study", study)
	}
	return "/vocabulary/concordance?" + q.Encode()
}

func (v concordancePrototypeView) variantStep(delta int) string {
	variants := []string{"margin", "inline", "original"}
	for i, variant := range variants {
		if variant == v.Variant {
			return v.link(variants[(i+delta+len(variants))%len(variants)], v.Scenario, v.Page, "")
		}
	}
	return ""
}
