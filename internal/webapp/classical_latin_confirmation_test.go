package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/assert"
)

func TestClassicalAddConfirmationNamesTheAddedLanguage(t *testing.T) {
	report := classical.AddReport{Books: []classical.AddedBook{{ReadingUnitKey: "caesar-gallic-war", BookID: "book-1", Created: true}}}

	other := classicalAddConfirmationIn(report, classicalAddPreparation{}, "de", classical.LanguageLatin)
	assert.Equal(t, "Added 1 Book to My Books in Latin. Your study language is unchanged.", other.Message)
	assert.Equal(t, "book-1", other.BookID)

	same := classicalAddConfirmationIn(report, classicalAddPreparation{}, "la", classical.LanguageLatin)
	assert.Equal(t, "Added 1 Book to My Books.", same.Message, "Latin active names no other language")

	greekActive := classicalAddConfirmationIn(report, classicalAddPreparation{}, "grc", classical.LanguageLatin)
	assert.Contains(t, greekActive.Message, "in Latin", "a Latin Add never claims Ancient Greek")
}

func TestClassicalCatalogLinksBackToTheWorksLanguage(t *testing.T) {
	assert.Equal(t, "/catalogs?language=la", classicalCatalogURLIn(classical.LanguageLatin))
	assert.Equal(t, classicalCatalogURL(), classicalCatalogURLIn(classical.Language))
}
