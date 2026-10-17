//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekCardPassageContext proves the providerless Greek card back
// shows Brownson passage context for the cited sections its representative
// sentence touches: a single section, a section-crossing sentence that shows
// both, and a sentence whose section has no translation key, which shows no
// English. Nothing in a passage highlights a target, the exported deck carries
// the Brownson/Perseus notice, and a re-import never changes a frozen passage.
func TestAncientGreekCardPassageContext(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "ancient-greek-passage-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	learner := createAccount(t, ctx, store, "greek-passage-learner", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, registration.BookID, "oga-src-passage.conllu", ancientgreek.AnabasisBook1)
	importBrownsonFixture(t, ctx, store, learner.ID, "perseus-eng2-passage.xml")
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, registration.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	provider := &greekDeckRecordingProvider{}
	client := startGreekDeckWorkers(t, ctx, store, pool, provider)
	deckService := prepareddeck.NewService(store, client)
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), client), PreparedDeck: deckService,
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	started := perform(t, h, http.MethodPost, "/reading/books/"+registration.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, "start body=%s", started.Body.String())
	reading, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	require.Equal(t, 3, reading.SnapshotSize, "λόγος, δέω 1 and δέω 2 are the recurring Book vocabulary")

	submitted := perform(t, h, http.MethodPost, "/reading/books/"+registration.BookID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, "prepare body=%s", submitted.Body.String())
	preparation := waitForPreparedDeck(t, ctx, store, learner.ID, reading.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, preparation.State, "preparation error=%q", preparation.Error)
	assert.Equal(t, 3, preparation.TotalCards, "one card per frozen identity")
	assert.Zero(t, provider.calls, "a Greek deck makes no provider request")

	items := queryPassageItems(t, ctx, pool, learner.ID)
	require.Len(t, items, 3)

	// A single-section sentence shows that section and nothing is highlighted.
	logos := items["λόγοσ\x00"]
	require.NotEmpty(t, logos.brownsonPin, "the Brownson pin is frozen on the card")
	assert.Equal(t, "1.2.17", logos.passageCitation)
	assert.Contains(t, logos.passageContext, "He halted his chariot in front of the phalanx and gave the order to advance.")
	assert.NotContains(t, logos.passageContext, "<b>", "the passage highlights nothing")
	assert.NotContains(t, logos.passageContext, "</b>", "the passage highlights nothing")
	assert.NotContains(t, logos.passageContext, "must be removed", "a Perseus note is removed")

	// A crossing sentence shows both sections as a range.
	deo1 := items["δέω\x00morpheus:1"]
	assert.Equal(t, "1.2.17\u201318", deo1.passageCitation)
	assert.Contains(t, deo1.passageContext, "He halted his chariot in front of the phalanx and gave the order to advance.")
	assert.Contains(t, deo1.passageContext, "As for the barbarians, they were frightened; the Cilician queen took to flight.")
	assert.NotContains(t, deo1.passageContext, "<b>", "the crossing passage highlights nothing")
	assert.NotContains(t, deo1.passageContext, "</b>", "the crossing passage highlights nothing")

	// A touched section with no translation key withholds the whole passage.
	deo2 := items["δέω\x00morpheus:2"]
	assert.Empty(t, deo2.passageContext, "a missing translation key leaves the card with no English")
	assert.Empty(t, deo2.passageCitation)
	assert.Empty(t, deo2.brownsonPin, "a card with no passage freezes no pin")

	// The exported deck carries the passage and the Brownson/Perseus notice.
	artifact := preparation.Artifact
	require.NotEmpty(t, artifact)
	assert.Contains(t, string(artifact), "He halted his chariot in front of the phalanx")
	assert.Contains(t, string(artifact), "Brownson (1921-22)")
	assert.Contains(t, string(artifact), "CC BY-SA 4.0")
	assert.Contains(t, string(artifact), "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7")

	// The stored card back carries the labelled passage with no highlight.
	back := queryCardBack(t, ctx, pool, learner.ID, "λόγοσ")
	assert.Contains(t, back, "Passage context: Brownson (1922), Xen. Anab. 1.2.17. Not a translation of this sentence")
	assert.NotContains(t, back, "<b>", "a card back never highlights the passage")

	// A re-import of the pinned Brownson file never changes the frozen deck.
	importBrownsonFixture(t, ctx, store, learner.ID, "perseus-eng2-passage.xml")
	unchanged := waitForPreparedDeck(t, ctx, store, learner.ID, reading.SnapshotID)
	assert.Equal(t, artifact, unchanged.Artifact, "a re-import never changes a frozen deck")
	assert.Equal(t, items, queryPassageItems(t, ctx, pool, learner.ID))
}

// importBrownsonFixture imports a synthetic perseus-eng2 fixture with a pin
// computed from its bytes; the real pinned file never enters the repository.
func importBrownsonFixture(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, name string) ancientgreek.BrownsonImportReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", name)) //nolint:gosec // the fixture name is a test constant.
	require.NoError(t, err)
	source := ancientgreek.BrownsonAnabasisBook1
	source.FileSize = int64(len(data))
	sum := sha256.Sum256(data)
	source.FileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	report, err := ancientgreek.ImportBrownson(ctx, ancientgreek.BrownsonImportRequest{
		OwnerID: ownerID, SourcePath: path, Source: source,
	}, analysis.NewReferenceTranslationWriter(store))
	require.NoError(t, err)
	return report
}

type passageItem struct {
	passageContext, passageCitation, brownsonPin string
}

// queryPassageItems loads every accepted frozen item's passage keyed by lemma
// and discriminator.
func queryPassageItems(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID string) map[string]passageItem {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT canonical_lemma, lexical_discriminator, passage_context, passage_citation, brownson_pin
		FROM deck_preparation_manifest_items WHERE owner_id=$1 AND disposition='accepted'`, ownerID)
	require.NoError(t, err)
	defer rows.Close()
	items := map[string]passageItem{}
	for rows.Next() {
		var lemma, discriminator string
		var item passageItem
		require.NoError(t, rows.Scan(&lemma, &discriminator, &item.passageContext, &item.passageCitation, &item.brownsonPin))
		items[lemma+"\x00"+discriminator] = item
	}
	require.NoError(t, rows.Err())
	return items
}

func queryCardBack(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, lemma string) string {
	t.Helper()
	var back string
	require.NoError(t, pool.QueryRow(ctx, `SELECT back FROM cards WHERE owner_id=$1 AND canonical_lemma=$2`, ownerID, lemma).Scan(&back))
	return back
}
