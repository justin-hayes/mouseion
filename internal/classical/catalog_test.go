package classical

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogDefinitions() []Definition {
	first := validDefinition()
	second := first
	book := 2
	second.Key = "alpha-2"
	second.ConstituentBook = &book
	second.CitationStart = "1.1.3"
	second.CitationEnd = "1.1.4"
	second.WordCount = 12
	second.Translation = nil

	whole := validDefinition()
	whole.Key = "beta-1"
	whole.WorkKey = "beta"
	whole.WorkTitle = "Beta"
	whole.WorkAuthor = "Other"
	whole.GreekTitle = ""
	whole.EditionKey = "beta-test"
	whole.ConstituentBook = nil
	whole.ConstituentBooks = 1
	whole.WordCount = 5
	whole.Translation = nil

	withdrawn := validDefinition()
	withdrawn.Key = "gamma-1"
	withdrawn.WorkKey = "gamma"
	withdrawn.WorkTitle = "Gamma"
	withdrawn.WorkAuthor = "Hidden"
	withdrawn.EditionKey = "gamma-test"
	withdrawn.Withdrawn = true

	return []Definition{first, second, whole, withdrawn}
}

func TestCatalogHidesWithdrawnDefinitionsAndCountsTheRest(t *testing.T) {
	catalog, err := NewCatalog(catalogDefinitions())
	require.NoError(t, err)

	assert.Equal(t, CatalogCounts{Works: 2, Authors: 2, Units: 3}, catalog.Counts())
	_, hidden := catalog.Work("gamma")
	assert.False(t, hidden, "a withdrawn definition is hidden")
	_, hiddenAuthor := catalog.Author("Hidden")
	assert.False(t, hiddenAuthor, "an author with only withdrawn definitions is hidden")

	alpha, ok := catalog.Work("alpha")
	require.True(t, ok)
	assert.Equal(t, "Ἄλφα", alpha.GreekTitle)
	assert.Equal(t, 2, alpha.ConstituentBooks)
	assert.False(t, alpha.WholeWork)
	assert.Equal(t, 20, alpha.Words)
	assert.False(t, alpha.TranslationAvailable, "a work with one translated book and one without does not promise a translation")
	require.Len(t, alpha.Units, 2)
	assert.Equal(t, 1, alpha.Units[0].ConstituentBook)
	assert.Equal(t, 2, alpha.Units[1].ConstituentBook)

	beta, ok := catalog.Work("beta")
	require.True(t, ok)
	assert.True(t, beta.WholeWork)
	assert.Equal(t, 1, beta.ConstituentBooks)
	assert.Equal(t, 5, beta.Words)
	assert.Equal(t, 0, beta.Units[0].ConstituentBook, "a whole-work unit has no constituent book number")

	authors := catalog.Authors()
	require.Len(t, authors, 2)
	assert.Equal(t, "Author", authors[0].Name)
	assert.Equal(t, "Other", authors[1].Name)
	require.Len(t, authors[0].Works, 1)
	assert.Equal(t, "Alpha", authors[0].Works[0].Title)
}

func TestCatalogReportsATranslationOnlyWhenEveryBookHasOne(t *testing.T) {
	definitions := catalogDefinitions()
	definitions[1].Translation = &TranslationPin{
		Layer: "translation", FileName: "work2.xml", Size: 11, SHA256: strings.Repeat("c", 64),
		CitationStart: "1.1.3", CitationEnd: "1.1.4",
	}

	catalog, err := NewCatalog(definitions)
	require.NoError(t, err)
	alpha, ok := catalog.Work("alpha")
	require.True(t, ok)
	assert.True(t, alpha.TranslationAvailable)
}

func TestValidateManifestRefusesInconsistentDefinitions(t *testing.T) {
	t.Run("duplicate key", func(t *testing.T) {
		definitions := []Definition{validDefinition(), validDefinition()}
		require.ErrorContains(t, ValidateManifest(definitions), "repeats Reading-unit key")
	})
	t.Run("conflicting work title", func(t *testing.T) {
		definitions := catalogDefinitions()
		definitions[1].WorkTitle = "Another Alpha"
		require.ErrorContains(t, ValidateManifest(definitions), "conflicting titles")
	})
	t.Run("edition belongs to one work", func(t *testing.T) {
		definitions := catalogDefinitions()
		definitions[1].WorkKey = "another-work"
		require.ErrorContains(t, ValidateManifest(definitions), "belongs to more than one Work")
	})
	t.Run("repeated constituent book", func(t *testing.T) {
		definitions := catalogDefinitions()
		definitions[1].ConstituentBook = definitions[0].ConstituentBook
		require.ErrorContains(t, ValidateManifest(definitions), "repeats constituent book")
	})
	t.Run("empty manifest", func(t *testing.T) {
		require.ErrorContains(t, ValidateManifest(nil), "has no definitions")
	})
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, ValidateManifest(catalogDefinitions()))
	})
}

type recordingReconciler struct {
	definitions []Definition
	err         error
}

func (r *recordingReconciler) ReconcileClassicalManifest(_ context.Context, definitions []Definition) error {
	r.definitions = definitions
	return r.err
}

func TestReconcileParsesValidatesAndForwardsTheManifest(t *testing.T) {
	manifest, err := RenderDefinitions([]Definition{validDefinition()})
	require.NoError(t, err)
	recorder := &recordingReconciler{}
	require.NoError(t, Reconcile(context.Background(), recorder, manifest))
	require.Len(t, recorder.definitions, 1)
	assert.Equal(t, "alpha-1", recorder.definitions[0].Key)

	storeErr := errors.New("store failed")
	recorder = &recordingReconciler{err: storeErr}
	require.ErrorIs(t, Reconcile(context.Background(), recorder, manifest), storeErr)
}

func TestReconcileRefusesABrokenManifestBeforeTouchingTheStore(t *testing.T) {
	recorder := &recordingReconciler{}
	err := Reconcile(context.Background(), recorder, []byte(`{"key":"`))
	require.Error(t, err)
	assert.Empty(t, recorder.definitions)

	definition := validDefinition()
	definition.Language = "de"
	manifest, err := RenderDefinitions([]Definition{definition})
	require.NoError(t, err)
	recorder = &recordingReconciler{}
	require.Error(t, Reconcile(context.Background(), recorder, manifest))
	assert.Empty(t, recorder.definitions)
}

func TestCatalogUnitFindsAWorkByReadingUnitKey(t *testing.T) {
	catalog, err := NewCatalog(catalogDefinitions())
	require.NoError(t, err)

	work, unit, ok := catalog.Unit("alpha-2")
	require.True(t, ok)
	assert.Equal(t, "alpha", work.Key)
	assert.Equal(t, 2, unit.ConstituentBook)

	_, _, ok = catalog.Unit("no-such-unit")
	assert.False(t, ok)
}

func TestLoadCatalogReadsTheEmbeddedRelease(t *testing.T) {
	catalog, err := LoadCatalog(ManifestJSONL)
	require.NoError(t, err)
	assert.Equal(t, 7, catalog.Counts().Works)
	assert.Equal(t, 2, catalog.Counts().Authors)
	assert.Equal(t, 22, catalog.Counts().Units)
	work, ok := catalog.Work("xenophon-anabasis")
	require.True(t, ok)
	assert.Equal(t, "Anabasis", work.Title)
	for _, key := range []string{"xenophon-anabasis", "plato-apology", "plato-crito", "plato-phaedo", "plato-symposium", "plato-meno", "plato-republic"} {
		work, ok := catalog.Work(key)
		require.True(t, ok, key)
		assert.True(t, work.TranslationAvailable, key)
	}
}
