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
	Feature    string `json:"-"`
	Style      string `json:"-"`
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
}

type featureGroup struct {
	Source string
	Words  []token
}

type page struct {
	Sample                             sample
	Samples                            []sample
	Variant, VariantName               string
	Edges                              []edge
	FeatureGroups                      []featureGroup
	Width, Height, Budget              int
	Text, HidePunctuation, ManualWidth bool
	Previous, Next, TextURL, ChartURL  string
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
		http.Redirect(w, r, "/vocabulary/concordance/sentence?variant=curves&example=de-long", http.StatusSeeOther)
	})
	log.Print("THROWAWAY prototype: http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=curves&example=de-long")
	log.Fatal((&http.Server{Addr: "127.0.0.1:8106", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}

//nolint:gocognit // Keep the throwaway comparison's complete state setup in one place; not production code.
func makePage(q url.Values, samples []sample) page {
	p := page{Samples: samples, Sample: samples[len(samples)-1], Variant: q.Get("variant"), HidePunctuation: q.Get("hide_punct") == "1", Text: q.Get("text") == "1", ManualWidth: q.Get("manual_width") == "1"}
	for _, old := range []string{"support", "context", "punct", "focus", "word"} {
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
	variants := []string{"curves", "roles"}
	names := []string{"A · Curved edge labels", "B · Role waypoints"}
	vi := 0
	for i, v := range variants {
		if p.Variant == v {
			vi = i
		}
	}
	p.Variant, p.VariantName = variants[vi], names[vi]
	q.Set("example", p.Sample.ID)
	q.Set("variant", variants[(vi+1)%2])
	p.Previous = "?" + q.Encode()
	p.Next = "?" + q.Encode()
	q.Set("variant", p.Variant)
	q.Set("text", "1")
	p.TextURL = "?" + q.Encode() + "#chart"
	q.Del("text")
	p.ChartURL = "?" + q.Encode() + "#chart"
	if p.Text {
		q.Set("text", "1")
	}
	for i := range p.Sample.Tokens {
		t := &p.Sample.Tokens[i]
		t.Index = i + 1
		t.Label = labels[t.Relation]
		if t.Label == "" {
			t.Label = t.Relation // Unknown labels stay evidence; do not invent an explanation.
		}
		t.Width = max(56, utf8.RuneCountInString(t.Surface)*10+16, utf8.RuneCountInString(t.Label)*7+16)
		t.Style = "word-function"
		switch t.POS {
		case "VERB", "AUX":
			t.Style = "word-verb"
		case "NOUN", "PROPN":
			t.Style = "word-noun"
		case "ADJ":
			t.Style = "word-adjective"
		case "ADV":
			t.Style = "word-adverb"
		case "PRON":
			t.Style = "word-pronoun"
		}
	}
	p.treeLayout()
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
	layout := p.packBounded(root, p.Budget-32)
	if p.Variant == "roles" {
		waypoints := p.packWaypoints(root, p.Budget-32)
		layout = packedTile{nodes: waypoints.nodes, width: waypoints.width, height: waypoints.height}
	}
	p.Width, p.Height = layout.width+32, layout.height+96
	buses := map[int]int{}
	for i, point := range layout.nodes {
		t := &p.Sample.Tokens[i]
		t.X, t.Y = point.x+16, point.y+72
		if point.bus >= 0 {
			buses[i] = point.bus + 16
		}
	}
	r := p.Sample.Tokens[root]
	rx := r.X + r.Width/2
	rootY := r.Y - 18
	if p.Variant == "roles" {
		rootY = r.Y - 40
	}
	p.Edges = append(p.Edges, edge{Path: fmt.Sprintf("M %d %d C %d %d %d %d %d %d", rx, rootY+8, rx-4, rootY+16, rx+4, r.Y-4, rx, r.Y+6), Label: "Root", X: rx, Y: rootY})
	for i, t := range p.Sample.Tokens {
		if t.Head == i || (p.HidePunctuation && t.Relation == "punct") {
			continue
		}
		h := p.Sample.Tokens[t.Head]
		sx, sy, dx, dy := h.X+h.Width/2, h.Y+38, t.X+t.Width/2, t.Y+6
		labelY, destinationY := t.Y-18, dy
		if p.Variant == "roles" {
			labelY, destinationY = t.Y-40, t.Y-56
		}
		path := fmt.Sprintf("M %d %d C %d %d %d %d %d %d", sx, sy, sx+4, (sy+destinationY)/2, dx-4, (sy+destinationY)/2, dx, destinationY)
		pitch := 112
		if p.Variant == "roles" {
			pitch = 144
		}
		if bus, ok := buses[t.Head]; ok && t.Y-h.Y > pitch {
			// Only later rows need the gutter; first-row branches fan directly downward.
			// The long route bows within the gutter instead of sharing an angular bus.
			turnY, endY := sy+48, destinationY-32
			path = fmt.Sprintf("M %d %d C %d %d %d %d %d %d C %d %d %d %d %d %d C %d %d %d %d %d %d", sx, sy, sx, sy+24, bus, turnY-24, bus, turnY, bus-6, turnY+(endY-turnY)/3, bus-6, endY-(endY-turnY)/3, bus, endY, bus, endY+16, dx, destinationY-16, dx, destinationY)
		}
		if p.Variant == "roles" {
			path += fmt.Sprintf(" M %d %d C %d %d %d %d %d %d", dx, labelY+8, dx-4, labelY+24, dx+4, t.Y-8, dx, dy)
		}
		p.Edges = append(p.Edges, edge{Path: path, Label: t.Label, X: dx, Y: labelY})
	}
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

type waypointTile struct {
	nodes         map[int]packedPoint
	width, height int
	left, right   []int
}

//nolint:gocognit // Throwaway contour placement with vertically independent role/word pairs.
func (p *page) packWaypoints(i, budget int) waypointTile {
	result := waypointTile{nodes: map[int]packedPoint{}, left: []int{0}, right: []int{0}}
	available := max(160, budget-24)
	firstRow, lastRow, count := 0, 0, 0
	for child, t := range p.Sample.Tokens {
		if child == i || t.Head != i || (p.HidePunctuation && t.Relation == "punct") {
			continue
		}
		tile := p.packWaypoints(child, available)
		// Retry on lower rows rather than reserving the full rectangle of a tall sibling.
		row := max(1, lastRow)
		var shift int
		for {
			shift = 0
			for depth, l := range tile.left {
				if row+depth < len(result.right) {
					shift = max(shift, result.right[row+depth]-l+24)
				}
			}
			if shift == 0 || shift+tile.width <= available {
				break
			}
			row++
		}
		if count == 0 {
			firstRow = row
		}
		lastRow, count = row, count+1
		for j, point := range tile.nodes {
			point.x += shift
			point.y += row * 144
			if point.bus >= 0 {
				point.bus += shift
			}
			result.nodes[j] = point
		}
		for depth, l := range tile.left {
			index := row + depth
			for index >= len(result.left) {
				result.left = append(result.left, 0)
				result.right = append(result.right, 0)
			}
			if result.right[index] == 0 {
				result.left[index] = l + shift
			} else {
				result.left[index] = min(result.left[index], l+shift)
			}
			result.right[index] = max(result.right[index], tile.right[depth]+shift)
			result.width = max(result.width, result.right[index])
		}
	}
	gutter, bus := 0, -1
	if firstRow != lastRow {
		gutter, bus = 24, 8
		for j, point := range result.nodes {
			point.x += gutter
			if point.bus >= 0 {
				point.bus += gutter
			}
			result.nodes[j] = point
		}
		for depth := 1; depth < len(result.left); depth++ {
			result.left[depth] += gutter
			result.right[depth] += gutter
		}
	}
	result.width = max(p.Sample.Tokens[i].Width, result.width+gutter)
	rootX := result.width/2 - p.Sample.Tokens[i].Width/2
	result.nodes[i] = packedPoint{x: rootX, bus: bus}
	result.left[0], result.right[0] = rootX, rootX+p.Sample.Tokens[i].Width
	result.height = (len(result.left)-1)*144 + 44
	return result
}
