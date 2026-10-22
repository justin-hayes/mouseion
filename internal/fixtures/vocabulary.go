package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// Vocabulary Browse scenarios put the Working desk into the degraded and
// fully-accounted states that the in-memory store cannot reach on its own. They
// are set only by the fixture server, for browser acceptance.
const (
	VocabularyBrowseScenarioDefault           = ""
	VocabularyBrowseScenarioNoAnalysis        = "no-analysis"
	VocabularyBrowseScenarioNoVocabulary      = "no-vocabulary"
	VocabularyBrowseScenarioCountsUpdating    = "counts-updating"
	VocabularyBrowseScenarioCountsUnavailable = "counts-unavailable"
	// VocabularyBrowseScenarioAccounted holds only Known, Reserved, or Book-deck
	// identities, so the default view says everything is already accounted for
	// and revealing them shows the annotated rows.
	VocabularyBrowseScenarioAccounted = "accounted"
)

// SetVocabularyBrowseScenario selects the Browse scenario for every later Browse
// read. The empty name restores the default fixture.
func (s *Store) SetVocabularyBrowseScenario(name string) error {
	switch name {
	case VocabularyBrowseScenarioDefault, VocabularyBrowseScenarioNoAnalysis, VocabularyBrowseScenarioNoVocabulary,
		VocabularyBrowseScenarioCountsUpdating, VocabularyBrowseScenarioCountsUnavailable, VocabularyBrowseScenarioAccounted:
	default:
		return fmt.Errorf("unknown vocabulary Browse scenario %q", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.browseScenario = name
	return nil
}

func (s *Store) vocabularyBrowseScenario() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.browseScenario
}

// ListVocabularyBrowsePage provides representative browser-smoke rows for the
// default Current reading. It is fixture content, not simulated NLP output.
func (s *Store) ListVocabularyBrowsePage(ctx context.Context, owner, language string, query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	page := domain.VocabularyBrowsePage{Page: query.Page, IncludeAll: query.IncludeAll}
	current, err := s.GetCurrentReading(ctx, owner, language)
	if err != nil || !current.IsActive() {
		return page, err
	}
	page.CurrentBookID = current.BookID
	if current.BookID != BookID {
		return page, nil
	}
	page.Books = []domain.VocabularyBrowseBook{{ID: BookID, Title: "Der lange Weg nach Hause", HasCurrentAnalysis: true, HasVocabularyEvidence: true}}
	page.CorpusRevision = "fixture-current-reading-vocabulary-v1"
	scenario := s.vocabularyBrowseScenario()
	if fixtureVocabularyBrowseScenario(&page, scenario) {
		return page, nil
	}
	if query.Prefix == "paging" {
		return fixtureVocabularyBrowsePaging(page), nil
	}
	page.Total, page.InventoryTotal, page.ScopedInventoryTotal, page.Rows = fixtureVocabularyBrowseFilteredRows(fixtureVocabularyBrowseRows(scenario), query)
	lastPage := max(1, int((page.Total+24)/25))
	if page.Page > lastPage {
		page.Page = lastPage
	}
	return page, nil
}

// fixtureVocabularyBrowseScenario applies a named Browse scenario to the page
// and reports whether it replaces the ordinary rows.
func fixtureVocabularyBrowseScenario(page *domain.VocabularyBrowsePage, scenario string) bool {
	switch scenario {
	case VocabularyBrowseScenarioNoAnalysis:
		page.Books[0].HasCurrentAnalysis, page.Books[0].HasVocabularyEvidence = false, false
		page.BooksWithoutCurrentAnalysis = 1
	case VocabularyBrowseScenarioNoVocabulary:
		page.Books[0].HasVocabularyEvidence = false
	case VocabularyBrowseScenarioCountsUpdating:
		page.BrowseCountsUpdating = true
	case VocabularyBrowseScenarioCountsUnavailable:
		page.BrowseCountsUnavailable = true
	default:
		return false
	}
	return true
}

// fixtureVocabularyBrowsePaging serves the deliberately multi-page fixture the
// browser interaction checks page through.
func fixtureVocabularyBrowsePaging(page domain.VocabularyBrowsePage) domain.VocabularyBrowsePage {
	page.Total, page.InventoryTotal, page.ScopedInventoryTotal = 26, 26, 26
	page.Page = min(page.Page, 2)
	start := (page.Page - 1) * 25
	rows := make([]domain.VocabularyBrowseRow, 0, min(start+25, 26)-start)
	for i := start; i < min(start+25, 26); i++ {
		rows = append(rows, domain.VocabularyBrowseRow{CanonicalLemma: fmt.Sprintf("paging%02d", i), UPOS: "NOUN", OccurrenceCount: 1, AcrossBooksOccurrenceCount: 1})
	}
	page.Rows = rows
	return page
}

// fixtureVocabularyBrowseRows returns the representative rows, or the accounted
// scenario's already-known identities.
func fixtureVocabularyBrowseRows(scenario string) []domain.VocabularyBrowseRow {
	if scenario == VocabularyBrowseScenarioAccounted {
		return []domain.VocabularyBrowseRow{
			{CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 2, AcrossBooksOccurrenceCount: 2, Known: true, InBookDeck: true},
			{CanonicalLemma: "lesen", UPOS: "VERB", OccurrenceCount: 3, AcrossBooksOccurrenceCount: 3, Known: true, Reserved: true},
		}
	}
	return []domain.VocabularyBrowseRow{
		{CanonicalLemma: "gehen", UPOS: "VERB", OccurrenceCount: 5, AcrossBooksOccurrenceCount: 8, Generated: true},
		{CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 2, AcrossBooksOccurrenceCount: 2, Known: true, InBookDeck: true},
	}
}

// fixtureVocabularyBrowseFilteredRows applies the accounted-for and prefix
// filters and returns the totals they drive alongside the surviving rows.
func fixtureVocabularyBrowseFilteredRows(rows []domain.VocabularyBrowseRow, query domain.VocabularyBrowseQuery) (total, inventoryTotal, scopedInventoryTotal int64, filtered []domain.VocabularyBrowseRow) {
	inventoryTotal = int64(len(rows))
	if !query.IncludeAll {
		filtered = rows[:0]
		for _, row := range rows {
			if !row.Known && !row.Reserved && !row.InBookDeck {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	prefix := strings.ToLower(strings.TrimSpace(query.Prefix))
	if prefix != "" {
		filtered = rows[:0]
		for _, row := range rows {
			if strings.HasPrefix(strings.ToLower(row.CanonicalLemma), prefix) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	return int64(len(rows)), inventoryTotal, inventoryTotal, rows
}

// ListVocabularyConcordance serves deterministic synthetic evidence so browser
// smoke tests can exercise KWIC presentation without PostgreSQL or NLP.
func (s *Store) ListVocabularyConcordance(_ context.Context, _, _ string, query domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	result := domain.ConcordanceResult{Page: query.Page, HasPrevious: query.Page > 1}
	if query.Revision == "fixture-stale-revision" {
		result.Stale = true
		return result, nil
	}
	switch query.Term {
	case "fixture-server-error":
		return domain.ConcordanceResult{}, errors.New("fixture Concordance server error")
	case "fixture-server-timeout":
		return domain.ConcordanceResult{}, context.DeadlineExceeded
	}
	if strings.EqualFold(strings.TrimSpace(query.Term), "fixture-books") {
		return fixtureMultiBookConcordance(result), nil
	}
	if strings.EqualFold(strings.TrimSpace(query.Term), "fixture-whole-line") {
		return fixtureWholeLineConcordance(result), nil
	}
	if strings.EqualFold(strings.TrimSpace(query.Term), "fixture-poetry") {
		return fixturePoetryConcordance(result), nil
	}
	lemma := strings.ToLower(strings.TrimSpace(query.Term))
	// Mirror the persisted lookup: an evidenced effective lemma wins and
	// expands its forms; any other term matches only that source form.
	result.Match, result.Term = fixtureConcordanceMatch(query.Match, lemma), lemma
	if query.Priority == BookID {
		// The fixture corpus is a single Book, so a captured priority always matches.
		result.PriorityBookID, result.PriorityTitle = BookID, "Der lange Weg nach Hause"
		result.PriorityState = domain.ConcordancePriorityMatches
	}
	fixtureConcordancePage(&result, fixtureConcordanceMatches(result.Match, lemma, query.UPOS), query.Page)
	return result, nil
}

// fixtureConcordanceMatch applies the supplied interpretation, recognizing the
// fixture's evidenced lemmas as lemma matches when none is supplied.
func fixtureConcordanceMatch(supplied, lemma string) string {
	if supplied != "" {
		return supplied
	}
	if lemma == "haus" || lemma == "heim" {
		return domain.ConcordanceMatchLemma
	}
	return domain.ConcordanceMatchForm
}

// fixtureConcordanceMatches returns the synthetic occurrences the applied
// interpretation selects, skipping the deliberately excluded row.
func fixtureConcordanceMatches(match, lemma, upos string) []domain.ConcordanceResultOccurrence {
	fixtureRows := []domain.ConcordanceResultOccurrence{
		fixtureConcordanceOccurrence("heim", true, false),
		fixtureConcordanceOccurrence("haus", false, true),
		fixtureConcordanceOccurrence("haus", false, false),
	}
	for i := 3; i < 28; i++ {
		fixtureRows = append(fixtureRows, fixtureConcordanceOccurrence("haus", false, false))
	}
	var matches []domain.ConcordanceResultOccurrence
	for i, occurrence := range fixtureRows {
		// Each synthetic row represents a distinct sentence occurrence; keep IDs
		// unique just as the persisted corpus query does.
		occurrence.SentenceOrdinal = int64(i)
		if fixtureConcordanceRowMatched(occurrence, match, lemma, upos) {
			matches = append(matches, occurrence)
		}
	}
	return matches
}

// fixtureConcordanceRowMatched reports whether one occurrence fits the applied
// interpretation and requested part of speech.
func fixtureConcordanceRowMatched(occurrence domain.ConcordanceResultOccurrence, match, lemma, upos string) bool {
	if occurrence.Excluded {
		return false
	}
	switch match {
	case domain.ConcordanceMatchLemma:
		return lemma == occurrence.EffectiveLemma && (upos == "" || upos == occurrence.UPOS)
	case domain.ConcordanceMatchForm:
		return lemma == strings.ToLower(occurrence.Surface)
	}
	return false
}

// fixtureConcordancePage pages the matches onto result.
func fixtureConcordancePage(result *domain.ConcordanceResult, matches []domain.ConcordanceResultOccurrence, page int) {
	const pageSize = 25
	start := (page - 1) * pageSize
	if start >= len(matches) {
		return
	}
	end := min(start+pageSize, len(matches))
	result.HasNext = end < len(matches)
	result.Occurrences = matches[start:end]
}

func (s *Store) GetVocabularySentenceStudy(_ context.Context, _, book, _, _, unit string, sentence, target int64, targetSurface string) (domain.SentenceStudy, error) {
	if (book != BookID && !strings.HasPrefix(book, "fixture-kwic-")) || sentence < 0 || sentence > 27 {
		return domain.SentenceStudy{}, errors.New("sentence study not found")
	}
	switch unit {
	case "fixture-concordance-unit", "fixture-concordance-whole-line-unit":
	case "fixture-poetry-unit":
		return fixturePoetryStudy(book, sentence, target, targetSurface), nil
	case "fixture-stage-unit":
		return fixtureStageStudy(book, sentence, target, targetSurface), nil
	default:
		return domain.SentenceStudy{}, errors.New("sentence study not found")
	}
	study := fixtureSentenceStudy(book, sentence, target, targetSurface)
	if unit == "fixture-concordance-whole-line-unit" {
		// The stored sentence bounds disagree with the immutable unit text, so
		// the reader withholds the morphology line and opens the text
		// alternative by default.
		study.UnitText = "Ganz anderer gespeicherter Text."
		study.SentenceEndOffset = 4
	}
	return study, nil
}

// fixtureSentenceStudy builds the default, fully aligned fixture sentence so
// every sentence-study scenario starts from the same evidence.
func fixtureSentenceStudy(book string, sentence, target int64, targetSurface string) domain.SentenceStudy {
	return domain.SentenceStudy{BookID: book, BookTitle: "Der lange Weg nach Hause", ChapterTitle: "Kapitel 1", TargetSurface: targetSurface,
		SentenceText: "Das Haus sieht gut aus.", UnitText: "Das Haus sieht gut aus.",
		SentenceStartOffset: 0, SentenceEndOffset: 23, SentenceOrdinal: sentence, TargetOrdinal: target,
		Tokens: []domain.SentenceStudyToken{
			{Surface: "Das", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Dependency: "det", HeadOrdinal: 2, HeadSurface: "sieht", Ordinal: 0, StartOffset: 0, EndOffset: 3, Morphology: map[string]string{"Case": "Nom", "Gender": "Neut", "Number": "Sing"}},
			{Surface: "Haus", RawLemma: "haus", EffectiveLemma: "heim", UPOS: "NOUN", Dependency: "obj", HeadOrdinal: 2, HeadSurface: "sieht", Ordinal: 1, Corrected: true, StartOffset: 4, EndOffset: 8, Morphology: map[string]string{"Case": "Nom", "Gender": "Neut", "Number": "Sing"}},
			{Surface: "sieht", RawLemma: "sehen", EffectiveLemma: "sehen", UPOS: "VERB", Dependency: "root", HeadOrdinal: 2, HeadSurface: "sieht", Ordinal: 2, StartOffset: 9, EndOffset: 14, Morphology: map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}},
			{Surface: "gut", RawLemma: "gut", EffectiveLemma: "gut", UPOS: "ADV", Dependency: "advmod", HeadOrdinal: 2, HeadSurface: "sieht", Ordinal: 3, StartOffset: 15, EndOffset: 18},
			{Surface: "aus", RawLemma: "aus", EffectiveLemma: "aus", UPOS: "PART", Dependency: "compound:prt", HeadOrdinal: 2, HeadSurface: "sieht", Ordinal: 4, StartOffset: 19, EndOffset: 22, Morphology: map[string]string{"PartType": "Sep"}},
			{Surface: ".", RawLemma: ".", EffectiveLemma: ".", UPOS: "PUNCT", Dependency: "punct", HeadOrdinal: 2, HeadSurface: "sieht", Ordinal: 5, StartOffset: 22, EndOffset: 23},
		}}
}

// fixturePoetrySentence is a short synthetic verse excerpt with four authorial
// source lines, used only to exercise source-line presentation in the fixture
// server.
const fixturePoetrySentence = "Der Geist der Medicin ist leicht zu fassen;\nIhr durchstudirt die groß’ und kleine Welt,\nUm es am Ende gehn zu lassen,\nWie’s Gott gefällt."

// fixturePoetryConcordance returns an occurrence whose sentence has authorial
// breaks, so browser acceptance can see the one-line summary and expanded
// source lines without PostgreSQL or NLP.
func fixturePoetryConcordance(result domain.ConcordanceResult) domain.ConcordanceResult {
	result.Match, result.Term = domain.ConcordanceMatchForm, "fixture-poetry"
	occurrence := fixtureConcordanceOccurrence("Geist", false, false)
	occurrence.BookID = "fixture-kwic-poetry"
	occurrence.BookTitle = "Faust: Eine Tragödie [erster Teil]"
	occurrence.ChapterTitle = "Studirzimmer"
	occurrence.SentenceText = fixturePoetrySentence
	occurrence.SentenceStartOffset = 4
	occurrence.SentenceEndOffset = 9
	occurrence.Surface = "Geist"
	occurrence.UnitID = "fixture-poetry-unit"
	occurrence.TokenOrdinal = 1
	occurrence.Attribution = domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Mephistopheles"}
	result.Occurrences = []domain.ConcordanceResultOccurrence{occurrence}
	return result
}

// fixturePoetryStudy serves the aligned verse excerpt and its quiet speaker
// attribution. The speaker is name-only; nested directions never leave the
// structural role span.
func fixturePoetryStudy(book string, sentence, target int64, targetSurface string) domain.SentenceStudy {
	return domain.SentenceStudy{
		BookID: book, BookTitle: "Faust: Eine Tragödie [erster Teil]", ChapterTitle: "Studirzimmer",
		TargetSurface: targetSurface, SentenceText: fixturePoetrySentence, UnitText: fixturePoetrySentence,
		SentenceStartOffset: 0, SentenceEndOffset: int64(len([]rune(fixturePoetrySentence))),
		SentenceOrdinal: sentence, TargetOrdinal: target,
		Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Mephistopheles"},
		Tokens:      fixturePoetryTokens(),
	}
}

// fixturePoetryTokens keeps every verse word in source order so the fixture
// proves per-source-line morphology and target emphasis inside the right line.
func fixturePoetryTokens() []domain.SentenceStudyToken {
	return []domain.SentenceStudyToken{
		{Surface: "Der", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Dependency: "det", HeadOrdinal: 1, HeadSurface: "Geist", Ordinal: 0, StartOffset: 0, EndOffset: 3, Morphology: map[string]string{"Case": "Nom", "Gender": "Masc", "Number": "Sing"}},
		{Surface: "Geist", RawLemma: "Geist", EffectiveLemma: "Geist", UPOS: "NOUN", Dependency: "nsubj", HeadOrdinal: 4, HeadSurface: "ist", Ordinal: 1, StartOffset: 4, EndOffset: 9, Morphology: map[string]string{"Case": "Nom", "Gender": "Masc", "Number": "Sing"}},
		{Surface: "der", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Dependency: "det", HeadOrdinal: 3, HeadSurface: "Medicin", Ordinal: 2, StartOffset: 10, EndOffset: 13, Morphology: map[string]string{"Case": "Gen", "Gender": "Fem", "Number": "Sing"}},
		{Surface: "Medicin", RawLemma: "Medicin", EffectiveLemma: "Medicin", UPOS: "NOUN", Dependency: "nmod", HeadOrdinal: 1, HeadSurface: "Geist", Ordinal: 3, StartOffset: 14, EndOffset: 21, Morphology: map[string]string{"Case": "Gen", "Gender": "Fem", "Number": "Sing"}},
		{Surface: "ist", RawLemma: "sein", EffectiveLemma: "sein", UPOS: "AUX", Dependency: "cop", HeadOrdinal: 1, HeadSurface: "Geist", Ordinal: 4, StartOffset: 22, EndOffset: 25, Morphology: map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}},
		{Surface: "leicht", RawLemma: "leicht", EffectiveLemma: "leicht", UPOS: "ADJ", Dependency: "advmod", HeadOrdinal: 7, HeadSurface: "fassen", Ordinal: 5, StartOffset: 26, EndOffset: 32, Morphology: map[string]string{"Degree": "Pos"}},
		{Surface: "zu", RawLemma: "zu", EffectiveLemma: "zu", UPOS: "PART", Dependency: "mark", HeadOrdinal: 7, HeadSurface: "fassen", Ordinal: 6, StartOffset: 33, EndOffset: 35},
		{Surface: "fassen", RawLemma: "fassen", EffectiveLemma: "fassen", UPOS: "VERB", Dependency: "xcomp", HeadOrdinal: 4, HeadSurface: "ist", Ordinal: 7, StartOffset: 36, EndOffset: 42, Morphology: map[string]string{"VerbForm": "Inf"}},
		{Surface: ";", RawLemma: ";", EffectiveLemma: ";", UPOS: "PUNCT", Dependency: "punct", HeadOrdinal: 4, HeadSurface: "ist", Ordinal: 8, StartOffset: 42, EndOffset: 43},
		{Surface: "Ihr", RawLemma: "ihr", EffectiveLemma: "ihr", UPOS: "PRON", Dependency: "nsubj", HeadOrdinal: 10, HeadSurface: "durchstudirt", Ordinal: 9, StartOffset: 44, EndOffset: 47, Morphology: map[string]string{"Case": "Nom", "Number": "Plur", "Person": "2"}},
		{Surface: "durchstudirt", RawLemma: "durchstudiren", EffectiveLemma: "durchstudiren", UPOS: "VERB", Dependency: "root", HeadOrdinal: 10, HeadSurface: "durchstudirt", Ordinal: 10, StartOffset: 48, EndOffset: 60, Morphology: map[string]string{"Mood": "Ind", "Number": "Plur", "Person": "2", "Tense": "Pres", "VerbForm": "Fin"}},
		{Surface: "die", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Dependency: "det", HeadOrdinal: 12, HeadSurface: "Welt", Ordinal: 11, StartOffset: 61, EndOffset: 64, Morphology: map[string]string{"Case": "Acc", "Gender": "Fem", "Number": "Sing"}},
		{Surface: "groß’", RawLemma: "groß", EffectiveLemma: "groß", UPOS: "ADJ", Dependency: "amod", HeadOrdinal: 12, HeadSurface: "Welt", Ordinal: 12, StartOffset: 65, EndOffset: 70, Morphology: map[string]string{"Case": "Acc", "Gender": "Fem", "Number": "Sing"}},
		{Surface: "und", RawLemma: "und", EffectiveLemma: "und", UPOS: "CCONJ", Dependency: "cc", HeadOrdinal: 14, HeadSurface: "Welt", Ordinal: 13, StartOffset: 71, EndOffset: 74},
		{Surface: "kleine", RawLemma: "klein", EffectiveLemma: "klein", UPOS: "ADJ", Dependency: "amod", HeadOrdinal: 14, HeadSurface: "Welt", Ordinal: 14, StartOffset: 75, EndOffset: 81, Morphology: map[string]string{"Case": "Acc", "Gender": "Fem", "Number": "Sing"}},
		{Surface: "Welt", RawLemma: "Welt", EffectiveLemma: "Welt", UPOS: "NOUN", Dependency: "obj", HeadOrdinal: 10, HeadSurface: "durchstudirt", Ordinal: 15, StartOffset: 82, EndOffset: 86, Morphology: map[string]string{"Case": "Acc", "Gender": "Fem", "Number": "Sing"}},
		{Surface: ",", RawLemma: ",", EffectiveLemma: ",", UPOS: "PUNCT", Dependency: "punct", HeadOrdinal: 10, HeadSurface: "durchstudirt", Ordinal: 16, StartOffset: 86, EndOffset: 87},
		{Surface: "Um", RawLemma: "um", EffectiveLemma: "um", UPOS: "ADP", Dependency: "mark", HeadOrdinal: 21, HeadSurface: "lassen", Ordinal: 17, StartOffset: 88, EndOffset: 90},
		{Surface: "es", RawLemma: "es", EffectiveLemma: "es", UPOS: "PRON", Dependency: "obj", HeadOrdinal: 21, HeadSurface: "lassen", Ordinal: 18, StartOffset: 91, EndOffset: 93, Morphology: map[string]string{"Case": "Acc", "Gender": "Neut", "Number": "Sing"}},
		{Surface: "am", RawLemma: "an", EffectiveLemma: "an", UPOS: "ADP", Dependency: "case", HeadOrdinal: 20, HeadSurface: "Ende", Ordinal: 19, StartOffset: 94, EndOffset: 96},
		{Surface: "Ende", RawLemma: "Ende", EffectiveLemma: "Ende", UPOS: "NOUN", Dependency: "obl", HeadOrdinal: 21, HeadSurface: "lassen", Ordinal: 20, StartOffset: 97, EndOffset: 101, Morphology: map[string]string{"Case": "Dat", "Gender": "Neut", "Number": "Sing"}},
		{Surface: "gehn", RawLemma: "gehen", EffectiveLemma: "gehen", UPOS: "VERB", Dependency: "xcomp", HeadOrdinal: 21, HeadSurface: "lassen", Ordinal: 21, StartOffset: 102, EndOffset: 106, Morphology: map[string]string{"VerbForm": "Inf"}},
		{Surface: "zu", RawLemma: "zu", EffectiveLemma: "zu", UPOS: "PART", Dependency: "mark", HeadOrdinal: 21, HeadSurface: "lassen", Ordinal: 22, StartOffset: 107, EndOffset: 109},
		{Surface: "lassen", RawLemma: "lassen", EffectiveLemma: "lassen", UPOS: "VERB", Dependency: "conj", HeadOrdinal: 10, HeadSurface: "durchstudirt", Ordinal: 23, StartOffset: 110, EndOffset: 116, Morphology: map[string]string{"VerbForm": "Inf"}},
		{Surface: ",", RawLemma: ",", EffectiveLemma: ",", UPOS: "PUNCT", Dependency: "punct", HeadOrdinal: 10, HeadSurface: "durchstudirt", Ordinal: 24, StartOffset: 116, EndOffset: 117},
		{Surface: "Wie’s", RawLemma: "wie", EffectiveLemma: "wie", UPOS: "PRON", Dependency: "nsubj", HeadOrdinal: 27, HeadSurface: "gefällt", Ordinal: 25, StartOffset: 118, EndOffset: 123},
		{Surface: "Gott", RawLemma: "Gott", EffectiveLemma: "Gott", UPOS: "PROPN", Dependency: "nsubj", HeadOrdinal: 27, HeadSurface: "gefällt", Ordinal: 26, StartOffset: 124, EndOffset: 128, Morphology: map[string]string{"Case": "Nom", "Gender": "Masc", "Number": "Sing"}},
		{Surface: "gefällt", RawLemma: "gefallen", EffectiveLemma: "gefallen", UPOS: "VERB", Dependency: "root", HeadOrdinal: 27, HeadSurface: "gefällt", Ordinal: 27, StartOffset: 129, EndOffset: 136, Morphology: map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}},
		{Surface: ".", RawLemma: ".", EffectiveLemma: ".", UPOS: "PUNCT", Dependency: "punct", HeadOrdinal: 27, HeadSurface: "gefällt", Ordinal: 28, StartOffset: 136, EndOffset: 137},
	}
}

// fixtureStageStudy serves a standalone direction sentence with the agreed
// Stage direction slot and a separable verb target.
func fixtureStageStudy(book string, sentence, target int64, targetSurface string) domain.SentenceStudy {
	const text = "Er schlägt das Buch auf und erblickt das Zeichen des Makrokosmus."
	return domain.SentenceStudy{
		BookID: book, BookTitle: "Faust: Eine Tragödie [erster Teil]", ChapterTitle: "Nacht",
		TargetSurface: targetSurface, SentenceText: text, UnitText: text,
		SentenceStartOffset: 0, SentenceEndOffset: int64(len([]rune(text))),
		SentenceOrdinal: sentence, TargetOrdinal: target,
		Attribution: domain.SourceAttribution{Kind: domain.AttributionStageDirection},
		Tokens: []domain.SentenceStudyToken{
			{Surface: "Er", RawLemma: "er", EffectiveLemma: "er", UPOS: "PRON", Ordinal: 0, StartOffset: 0, EndOffset: 2, Morphology: map[string]string{"Case": "Nom", "Gender": "Masc", "Number": "Sing", "Person": "3"}},
			{Surface: "schlägt", RawLemma: "schlagen", EffectiveLemma: "schlagen", UPOS: "VERB", Ordinal: 1, StartOffset: 3, EndOffset: 10, Morphology: map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}},
			{Surface: "das", RawLemma: "der", EffectiveLemma: "der", UPOS: "DET", Ordinal: 2, StartOffset: 11, EndOffset: 14, Morphology: map[string]string{"Case": "Acc", "Gender": "Neut", "Number": "Sing"}},
			{Surface: "Buch", RawLemma: "Buch", EffectiveLemma: "Buch", UPOS: "NOUN", Ordinal: 3, StartOffset: 15, EndOffset: 19, Morphology: map[string]string{"Case": "Acc", "Gender": "Neut", "Number": "Sing"}},
			{Surface: "auf", RawLemma: "auf", EffectiveLemma: "auf", UPOS: "PART", Ordinal: 4, StartOffset: 20, EndOffset: 23, Morphology: map[string]string{"PartType": "Sep"}},
			{Surface: "und", RawLemma: "und", EffectiveLemma: "und", UPOS: "CCONJ", Ordinal: 5, StartOffset: 24, EndOffset: 27},
			{Surface: "erblickt", RawLemma: "erblicken", EffectiveLemma: "erblicken", UPOS: "VERB", Ordinal: 6, StartOffset: 28, EndOffset: 36, Morphology: map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}},
			{Surface: "Zeichen", RawLemma: "Zeichen", EffectiveLemma: "Zeichen", UPOS: "NOUN", Ordinal: 7, StartOffset: 41, EndOffset: 48, Morphology: map[string]string{"Case": "Acc", "Gender": "Neut", "Number": "Sing"}},
			{Surface: "Makrokosmus", RawLemma: "Makrokosmus", EffectiveLemma: "Makrokosmus", UPOS: "NOUN", Ordinal: 8, StartOffset: 53, EndOffset: 64, Morphology: map[string]string{"Case": "Gen", "Gender": "Masc", "Number": "Sing"}},
			{Surface: ".", RawLemma: ".", EffectiveLemma: ".", UPOS: "PUNCT", Ordinal: 9, StartOffset: 64, EndOffset: 65},
		},
	}
}

// fixtureWholeLineConcordance points Study at the fixture sentence whose stored
// bounds disagree with its immutable source text, exercising whole-line
// fallback through the browser journey.
func fixtureWholeLineConcordance(result domain.ConcordanceResult) domain.ConcordanceResult {
	result.Match, result.Term = domain.ConcordanceMatchForm, "fixture-whole-line"
	occurrence := fixtureConcordanceOccurrence("haus", false, false)
	occurrence.UnitID = "fixture-concordance-whole-line-unit"
	result.Occurrences = []domain.ConcordanceResultOccurrence{occurrence}
	return result
}

// fixtureMultiBookConcordance returns one page spanning several Books: a
// long-title multi-hit Book, two adjacent one-hit Books, and a short-title
// two-hit Book, so browser tests can check that quiet source labels never
// size rows or leave gaps after a one-hit Book.
func fixtureMultiBookConcordance(result domain.ConcordanceResult) domain.ConcordanceResult {
	books := []struct {
		id, title string
		hits      int
	}{
		{"fixture-kwic-long", "Die außerordentlich lange und ausführliche Geschichte vom Haus am Ende der Welt: Ein Roman in drei Büchern", 3},
		{"fixture-kwic-one-a", "Kurz", 1},
		{"fixture-kwic-one-b", "Noch ein sehr langer Titel für ein einziges Vorkommen im Korpus", 1},
		{"fixture-kwic-two", "Zwei Treffer", 2},
	}
	result.Match, result.Term = domain.ConcordanceMatchForm, "fixture-books"
	ordinal := int64(0)
	for _, book := range books {
		for range book.hits {
			occurrence := fixtureConcordanceOccurrence("haus", false, false)
			occurrence.BookID, occurrence.BookTitle = book.id, book.title
			occurrence.SentenceOrdinal = ordinal
			ordinal++
			result.Occurrences = append(result.Occurrences, occurrence)
		}
	}
	return result
}

func fixtureConcordanceOccurrence(effectiveLemma string, corrected, excluded bool) domain.ConcordanceResultOccurrence {
	return domain.ConcordanceResultOccurrence{
		Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "obj",
		HeadOrdinal: 1, HeadSurface: "sieht", SentenceText: "Das Haus sieht gut aus.",
		SentenceStartOffset: 4, SentenceEndOffset: 8, BookID: BookID,
		BookTitle: "Der lange Weg nach Hause", SourceMaterialID: SourceID,
		AnalysisRunID: ResultRunID, CorpusID: "fixture-corpus",
		UnitID: "fixture-concordance-unit", ChapterTitle: "Kapitel 1", UnitOrder: 0,
		SentenceOrdinal: 0, TokenOrdinal: 1,
		RawLemma: "haus", EffectiveLemma: effectiveLemma, Corrected: corrected, Excluded: excluded,
	}
}

// corpusVocabulary provides deterministic fixture facts for the coverage
// summary without involving a real analyzer.
func (s *Store) corpusVocabulary(corpusID string) (domain.AnalysisCorpusVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var source domain.SourceMaterialSummary
	for _, book := range s.books {
		if book.CorpusID == corpusID {
			source = book
			break
		}
	}
	if source.CorpusID == "" {
		return domain.AnalysisCorpusVocabulary{}, errNotFound
	}
	knownTokens := fixtureKnownCorpusTokens(corpusID)
	sharedTokens := int64(5)
	if knownTokens+sharedTokens > 100 {
		sharedTokens = max(100-knownTokens, 0)
	}
	lemmas := []domain.LemmaOccurrence{}
	if knownTokens > 0 {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: source.Source.Language, CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: knownTokens})
	}
	if sharedTokens > 0 {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: source.Source.Language, CanonicalLemma: "fixture-recurring", UPOS: "NOUN", OccurrenceCount: sharedTokens})
	}
	if remaining := 100 - knownTokens - sharedTokens; remaining > 0 {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: source.Source.Language, CanonicalLemma: "fixture-" + corpusID, UPOS: "NOUN", OccurrenceCount: remaining})
	}
	return domain.AnalysisCorpusVocabulary{
		CorpusID: corpusID, SourceMaterialID: source.Source.ID, AnalysisRunID: source.AnalysisRunID,
		Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: int64(len(lemmas))}, Lemmas: lemmas,
	}, nil
}

// GetProjectedCorpusVocabulary supplies the fixture Book's projected effective
// counts as data, always ready, so coverage never depends on a real projection.
func (s *Store) GetProjectedCorpusVocabulary(ctx context.Context, owner, corpusID string) (domain.ProjectedCorpusVocabulary, error) {
	vocabulary, err := s.corpusVocabulary(corpusID)
	if err != nil {
		return domain.ProjectedCorpusVocabulary{}, err
	}
	return domain.ProjectedCorpusVocabulary{AnalysisCorpusVocabulary: vocabulary, Ready: true}, nil
}

func (s *Store) IsReservedVocabulary(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}

func fixtureKnownCorpusTokens(corpusID string) int64 {
	switch corpusID {
	case "fixture-corpus", "fixture-route-match-corpus":
		return 90
	case "fixture-route-differs-corpus":
		return 20
	case "fixture-route-tie-a-corpus", "fixture-route-tie-b-corpus":
		return 50
	case "fixture-italian-goal-corpus":
		return 60
	default:
		return 0
	}
}

func (s *Store) ListKnownVocabulary(_ context.Context, owner, language string) ([]domain.KnownVocabulary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.knownForOwnerLocked(owner, normalizeFixtureLanguage(language)), nil
}

// knownForOwnerLocked returns the owner's Known rows in language. Callers hold s.mu.
func (s *Store) knownForOwnerLocked(owner, language string) []domain.KnownVocabulary {
	var result []domain.KnownVocabulary
	for _, entry := range s.known {
		if entry.OwnerID == owner && normalizeFixtureLanguage(entry.Language) == language {
			result = append(result, entry)
		}
	}
	return result
}
