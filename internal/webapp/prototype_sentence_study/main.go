// Command prototype_sentence_study compares two throwaway Sentence study trees.
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
	Path     string
	Selected bool
}

type page struct {
	Sample                                     sample
	Samples                                    []sample
	Variant, VariantName                       string
	Selected                                   token
	Head                                       token
	Dependents                                 []token
	Edges                                      []edge
	Width, Height, WordOrdinal                 int
	Inspect, Text, HidePunctuation             bool
	Previous, Next, Dismiss, TextURL, ChartURL string
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
		http.Redirect(w, r, "/vocabulary/concordance/sentence?variant=tree&example=de-long", http.StatusSeeOther)
	})
	log.Print("THROWAWAY prototype: http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=tree&example=de-long")
	log.Fatal((&http.Server{Addr: "127.0.0.1:8106", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}

//nolint:gocognit // Keep the throwaway comparison's complete state setup in one place; not production code.
func makePage(q url.Values, samples []sample) page {
	p := page{Samples: samples, Sample: samples[len(samples)-1], Variant: q.Get("variant"), HidePunctuation: q.Get("hide_punct") == "1", Text: q.Get("text") == "1"}
	for _, old := range []string{"support", "context", "punct", "focus"} {
		q.Del(old)
	}
	for _, s := range samples {
		if s.ID == q.Get("example") {
			p.Sample = s
		}
	}
	p.Sample.Tokens = append([]token(nil), p.Sample.Tokens...)
	variants := []string{"tree", "side"}
	names := []string{"A · Compact top-down", "B · Side-growing"}
	vi := 0
	for i, v := range variants {
		if p.Variant == v {
			vi = i
		}
	}
	p.Variant, p.VariantName = variants[vi], names[vi]
	selected, err := strconv.Atoi(q.Get("word"))
	p.Inspect = err == nil && selected >= 0 && selected < len(p.Sample.Tokens)
	if err != nil || selected < 0 || selected >= len(p.Sample.Tokens) {
		selected = p.Sample.Target
	}
	p.WordOrdinal = selected
	q.Set("example", p.Sample.ID)
	q.Set("variant", variants[(vi+1)%2])
	p.Previous = "?" + q.Encode()
	p.Next = "?" + q.Encode()
	q.Set("variant", p.Variant)
	q.Del("word")
	p.Dismiss = "?" + q.Encode() + "#chart"
	q.Set("text", "1")
	p.TextURL = "?" + q.Encode() + "#chart"
	q.Del("text")
	p.ChartURL = "?" + q.Encode() + "#chart"
	if p.Text {
		q.Set("text", "1")
	}
	for i := range p.Sample.Tokens {
		t := &p.Sample.Tokens[i]
		t.Index, t.Selected = i+1, p.Inspect && i == selected
		t.Label = labels[t.Relation]
		if t.Label == "" {
			t.Label = t.Relation // Unknown labels stay evidence; do not invent an explanation.
		}
		t.Width = max(112, utf8.RuneCountInString(t.Surface)*11+24, utf8.RuneCountInString(t.Label)*7+20)
		q.Set("word", strconv.Itoa(i))
		t.URL = "?" + q.Encode() + "#inspector"
	}
	p.treeLayout()
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
	// Pack subtree contours rather than reserving a full column for each leaf.
	// Side-growing rotates the breadth axis, not the text or the source sentence.
	root := 0
	for i, t := range p.Sample.Tokens {
		if t.Head == i {
			root = i
		}
	}
	layout := p.packSubtree(root)
	minBreadth, maxBreadth := 0, 0
	levelWidths := make([]int, len(layout.left))
	for i, b := range layout.left {
		minBreadth = min(minBreadth, b)
		maxBreadth = max(maxBreadth, layout.right[i])
	}
	for i, point := range layout.nodes {
		levelWidths[point.depth] = max(levelWidths[point.depth], p.Sample.Tokens[i].Width)
	}
	columns := make([]int, len(levelWidths))
	x := 20
	for depth, width := range levelWidths {
		columns[depth] = x
		x += width + 40
	}
	p.Width, p.Height = maxBreadth-minBreadth+40, len(levelWidths)*144-40+40
	if p.Variant == "side" {
		p.Width, p.Height = x-40+20, maxBreadth-minBreadth+40
	}
	for i, point := range layout.nodes {
		t := &p.Sample.Tokens[i]
		t.X, t.Y = point.breadth-minBreadth+20-t.Width/2, point.depth*144+20
		if p.Variant == "side" {
			t.X, t.Y = columns[point.depth], point.breadth-minBreadth+20-52
		}
	}
	for i, t := range p.Sample.Tokens {
		if t.Head == i || (p.HidePunctuation && t.Relation == "punct") {
			continue
		}
		h := p.Sample.Tokens[t.Head]
		sx, sy, dx, dy := h.X+h.Width/2, h.Y+104, t.X+t.Width/2, t.Y
		cx1, cy1, cx2, cy2 := sx, (sy+dy)/2, dx, (sy+dy)/2
		if p.Variant == "side" {
			sx, sy, dx, dy = h.X+h.Width, h.Y+52, t.X, t.Y+52
			cx1, cy1, cx2, cy2 = (sx+dx)/2, sy, (sx+dx)/2, dy
		}
		p.Edges = append(p.Edges, edge{Path: fmt.Sprintf("M %d %d C %d %d %d %d %d %d", sx, sy, cx1, cy1, cx2, cy2, dx, dy), Selected: t.Selected || h.Selected})
	}
}

type position struct{ breadth, depth int }
type subtree struct {
	nodes       map[int]position
	left, right []int
}

//nolint:gocognit // Small throwaway contour-packing experiment, not a production tree-layout API.
func (p *page) packSubtree(i int) subtree {
	half := p.Sample.Tokens[i].Width / 2
	if p.Variant == "side" {
		half = 52
	}
	result := subtree{nodes: map[int]position{i: {}}, left: []int{-half}, right: []int{half}}
	var left, right []int
	first, last, count := 0, 0, 0
	for j, t := range p.Sample.Tokens {
		if j == i || t.Head != i || (p.HidePunctuation && t.Relation == "punct") {
			continue
		}
		child := p.packSubtree(j)
		shift := 0
		for depth := range min(len(right), len(child.left)) {
			shift = max(shift, right[depth]-child.left[depth]+24)
		}
		if count == 0 {
			first = shift
		}
		last, count = shift, count+1
		for k, point := range child.nodes {
			result.nodes[k] = position{point.breadth + shift, point.depth + 1}
		}
		for depth, l := range child.left {
			if depth >= len(left) {
				left = append(left, l+shift)
				right = append(right, child.right[depth]+shift)
			} else {
				left[depth] = min(left[depth], l+shift)
				right[depth] = max(right[depth], child.right[depth]+shift)
			}
		}
	}
	center := (first + last) / 2
	for k, point := range result.nodes {
		if k != i {
			result.nodes[k] = position{point.breadth - center, point.depth}
		}
	}
	for depth, l := range left {
		result.left = append(result.left, l-center)
		result.right = append(result.right, right[depth]-center)
	}
	return result
}
