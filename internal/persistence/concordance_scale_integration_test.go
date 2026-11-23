//go:build integration

package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// concordanceScaleBudget is well under the 8 s interactive read limit, so a
// regression to a plan that reads the whole corpus fails here rather than in
// the UI. Lookups measured about 0.4 s at most when this was written.
const concordanceScaleBudget = 2 * time.Second

// scaleLibrary is the shape of a seeded library: Books of units of sentences
// of ten tokens each.
type scaleLibrary struct{ books, units, sentencesPerUnit int }

// TestConcordanceLookupStaysInteractiveOnALargeLibrary seeds about 550,000
// token occurrences across a dozen Books, the documentation-screenshot library
// in #2595, and holds that lemma and form lookups answer well within the
// interactive read limit. Nothing collects statistics, as on a freshly loaded
// database: the planner must not need them. A plan that joins and sorts the
// whole corpus before applying the lemma or form filter, or that rescans the
// candidates once per small-table row, takes many seconds here.
func TestConcordanceLookupStaysInteractiveOnALargeLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a large corpus")
	}
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "concordance-scale", false)
	require.NoError(t, err)
	library := scaleLibrary{books: 12, units: 20, sentencesPerUnit: 230}
	books := seedScaleLibrary(t, ctx, store, owner.ID, library)
	priority := books[len(books)/2]
	var tokens int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM corpus_tokens WHERE owner_id=$1`, owner.ID).Scan(&tokens))
	require.Equal(t, int64(library.books*library.units*library.sentencesPerUnit*10), tokens)

	lookups := []struct {
		name   string
		lookup domain.ConcordanceLookup
		match  string
	}{
		{"recognized lemma", domain.ConcordanceLookup{Term: "aufstehen", Page: 1}, domain.ConcordanceMatchLemma},
		{"lemma with part of speech", domain.ConcordanceLookup{Term: "aufstehen", Match: domain.ConcordanceMatchLemma, UPOS: "VERB", Page: 1}, domain.ConcordanceMatchLemma},
		{"lemma later page with priority", domain.ConcordanceLookup{Term: "aufstehen", Priority: priority, Page: 3}, domain.ConcordanceMatchLemma},
		{"frequent lemma", domain.ConcordanceLookup{Term: "der", Page: 2}, domain.ConcordanceMatchLemma},
		{"recognized form", domain.ConcordanceLookup{Term: "Stand", Page: 1}, domain.ConcordanceMatchForm},
		{"explicit form with priority", domain.ConcordanceLookup{Term: "stand", Match: domain.ConcordanceMatchForm, Priority: priority, Page: 2}, domain.ConcordanceMatchForm},
		{"no match", domain.ConcordanceLookup{Term: "nirgendwo", Page: 1}, domain.ConcordanceMatchForm},
	}
	for _, tc := range lookups {
		t.Run(tc.name, func(t *testing.T) {
			started := time.Now()
			result, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", tc.lookup)
			elapsed := time.Since(started)
			require.NoError(t, err)
			assert.Equal(t, tc.match, result.Match)
			if tc.lookup.Term == "nirgendwo" {
				assert.Empty(t, result.Occurrences)
			} else {
				assert.Len(t, result.Occurrences, 25)
				assert.True(t, result.HasNext)
			}
			if tc.lookup.Priority != "" {
				assert.Equal(t, domain.ConcordancePriorityMatches, result.PriorityState)
			}
			assert.Less(t, elapsed, concordanceScaleBudget, "lookup took %s", elapsed)
		})
	}
}

// TestConcordanceLookupMatchesTheOccurrenceView holds that the selective
// lookup returns exactly the concordance_occurrences rows, in the same order,
// that the view-based lookup it replaced returned: across every page, with
// corrections towards and away from the lemma, an exclusion, a Book with no
// current analysis, a Book in another study language, and a priority Book.
func TestConcordanceLookupMatchesTheOccurrenceView(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "concordance-equivalence", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "concordance-equivalence-other", false)
	require.NoError(t, err)
	library := scaleLibrary{books: 5, units: 3, sentencesPerUnit: 40}
	books := seedScaleLibrary(t, ctx, store, owner.ID, library)
	seedScaleLibrary(t, ctx, store, other.ID, scaleLibrary{books: 1, units: 1, sentencesPerUnit: 60})

	// Lower-case titles sort by lower(title) before the exact title.
	_, err = store.Pool().Exec(ctx, `UPDATE books SET title='a lower-case Book' WHERE owner_id=$1 AND id=$2`, owner.ID, books[3])
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, books[4])
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE books SET language_tag='it' WHERE owner_id=$1 AND id=$2`, owner.ID, books[2])
	require.NoError(t, err)
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, books[0])
	decide := func(sentence, token int64, lemma string, excluded bool) {
		unitIndex, within := sentence/int64(library.sentencesPerUnit), sentence%int64(library.sentencesPerUnit)
		unit := fmt.Sprintf("epub-unit-v1:%d:scale-0-%d", unitIndex, unitIndex)
		start := within*60 + token*6
		insertOccurrenceDecision(t, ctx, store, owner.ID, books[0], corpus, run, unit, start, start+5, lemma, excluded)
	}
	decide(5, 0, "aufstehen", false) // a "der" token attributed to "aufstehen"
	decide(6, 0, "", true)           // a "der" token excluded
	decide(53, 4, "gehen", false)    // an "aufstehen" token attributed to "gehen"
	decide(45, 2, "gehen", false)    // a token known only through its correction

	lookups := []struct {
		name   string
		lookup domain.ConcordanceLookup
	}{
		{"lemma across pages", domain.ConcordanceLookup{Term: "der", Match: domain.ConcordanceMatchLemma}},
		{"lemma with a priority Book", domain.ConcordanceLookup{Term: "der", Match: domain.ConcordanceMatchLemma, Priority: books[1]}},
		{"corrected lemma", domain.ConcordanceLookup{Term: "aufstehen", Match: domain.ConcordanceMatchLemma}},
		{"lemma with part of speech", domain.ConcordanceLookup{Term: "aufstehen", Match: domain.ConcordanceMatchLemma, UPOS: "VERB"}},
		{"lemma with another part of speech", domain.ConcordanceLookup{Term: "aufstehen", Match: domain.ConcordanceMatchLemma, UPOS: "DET"}},
		{"lemma known only from corrections", domain.ConcordanceLookup{Term: "gehen", Match: domain.ConcordanceMatchLemma}},
		{"form keeps corrected occurrences", domain.ConcordanceLookup{Term: "Der", Match: domain.ConcordanceMatchForm, Priority: books[3]}},
		{"form", domain.ConcordanceLookup{Term: "stand", Match: domain.ConcordanceMatchForm}},
	}
	for _, tc := range lookups {
		t.Run(tc.name, func(t *testing.T) {
			lemma, form, err := concordanceTerms("de", tc.lookup.Term)
			require.NoError(t, err)
			term := form
			if tc.lookup.Match == domain.ConcordanceMatchLemma {
				term = lemma
			}
			want := viewConcordanceKeys(t, ctx, store, owner.ID, tc.lookup, term)
			require.NotEmpty(t, want)
			var got []string
			for page := 1; ; page++ {
				lookup := tc.lookup
				lookup.Page = page
				result, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", lookup)
				require.NoError(t, err)
				for _, occurrence := range result.Occurrences {
					got = append(got, concordanceKey(occurrence.BookID, occurrence.UnitID, occurrence.SentenceOrdinal,
						occurrence.TokenOrdinal, occurrence.EffectiveLemma, occurrence.Corrected))
				}
				if !result.HasNext {
					break
				}
				require.Less(t, page, 100, "paging must end")
			}
			assert.Equal(t, want, got)
		})
	}
}

func concordanceKey(book, unit string, sentence, token int64, lemma string, corrected bool) string {
	return fmt.Sprintf("%s/%s/%d/%d/%s/%t", book, unit, sentence, token, lemma, corrected)
}

// viewConcordanceKeys reads the complete ordered result of the lookup that
// ListVocabularyConcordance replaced in #2595, straight from the
// concordance_occurrences view, as the reference for the selective lookup.
func viewConcordanceKeys(t *testing.T, ctx context.Context, store *PostgresStore, owner string, lookup domain.ConcordanceLookup, term string) []string {
	t.Helper()
	rows, err := store.Pool().Query(ctx, `
		SELECT o.book_id, o.unit_id, o.sentence_ordinal, o.token_ordinal,
		       COALESCE(d.canonical_lemma, o.canonical_lemma),
		       (d.canonical_lemma IS NOT NULL AND NOT COALESCE(d.excluded, false))
		  FROM concordance_occurrences o
		  JOIN corpus_tokens t ON t.owner_id=o.owner_id AND t.language=o.language
		    AND t.analysis_run_id=o.analysis_run_id::uuid AND t.corpus_id=o.corpus_id::uuid
		    AND t.sentence_ordinal=o.sentence_ordinal AND t.token_ordinal=o.token_ordinal
		  LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=o.owner_id
		    AND d.book_id=o.book_id::uuid AND d.corpus_id=o.corpus_id::uuid
		    AND d.analysis_run_id=o.analysis_run_id::uuid AND d.source_document_id=o.unit_id
		    AND d.start_offset=o.unit_start_offset AND d.end_offset=o.unit_end_offset
		 WHERE o.owner_id=$1 AND o.language='de'
		   AND NOT COALESCE(d.excluded, false)
		   AND (($2='lemma' AND COALESCE(d.canonical_lemma, o.canonical_lemma)=$3
		         AND ($4='' OR o.upos=$4))
		     OR ($2='form' AND translate(lower(o.surface), 'ς', 'σ')=$3))
		 ORDER BY (o.book_id=$5) DESC, lower(o.book_title), o.book_title, o.book_id,
		          o.unit_order, o.sentence_ordinal, o.token_ordinal`,
		owner, lookup.Match, term, lookup.UPOS, lookup.Priority)
	require.NoError(t, err)
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var book, unit, lemma string
		var sentence, token int64
		var corrected bool
		require.NoError(t, rows.Scan(&book, &unit, &sentence, &token, &lemma, &corrected))
		keys = append(keys, concordanceKey(book, unit, sentence, token, lemma, corrected))
	}
	require.NoError(t, rows.Err())
	return keys
}

// seedScaleLibrary creates analyzed Books and bulk-loads their sentences and
// tokens, returning the Book identities in creation order.
func seedScaleLibrary(t *testing.T, ctx context.Context, store *PostgresStore, owner string, library scaleLibrary) []string {
	t.Helper()
	books := make([]string, library.books)
	for index := range books {
		books[index] = seedScaleBook(t, ctx, store, owner, index, library)
	}
	return books
}

// seedScaleBook creates one analyzed Book. Every sentence starts with "Der"
// (lemma "der"); one sentence in fifty carries "aufstehen", written "stand",
// as its fifth token; the rest are drawn from a 20,000-word vocabulary.
// Token and sentence offsets are unit-relative, 60 characters per sentence.
func seedScaleBook(t *testing.T, ctx context.Context, store *PostgresStore, owner string, index int, library scaleLibrary) string {
	t.Helper()
	units := make([]domain.ExtractedUnit, library.units)
	var offset uint64
	for u := range units {
		text := fmt.Sprintf("Kapitel %d", u)
		units[u] = concordanceUnit(uint64(u), fmt.Sprintf("scale-%d-%d", index, u), text, offset, offset+uint64(len(text)))
		offset += uint64(len(text)) + 2
	}
	book, source := createConcordanceBook(t, ctx, store, owner, fmt.Sprintf("Scale Book %02d", index), fmt.Sprintf("scale-%s-%d", owner, index), false, units)
	insertConcordanceAnalysis(t, ctx, store, source, true, nil)
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner, book.ID)
	sentences := library.units * library.sentencesPerUnit
	_, err := store.Pool().Exec(ctx, `
		INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
		SELECT $1, $2, $3, 'epub-unit-v1:' || (n / $5) || ':scale-' || $4::int || '-' || (n / $5),
		       n, 'Satz ' || n, (n % $5) * 60, (n % $5) * 60 + 59
		  FROM generate_series(0, $6::bigint - 1) AS n`,
		owner, run, corpus, index, library.sentencesPerUnit, sentences)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,
			surface,raw_lemma,canonical_lemma,upos,dependency,head,start_offset,end_offset)
		SELECT $1, 'de', $2, $3, s, k, w.surface, w.lemma, w.lemma, w.upos, 'dep', 0,
		       (s % $4) * 60 + k * 6, (s % $4) * 60 + k * 6 + 5
		  FROM generate_series(0, $5::bigint - 1) AS s
		 CROSS JOIN generate_series(0, 9) AS k
		 CROSS JOIN LATERAL (SELECT
		       CASE WHEN k = 0 THEN 'Der' WHEN k = 4 AND s % 50 = 3 THEN 'stand'
		            ELSE 'w' || ((s * 7 + k * 13 + $6::bigint) % 20000) END AS surface,
		       CASE WHEN k = 0 THEN 'der' WHEN k = 4 AND s % 50 = 3 THEN 'aufstehen'
		            ELSE 'w' || ((s * 7 + k * 13 + $6::bigint) % 20000) END AS lemma,
		       CASE WHEN k = 0 THEN 'DET' WHEN k = 4 THEN 'VERB' ELSE 'NOUN' END AS upos) w`,
		owner, run, corpus, library.sentencesPerUnit, sentences, index)
	require.NoError(t, err)
	return book.ID
}
