// Command prototype_sentence_study compares three throwaway Sentence study views.
// It reuses the real Mouseion shell, but serves only invented, read-only examples.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/webapp"
)

//go:embed page.html prototype.css prototype.js samples.json
var assets embed.FS

type token struct {
	Surface    string `json:"Surface"`
	Lemma      string `json:"Lemma"`
	POS        string `json:"POS"`
	Relation   string `json:"Relation"`
	Morphology string `json:"Morphology"`
	Span       string `json:"Span"`
	Head       int    `json:"Head"` // Zero-based, root self-headed, as in Mouseion.
	Index      int    `json:"-"`
	X          int    `json:"-"`
	Y          int    `json:"-"`
	Width      int    `json:"-"`
	Label      string `json:"-"`
	URL        string `json:"-"`
	Selected   bool   `json:"-"`
}

type sample struct {
	ID       string  `json:"ID"`
	Name     string  `json:"Name"`
	Language string  `json:"Language"`
	Book     string  `json:"Book"`
	Location string  `json:"Location"`
	Sentence string  `json:"Sentence"`
	Before   string  `json:"Before"`
	After    string  `json:"After"`
	Note     string  `json:"Note"`
	Target   int     `json:"Target"`
	Tokens   []token `json:"Tokens"`
}

type edge struct {
	Path, Label string
	X, Y        int
	Selected    bool
}

type page struct {
	Sample                                       sample
	Samples                                      []sample
	Variant, VariantName                         string
	Selected                                     token
	Head                                         token
	Dependents                                   []token
	Edges                                        []edge
	Width, Height, WordOrdinal                   int
	ShowPunctuation, Support, OnlyFocus, Context bool
	Previous, Next                               string
}

var labels = map[string]string{
	"root": "Sentence root", "nsubj": "Subject", "obj": "Object", "iobj": "Indirect object",
	"det": "Article / determiner", "case": "Preposition", "obl": "Oblique", "advmod": "Adverbial modifier",
	"advcl": "Adverbial clause", "acl:relcl": "Relative clause", "mark": "Clause marker",
	"aux": "Auxiliary", "amod": "Adjective", "compound:prt": "Separable particle", "punct": "Punctuation",
}

func main() {
	data, err := assets.ReadFile("samples.json")
	if err != nil {
		log.Fatal(err)
	}
	var samples []sample
	if err := json.Unmarshal(data, &samples); err != nil {
		log.Fatal(err)
	}
	view := template.Must(template.ParseFS(assets, "page.html"))
	mux := http.NewServeMux()
	mux.Handle("GET /static/", webapp.StaticHandler())
	mux.Handle("GET /prototype-assets/", http.StripPrefix("/prototype-assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /vocabulary/concordance/sentence", func(w http.ResponseWriter, r *http.Request) {
		p := makePage(r.URL.Query(), samples)
		child := templ.ComponentFunc(func(_ context.Context, writer io.Writer) error {
			return view.Execute(writer, p)
		})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		user := domain.User{Username: "Prototype learner"}
		if err := webapp.ShellLayout("Sentence study prototype", &user, "", webapp.NavigationVocabulary).
			Render(templ.WithChildren(r.Context(), child), w); err != nil {
			log.Printf("render prototype: %v", err)
		}
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/vocabulary/concordance/sentence?variant=tree", http.StatusSeeOther)
	})
	log.Print("THROWAWAY prototype: http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=tree")
	log.Fatal((&http.Server{Addr: "127.0.0.1:8106", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}

//nolint:gocognit // Keep the throwaway comparison's complete state setup in one place; not production code.
func makePage(q url.Values, samples []sample) page {
	p := page{Samples: samples, Sample: samples[0], Variant: q.Get("variant"), ShowPunctuation: q.Get("punct") == "1", Support: q.Get("support") == "1", OnlyFocus: q.Get("focus") == "1", Context: q.Get("context") == "1"}
	for _, s := range samples {
		if s.ID == q.Get("example") {
			p.Sample = s
		}
	}
	p.Sample.Tokens = append([]token(nil), p.Sample.Tokens...)
	variants := []string{"tree", "arcs", "focus"}
	names := []string{"A · Branching tree", "B · Word-order arcs", "C · Selected-word relationships"}
	vi := 0
	for i, v := range variants {
		if p.Variant == v {
			vi = i
		}
	}
	p.Variant, p.VariantName = variants[vi], names[vi]
	selected, err := strconv.Atoi(q.Get("word"))
	if err != nil || selected < 0 || selected >= len(p.Sample.Tokens) {
		selected = p.Sample.Target
	}
	p.WordOrdinal = selected
	q.Set("example", p.Sample.ID)
	q.Set("word", strconv.Itoa(selected))
	q.Set("variant", variants[(vi+2)%3])
	p.Previous = "?" + q.Encode()
	q.Set("variant", variants[(vi+1)%3])
	p.Next = "?" + q.Encode()
	q.Set("variant", p.Variant)
	x := 20
	for i := range p.Sample.Tokens {
		t := &p.Sample.Tokens[i]
		t.Index, t.Selected = i+1, i == selected
		t.Label = labels[t.Relation]
		if t.Label == "" {
			t.Label = t.Relation // Unknown labels stay evidence; do not invent an explanation.
		}
		t.Width = max(116, utf8.RuneCountInString(t.Surface)*12+32)
		t.X, t.Y = x+t.Width/2, 0
		x += t.Width + 20
		q.Set("word", strconv.Itoa(i))
		q.Set("support", "1")
		t.URL = "?" + q.Encode() + "#relationships"
	}
	p.Width = x
	if p.Variant == "tree" {
		p.treeLayout()
	} else {
		p.arcLayout()
	}
	p.Selected = p.Sample.Tokens[selected]
	p.Head = p.Sample.Tokens[p.Selected.Head]
	for i, t := range p.Sample.Tokens {
		if t.Head == selected && i != selected {
			p.Dependents = append(p.Dependents, t)
		}
	}
	return p
}

//nolint:gocognit // Deliberately inline throwaway geometry; not a production layout abstraction.
func (p *page) treeLayout() {
	// Deliberately simple ordered-leaf layout, not a proposed production algorithm.
	x, maxDepth := 20, 0
	var place func(int, int)
	place = func(i, depth int) {
		t := &p.Sample.Tokens[i]
		t.Y = 42 + depth*128
		maxDepth = max(maxDepth, depth)
		first, last := 0, 0
		for j, c := range p.Sample.Tokens {
			if j != i && c.Head == i && (p.ShowPunctuation || c.Relation != "punct") {
				place(j, depth+1)
				if first == 0 {
					first = p.Sample.Tokens[j].X
				}
				last = p.Sample.Tokens[j].X
			}
		}
		if first == 0 {
			t.X = x + t.Width/2
			x += t.Width + 44
		} else {
			t.X = (first + last) / 2
		}
	}
	for i, t := range p.Sample.Tokens {
		if t.Head == i {
			place(i, 0)
		}
	}
	p.Width, p.Height = x+20, (maxDepth+1)*128+40
	for i, t := range p.Sample.Tokens {
		if t.Head == i || (!p.ShowPunctuation && t.Relation == "punct") {
			continue
		}
		h := p.Sample.Tokens[t.Head]
		p.Edges = append(p.Edges, edge{
			Path:  fmt.Sprintf("M %d %d C %d %d %d %d %d %d", h.X, h.Y+28, h.X, h.Y+80, t.X, t.Y-80, t.X, t.Y-30),
			Label: t.Label, X: t.X, Y: t.Y - 48, Selected: t.Selected || h.Selected,
		})
	}
}

//nolint:gocognit // Deliberately inline throwaway geometry; not a production layout abstraction.
func (p *page) arcLayout() {
	// Greedy non-overlapping lanes make crossings and length costs visible.
	type interval struct{ lo, hi int }
	var lanes [][]interval
	for i, t := range p.Sample.Tokens {
		if t.Head == i || (!p.ShowPunctuation && t.Relation == "punct") {
			continue
		}
		h := p.Sample.Tokens[t.Head]
		lo, hi := min(t.X, h.X), max(t.X, h.X)
		lane := 0
		for ; lane < len(lanes); lane++ {
			free := true
			for _, existing := range lanes[lane] {
				if lo <= existing.hi+60 && hi >= existing.lo-60 {
					free = false
				}
			}
			if free {
				break
			}
		}
		if lane == len(lanes) {
			lanes = append(lanes, nil)
		}
		lanes[lane] = append(lanes[lane], interval{lo, hi})
		p.Edges = append(p.Edges, edge{Label: t.Label, X: (lo + hi) / 2, Y: lane, Selected: t.Selected || h.Selected,
			Path: fmt.Sprintf("%d,%d", h.X, t.X)})
	}
	base := len(lanes)*44 + 70
	p.Height = base + 105
	for i := range p.Sample.Tokens {
		p.Sample.Tokens[i].Y = base + 35
	}
	for i := range p.Edges {
		e := &p.Edges[i]
		var hx, tx int
		if _, err := fmt.Fscanf(strings.NewReader(e.Path), "%d,%d", &hx, &tx); err != nil {
			panic(err)
		}
		top := base - (e.Y+1)*44
		e.Path = fmt.Sprintf("M %d %d L %d %d Q %d %d %d %d L %d %d Q %d %d %d %d L %d %d", hx, base, hx, top+12, hx, top, (hx+tx)/2, top, tx-12*sign(tx-hx), top, tx, top, tx, top+12, tx, base)
		e.Y = top - 7
	}
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}
