package classical

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogDefinitions() []Definition {
	first := validWholeWorkDefinition()

	whole := validDefinition()
	whole.Key = "beta"
	whole.WorkKey = "beta"
	whole.WorkTitle = "Beta"
	whole.WorkAuthor = "Other"
	whole.OriginalTitle = ""
	whole.EditionKey = "beta-test"
	whole.ConstituentBook = nil
	whole.ConstituentBooks = 1
	whole.WordCount = 5
	whole.Opening = "ἄλλος λόγος."
	whole.Translation = nil

	withdrawn := validDefinition()
	withdrawn.Key = "gamma-1"
	withdrawn.WorkKey = "gamma"
	withdrawn.WorkTitle = "Gamma"
	withdrawn.WorkAuthor = "Hidden"
	withdrawn.EditionKey = "gamma-test"
	withdrawn.Withdrawn = true

	return []Definition{first, whole, withdrawn}
}

// withSecondEdition inserts a second Edition of Work alpha at index 1.
func withSecondEdition(definitions []Definition) []Definition {
	second := definitions[0]
	second.Key = "alpha-other"
	second.EditionKey = "alpha-other-test"
	second.EditionLabel = "Other Edition"
	return append([]Definition{definitions[0], second}, definitions[1:]...)
}

func TestCatalogCreditsEachSourceLayerOnce(t *testing.T) {
	definitions := catalogDefinitions()
	for i := range definitions {
		if !definitions[i].Withdrawn {
			definitions[i].Source.Layer = "oga-src-v0.2.0-text"
		}
	}
	catalog, err := NewCatalog(definitions)
	require.NoError(t, err)

	attributions := catalog.SourceAttributions()
	require.Len(t, attributions, 1, "two Works on one layer produce one credit")
	assert.Equal(t, "oga-src-v0.2.0-text", attributions[0].Layer)
	assert.Contains(t, attributions[0].Text, "OGA oga_src v0.2.0")
	assert.Contains(t, attributions[0].Text, "CC BY-SA 4.0")
}

func TestCatalogHidesWithdrawnDefinitionsAndCountsTheRest(t *testing.T) {
	catalog, err := NewCatalog(catalogDefinitions())
	require.NoError(t, err)

	assert.Equal(t, CatalogCounts{Works: 2, Authors: 2, Units: 2}, catalog.Counts())
	_, hidden := catalog.Work("gamma")
	assert.False(t, hidden, "a withdrawn definition is hidden")
	_, hiddenAuthor := catalog.Author("Hidden")
	assert.False(t, hiddenAuthor, "an author with only withdrawn definitions is hidden")

	alpha, ok := catalog.Work("alpha")
	require.True(t, ok)
	assert.Equal(t, "Ἄλφα", alpha.OriginalTitle)
	assert.Equal(t, "Test Edition", alpha.EditionLabel)
	assert.Equal(t, 2, alpha.ConstituentBooks)
	assert.Equal(t, 14, alpha.Words)
	assert.Equal(t, "ἄρχεται ὁ λόγος.", alpha.Opening)
	assert.True(t, alpha.TranslationAvailable)
	require.Len(t, alpha.Units, 1)
	assert.Zero(t, alpha.Units[0].ConstituentBook, "a whole-work unit has no constituent book number")
	assert.Len(t, alpha.Units[0].Contents, 2)

	beta, ok := catalog.Work("beta")
	require.True(t, ok)
	assert.Equal(t, 1, beta.ConstituentBooks)
	assert.Equal(t, 5, beta.Words)
	assert.Equal(t, "ἄλλος λόγος.", beta.Opening)
	assert.Equal(t, 0, beta.Units[0].ConstituentBook, "a whole-work unit has no constituent book number")

	authors := catalog.Authors()
	require.Len(t, authors, 2)
	assert.Equal(t, "Author", authors[0].Name)
	assert.Equal(t, "Other", authors[1].Name)
	require.Len(t, authors[0].Works, 1)
	assert.Equal(t, "Alpha", authors[0].Works[0].Title)
}

func TestCatalogReportsATranslationOnlyWhenTheEditionHasOne(t *testing.T) {
	definitions := catalogDefinitions()
	definitions[0].Translation = nil

	catalog, err := NewCatalog(definitions)
	require.NoError(t, err)
	alpha, ok := catalog.Work("alpha")
	require.True(t, ok)
	assert.False(t, alpha.TranslationAvailable)
}

func TestValidateManifestRefusesInconsistentDefinitions(t *testing.T) {
	t.Run("duplicate key", func(t *testing.T) {
		definitions := []Definition{validWholeWorkDefinition(), validWholeWorkDefinition()}
		require.ErrorContains(t, ValidateManifest(definitions), "repeats Reading-unit key")
	})
	t.Run("conflicting work title", func(t *testing.T) {
		definitions := withSecondEdition(catalogDefinitions())
		definitions[1].WorkTitle = "Another Alpha"
		require.ErrorContains(t, ValidateManifest(definitions), "conflicting titles")
	})
	t.Run("edition belongs to one work", func(t *testing.T) {
		definitions := withSecondEdition(catalogDefinitions())
		definitions[1].WorkKey = "another-work"
		definitions[1].EditionKey = definitions[0].EditionKey
		require.ErrorContains(t, ValidateManifest(definitions), "belongs to more than one Work")
	})
	t.Run("empty opening", func(t *testing.T) {
		definitions := catalogDefinitions()
		definitions[0].Opening = ""
		require.ErrorContains(t, ValidateManifest(definitions), "has no Opening")
	})
	t.Run("withdrawn tombstone needs no opening", func(t *testing.T) {
		definitions := catalogDefinitions()
		definitions[2].Opening = ""
		require.NoError(t, ValidateManifest(definitions))
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
	manifest, err := RenderDefinitions([]Definition{validWholeWorkDefinition()})
	require.NoError(t, err)
	recorder := &recordingReconciler{}
	require.NoError(t, Reconcile(context.Background(), recorder, manifest))
	require.Len(t, recorder.definitions, 1)
	assert.Equal(t, "alpha", recorder.definitions[0].Key)

	storeErr := errors.New("store failed")
	recorder = &recordingReconciler{err: storeErr}
	require.ErrorIs(t, Reconcile(context.Background(), recorder, manifest), storeErr)
}

func TestReconcileRefusesABrokenManifestBeforeTouchingTheStore(t *testing.T) {
	recorder := &recordingReconciler{}
	err := Reconcile(context.Background(), recorder, []byte(`{"key":"`))
	require.Error(t, err)
	assert.Empty(t, recorder.definitions)

	definition := validWholeWorkDefinition()
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

	work, unit, ok := catalog.Unit("alpha")
	require.True(t, ok)
	assert.Equal(t, "alpha", work.Key)
	assert.Equal(t, "alpha", unit.Key)

	_, _, ok = catalog.Unit("no-such-unit")
	assert.False(t, ok)
}

func TestLoadCatalogReadsTheEmbeddedRelease(t *testing.T) {
	catalog, err := LoadCatalog(ManifestJSONL)
	require.NoError(t, err)
	assert.Equal(t, 18, catalog.Counts().Works, "7 Greek and 11 Latin Works")
	assert.Equal(t, 6, catalog.Counts().Authors)
	assert.Equal(t, 18, catalog.Counts().Units, "each Work is one whole-work Reading unit")
	work, ok := catalog.Work("xenophon-anabasis")
	require.True(t, ok)
	assert.Equal(t, "Anabasis", work.Title)
	assert.Equal(t, 7, work.ConstituentBooks)
	require.Len(t, work.Units, 1)
	assert.Len(t, work.Units[0].Contents, 7)
	for _, key := range []string{
		"xenophon-anabasis", "plato-apology", "plato-crito", "plato-phaedo", "plato-symposium", "plato-meno", "plato-republic",
		"caesar-gallic-war", "sallust-catiline", "sallust-jugurthine-war", "cicero-in-catilinam", "cicero-pro-archia", "cicero-pro-milone",
		"cicero-pro-caelio", "cicero-de-amicitia", "cicero-de-senectute", "tacitus-agricola", "tacitus-germania",
	} {
		work, ok := catalog.Work(key)
		require.True(t, ok, key)
		assert.True(t, work.TranslationAvailable, key)
	}
}

func TestCatalogListsTheClassicalLanguagesPresent(t *testing.T) {
	definitions := catalogDefinitions()
	greekOnly, err := NewCatalog(definitions)
	require.NoError(t, err)
	assert.Equal(t, []string{"grc"}, greekOnly.Languages())

	latin := validWholeWorkDefinition()
	latin.Key, latin.WorkKey, latin.EditionKey = "gamma-la", "gamma-la", "gamma-la-test"
	latin.Language = LanguageLatin
	both, err := NewCatalog(append(definitions, latin))
	require.NoError(t, err)
	assert.Equal(t, []string{"grc", "la"}, both.Languages())
	work, ok := both.Work("gamma-la")
	require.True(t, ok)
	assert.Equal(t, "la", work.Language)
}
