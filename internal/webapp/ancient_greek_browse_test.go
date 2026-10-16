package webapp

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

// TestVocabularyBrowseRowShowsCitationHeadwordAndBareLemmaNote pins the
// learner-facing Browse row for an imported classical identity: the citation
// headword, never the stored canonical key, and the bare-lemma label.
func TestVocabularyBrowseRowShowsCitationHeadwordAndBareLemmaNote(t *testing.T) {
	page := domain.VocabularyBrowsePage{
		Language: "grc",
		Books:    []domain.VocabularyBrowseBook{{ID: "book-1", HasCurrentAnalysis: true, HasVocabularyEvidence: true}},
		Total:    2, InventoryTotal: 2, ScopedInventoryTotal: 2,
		Rows: []domain.VocabularyBrowseRow{
			{CanonicalLemma: "λόγοσ", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1", Headword: "λόγος", OccurrenceCount: 1, AcrossBooksOccurrenceCount: 1},
			{CanonicalLemma: "λόγοσ", UPOS: "NOUN", Headword: "λόγος", OccurrenceCount: 1, AcrossBooksOccurrenceCount: 1, BareLemmaWithNumberedSibling: true},
		},
	}
	html := renderReadingBrowse(t, "grc", page, "")
	assert.Contains(t, html, ">λόγος</a>", "the headword is shown")
	assert.NotContains(t, html, ">λόγοσ</a>", "the stored canonical key is never shown")
	assert.Equal(t, 1, strings.Count(html, "the source gives no homograph number"), "only the bare lemma is labelled")
}

// TestBrowseRowKeyDistinguishesHomographs proves a discriminator distinguishes
// two rows sharing a canonical lemma and part of speech, while a modern-language
// row's key is unchanged.
func TestBrowseRowKeyDistinguishesHomographs(t *testing.T) {
	assert.Equal(t, "VERB:δέω", browseRowKey("VERB", "δέω", ""), "a modern-language key is unchanged")
	assert.NotEqual(t, browseRowKey("VERB", "δέω", "morpheus:1"), browseRowKey("VERB", "δέω", "morpheus:2"))
}
