package fixtures

// Contract status: illustrative. The browser fixture models the built-in
// Classical catalog with a small in-memory release; canned catalog state is
// not held to internal/storecontract parity (ADR 0088).

import (
	"context"
	"strings"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// NewClassicalCatalog builds the fixture server's built-in Classical catalog:
// two whole-work units with contents and one whole-work unit without books.
func NewClassicalCatalog() (*classical.Catalog, error) {
	return classical.NewCatalog(fixtureClassicalDefinitions())
}

// fixtureClassicalDefinition builds one valid definition for the fixture
// release. The source pin values are illustrative; no fixture path reads them.
func fixtureClassicalDefinition(key, workKey, title, author, originalTitle, editionKey, citationStart, citationEnd, opening string, book *int, books, words int) classical.Definition {
	return classical.Definition{
		Key:                   key,
		WorkKey:               workKey,
		WorkTitle:             title,
		WorkAuthor:            author,
		OriginalTitle:         originalTitle,
		EditionKey:            editionKey,
		EditionLabel:          "Fixture reviewed edition",
		Language:              classical.Language,
		ConstituentBook:       book,
		ConstituentBooks:      books,
		CitationStart:         citationStart,
		CitationEnd:           citationEnd,
		WordCount:             words,
		Opening:               opening,
		Source:                classical.SourcePin{Layer: "oga-src-v0.2.0-text", FileName: workKey + ".conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
		InterpretationVersion: "fixture-oga/1",
		Revision:              1,
	}
}

// fixtureClassicalDefinitions is the small release the fixture server exposes.
// The Odyssey and Iliad list contents as the shipped Anabasis does. Withdrawn
// per-book Iliad definitions let the browser exercise leftover Books.
func fixtureClassicalDefinitions() []classical.Definition {
	one, two, three := 1, 2, 3
	odyssey := fixtureClassicalDefinition("fixture-odyssey", "fixture-odyssey", "Odyssey", "Homer", "Ὀδύσσεια",
		"fixture-odyssey-monro-1901", "1.1.1", "3.10.19", "ἄνδρα μοι ἔννεπε, μοῦσα, πολύτροπον, ὃς μάλα πολλὰ", nil, 3, 600)
	odyssey.Contents = []classical.ConstituentBookSpan{
		{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.10.19", WordCount: 100},
		{Book: 2, CitationStart: "2.1.1", CitationEnd: "2.10.19", WordCount: 200},
		{Book: 3, CitationStart: "3.1.1", CitationEnd: "3.10.19", WordCount: 300},
	}
	iliad := fixtureClassicalDefinition("fixture-iliad", "fixture-iliad", "Iliad", "Homer", "Ἰλιάς",
		"fixture-iliad-monro-1902", "1.1.1", "3.10.19", "μῆνιν ἄειδε θεὰ Πηληϊάδεω Ἀχιλῆος", nil, 3, 600)
	iliad.Contents = odyssey.Contents
	withdrawn := []classical.Definition{
		fixtureClassicalDefinition("fixture-iliad-1", "fixture-iliad", "Iliad", "Homer", "Ἰλιάς",
			"fixture-iliad-monro-1902", "1.1.1", "1.10.19", "μῆνιν ἄειδε θεὰ Πηληϊάδεω Ἀχιλῆος", &one, 3, 100),
		fixtureClassicalDefinition("fixture-iliad-2", "fixture-iliad", "Iliad", "Homer", "Ἰλιάς",
			"fixture-iliad-monro-1902", "2.1.1", "2.10.19", "ἄλλοι μέν ῥα θεοί τε καὶ ἀνέρες ἱπποκορυσταὶ", &two, 3, 200),
		fixtureClassicalDefinition("fixture-iliad-3", "fixture-iliad", "Iliad", "Homer", "Ἰλιάς",
			"fixture-iliad-monro-1902", "3.1.1", "3.10.19", "αὐτὰρ ἐπεὶ κόσμηθεν ἅμʼ ἡγεμόνεσσιν ἕκαστοι", &three, 3, 300),
	}
	for i := range withdrawn {
		withdrawn[i].Withdrawn = true
	}
	hellenica := fixtureClassicalDefinition("fixture-hellenica", "fixture-hellenica", "Hellenica", "Xenophon", "Ἑλληνικά",
		"fixture-hellenica-dindorf-1866", "1.1.1", "1.1.10", "μετὰ δὲ ταῦτα οὐ πολλαῖς ἡμέραις ὕστερον", nil, 1, 500)
	return append([]classical.Definition{odyssey, iliad, hellenica}, withdrawn...)
}

// SeedWithdrawnClassicalBooks models a learner who owned per-book units
// before their definitions were withdrawn.
func (s *Store) SeedWithdrawnClassicalBooks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, definition := range fixtureClassicalDefinitions() {
		if definition.Withdrawn {
			s.addFixtureClassicalReadingUnitLocked(OwnerID, definition)
		}
	}
	s.ensureFixtureSupportedLanguageLocked(classical.Language, classical.LanguageDisplayName)
}

func fixtureClassicalDefinitionsByKey() map[string]classical.Definition {
	definitions := fixtureClassicalDefinitions()
	byKey := make(map[string]classical.Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	return byKey
}

func fixtureClassicalSourceID(key string) string   { return "fixture-classical-source-" + key }
func fixtureClassicalRunID(key string) string      { return "fixture-classical-run-" + key }
func fixtureClassicalCorpusID(key string) string   { return "fixture-classical-corpus-" + key }
func fixtureClassicalSnapshotID(key string) string { return "fixture-classical-snapshot-" + key }

// fixtureClassicalSource is the completed acquisition and analysis the fixture
// projects for one To Read Reading unit.
func fixtureClassicalSource(owner, bookID string, unit domain.BookReadingUnit) *domain.SourceMaterialSummary {
	key := unit.ReadingUnitKey
	return &domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{
			ID:                fixtureClassicalSourceID(key),
			OwnerID:           owner,
			Language:          classical.Language,
			Title:             unit.WorkTitle,
			MediaType:         "text/plain",
			ContentRevisionID: fixtureClassicalSnapshotID(key) + "-revision",
			ContentSnapshotID: fixtureClassicalSnapshotID(key),
		},
		BookID:        bookID,
		BookTitle:     unit.WorkTitle,
		BookAuthor:    unit.WorkAuthor,
		Signals:       analyzedSignals,
		AnalysisRunID: fixtureClassicalRunID(key),
		CorpusID:      fixtureClassicalCorpusID(key),
	}
}

// ClassicalInsights gives the leftover fixture Books different coverage bands
// so the chooser can show independent evidence for each Book.
type ClassicalInsights struct{}

// Coverage returns the fixture Insights projection with the classical corpus
// bands adjusted per Reading unit.
func (ClassicalInsights) Coverage(ctx context.Context, owner, corpusID string) (domain.AnalysisCoverage, error) {
	coverage, err := (Insights{}).Coverage(ctx, owner, corpusID)
	if err != nil {
		return coverage, err
	}
	switch corpusID {
	case fixtureClassicalCorpusID("fixture-iliad-1"):
		coverage.KnownTokenCount = coverage.AnalyzableTokenCount
	case fixtureClassicalCorpusID("fixture-iliad-2"):
		coverage.KnownTokenCount = coverage.AnalyzableTokenCount * 98 / 100
	case fixtureClassicalCorpusID("fixture-iliad-3"):
		coverage.KnownTokenCount = coverage.AnalyzableTokenCount * 93 / 100
	}
	coverage.UnknownTokenCount = coverage.AnalyzableTokenCount - coverage.KnownTokenCount
	return coverage, nil
}
