//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dispositionCutoverScenario is a realistic legacy database: every history
// category as Set Aside, plus the state the conversion must never touch.
type dispositionCutoverScenario struct {
	Owner, OtherOwner string

	Unread, Assertion, Completion, Mixed, HiddenLegacy string // legacy Set Aside
	Inbox, ToRead, HiddenToRead                        string // preserved
	Active                                             string // Current reading with active snapshot
	CompletionDeckID, ActiveDeckID, PreparingDeckID    string
	OtherOwnerLegacy                                   string
}

func seedLegacySetAside(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, books ...string) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	// Reproduce a production database that was migrated structurally (000033)
	// while still holding legacy rows: only a NOT VALID constraint can coexist.
	for _, statement := range []string{
		`ALTER TABLE book_dispositions DROP CONSTRAINT book_dispositions_disposition_check`,
	} {
		_, err = tx.Exec(ctx, statement)
		require.NoError(t, err)
	}
	for _, book := range books {
		_, err = tx.Exec(ctx, `UPDATE book_dispositions SET disposition='set_aside', revision=revision+1 WHERE owner_id=$1 AND book_id=$2`, owner, book)
		require.NoError(t, err)
	}
	_, err = tx.Exec(ctx, `ALTER TABLE book_dispositions ADD CONSTRAINT book_dispositions_disposition_check
		CHECK (disposition = ANY (ARRAY['inbox'::text, 'to_read'::text])) NOT VALID`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
}

func newCutoverBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, title string) domain.Book {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	return book
}

func seedDispositionCutoverScenario(t *testing.T, ctx context.Context, store *PostgresStore, pool *pgxpool.Pool) dispositionCutoverScenario {
	t.Helper()
	owner, err := store.CreateUser(ctx, "cutover-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "cutover-other", false)
	require.NoError(t, err)
	s := dispositionCutoverScenario{Owner: owner.ID, OtherOwner: other.ID}

	s.Unread = newCutoverBook(t, ctx, store, owner.ID, "Unread legacy").ID
	assertion := newCutoverBook(t, ctx, store, owner.ID, "Assertion legacy")
	s.Assertion = assertion.ID
	_, err = pool.Exec(ctx, `INSERT INTO reading_history(owner_id, language, book_id, completed_at, completion_source) VALUES ($1,'de',$2,now(),'previously_read_import')`, owner.ID, assertion.ID)
	require.NoError(t, err)
	mixed := newCutoverBook(t, ctx, store, owner.ID, "Mixed legacy")
	s.Mixed = mixed.ID
	_, err = pool.Exec(ctx, `INSERT INTO reading_history(owner_id, language, book_id, completed_at, completion_source) VALUES ($1,'de',$2,now(),'primary_goal'), ($1,'it',$2,now(),'previously_read_import')`, owner.ID, mixed.ID)
	require.NoError(t, err)

	// Completion: a real Finish with a frozen snapshot, Known vocabulary, and a
	// ready deck artifact that must outlive the conversion.
	completion, completionSource, completionDeck := createReadingFixture(t, ctx, store, owner.ID, "cutover-completion")
	makeAnalyzedToReadBook(t, ctx, store, completion, completionSource)
	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", completion.ID)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", completion.ID, reading.SnapshotID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos) VALUES ($1,'de','gehen','VERB')`, owner.ID)
	require.NoError(t, err)
	s.Completion, s.CompletionDeckID = completion.ID, completionDeck.ID

	// An unread legacy Book with a preparation still in progress.
	preparing := newCutoverBook(t, ctx, store, owner.ID, "Preparing legacy")
	preparingSource := putBookSourceInLanguage(t, ctx, store, owner.ID, "de", "reading-cutover-preparing", "Preparing", []byte("cutover-preparing"), "preparing")
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, preparing.ID, preparingSource.ID))
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: preparingSource.ID, Filename: "preparing.apkg", DeckName: "Preparing", ContentHash: preparingSource.ContentHash})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, prep.ID)
	require.NoError(t, err)
	s.PreparingDeckID = prep.ID
	s.HiddenLegacy = preparing.ID

	s.Inbox = newCutoverBook(t, ctx, store, owner.ID, "Plain inbox").ID
	toRead := newCutoverBook(t, ctx, store, owner.ID, "Plain to read")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, toRead.ID, domain.BookDispositionToRead))
	s.ToRead = toRead.ID
	hiddenToRead := newCutoverBook(t, ctx, store, owner.ID, "Hidden to read")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, hiddenToRead.ID, domain.BookDispositionToRead))
	applied, err := store.SetBookHidden(ctx, owner.ID, hiddenToRead.ID, 0, true)
	require.NoError(t, err)
	require.True(t, applied)
	s.HiddenToRead = hiddenToRead.ID
	// The preparing legacy Book is also Hidden: the conversion must keep it so.
	applied, err = store.SetBookHidden(ctx, owner.ID, preparing.ID, 0, true)
	require.NoError(t, err)
	require.True(t, applied)

	active, activeSource, activeDeck := createReadingFixture(t, ctx, store, owner.ID, "cutover-active")
	makeAnalyzedToReadBook(t, ctx, store, active, activeSource)
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", active.ID)
	require.NoError(t, err)
	s.Active, s.ActiveDeckID = active.ID, activeDeck.ID

	// A second learner proves nothing assumes a single owner.
	s.OtherOwnerLegacy = newCutoverBook(t, ctx, store, other.ID, "Other owner legacy").ID

	seedLegacySetAside(t, ctx, pool, owner.ID, s.Unread, s.Assertion, s.Completion, s.Mixed, s.HiddenLegacy)
	seedLegacySetAside(t, ctx, pool, other.ID, s.OtherOwnerLegacy)
	return s
}

func dispositionRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, book string) (string, int64) {
	t.Helper()
	var disposition string
	var revision int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT disposition, revision FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, owner, book).Scan(&disposition, &revision))
	return disposition, revision
}

func cutoverCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, pool.QueryRow(ctx, query, args...).Scan(&n))
	return n
}

func plannedManifest(t *testing.T, ctx context.Context, store *PostgresStore) DispositionCutoverManifest {
	t.Helper()
	manifest, err := store.PlanBookDispositionCutover(ctx)
	require.NoError(t, err)
	return manifest
}

func preservationByName(m DispositionCutoverManifest) map[string]DispositionCutoverPreservation {
	out := make(map[string]DispositionCutoverPreservation, len(m.Preservation))
	for _, item := range m.Preservation {
		out[item.Name] = item
	}
	return out
}

func TestBookDispositionCutoverConvertsOnlyLegacyAndPreservesEvidence(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	s := seedDispositionCutoverScenario(t, ctx, store, pool)

	// Before the conversion the legacy rows are unconverted and verification fails.
	before, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, before.Failures)
	require.Error(t, store.RequireNoLegacyDispositions(ctx))

	manifest := plannedManifest(t, ctx, store)
	require.Empty(t, manifest.Blockers)
	assert.EqualValues(t, 2, manifest.Totals.Owners)
	assert.EqualValues(t, 6, manifest.Totals.ConvertedDispositions)
	assert.EqualValues(t, 3, manifest.Totals.ProjectedInbox, "unread legacy Books project Inbox")
	assert.EqualValues(t, 3, manifest.Totals.ProjectedRead, "assertion/completion history projects Read")
	assert.EqualValues(t, manifest.Totals.Books, manifest.Totals.InitializedVisibility+2, "only Books with a visibility row are not initialized")
	categories := map[string]string{}
	for _, change := range manifest.Changes {
		assert.Equal(t, "set_aside", change.Disposition, "manifest records original values")
		categories[change.BookID] = change.HistoryCategory
	}
	assert.Equal(t, "unread", categories[s.Unread])
	assert.Equal(t, "assertion", categories[s.Assertion])
	assert.Equal(t, "completion", categories[s.Completion])
	assert.Equal(t, "mixed", categories[s.Mixed])
	require.Len(t, manifest.Commitments, 1)
	assert.Equal(t, s.Active, manifest.Commitments[0].BookID)

	inboxDisposition, inboxRevision := dispositionRow(t, ctx, pool, s.Owner, s.Inbox)
	toReadDisposition, toReadRevision := dispositionRow(t, ctx, pool, s.Owner, s.ToRead)
	_, legacyRevisionBefore := dispositionRow(t, ctx, pool, s.Owner, s.Unread)

	// A manifest that was edited after planning is not trusted.
	tampered := manifest
	tampered.Totals.ConvertedDispositions++
	_, err = store.ApplyBookDispositionCutover(ctx, tampered)
	require.ErrorIs(t, err, ErrDispositionCutoverManifestMismatch)

	report, err := store.ApplyBookDispositionCutover(ctx, manifest)
	require.NoError(t, err)
	assert.False(t, report.AlreadyApplied)
	assert.Equal(t, 6, report.ConvertedDispositions)
	assert.Equal(t, int(manifest.Totals.InitializedVisibility), report.InitializedVisibility)

	for _, book := range []string{s.Unread, s.Assertion, s.Completion, s.Mixed, s.HiddenLegacy} {
		disposition, revision := dispositionRow(t, ctx, pool, s.Owner, book)
		assert.Equal(t, "inbox", disposition)
		assert.Greater(t, revision, int64(0))
	}
	_, legacyRevisionAfter := dispositionRow(t, ctx, pool, s.Owner, s.Unread)
	assert.Equal(t, legacyRevisionBefore+1, legacyRevisionAfter, "conversion bumps the revision exactly once")
	disposition, revision := dispositionRow(t, ctx, pool, s.OtherOwner, s.OtherOwnerLegacy)
	assert.Equal(t, "inbox", disposition)
	assert.NotZero(t, revision)
	disposition, revision = dispositionRow(t, ctx, pool, s.Owner, s.Inbox)
	assert.Equal(t, inboxDisposition, disposition)
	assert.Equal(t, inboxRevision, revision, "existing Inbox untouched")
	disposition, revision = dispositionRow(t, ctx, pool, s.Owner, s.ToRead)
	assert.Equal(t, toReadDisposition, disposition)
	assert.Equal(t, toReadRevision, revision, "existing To Read untouched")
	disposition, _ = dispositionRow(t, ctx, pool, s.Owner, s.Active)
	assert.Equal(t, "to_read", disposition)

	// Projection: unread -> Inbox, history -> Read; nothing inferred Hidden or To Read.
	browse, err := store.ListMyBooksBrowseWithVisibility(ctx, s.Owner, "", "de", "", false, true, 0, 100)
	require.NoError(t, err)
	buckets := map[string]domain.MyBookBucket{}
	for _, item := range browse.Items {
		buckets[item.Book.ID] = item.WorkflowBucket()
	}
	assert.Equal(t, domain.MyBookBucketInbox, buckets[s.Unread])
	assert.Equal(t, domain.MyBookBucketInbox, buckets[s.HiddenLegacy])
	assert.Equal(t, domain.MyBookBucketRead, buckets[s.Assertion])
	assert.Equal(t, domain.MyBookBucketRead, buckets[s.Completion])
	assert.Equal(t, domain.MyBookBucketRead, buckets[s.Mixed])
	assert.Equal(t, domain.MyBookBucketCurrentReading, buckets[s.Active])
	assert.Equal(t, domain.MyBookBucketToRead, buckets[s.ToRead])

	// Visibility: everything initialized; existing Hide choices preserved; no
	// legacy Book became Hidden.
	assert.Zero(t, cutoverCount(t, ctx, pool, `SELECT count(*) FROM books b WHERE NOT EXISTS (SELECT 1 FROM book_visibility v WHERE v.owner_id=b.owner_id AND v.book_id=b.id)`))
	assert.EqualValues(t, 2, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_visibility WHERE hidden`), "only the two learner Hides remain Hidden")
	for _, hidden := range []string{s.HiddenToRead, s.HiddenLegacy} {
		isHidden, hiddenRevision, hiddenErr := store.GetBookVisibility(ctx, s.Owner, hidden)
		require.NoError(t, hiddenErr)
		assert.True(t, isHidden)
		assert.EqualValues(t, 1, hiddenRevision)
	}

	// Ownership, membership, snapshots, reservations, history, Known, source
	// evidence, and artifact provenance are byte-for-byte unchanged.
	after := plannedManifest(t, ctx, store)
	beforeFingerprints, afterFingerprints := preservationByName(manifest), preservationByName(after)
	for name, want := range beforeFingerprints {
		if name == "book_visibility" || name == "book_dispositions.unchanged" {
			continue
		}
		assert.Equal(t, want, afterFingerprints[name], "%s changed during conversion", name)
	}
	assert.Empty(t, after.Changes)
	assert.Empty(t, after.Blockers)
	for _, deck := range []string{s.CompletionDeckID, s.ActiveDeckID, s.PreparingDeckID} {
		_, deckErr := store.GetDeckPreparation(ctx, s.Owner, deck)
		require.NoError(t, deckErr)
	}
	artifact, err := store.DownloadDeckPreparation(ctx, s.Owner, s.CompletionDeckID)
	require.NoError(t, err)
	assert.NotEmpty(t, artifact.Artifact)
	assert.EqualValues(t, 1, cutoverCount(t, ctx, pool, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND book_id=$2 AND snapshot_id IS NOT NULL`, s.Owner, s.Active))
	assert.EqualValues(t, 1, cutoverCount(t, ctx, pool, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND book_id=$2 AND released_at IS NULL`, s.Owner, s.Active))

	// Committed data, checkpoint, and audit agree; validation passes; the
	// retired value can no longer be written by anyone.
	verification, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.Empty(t, verification.Failures)
	assert.True(t, verification.ConstraintValidated)
	assert.EqualValues(t, 6, verification.AuditedChanges)
	assert.EqualValues(t, 6, verification.CheckpointedChanges)
	require.NoError(t, store.RequireNoLegacyDispositions(ctx))
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='set_aside' WHERE owner_id=$1 AND book_id=$2`, s.Owner, s.Inbox)
	require.Error(t, err, "set_aside must be rejected once the constraint is validated")
	require.Error(t, store.SetBookDisposition(ctx, s.Owner, s.Inbox, domain.BookDisposition("set_aside")))
}

func TestBookDispositionCutoverRerunAndLostAcknowledgementConverge(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	s := seedDispositionCutoverScenario(t, ctx, store, pool)
	manifest := plannedManifest(t, ctx, store)

	first, err := store.ApplyBookDispositionCutover(ctx, manifest)
	require.NoError(t, err)

	// The learner makes later choices; then the operator, whose acknowledgement
	// was lost, reruns the very same reviewed manifest.
	applied, err := store.TransitionBookDisposition(ctx, s.Owner, "de", s.Unread, mustRevision(t, ctx, pool, s.Owner, s.Unread), domain.BookDispositionToRead)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = store.SetBookHidden(ctx, s.Owner, s.Assertion, 1, true)
	require.NoError(t, err)
	require.True(t, applied)
	editedDisposition, editedRevision := dispositionRow(t, ctx, pool, s.Owner, s.Unread)
	historyBefore := cutoverCount(t, ctx, pool, `SELECT count(*) FROM reading_history`)
	knownBefore := cutoverCount(t, ctx, pool, `SELECT count(*) FROM known_vocabulary`)

	again, err := store.ApplyBookDispositionCutover(ctx, manifest)
	require.NoError(t, err)
	assert.True(t, again.AlreadyApplied)
	assert.Equal(t, first.CutoverID, again.CutoverID)
	assert.Equal(t, first.ConvertedDispositions, again.ConvertedDispositions)
	disposition, revision := dispositionRow(t, ctx, pool, s.Owner, s.Unread)
	assert.Equal(t, editedDisposition, disposition, "rerun reset a later learner edit")
	assert.Equal(t, editedRevision, revision)
	isHidden, _, err := store.GetBookVisibility(ctx, s.Owner, s.Assertion)
	require.NoError(t, err)
	assert.True(t, isHidden, "rerun reset a later Hide")

	// A freshly planned manifest of the already-converted database is a no-op.
	replan := plannedManifest(t, ctx, store)
	assert.Empty(t, replan.Changes)
	noop, err := store.ApplyBookDispositionCutover(ctx, replan)
	require.NoError(t, err)
	assert.True(t, noop.NothingToConvert)
	assert.EqualValues(t, 1, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_disposition_cutovers`), "reruns add no duplicate checkpoint")
	assert.EqualValues(t, 6, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_disposition_cutover_changes`))
	assert.Equal(t, historyBefore, cutoverCount(t, ctx, pool, `SELECT count(*) FROM reading_history`))
	assert.Equal(t, knownBefore, cutoverCount(t, ctx, pool, `SELECT count(*) FROM known_vocabulary`))

	// Later learner edits are reported, not treated as verification failures.
	verification, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.Empty(t, verification.Failures)
	assert.EqualValues(t, 1, verification.ChangedSinceCutover)
}

func mustRevision(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, book string) int64 {
	t.Helper()
	_, revision := dispositionRow(t, ctx, pool, owner, book)
	return revision
}

func TestBookDispositionCutoverFailureLeavesNoHalfMappedState(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	s := seedDispositionCutoverScenario(t, ctx, store, pool)
	manifest := plannedManifest(t, ctx, store)
	legacyBefore := cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_dispositions WHERE disposition='set_aside'`)

	// Fail after the dispositions were mapped and audited, during the final
	// visibility initialization step.
	_, err := pool.Exec(ctx, `CREATE FUNCTION fail_visibility_init() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE TRIGGER fail_visibility_init BEFORE INSERT ON book_visibility FOR EACH ROW EXECUTE FUNCTION fail_visibility_init()`)
	require.NoError(t, err)
	_, err = store.ApplyBookDispositionCutover(ctx, manifest)
	require.Error(t, err)

	assert.Equal(t, legacyBefore, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_dispositions WHERE disposition='set_aside'`), "failure left half-mapped dispositions")
	assert.Zero(t, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_disposition_cutovers`))
	assert.Zero(t, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_disposition_cutover_changes`))
	assert.EqualValues(t, 2, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_visibility`), "failure initialized visibility")
	disposition, _ := dispositionRow(t, ctx, pool, s.Owner, s.Completion)
	assert.Equal(t, "set_aside", disposition)

	// Retrying after the cause is fixed converges with exactly one checkpoint.
	_, err = pool.Exec(ctx, `DROP TRIGGER fail_visibility_init ON book_visibility`)
	require.NoError(t, err)
	report, err := store.ApplyBookDispositionCutover(ctx, manifest)
	require.NoError(t, err)
	assert.False(t, report.AlreadyApplied)
	assert.Equal(t, 6, report.ConvertedDispositions)
	verification, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.Empty(t, verification.Failures)
}

func TestBookDispositionCutoverRefusesInvalidInventoryAndDrift(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	s := seedDispositionCutoverScenario(t, ctx, store, pool)
	manifest := plannedManifest(t, ctx, store)
	require.Empty(t, manifest.Blockers)

	// Drift after review: a stray writer changes a preserved table.
	_, err := pool.Exec(ctx, `INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos) VALUES ($1,'de','sehen','VERB')`, s.Owner)
	require.NoError(t, err)
	_, err = store.ApplyBookDispositionCutover(ctx, manifest)
	require.ErrorIs(t, err, ErrDispositionCutoverManifestMismatch)
	assert.EqualValues(t, 6, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_dispositions WHERE disposition='set_aside'`))

	// Invalid relationship: a Set Aside Book that is also the Current reading.
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='set_aside' WHERE owner_id=$1 AND book_id=$2`, s.Owner, s.Active)
	require.Error(t, err, "the NOT VALID constraint must block new legacy writes")
	seedLegacySetAside(t, ctx, pool, s.Owner, s.Active)
	invalid := plannedManifest(t, ctx, store)
	require.NotEmpty(t, invalid.Blockers)
	_, err = store.ApplyBookDispositionCutover(ctx, invalid)
	require.ErrorIs(t, err, ErrDispositionCutoverBlocked)
	disposition, _ := dispositionRow(t, ctx, pool, s.Owner, s.Unread)
	assert.Equal(t, "set_aside", disposition, "a blocked cutover must not repair silently")
	assert.Zero(t, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_disposition_cutovers`))

	// Invalid relationship: Current reading whose Book is not To Read.
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='inbox' WHERE owner_id=$1 AND book_id=$2`, s.Owner, s.Active)
	require.NoError(t, err)
	inbox := plannedManifest(t, ctx, store)
	require.NotEmpty(t, inbox.Blockers)
	_, err = store.ApplyBookDispositionCutover(ctx, inbox)
	require.ErrorIs(t, err, ErrDispositionCutoverBlocked)
}

func TestBookDispositionCutoverGuardedCorrectionRefusesLaterLearnerChanges(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	s := seedDispositionCutoverScenario(t, ctx, store, pool)
	report, err := store.ApplyBookDispositionCutover(ctx, plannedManifest(t, ctx, store))
	require.NoError(t, err)

	// A learner chose To Read, and another Book was hidden, after reopening.
	applied, err := store.TransitionBookDisposition(ctx, s.Owner, "de", s.Assertion, mustRevision(t, ctx, pool, s.Owner, s.Assertion), domain.BookDispositionToRead)
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = store.SetBookHidden(ctx, s.Owner, s.Mixed, 1, true)
	require.NoError(t, err)
	require.True(t, applied)

	for _, book := range []string{s.Assertion, s.Mixed} {
		_, err = store.CorrectBookDispositionCutover(ctx, report.CutoverID, s.Owner, book, domain.BookDispositionInbox)
		require.ErrorIs(t, err, ErrDispositionCutoverStateChanged, "correction overwrote a later learner choice")
	}
	disposition, _ := dispositionRow(t, ctx, pool, s.Owner, s.Assertion)
	assert.Equal(t, "to_read", disposition)

	// A proven-unchanged Book can be corrected exactly once; replay is a no-op,
	// and after the correction the learner's next change is again protected.
	applied, err = store.CorrectBookDispositionCutover(ctx, report.CutoverID, s.Owner, s.Unread, domain.BookDispositionToRead)
	require.NoError(t, err)
	assert.True(t, applied)
	revision := mustRevision(t, ctx, pool, s.Owner, s.Unread)
	applied, err = store.CorrectBookDispositionCutover(ctx, report.CutoverID, s.Owner, s.Unread, domain.BookDispositionToRead)
	require.NoError(t, err)
	assert.False(t, applied)
	assert.Equal(t, revision, mustRevision(t, ctx, pool, s.Owner, s.Unread))
	applied, err = store.TransitionBookDisposition(ctx, s.Owner, "de", s.Unread, revision, domain.BookDispositionInbox)
	require.NoError(t, err)
	require.True(t, applied)
	_, err = store.CorrectBookDispositionCutover(ctx, report.CutoverID, s.Owner, s.Unread, domain.BookDispositionToRead)
	require.ErrorIs(t, err, ErrDispositionCutoverStateChanged)

	// The correction never accepts a retired value.
	_, err = store.CorrectBookDispositionCutover(ctx, report.CutoverID, s.Owner, s.Completion, domain.BookDisposition("set_aside"))
	require.Error(t, err)

	verification, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.Empty(t, verification.Failures)
}

func TestBookDispositionStructuralMigrationRejectsRetiredValueWithoutConverting(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// A database with no legacy row is fully validated by the structural stage.
	verification, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.Empty(t, verification.Failures)
	assert.True(t, verification.ConstraintValidated)

	// A database holding legacy rows when 000033 applies keeps them untouched
	// and unconverted until the data stage runs.
	moveApplicationMigrationsTo(t, databaseURL, 32)
	owner, err := store.CreateUser(ctx, "structural-owner", false)
	require.NoError(t, err)
	book := insertLegacyBook(t, ctx, store, owner.ID, "Legacy at 32", "de")
	_, err = pool.Exec(ctx, `INSERT INTO book_dispositions(owner_id, book_id, disposition) VALUES ($1,$2,'set_aside')`, owner.ID, book.ID)
	require.NoError(t, err)
	migrateApplicationMigrationsToLatest(t, databaseURL)

	disposition, _ := dispositionRow(t, ctx, pool, owner.ID, book.ID)
	assert.Equal(t, "set_aside", disposition, "the structural stage must not convert data")
	assert.Zero(t, cutoverCount(t, ctx, pool, `SELECT count(*) FROM book_visibility`), "the structural stage must not initialize visibility")
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='set_aside', updated_at=now() WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.Error(t, err, "even legacy rows cannot be rewritten as the retired value")
	unverified, err := store.VerifyBookDispositionCutover(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, unverified.Failures)
	assert.False(t, unverified.ConstraintValidated)

	manifest := plannedManifest(t, ctx, store)
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	var roundTripped DispositionCutoverManifest
	require.NoError(t, json.Unmarshal(raw, &roundTripped))
	report, err := store.ApplyBookDispositionCutover(ctx, roundTripped)
	require.NoError(t, err, "a manifest written to disk and read back must still apply")
	assert.Equal(t, 1, report.ConvertedDispositions)

	// Down is non-destructive to learner state but never restores Set Aside.
	moveApplicationMigrationsTo(t, databaseURL, 32)
	disposition, _ = dispositionRow(t, ctx, pool, owner.ID, book.ID)
	assert.Equal(t, "inbox", disposition)
	migrateApplicationMigrationsToLatest(t, databaseURL)
}
