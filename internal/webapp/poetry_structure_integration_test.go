//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// switchPoetrySourceToV2 adds a v2 snapshot with a recorded speaker label and
// standalone direction, then makes it the source's and run's exact snapshot.
func switchPoetrySourceToV2(t *testing.T, ctx context.Context, store *persistence.PostgresStore, source domain.SourceMaterial, text string, speakerEnd, directionStart int) string {
	t.Helper()
	var snapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `
INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, content_revision_id, schema_version, structure_convention_set)
VALUES($1,$2,$3,$4,$5) RETURNING snapshot_id::text`,
		source.OwnerID, source.ID, source.ContentRevisionID, domain.ExtractedUnitsSchemaVersionV2, domain.StructureConventionSet).Scan(&snapshotID))
	unitID := domain.EPUBUnitID(0, "poetry")
	_, err := store.Pool().Exec(ctx, `
INSERT INTO source_material_units(owner_id, source_material_id, snapshot_id, unit_id, unit_order, spine_index, title, title_source, text,
                                  start_offset, end_offset, package_path, manifest_id, source_href, resolved_href, media_type,
                                  properties, linear, navigation_labels, landmark_types)
VALUES($1,$2,$3,$4,0,0,'','heading',$5,0,$6,'','poetry','','','application/xhtml+xml','[]'::jsonb,true,'[]'::jsonb,'[]'::jsonb)`,
		source.OwnerID, source.ID, snapshotID, unitID, text, len([]rune(text)))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
INSERT INTO source_material_unit_role_spans(owner_id, source_material_id, snapshot_id, unit_id, role, start_offset, end_offset)
VALUES($1,$2,$3,$4,'speaker',0,$5),($1,$2,$3,$4,'stage_direction',$6,$7)`,
		source.OwnerID, source.ID, snapshotID, unitID, speakerEnd, directionStart, len([]rune(text)))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET snapshot_id=$3 WHERE owner_id=$1 AND source_material_id=$2`, source.OwnerID, source.ID, snapshotID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$3 WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID, snapshotID)
	require.NoError(t, err)
	return snapshotID
}

// TestPoetrySpeakerLabelRefusesStudyAndLabelsConcordance walks the HTTP
// boundary: the label stays searchable and named, its sentence offers no Study,
// a direct request is refused by the same rule, and a standalone direction
// sentence remains available.
func TestPoetrySpeakerLabelRefusesStudyAndLabelsConcordance(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "poetry-structure-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "poetry-alice", "alice-password")
	book, source, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "poetry", "Faust", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "valentin", UPOS: "PROPN", OccurrenceCount: 1}})

	const text = "Valentin fällt Faust tritt ein"
	switchPoetrySourceToV2(t, ctx, store, source, text, 14, 15)
	seedLookupSentences(t, ctx, store, book, "de", [][]lookupToken{
		{{"Valentin", "Valentin", "PROPN"}, {"fällt", "fallen", "VERB"}},
		{{"Faust", "Faust", "PROPN"}, {"tritt", "treten", "VERB"}, {"ein", "ein", "ADV"}},
	})

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "poetry-alice", "alice-password")
	require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, "de"))

	t.Run("speaker label stays searchable and labelled without a Study affordance", func(t *testing.T) {
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance?"+url.Values{"language": {"de"}, "term": {"Valentin"}}.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		require.Contains(t, body, ">Speaker label</span>")
		require.NotContains(t, lookupResults(body), `class="concordance-study-link"`)
	})

	t.Run("standalone direction sentence keeps its Study affordance", func(t *testing.T) {
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance?"+url.Values{"language": {"de"}, "term": {"tritt"}}.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		require.NotContains(t, lookupResults(body), "Speaker label")
		require.Contains(t, lookupResults(body), `class="concordance-study-link"`)
	})

	var runID, corpusID, unitID string
	require.NoError(t, store.Pool().QueryRow(ctx, `
SELECT cai.analysis_run_id::text, cai.corpus_id::text, u.unit_id
FROM current_analysis_identity cai
JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id AND u.snapshot_id=cai.snapshot_id
WHERE cai.book_id=$1`, book.ID).Scan(&runID, &corpusID, &unitID))

	studyURL := func(sentence, target int, surface string) string {
		return fmt.Sprintf("/vocabulary/concordance/sentence?language=de&book=%s&run=%s&corpus=%s&unit=%s&sentence=%d&target=%d&target_surface=%s",
			book.ID, runID, corpusID, unitID, sentence, target, url.QueryEscape(surface))
	}

	t.Run("a direct Study request for a label-overlapping sentence is refused", func(t *testing.T) {
		response := perform(t, h, http.MethodGet, studyURL(0, 0, "Valentin"), nil, cookies)
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Contains(t, response.Body.String(), "overlaps a speaker label")
	})

	t.Run("the same direct request for the direction sentence still renders Study", func(t *testing.T) {
		response := perform(t, h, http.MethodGet, studyURL(1, 0, "Faust"), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), "Faust tritt ein")
	})
}
