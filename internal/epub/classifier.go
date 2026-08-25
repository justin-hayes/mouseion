package epub

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	ClassifierName    = "mouseion-epub-structure"
	ClassifierVersion = "1.1.0"
)

var (
	yearPattern             = regexp.MustCompile(`\b(?:1[5-9]\d{2}|20\d{2})\b`)
	urlPattern              = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	isbnPattern             = regexp.MustCompile(`(?i)\bISBN(?:-1[03])?\b`)
	pagePattern             = regexp.MustCompile(`(?i)\b(?:p(?:p)?\.|pages?|seiten?|pagine?)\s*\d+`)
	citationPattern         = regexp.MustCompile(`(?:\[[0-9]{1,3}\]|\([A-ZÀ-ÖØ-Þ][[:alpha:]'’-]+(?:\s+(?:et\s+al\.|e\s+al\.|u\.\s*a\.))?,?\s+(?:1[5-9]\d{2}|20\d{2})\))`)
	sentencePattern         = regexp.MustCompile(`[.!?](?:\s|$)`)
	bibliographyLinePattern = regexp.MustCompile(`(?i)(?:^|\n)\s*(?:\[[0-9]{1,3}\]|[[:alpha:]À-ÖØ-öø-ÿ'’-]+,?\s+(?:[[:alpha:]À-ÖØ-öø-ÿ'’-]+\s+){0,3}\(?(?:1[5-9]\d{2}|20\d{2})\)?)`)
)

type classificationScore struct {
	category UnitCategory
	points   int
}

type unitDecision struct {
	classification UnitClassification
	excluded       bool
}

type snapshotSignals struct {
	repeatedHeaders map[string]bool
	repeatedFooters map[string]bool
}

// ClassifyUnits classifies one immutable extracted-unit snapshot. The whole
// snapshot is required because the inclusion fallback is a book-level policy.
func ClassifyUnits(snapshotID string, extracted ExtractedUnits) ([]UnitClassification, error) {
	if strings.TrimSpace(snapshotID) == "" || snapshotID != strings.TrimSpace(snapshotID) {
		return nil, fmt.Errorf("epub: source snapshot identity is required and must not contain surrounding whitespace")
	}
	if extracted.SchemaVersion != ExtractedUnitsSchemaVersion {
		return nil, fmt.Errorf("epub: unsupported extracted-unit schema version %d", extracted.SchemaVersion)
	}
	if len(extracted.Units) == 0 {
		return nil, ErrExtractedUnitsUnavailable
	}

	snapshot := collectSnapshotSignals(extracted.Units)
	decisions := make([]unitDecision, len(extracted.Units))
	for i, unit := range extracted.Units {
		decisions[i] = classifyUnit(snapshotID, extracted.SchemaVersion, unit, i, len(extracted.Units), snapshot)
	}

	hasIncludedMain := false
	for i := range decisions {
		c := &decisions[i].classification
		if c.Category == CategoryMainMatter && c.Confidence >= ConfidenceHighMinimum {
			c.RecommendedInclusion = true
			hasIncludedMain = true
		} else {
			c.RecommendedInclusion = false
		}
	}
	if !hasIncludedMain {
		for i := range decisions {
			if decisions[i].excluded {
				continue
			}
			c := &decisions[i].classification
			if c.Category == CategoryUnknown || c.Confidence <= ConfidenceLowMaximum {
				c.RecommendedInclusion = true
				c.Reasons = append(c.Reasons, ClassificationReason{Signal: "whole_book_fallback", Message: "Included because no high-confidence main matter was found in the source snapshot."})
			}
		}
	}

	result := make([]UnitClassification, len(decisions))
	for i := range decisions {
		result[i] = decisions[i].classification
		if err := result[i].Validate(); err != nil {
			return nil, fmt.Errorf("epub: classify unit %q: %w", extracted.Units[i].ID, err)
		}
	}
	return result, nil
}

func classifyUnit(snapshotID string, schemaVersion int, unit ExtractedUnit, index, total int, snapshot snapshotSignals) unitDecision {
	c := UnitClassification{
		SchemaVersion:      ClassificationSchemaVersion,
		Classifier:         ClassifierIdentity{Name: ClassifierName, Version: ClassifierVersion},
		SourceUnitSnapshot: SourceUnitSnapshotIdentity{SnapshotID: snapshotID, ExtractedUnitsSchemaVersion: schemaVersion, UnitID: unit.ID},
	}

	if !unit.Linear {
		c.Category, c.Confidence = CategoryUnknown, 20
		c.Reasons = []ClassificationReason{{Signal: "linear_no", Message: "The spine marks this unit as non-linear content."}}
		return unitDecision{classification: c, excluded: true}
	}
	if containsFold(unit.Properties, "nav") {
		c.Category, c.Confidence = CategoryUnknown, 20
		c.Reasons = []ClassificationReason{{Signal: "navigation_only", Message: "The manifest identifies this unit as a navigation document."}}
		return unitDecision{classification: c, excluded: true}
	}

	scores := map[UnitCategory]int{CategoryFrontMatter: 0, CategoryMainMatter: 0, CategoryBackMatter: 0}
	reasons := make([]ClassificationReason, 0, 8)
	reviewRequired := false
	add := func(category UnitCategory, points int, signal, message string) {
		scores[category] += points
		reasons = append(reasons, ClassificationReason{Signal: signal, Message: message})
	}

	if category, token := landmarkCategory(unit.LandmarkTypes); category != CategoryUnknown {
		add(category, 90, "landmark_"+token, "The EPUB landmark identifies "+categoryDescription(category)+".")
	}
	labels := append([]string{unit.Title}, unit.NavigationLabels...)
	if category, marker := markerCategory(labels); category != CategoryUnknown {
		add(category, 45, "label_"+marker, "A title or navigation label matches the "+markerDisplay(marker)+" marker.")
	}
	if category, marker := pathCategory(unit.SourceHref, unit.ResolvedHref); category != CategoryUnknown {
		add(category, 25, "path_"+marker, "The package path matches the "+markerDisplay(marker)+" marker.")
	}
	for _, edge := range repeatedUnitEdges(unit.Text, snapshot) {
		reasons = append(reasons, ClassificationReason{Signal: "text_repeated_" + edge, Message: "The same short " + edge + " appears in multiple readable units."})
		reviewRequired = reviewRequired || len(strings.Fields(unit.Text)) <= 16
	}
	if marker := neutralMarker(labels); marker != "" {
		reasons = append(reasons, ClassificationReason{Signal: "label_" + marker, Message: "A title or navigation label matches the " + markerDisplay(marker) + " marker for non-prose structural content."})
		reviewRequired = true
	}
	if total >= 3 {
		switch {
		case index == 0:
			add(CategoryFrontMatter, 10, "spine_front", "The unit is at the beginning of the readable spine.")
		case index == total-1:
			add(CategoryBackMatter, 10, "spine_back", "The unit is at the end of the readable spine.")
		default:
			add(CategoryMainMatter, 15, "spine_central", "The unit is in the central readable spine range.")
		}
	}
	addTextSignals(unit.Text, add)

	ordered := []classificationScore{{CategoryFrontMatter, scores[CategoryFrontMatter]}, {CategoryMainMatter, scores[CategoryMainMatter]}, {CategoryBackMatter, scores[CategoryBackMatter]}}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].points > ordered[j].points })
	top, second := ordered[0], ordered[1]
	contradictory := top.points >= 45 && second.points >= 45
	if top.points < 25 || contradictory || reviewRequired {
		c.Category = CategoryUnknown
		if contradictory {
			c.Confidence = 35
			reasons = append(reasons, ClassificationReason{Signal: "contradictory_evidence", Message: "Strong signals support conflicting structural categories."})
		} else if reviewRequired {
			c.Confidence = 35
			reasons = append(reasons, ClassificationReason{Signal: "review_required", Message: "Structural-fragment evidence requires learner review instead of automatic promotion."})
		} else {
			c.Confidence = 20
			reasons = append(reasons, ClassificationReason{Signal: "insufficient_evidence", Message: "The available signals do not establish a structural category."})
		}
	} else {
		c.Category = top.category
		margin := top.points - second.points
		switch {
		case top.points >= 90 && margin >= 45:
			c.Confidence = 95
		case top.points >= 70 && margin >= 35:
			c.Confidence = 85
		case top.points >= 45 && margin >= 20:
			c.Confidence = 70
		default:
			c.Confidence = 45
		}
		if c.Category == CategoryMainMatter && c.Confidence >= ConfidenceHighMinimum && !hasMainContentEvidence(reasons) {
			c.Confidence = ConfidenceHighMinimum - 1
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, ClassificationReason{Signal: "insufficient_evidence", Message: "The available signals do not establish a structural category."})
	}
	c.Reasons = reasons
	return unitDecision{classification: c}
}

func landmarkCategory(values []string) (UnitCategory, string) {
	front := map[string]bool{"frontmatter": true, "titlepage": true, "copyright-page": true, "toc": true, "preface": true, "acknowledgments": true}
	main := map[string]bool{"bodymatter": true, "chapter": true, "part": true, "introduction": true, "conclusion": true, "epilogue": true}
	back := map[string]bool{"backmatter": true, "bibliography": true, "endnotes": true, "footnotes": true, "index": true, "glossary": true, "appendix": true}
	for _, raw := range values {
		for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(raw))) {
			switch {
			case front[token]:
				return CategoryFrontMatter, token
			case main[token]:
				return CategoryMainMatter, token
			case back[token]:
				return CategoryBackMatter, token
			}
		}
	}
	return CategoryUnknown, ""
}

var markerRules = []struct {
	category UnitCategory
	token    string
	terms    []string
}{
	{CategoryFrontMatter, "contents", []string{"contents", "table of contents", "inhalt", "inhaltsverzeichnis", "indice generale", "sommario"}},
	{CategoryFrontMatter, "preface", []string{"preface", "foreword", "vorwort", "prefazione", "introduzione dell'autore"}},
	{CategoryFrontMatter, "copyright", []string{"copyright", "impressum", "colophon"}},
	{CategoryFrontMatter, "acknowledgments", []string{"acknowledgments", "acknowledgements", "danksagung", "ringraziamenti"}},
	{CategoryFrontMatter, "editorial", []string{"editorial", "editorial note", "editors note", "editorial notice", "redaktion", "redaktionelle hinweise", "redaktionelle notiz", "redazione", "nota editoriale", "avvertenza editoriale"}},
	{CategoryBackMatter, "bibliography", []string{"bibliography", "references", "works cited", "bibliografie", "literaturverzeichnis", "quellenverzeichnis", "bibliografia", "riferimenti bibliografici", "fonti bibliografiche"}},
	{CategoryBackMatter, "notes", []string{"notes", "endnotes", "anmerkungen", "note", "note finali"}},
	{CategoryBackMatter, "index", []string{"index", "register", "sachregister", "indice analitico", "indice dei nomi"}},
	{CategoryBackMatter, "glossary", []string{"glossary", "glossar", "glossario"}},
	{CategoryBackMatter, "appendix", []string{"appendix", "appendices", "anhang", "appendice", "appendici"}},
	{CategoryMainMatter, "chapter", []string{"chapter", "kapitel", "capitolo"}},
	{CategoryMainMatter, "part", []string{"part", "teil", "parte"}},
	{CategoryMainMatter, "introduction", []string{"introduction", "einleitung", "introduzione"}},
	{CategoryMainMatter, "conclusion", []string{"conclusion", "conclusions", "schluss", "fazit", "conclusione", "conclusioni"}},
	{CategoryMainMatter, "epilogue", []string{"epilogue", "epilog", "epilogo"}},
}

var neutralMarkerRules = []struct {
	token string
	terms []string
}{
	{"caption", []string{"caption", "figure", "fig", "table", "abbildung", "bildunterschrift", "tabelle", "figura", "didascalia", "tabella"}},
	{"structural_fragment", []string{"page break", "section break", "separator", "blank page", "seitenumbruch", "abschnittstrenner", "leere seite", "interruzione di pagina", "separatore", "pagina vuota"}},
}

func neutralMarker(values []string) string {
	for _, rule := range neutralMarkerRules {
		for _, value := range values {
			normalized := normalizeMarker(value)
			for _, term := range rule.terms {
				if normalized == term || strings.HasPrefix(normalized, term+" ") {
					return rule.token
				}
			}
		}
	}
	return ""
}

func markerCategory(values []string) (UnitCategory, string) {
	for _, rule := range markerRules {
		for _, value := range values {
			normalized := normalizeMarker(value)
			for _, term := range rule.terms {
				if normalized == term || strings.HasPrefix(normalized, term+" ") {
					return rule.category, rule.token
				}
			}
		}
	}
	return CategoryUnknown, ""
}

func pathCategory(values ...string) (UnitCategory, string) {
	parts := make([]string, 0)
	for _, value := range values {
		clean := strings.ToLower(path.Clean(strings.ReplaceAll(value, "\\", "/")))
		for _, part := range strings.FieldsFunc(clean, func(r rune) bool { return !(unicode.IsLetter(r) || unicode.IsDigit(r)) }) {
			parts = append(parts, part)
		}
	}
	for _, rule := range markerRules {
		for _, term := range rule.terms {
			if !strings.Contains(term, " ") && containsPathMarker(parts, term) {
				return rule.category, rule.token
			}
		}
	}
	return CategoryUnknown, ""
}

func addTextSignals(text string, add func(UnitCategory, int, string, string)) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return
	}
	citations := len(citationPattern.FindAllString(text, -1))
	years := len(yearPattern.FindAllString(text, -1))
	urls := len(urlPattern.FindAllString(text, -1))
	isbn := len(isbnPattern.FindAllString(text, -1))
	pages := len(pagePattern.FindAllString(text, -1))
	referenceMarkers := citations + urls + isbn + pages
	bibliographyLines := len(bibliographyLinePattern.FindAllString(text, -1))
	if bibliographyLines >= 3 {
		add(CategoryBackMatter, 60, "text_bibliography_cluster", "The text contains a cluster of bibliography-style entries.")
	}
	if (referenceMarkers >= 2 && referenceMarkers*40 >= len(words)) || (years >= 3 && years*20 >= len(words)) {
		add(CategoryBackMatter, 45, "text_reference_density", "The text has a high density of citations, years, ISBN/URL, or page-reference markers.")
	}
	sentences := len(sentencePattern.FindAllString(text, -1))
	if sentences >= 3 && len(words)/sentences >= 5 {
		add(CategoryMainMatter, 50, "text_sentence_density", "The text contains sustained sentence-like prose.")
	}
}

func collectSnapshotSignals(units []ExtractedUnit) snapshotSignals {
	headerCounts := make(map[string]int)
	footerCounts := make(map[string]int)
	for _, unit := range units {
		header, footer := unitEdges(unit.Text)
		if header != "" {
			headerCounts[header]++
		}
		if footer != "" {
			footerCounts[footer]++
		}
	}
	repeatedHeaders := make(map[string]bool)
	for edge, count := range headerCounts {
		if count >= 2 {
			repeatedHeaders[edge] = true
		}
	}
	repeatedFooters := make(map[string]bool)
	for edge, count := range footerCounts {
		if count >= 2 {
			repeatedFooters[edge] = true
		}
	}
	return snapshotSignals{repeatedHeaders: repeatedHeaders, repeatedFooters: repeatedFooters}
}

func repeatedUnitEdges(text string, snapshot snapshotSignals) []string {
	header, footer := unitEdges(text)
	matched := make([]string, 0, 2)
	if header != "" && snapshot.repeatedHeaders[header] {
		matched = append(matched, "header")
	}
	if footer != "" && snapshot.repeatedFooters[footer] {
		matched = append(matched, "footer")
	}
	return matched
}

func unitEdges(text string) (string, string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	nonBlank := make([]string, 0, len(lines))
	for _, line := range lines {
		normalized := normalizeMarker(line)
		if normalized != "" {
			nonBlank = append(nonBlank, normalized)
		}
	}
	if len(nonBlank) < 2 {
		return "", ""
	}
	var header, footer string
	if len(strings.Fields(nonBlank[0])) <= 8 {
		header = nonBlank[0]
	}
	if last := nonBlank[len(nonBlank)-1]; len(strings.Fields(last)) <= 8 && last != header {
		footer = last
	}
	return header, footer
}

func normalizeMarker(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool { return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'') }), " ")
}
func containsPathMarker(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
		if strings.HasPrefix(value, wanted) {
			suffix := strings.TrimPrefix(value, wanted)
			if suffix != "" && strings.IndexFunc(suffix, func(r rune) bool { return !unicode.IsDigit(r) }) == -1 {
				return true
			}
		}
	}
	return false
}
func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		for _, token := range strings.Fields(value) {
			if strings.EqualFold(token, wanted) {
				return true
			}
		}
	}
	return false
}
func categoryDescription(category UnitCategory) string {
	switch category {
	case CategoryFrontMatter:
		return "front matter"
	case CategoryMainMatter:
		return "main matter"
	default:
		return "back matter"
	}
}
func markerDisplay(marker string) string { return strings.ReplaceAll(marker, "_", " ") }

func hasMainContentEvidence(reasons []ClassificationReason) bool {
	for _, reason := range reasons {
		if reason.Signal == "landmark_bodymatter" || reason.Signal == "landmark_chapter" || reason.Signal == "landmark_part" || reason.Signal == "text_sentence_density" {
			return true
		}
	}
	return false
}
