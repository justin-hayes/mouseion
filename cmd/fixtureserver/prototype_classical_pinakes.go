package main

// PROTOTYPE — throwaway design exploration, not production code.
//
// Three variants of the Classical catalog index, switchable with ?variant= on
// the existing /catalogs/classical route of the fixture server:
//
//	go run ./cmd/fixtureserver   # then sign in (fixture-learner / fixture-password)
//	http://127.0.0.1:8099/catalogs/classical?variant=A
//
// Without ?variant the shipped page is served untouched, so browser smoke tests
// are unaffected. The variants render the real release manifest
// (internal/classical/manifest.jsonl) inside the real shell. Ownership, the
// opening sentences and the Add controls are stubbed here.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/classical"
)

//go:embed prototype_classical_pinakes.html
var prototypePinakesHTML string

//go:embed prototype_literata-greek-ext-opsz-normal.woff2
var prototypeGreekExtFont []byte

type protoUnit struct {
	Book       int
	Start, End string
	Words      int
	Owned      bool
	Pct        float64
}

type protoWork struct {
	Key, Title, Greek, Author, Edition string
	Opening, OpeningRef                string
	Whole                              bool
	Words, OwnedCount                  int
	Units                              []protoUnit
	Pct                                float64
}

type protoAuthor struct {
	Name  string
	Works []*protoWork
}

type protoPage struct {
	Variant               string
	Authors               []*protoAuthor
	Works, Units, MaxWork int
	Ticks                 []protoTick
}

type protoTick struct {
	Label string
	Pct   float64
}

// Hand-entered for the prototype. Production would take the first sentence of
// each pinned source when the manifest is generated.
var protoOpenings = map[string]string{
	"plato-apology":     "Ὅτι μὲν ὑμεῖς, ὦ ἄνδρες Ἀθηναῖοι, πεπόνθατε ὑπὸ τῶν ἐμῶν κατηγόρων, οὐκ οἶδα·",
	"plato-crito":       "Τί τηνικάδε ἀφῖξαι, ὦ Κρίτων; ἢ οὐ πρῲ ἔτι ἐστίν;",
	"plato-meno":        "Ἔχεις μοι εἰπεῖν, ὦ Σώκρατες, ἆρα διδακτὸν ἡ ἀρετή;",
	"plato-phaedo":      "Αὐτός, ὦ Φαίδων, παρεγένου Σωκράτει ἐκείνῃ τῇ ἡμέρᾳ ᾗ τὸ φάρμακον ἔπιεν ἐν τῷ δεσμωτηρίῳ, ἢ ἄλλου του ἤκουσας;",
	"plato-republic":    "Κατέβην χθὲς εἰς Πειραιᾶ μετὰ Γλαύκωνος τοῦ Ἀρίστωνος προσευξόμενός τε τῇ θεῷ…",
	"plato-symposium":   "Δοκῶ μοι περὶ ὧν πυνθάνεσθε οὐκ ἀμελέτητος εἶναι.",
	"xenophon-anabasis": "Δαρείου καὶ Παρυσάτιδος γίγνονται παῖδες δύο, πρεσβύτερος μὲν Ἀρταξέρξης, νεώτερος δὲ Κῦρος·",
}

// Stub ownership so the variants show a partly owned work and an owned one.
var protoOwned = map[string]bool{
	"plato-crito": true, "xenophon-anabasis-1": true, "xenophon-anabasis-2": true, "xenophon-anabasis-3": true,
}

func buildProtoPage(variant string) (protoPage, error) {
	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	if err != nil {
		return protoPage{}, err
	}
	works := map[string]*protoWork{}
	authors := map[string]*protoAuthor{}
	page := protoPage{Variant: variant}
	for _, d := range definitions {
		if d.Withdrawn {
			continue
		}
		w := works[d.WorkKey]
		if w == nil {
			w = &protoWork{Key: d.WorkKey, Title: d.WorkTitle, Greek: d.GreekTitle, Author: d.WorkAuthor, Edition: d.EditionLabel, Opening: protoOpenings[d.WorkKey]}
			works[d.WorkKey] = w
			a := authors[d.WorkAuthor]
			if a == nil {
				a = &protoAuthor{Name: d.WorkAuthor}
				authors[d.WorkAuthor] = a
				page.Authors = append(page.Authors, a)
			}
			a.Works = append(a.Works, w)
			page.Works++
		}
		u := protoUnit{Start: d.CitationStart, End: d.CitationEnd, Words: d.WordCount, Owned: protoOwned[d.Key]}
		if d.ConstituentBook != nil {
			u.Book = *d.ConstituentBook
		} else {
			w.Whole = true
		}
		if u.Owned {
			w.OwnedCount++
		}
		w.Units = append(w.Units, u)
		w.Words += d.WordCount
		page.Units++
	}
	sort.Slice(page.Authors, func(i, j int) bool { return page.Authors[i].Name < page.Authors[j].Name })
	for _, a := range page.Authors {
		sort.Slice(a.Works, func(i, j int) bool { return a.Works[i].Title < a.Works[j].Title })
		for _, w := range a.Works {
			sort.Slice(w.Units, func(i, j int) bool { return w.Units[i].Book < w.Units[j].Book })
			w.OpeningRef = w.Units[0].Start
			if w.Words > page.MaxWork {
				page.MaxWork = w.Words
			}
		}
	}
	// One scale for the whole page: the axis ends at the next 10,000 words.
	axis := (page.MaxWork/10000 + 1) * 10000
	for _, a := range page.Authors {
		for _, w := range a.Works {
			w.Pct = 100 * float64(w.Words) / float64(axis)
			for i := range w.Units {
				w.Units[i].Pct = 100 * float64(w.Units[i].Words) / float64(axis)
			}
		}
	}
	for t := 0; t <= axis; t += 20000 {
		page.Ticks = append(page.Ticks, protoTick{Label: protoThousands(t), Pct: 100 * float64(t) / float64(axis)})
	}
	return page, nil
}

func protoThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

var protoTemplate = template.Must(template.New("pinakes").Funcs(template.FuncMap{
	"n": protoThousands,
	"passage": func(w *protoWork) string {
		first, last := w.Units[0], w.Units[len(w.Units)-1]
		if first.Start == last.End {
			return first.Start
		}
		return first.Start + "–" + last.End
	},
	"css": func(format string, v float64) template.CSS { return template.CSS(fmt.Sprintf(format, v)) },
}).Parse(prototypePinakesHTML))

// prototypeClassical serves the shipped page unless ?variant is present. With
// it, the shipped response supplies the shell and the variant replaces <main>.
func prototypeClassical(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		variant := r.URL.Query().Get("variant")
		if variant == "" {
			next.ServeHTTP(w, r)
			return
		}
		inner := r.Clone(r.Context())
		q := inner.URL.Query()
		q.Del("variant")
		inner.URL.RawQuery = q.Encode()
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, inner)
		body := rec.Body.String()
		mainOpen := strings.Index(body, "<main")
		mainClose := strings.LastIndex(body, "</main>")
		if rec.Code != http.StatusOK || mainOpen < 0 || mainClose < 0 {
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		mainStart := mainOpen + strings.Index(body[mainOpen:], ">") + 1
		page, err := buildProtoPage(variant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var content, switcher bytes.Buffer
		if variant != "current" {
			if err := protoTemplate.ExecuteTemplate(&content, "variant", page); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			content.WriteString(body[mainStart:mainClose])
		}
		if err := protoTemplate.ExecuteTemplate(&switcher, "switcher", page); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body[:mainStart], content.String(), "</main>", switcher.String(), "</body></html>")
	})
}

func prototypeGreekExtFontHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "font/woff2")
	_, _ = w.Write(prototypeGreekExtFont)
}
