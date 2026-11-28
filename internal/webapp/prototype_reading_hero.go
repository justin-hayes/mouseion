package webapp

// PROTOTYPE (#2843): throwaway Current reading hero variants. Lives only on the
// prototype/2843-current-reading-hero branch; never merge.
//
// Three variants of the Current reading hero, switchable via ?variant=A|B|C on
// the existing My Books route, with ?hero_state= to push edge cases.
// Gated on MOUSEION_PROTOTYPE_HERO=1 (the fixture server sets it).

import (
	"net/http"
	"os"

	"github.com/justin-hayes/mouseion/internal/domain"
)

var prototypeHeroVariants = []prototypeHeroVariantInfo{
	{"A", "Frontispiece"},
	{"B", "Banner & ledger"},
	{"C", "Bookmark spread"},
}

var prototypeHeroStates = []string{"normal", "nocover", "updating", "failed", "classical", "longtitle"}

type prototypeHeroVariantInfo struct{ Key, Name string }

type prototypeHeroView struct {
	Item          readingBookView
	Variant       string
	State         string
	Title         string
	OriginalTitle string
	OriginalLang  string
	Author        string
	StatusLabel   string
	StatusDesc    string
	HasCoverage   bool
	MaturePct     float64
	InStudyPct    float64
	MatureText    string
	InStudyText   string
	MatureCount   string
	InStudyCount  string
	Analyzable    string
	TargetText    string
	CoverageNote  string
}

func prototypeHeroEnabled() bool { return os.Getenv("MOUSEION_PROTOTYPE_HERO") == "1" }

// prototypeHeroFromRequest returns nil unless the prototype is enabled and a
// Current reading exists. With no ?variant= it defaults to A.
func prototypeHeroFromRequest(r *http.Request, current *readingBookView) *prototypeHeroView {
	if !prototypeHeroEnabled() || current == nil {
		return nil
	}
	q := r.URL.Query()
	variant := q.Get("variant")
	if variant == "" {
		variant = "A"
	}
	if variant == "off" {
		return nil
	}
	state := q.Get("hero_state")
	if state == "" {
		state = "normal"
	}
	item := *current
	v := &prototypeHeroView{Variant: variant, State: state}
	switch state {
	case "nocover":
		item.Cover = domain.BookCover{}
	case "updating":
		item.Coverage = nil
		item.CountsUpdating = true
	case "failed":
		item.Coverage = nil
		v.StatusLabel = "Analysis failed"
		v.StatusDesc = "The last analysis failed. Retry analysis to produce current evidence."
	}
	v.Item = item
	v.Title = canonicalBookTitle(item.Book)
	v.Author = canonicalBookAuthor(item.Book)
	v.OriginalTitle = readingUnitOriginalTitle(item.ReadingUnit)
	v.OriginalLang = item.Book.Source.Language
	switch state {
	case "classical":
		v.Title, v.OriginalTitle, v.OriginalLang, v.Author = "Odyssey", "Ὀδύσσεια", "grc", "Homer"
		v.Item.Cover = domain.BookCover{}
	case "longtitle":
		v.Title = "Die Leiden des jungen Werthers und andere Erzählungen aus der Zeit des Sturm und Drang"
		v.Author = "Johann Wolfgang von Goethe"
	}
	if v.StatusLabel == "" && readingEvidenceState(item) != readingEvidenceCurrent {
		v.StatusLabel = readingEvidenceLabel(item)
		v.StatusDesc = readingEvidenceDescription(item)
	}
	if c := item.Coverage; c != nil && !item.StatisticsUnavailable && c.AnalyzableTokenCount > 0 {
		v.HasCoverage = true
		v.MaturePct = min(float64(c.MatureTokenCount)*100/float64(c.AnalyzableTokenCount), 100)
		v.InStudyPct = min(float64(c.InStudyTokenCount)*100/float64(c.AnalyzableTokenCount), 100)
		v.MatureText = readingCoverageMatureDisplay(*c)
		v.InStudyText = readingCoverageInStudyDisplay(*c)
		v.MatureCount = readingCoverageCount(c.MatureTokenCount)
		v.InStudyCount = readingCoverageCount(c.InStudyTokenCount)
		v.Analyzable = readingCoverageCount(c.AnalyzableTokenCount)
		v.TargetText = readingCoverageToTargetText(*c)
	} else {
		v.CoverageNote = "Coverage unavailable"
		if item.CountsUpdating {
			v.CoverageNote = "Coverage updating"
		}
	}
	return v
}

// InStudyExtraPct is the in-study share beyond Mature: "with words in study"
// also counts Mature words, so the bar stacks the difference after Mature.
func (v prototypeHeroView) InStudyExtraPct() float64 {
	return max(v.InStudyPct-v.MaturePct, 0)
}

func (v prototypeHeroView) BookURL() string { return bookDetailURL(readingBookID(v.Item)) }
