//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListAncientGreekVocabularyIdentitiesKeepsHomographsDistinct proves the
// hit-rate report's source keeps two homographs that share a canonical lemma
// and part of speech apart, and lists no other language.
func TestListAncientGreekVocabularyIdentitiesKeepsHomographsDistinct(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "greek-hit-rate", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "grc", "hit-rate")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count,lexical_discriminator) VALUES
		($1,$2,'grc',$3,$4,'ανθρωποσ','NOUN',5,''),
		($1,$2,'grc',$3,$4,'δεω','VERB',3,'morpheus:1'),
		($1,$2,'grc',$3,$4,'δεω','VERB',2,'morpheus:2'),
		($1,$2,'grc',$3,$4,'λογοσ','NOUN',4,''),
		($1,$2,'de',$3,$4,'haus','NOUN',9,'')`, owner.ID, book.ID, runID, corpusID)
	require.NoError(t, err)

	identities, err := store.ListAncientGreekVocabularyIdentities(ctx)
	require.NoError(t, err)
	assert.Equal(t, []AncientGreekVocabularyIdentity{
		{CanonicalLemma: "ανθρωποσ", UPOS: "NOUN"},
		{CanonicalLemma: "δεω", UPOS: "VERB", Discriminator: "morpheus:1"},
		{CanonicalLemma: "δεω", UPOS: "VERB", Discriminator: "morpheus:2"},
		{CanonicalLemma: "λογοσ", UPOS: "NOUN"},
	}, identities)
}
