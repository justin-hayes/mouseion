package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassicalAddConfirmationNamesTheAddedLanguage(t *testing.T) {
	report := classical.AddReport{Books: []classical.AddedBook{{ReadingUnitKey: "caesar-gallic-war", BookID: "book-1", Created: true}}}

	t.Run("another study language names Latin", func(t *testing.T) {
		other := classicalAddConfirmationIn(report, classicalAddPreparation{}, "de", classical.LanguageLatin)

		require.Equal(t, "Added 1 Book to My Books in Latin. Your study language is unchanged.", other.Message)
		assert.Equal(t, "book-1", other.BookID)
	})
	t.Run("Latin active names no other language", func(t *testing.T) {
		same := classicalAddConfirmationIn(report, classicalAddPreparation{}, "la", classical.LanguageLatin)

		assert.Equal(t, "Added 1 Book to My Books.", same.Message)
	})
	t.Run("a Latin Add never claims Ancient Greek", func(t *testing.T) {
		greekActive := classicalAddConfirmationIn(report, classicalAddPreparation{}, "grc", classical.LanguageLatin)

		assert.Contains(t, greekActive.Message, "in Latin")
	})
}

func TestClassicalCatalogLinksBackToTheWorksLanguage(t *testing.T) {
	t.Run("Latin", func(t *testing.T) {
		require.Equal(t, "/catalogs?language=la", classicalCatalogURLIn(classical.LanguageLatin))
	})
	t.Run("Ancient Greek", func(t *testing.T) {
		assert.Equal(t, classicalCatalogURL(), classicalCatalogURLIn(classical.Language))
	})
}
