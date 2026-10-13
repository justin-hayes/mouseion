// Command prototype_sentence_study compares throwaway Sentence study trees and a feature line.
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

//go:embed page.html prototype.css prototype.js samples.json feature_labels.json
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
	Feature    string `json:"-"`
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

type featureGroup struct {
	Source string
	Words  []token
}

type page struct {
	Sample                                      sample
	Samples                                     []sample
	Variant, VariantName                        string
	Selected                                    token
	Head                                        token
	Dependents                                  []token
	Edges                                       []edge
	FeatureGroups                               []featureGroup
	Width, Height, WordOrdinal, Budget          int
	Inspect, Text, HidePunctuation, ManualWidth bool
	Previous, Next, Dismiss, TextURL, ChartURL  string
}

var labels = map[string]string{
	"root": "Root", "nsubj": "Subject", "obj": "Object", "iobj": "Indirect object",
	"det": "Determiner", "case": "Preposition", "obl": "Oblique", "advmod": "Adverbial modifier",
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
	featureData, err := assets.ReadFile("feature_labels.json")
	if err != nil {
		log.Fatal(err)
	}
	var features map[string]map[int]string
	if err := json.Unmarshal(featureData, &features); err != nil {
		log.Fatal(err)
	}
	for i := range samples {
		for j := range samples[i].Tokens {
			samples[i].Tokens[j].Feature = features[samples[i].ID][j]
		}
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
	p := page{Samples: samples, Sample: samples[len(samples)-1], Variant: q.Get("variant"), HidePunctuation: q.Get("hide_punct") == "1", Text: q.Get("text") == "1", ManualWidth: q.Get("manual_width") == "1"}
	for _, old := range []string{"support", "context", "punct", "focus"} {
		q.Del(old)
	}
	for _, s := range samples {
		if s.ID == q.Get("example") {
			p.Sample = s
		}
	}
	p.Sample.Tokens = append([]token(nil), p.Sample.Tokens...)
	budget, err := strconv.Atoi(q.Get("width"))
	p.Budget = budget
	if err != nil {
		p.Budget = 1100
	}
	p.Budget = min(1800, max(240, p.Budget))
	q.Set("width", strconv.Itoa(p.Budget))
	variants := []string{"tree", "stagger", "gloss"}
	names := []string{"A · Bare compact tree", "B · Width-aware tree", "C · Tree with feature line"}
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
	q.Set("variant", variants[(vi+2)%3])
	p.Previous = "?" + q.Encode()
	q.Set("variant", variants[(vi+1)%3])
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
		t.Width = max(56, utf8.RuneCountInString(t.Surface)*10+16, utf8.RuneCountInString(t.Label)*7+16)
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
	for _, t := range p.Sample.Tokens {
		if t.Span != "" && len(p.FeatureGroups) > 0 && p.FeatureGroups[len(p.FeatureGroups)-1].Words[0].Span == t.Span {
			group := &p.FeatureGroups[len(p.FeatureGroups)-1]
			group.Words = append(group.Words, t)
		} else {
			source := t.Surface
			if t.Span != "" {
				source = t.Span
			}
			p.FeatureGroups = append(p.FeatureGroups, featureGroup{Source: source, Words: []token{t}})
		}
	}
	return p
}

//nolint:gocognit // Deliberately inline throwaway geometry; not a production layout abstraction.
func (p *page) treeLayout() {
	// Both experiments keep heads above dependents. Equal depth need not mean equal height.
	root := 0
	for i, t := range p.Sample.Tokens {
		if t.Head == i {
			root = i
		}
	}
	var buses map[int]int
	if p.Variant != "tree" {
		layout := p.packBounded(root, p.Budget-32)
		p.Width, p.Height = layout.width+32, layout.height+64
		buses = map[int]int{}
		for i, point := range layout.nodes {
			t := &p.Sample.Tokens[i]
			t.X, t.Y = point.x+16, point.y+48
			if point.bus >= 0 {
				buses[i] = point.bus + 16
			}
		}
	} else {
		layout := p.packSubtree(root)
		minBreadth, maxBreadth := 0, 0
		for i, b := range layout.left {
			minBreadth = min(minBreadth, b)
			maxBreadth = max(maxBreadth, layout.right[i])
		}
		p.Width, p.Height = maxBreadth-minBreadth+32, len(layout.left)*112+16
		for i, point := range layout.nodes {
			t := &p.Sample.Tokens[i]
			t.X, t.Y = point.breadth-minBreadth+16-t.Width/2, point.depth*112+48
		}
	}
	r := p.Sample.Tokens[root]
	p.Edges = append(p.Edges, edge{Path: fmt.Sprintf("M %d 8 L %d %d", r.X+r.Width/2, r.X+r.Width/2, r.Y+6), Label: "Root", X: r.X + r.Width/2, Y: r.Y - 18, Selected: r.Selected})
	for i, t := range p.Sample.Tokens {
		if t.Head == i || (p.HidePunctuation && t.Relation == "punct") {
			continue
		}
		h := p.Sample.Tokens[t.Head]
		sx, sy, dx, dy := h.X+h.Width/2, h.Y+38, t.X+t.Width/2, t.Y+6
		path := fmt.Sprintf("M %d %d C %d %d %d %d %d %d", sx, sy, sx, (sy+dy)/2, dx, (sy+dy)/2, dx, dy)
		if bus, ok := buses[t.Head]; ok {
			// A shared head's branches use its own empty gutter to reach staggered rows.
			path = fmt.Sprintf("M %d %d L %d %d L %d %d L %d %d L %d %d L %d %d", sx, sy, sx, sy+12, bus, sy+12, bus, t.Y-38, dx, t.Y-38, dx, dy)
		}
		p.Edges = append(p.Edges, edge{Path: path, Label: t.Label, X: dx, Y: t.Y - 18, Selected: t.Selected || h.Selected})
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

type packedPoint struct{ x, y, bus int }
type packedTile struct {
	nodes         map[int]packedPoint
	width, height int
}

//nolint:gocognit // Experimental width-budget packing and gutter routing, not production layout code.
func (p *page) packBounded(i, budget int) packedTile {
	var children []int
	for j, t := range p.Sample.Tokens {
		if j != i && t.Head == i && (!p.HidePunctuation || t.Relation != "punct") {
			children = append(children, j)
		}
	}
	childBudget, available := budget, budget
	if len(children) > 1 {
		available = budget - 24
		childBudget = max(160, available)
	}
	result := packedTile{nodes: map[int]packedPoint{}, width: p.Sample.Tokens[i].Width, height: 44}
	rowX, rowY, rowHeight, maxRowWidth, row := 0, 112, 0, 0, 0
	for _, child := range children {
		tile := p.packBounded(child, childBudget)
		if rowX > 0 && rowX+tile.width > available {
			rowY += rowHeight + 72
			rowX, rowHeight, row = 0, 0, row+1
		}
		for j, point := range tile.nodes {
			point.x += rowX
			point.y += rowY
			if point.bus >= 0 {
				point.bus += rowX
			}
			result.nodes[j] = point
		}
		rowX += tile.width + 24
		rowHeight = max(rowHeight, tile.height)
		maxRowWidth = max(maxRowWidth, rowX-24)
	}
	gutter, bus := 0, -1
	if row > 0 {
		gutter, bus = 24, 8
		for j, point := range result.nodes {
			point.x += gutter
			if point.bus >= 0 {
				point.bus += gutter
			}
			result.nodes[j] = point
		}
	}
	result.width = max(result.width, maxRowWidth+gutter)
	if len(children) > 0 {
		result.height = rowY + rowHeight
	}
	result.nodes[i] = packedPoint{x: result.width/2 - p.Sample.Tokens[i].Width/2, bus: bus}
	return result
}
