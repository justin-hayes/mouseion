//go:build integration

package prepareddeck

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

// idiomReviewBook seeds an analyzed Italian Book whose current deck
// preparation selects "gamba" in three "in gamba" sentences. No run is frozen
// and no idiom assessment is recorded.
func idiomReviewBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore) (bookID string, preparation domain.DeckPreparation) {
	t.Helper()
	owner, err := store.CreateUser(ctx, "idiom-review-"+uuid.NewString(), false)
	require.NoError(t, err)
	hash := uuid.NewString()
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: hash, Language: "it", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, nil))
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "it", SourceIdentifier: uuid.NewString(), Title: "Libro", MediaType: "text/plain", ContentHash: hash, Content: []byte("testo"), FullText: "testo"})
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
	require.NoError(t, err)
	preparation, err = store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash})
	require.NoError(t, err)

	sentences := make([]analyzer.Sentence, 0, 3)
	for i := range 3 {
		start := uint64(i * 40)
		text, tokens := integrationGambaSentence(start)
		for j := range tokens {
			tokens[j].Location.SourceDocumentID = source.ID
		}
		sentences = append(sentences, analyzer.Sentence{Text: text, Tokens: tokens, Location: analyzer.SourceLocation{SourceDocumentID: source.ID, StartOffset: start, EndOffset: start + uint64(len(text))}})
	}
	_, err = selection.NewService(store).Select(ctx, owner.ID, analyzer.Result{Language: "it", Sentences: sentences}, selection.DefaultConfig(corpus.ID))
	require.NoError(t, err)
	return book.ID, preparation
}

func idiomReviewFreezer(t *testing.T, store *persistence.PostgresStore) IdiomReviewFreezer {
	t.Helper()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "review-model"})
	require.NoError(t, err)
	lexicon := integrationIdiomLexicon{entry: idiom.Entry{
		Language: "it", Headword: "in gamba", UPOS: "ADJ", Words: []string{"in", "gamba"},
		Senses: []enrichment.LexicalSense{{EvidenceID: "wikt:capable", Gloss: "capable, on the ball"}},
		Source: "wiktionary", Version: "fixture-v1",
	}}
	return IdiomReviewFreezer{Store: store, Presentation: cardexport.NewPresentation(lexicon), Codec: codec}
}

// idiomReviewFixtureAssembler returns one "gamba" projection carrying the
// analyzed "in gamba" sentence for the preparation's owner.
type idiomReviewFixtureAssembler struct {
	t *testing.T
}

func (a idiomReviewFixtureAssembler) AssemblePreparedDeckInputs(_ context.Context, tx pgx.Tx, preparation domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	require.NotNil(a.t, tx, "the projections are assembled inside the review transaction")
	projection := integrationIdiomProjection(a.t)
	projection.OwnerID = preparation.OwnerID
	projection.Candidate.OwnerID = preparation.OwnerID
	return []cardexport.CandidateProjection{projection}, projection.DeckName, nil
}

// idiomReviewAssemblerFunc adapts a function to the input assembler.
type idiomReviewAssemblerFunc func(domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error)

func (f idiomReviewAssemblerFunc) AssemblePreparedDeckInputs(_ context.Context, _ pgx.Tx, preparation domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	return f(preparation)
}

func TestIdiomReviewFreezerRejectsDecksItCannotSample(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	bookID, _ := idiomReviewBook(t, ctx, store)

	t.Run("a source-evidence deck carries no idiom cards", func(t *testing.T) {
		freezer := idiomReviewFreezer(t, store)
		freezer.Assembler = idiomReviewAssemblerFunc(func(domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
			projection := integrationIdiomProjection(t)
			projection.Candidate.Language = "grc"
			return []cardexport.CandidateProjection{projection}, projection.DeckName, nil
		})
		_, freezeErr := freezer.FreezeIdiomReviewItems(ctx, bookID, "it")
		require.ErrorIs(t, freezeErr, errIdiomReviewSourceEvidenceDeck)
	})
	t.Run("an assembler failure is reported", func(t *testing.T) {
		freezer := idiomReviewFreezer(t, store)
		freezer.Assembler = idiomReviewAssemblerFunc(func(domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
			return nil, "", errors.New("assembly failed")
		})
		_, freezeErr := freezer.FreezeIdiomReviewItems(ctx, bookID, "it")
		require.ErrorContains(t, freezeErr, "assembly failed")
	})
}

func TestIdiomReviewFreezerReFreezesTheCurrentPreparationWithoutWriting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	bookID, preparation := idiomReviewBook(t, ctx, store)
	freezer := idiomReviewFreezer(t, store)
	freezer.Assembler = idiomReviewFixtureAssembler{t: t}

	freeze, err := freezer.FreezeIdiomReviewItems(ctx, bookID, "it")
	require.NoError(t, err)
	items := freeze.Items

	t.Run("the idiom item is frozen without a recorded assessment", func(t *testing.T) {
		require.Len(t, items, 1)
		require.Equal(t, "in gamba", items[0].Headword)
		require.Equal(t, "gamba", items[0].Evidence.AnchorCanonicalLemma)
	})
	t.Run("the request is the prepared-deck idiom request", func(t *testing.T) {
		require.Len(t, items, 1)
		request := items[0].Work.Request
		require.True(t, request.RequireIdiomCard)
		require.Equal(t, "in gamba", request.IdiomHeadword)
		require.Equal(t, []string{"in", "gamba"}, request.IdiomMatchedForms)
		require.Equal(t, "È davvero un ragazzo in gamba.", request.ExampleSentence)
		require.Equal(t, freezer.Codec.IdiomCardProviderVersion(), items[0].Work.CacheKey.ProviderVersion)
	})
	t.Run("the bound's ranking lists the idiom", func(t *testing.T) {
		require.Len(t, freeze.Ranking, 1)
		require.Equal(t, "in gamba", freeze.Ranking[0].Headword)
	})
	t.Run("another language has no idiom items", func(t *testing.T) {
		other, otherErr := freezer.FreezeIdiomReviewItems(ctx, bookID, "de")
		require.NoError(t, otherErr)
		require.Empty(t, other.Items)
		require.Empty(t, other.Ranking)
	})
	t.Run("the real input assembler runs in the read-only transaction", func(t *testing.T) {
		assembled := idiomReviewFreezer(t, store)
		_, realErr := assembled.FreezeIdiomReviewItems(ctx, bookID, "it")
		require.NoError(t, realErr)
	})
	t.Run("nothing is written", func(t *testing.T) {
		var runs int
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparation_runs WHERE preparation_id=$1`, preparation.ID).Scan(&runs))
		require.Zero(t, runs)
		current, getErr := store.GetDeckPreparation(ctx, preparation.OwnerID, preparation.ID)
		require.NoError(t, getErr)
		require.Equal(t, preparation.State, current.State)
		require.Equal(t, preparation.UpdatedAt, current.UpdatedAt)
	})
}

func TestIdiomReviewFreezerRejectsABookWithoutAPreparation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	freezer := idiomReviewFreezer(t, store)

	t.Run("unknown Book", func(t *testing.T) {
		_, freezeErr := freezer.FreezeIdiomReviewItems(ctx, uuid.NewString(), "it")
		require.ErrorIs(t, freezeErr, persistence.ErrNotFound)
	})
	t.Run("invalid input", func(t *testing.T) {
		_, freezeErr := freezer.FreezeIdiomReviewItems(ctx, " ", "it")
		require.ErrorIs(t, freezeErr, ErrInvalidInput)
		_, freezeErr = IdiomReviewFreezer{}.FreezeIdiomReviewItems(ctx, uuid.NewString(), "it")
		require.ErrorIs(t, freezeErr, ErrInvalidInput)
	})
}
