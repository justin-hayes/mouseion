//go:build integration

package webapp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lookupToken struct{ surface, lemma, upos string }

// seedLookupSentences adds one sentence per entry to an analyzed Book. Each
// token is a single-space-separated word so sentence and token offsets are
// deterministic; the canonical lemma follows the language's normalization
// profile exactly as the analysis pipeline stores it.
func seedLookupSentences(t *testing.T, ctx context.Context, store *persistence.PostgresStore, book domain.Book, language string, sentences [][]lookupToken) {
	t.Helper()
	var owner, runID, corpusID, unitID string
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT cai.owner_id::text, cai.analysis_run_id::text, cai.corpus_id::text, u.unit_id
		  FROM current_analysis_identity cai
		  JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id AND u.snapshot_id=cai.snapshot_id
		 WHERE cai.book_id=$1`, book.ID).Scan(&owner, &runID, &corpusID, &unitID))
	offset := 0
	for ordinal, tokens := range sentences {
		words := make([]string, len(tokens))
		for i, token := range tokens {
			words[i] = token.surface
		}
		text := strings.Join(words, " ")
		length := len([]rune(text))
		_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			owner, runID, corpusID, unitID, ordinal, text, offset, offset+length)
		require.NoError(t, err)
		position := offset
		for i, token := range tokens {
			canonical := canonicalization.NormalizeWith(mustProfile(t, language), token.lemma).CanonicalLemma
			end := position + len([]rune(token.surface))
			_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'dep',0,'{}',$11,$12)`,
				owner, language, runID, corpusID, ordinal, i, token.surface, token.lemma, canonical, token.upos, position, end)
			require.NoError(t, err)
			position = end + 1
		}
		offset += length + 1
	}
}

func mustProfile(t *testing.T, language string) canonicalization.Profile {
	t.Helper()
	profile, err := canonicalization.For(language)
	require.NoError(t, err)
	return profile
}

func setLookupBookLanguage(t *testing.T, ctx context.Context, store *persistence.PostgresStore, book domain.Book, source domain.SourceMaterial, language string) {
	t.Helper()
	_, err := store.Pool().Exec(ctx, `UPDATE books SET language_tag=$3 WHERE owner_id=$1 AND id=$2`, book.OwnerID, book.ID, language)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE source_materials SET language=$3 WHERE owner_id=$1 AND id=$2`, book.OwnerID, source.ID, language)
	require.NoError(t, err)
}

func lookupResults(body string) string {
	parts := strings.SplitN(body, `<section id="concordance-results"`, 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func lookupRowCount(body string) int {
	return strings.Count(lookupResults(body), `class="concordance-row"`)
}

func TestConcordanceFindsEvidencedLemmasOrLiteralWordFormsOverHTTP(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "concordance-lookup-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "lookup-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "lookup-bob", "bob-password", false)
	seed := func(owner, key, title string) (domain.Book, domain.SourceMaterial) {
		book, source, _, _ := seedMigrationAnalyzedBook(t, ctx, store, owner, key, title, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
		return book, source
	}
	alpha, _ := seed(alice.ID, "lookup-alpha", "Alpha")
	beta, _ := seed(alice.ID, "lookup-beta", "Beta Hidden")
	gamma, _ := seed(alice.ID, "lookup-gamma", "Gamma")
	stale, _ := seed(alice.ID, "lookup-stale", "Stale")
	bobBook, _ := seed(bob.ID, "lookup-bob", "Bobs Buch")
	italian, italianSource := seed(alice.ID, "lookup-italian", "Libro")
	greek, greekSource := seed(alice.ID, "lookup-greek", "Biblio")
	setLookupBookLanguage(t, ctx, store, italian, italianSource, "it")
	setLookupBookLanguage(t, ctx, store, greek, greekSource, "el")

	seedLookupSentences(t, ctx, store, alpha, "de", [][]lookupToken{
		{{"Wir", "wir", "PRON"}, {"gehen", "gehen", "VERB"}, {"heim", "heim", "ADV"}},
		{{"Er", "er", "PRON"}, {"ging", "gehen", "VERB"}},
		{{"Ich", "ich", "PRON"}, {"stehe", "aufstehen", "VERB"}, {"früh", "früh", "ADJ"}, {"auf", "auf", "ADP"}},
		{{"Der", "der", "DET"}, {"Mann", "mann", "NOUN"}, {"steht", "stehen", "VERB"}},
		{{"Das", "das", "DET"}, {"Gehen", "gehen", "NOUN"}, {"fällt", "fallen", "VERB"}},
		{{"Ein", "ein", "DET"}, {"Gang", "gang", "NOUN"}},
		{{"Süße", "süß", "ADJ"}, {"Äpfel", "apfel", "NOUN"}},
	})
	seedLookupSentences(t, ctx, store, beta, "de", [][]lookupToken{{{"Sie", "sie", "PRON"}, {"gingen", "gehen", "VERB"}}})
	seedLookupSentences(t, ctx, store, gamma, "de", [][]lookupToken{
		{{"Wir", "wir", "PRON"}, {"gingen", "gehen", "VERB"}},
		{{"Ein", "ein", "DET"}, {"Gang", "gang", "NOUN"}},
	})
	seedLookupSentences(t, ctx, store, stale, "de", [][]lookupToken{{{"Staleform", "gehen", "VERB"}}})
	seedLookupSentences(t, ctx, store, bobBook, "de", [][]lookupToken{{{"Bobform", "gehen", "VERB"}}})
	seedLookupSentences(t, ctx, store, italian, "it", [][]lookupToken{{{"Città", "città", "NOUN"}, {"antiche", "antico", "ADJ"}}})
	seedLookupSentences(t, ctx, store, greek, "el", [][]lookupToken{{{"Λόγος", "λόγος", "NOUN"}, {"λόγου", "λόγος", "NOUN"}}})

	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, stale.ID)
	require.NoError(t, err)
	// History, disposition, and Hidden never narrow Concordance.
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, beta.ID, domain.BookDispositionToRead))
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_visibility(owner_id,book_id,hidden) VALUES($1,$2,true)`, alice.ID, beta.ID)
	require.NoError(t, err)
	// Gamma's second "gingen" is excluded; its "Gang" is corrected to gehen.
	var runID, corpusID, unitID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT cai.analysis_run_id::text, cai.corpus_id::text, u.unit_id FROM current_analysis_identity cai JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id AND u.snapshot_id=cai.snapshot_id WHERE cai.book_id=$1`, gamma.ID).Scan(&runID, &corpusID, &unitID))
	// "Wir gingen": gingen spans code points 4-10. "Ein Gang" starts at 11; Gang spans 15-19.
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,4,10,NULL,NULL,NULL,true),($1,$2,$3,$4,$5,15,19,'gehen','de','1',false)`,
		alice.ID, gamma.ID, corpusID, runID, unitID)
	require.NoError(t, err)

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, "lookup-alice", "alice-password")
	get := func(language string, values url.Values) *httptest.ResponseRecorder {
		if language != "" {
			values.Set("language", language)
			require.NoError(t, store.SetActiveStudyLanguage(ctx, alice.ID, language))
		}
		return perform(t, h, http.MethodGet, "/vocabulary/concordance?"+values.Encode(), nil, cookies)
	}
	surfacesOf := func(body string) []string {
		var surfaces []string
		for _, part := range strings.Split(lookupResults(body), `<strong class="concordance-observed-target">`)[1:] {
			surfaces = append(surfaces, part[:strings.Index(part, "<")])
		}
		return surfaces
	}

	t.Run("an evidenced lemma expands every observed form across all parts of speech and eligible Books", func(t *testing.T) {
		response := get("de", url.Values{"term": {"gehen"}})
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, ">Forms of gehen</h2>")
		assert.ElementsMatch(t, []string{"gehen", "Gehen", "ging", "gingen", "Gang"}, surfacesOf(body))
		for _, absent := range []string{"Staleform", "Bobform", "steht", "stehe"} {
			assert.NotContains(t, lookupResults(body), absent)
		}
		assert.Contains(t, body, "Beta Hidden", "Hidden, To Read Books still contribute")
		assert.Contains(t, body, `value="gehen"`)
		formStart := strings.Index(body, `class="concordance-query"`)
		form := body[formStart : formStart+strings.Index(body[formStart:], "</form>")]
		assert.NotContains(t, form, `name="upos"`)
		assert.NotContains(t, form, `name="as"`)
		assert.NotContains(t, body, "Lookup evidence")
		assert.NotContains(t, body, "Grammar direction")
		assert.NotContains(t, body, `name="book"`)
		assert.Contains(t, body, "Lemma or word form")
	})

	t.Run("case is normalized and recognition does not depend on typed capitalization", func(t *testing.T) {
		upper := get("de", url.Values{"term": {"  GEHEN  "}})
		require.Equal(t, http.StatusOK, upper.Code)
		assert.Contains(t, upper.Body.String(), ">Forms of gehen</h2>")
		assert.Equal(t, lookupRowCount(get("de", url.Values{"term": {"gehen"}}).Body.String()), lookupRowCount(upper.Body.String()))
		assert.Contains(t, upper.Body.String(), `value="GEHEN"`, "the editable input keeps what was typed, trimmed")
	})

	t.Run("a non-lemma form matches only that observed source form", func(t *testing.T) {
		response := get("de", url.Values{"term": {"ging"}})
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), ">Word form ging</h2>")
		assert.Equal(t, []string{"ging"}, surfacesOf(response.Body.String()))
		steht := get("de", url.Values{"term": {"steht"}})
		assert.Contains(t, steht.Body.String(), ">Word form steht</h2>")
		assert.Equal(t, []string{"steht"}, surfacesOf(steht.Body.String()))
		umlaut := get("de", url.Values{"term": {"ÄPFEL"}})
		assert.Contains(t, umlaut.Body.String(), ">Word form äpfel</h2>")
		assert.Equal(t, []string{"Äpfel"}, surfacesOf(umlaut.Body.String()))
	})

	t.Run("full separable lemmas match the split construction and a form matches only itself", func(t *testing.T) {
		response := get("de", url.Values{"term": {"aufstehen"}})
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), ">Forms of aufstehen</h2>")
		assert.Equal(t, []string{"stehe"}, surfacesOf(response.Body.String()))
		stehe := get("de", url.Values{"term": {"stehe"}})
		assert.Equal(t, []string{"stehe"}, surfacesOf(stehe.Body.String()))
	})

	t.Run("Browse identity links restrict to one exact part of speech and a new submission clears it", func(t *testing.T) {
		exact := get("de", url.Values{"term": {"gehen"}, "as": {"lemma"}, "upos": {"VERB"}})
		require.Equal(t, http.StatusOK, exact.Code)
		assert.Contains(t, exact.Body.String(), ">Forms of gehen · verb</h2>")
		assert.ElementsMatch(t, []string{"gehen", "ging", "gingen"}, surfacesOf(exact.Body.String()),
			"the NOUN readings Gehen and corrected Gang fall outside the exact VERB identity")
	})

	t.Run("supported interpretation is carried, not silently recognized again", func(t *testing.T) {
		literal := get("de", url.Values{"term": {"gehen"}, "as": {"form"}})
		require.Equal(t, http.StatusOK, literal.Code)
		assert.Contains(t, literal.Body.String(), ">Word form gehen</h2>")
		assert.Equal(t, []string{"gehen", "Gehen"}, surfacesOf(literal.Body.String()), "case folds; the lemma expansion does not apply")
	})

	t.Run("corrections govern lemma attribution while surface lookup keeps source matching; exclusions omit both", func(t *testing.T) {
		gang := get("de", url.Values{"term": {"gang"}})
		assert.Equal(t, http.StatusOK, gang.Code)
		// Alpha's Gang is still an evidenced lemma "gang" (uncorrected), so recognition expands it.
		assert.Contains(t, gang.Body.String(), ">Forms of gang</h2>")
		assert.Equal(t, []string{"Gang"}, surfacesOf(gang.Body.String()), "the corrected Gamma occurrence leaves the gang lemma")
		literal := get("de", url.Values{"term": {"gang"}, "as": {"form"}})
		assert.Equal(t, []string{"Gang", "Gang"}, surfacesOf(literal.Body.String()), "surface lookup retains unchanged source matching after correction")
		gingen := get("de", url.Values{"term": {"gingen"}})
		assert.Contains(t, gingen.Body.String(), ">Word form gingen</h2>")
		assert.Equal(t, []string{"gingen"}, surfacesOf(gingen.Body.String()), "the excluded Gamma occurrence is omitted from form lookup")
	})

	t.Run("Italian and Greek keep accents and fold case, including Greek final sigma", func(t *testing.T) {
		italianLemma := get("it", url.Values{"term": {"CITTÀ"}})
		require.Equal(t, http.StatusOK, italianLemma.Code, italianLemma.Body.String())
		assert.Equal(t, []string{"Città"}, surfacesOf(italianLemma.Body.String()))
		assert.Equal(t, 0, lookupRowCount(get("it", url.Values{"term": {"citta"}}).Body.String()), "an unaccented spelling is a different word form")
		greekLemma := get("el", url.Values{"term": {"ΛΌΓΟΣ"}})
		require.Equal(t, http.StatusOK, greekLemma.Code, greekLemma.Body.String())
		assert.Contains(t, greekLemma.Body.String(), ">Forms of λόγοσ</h2>")
		assert.ElementsMatch(t, []string{"Λόγος", "λόγου"}, surfacesOf(greekLemma.Body.String()))
		greekForm := get("el", url.Values{"term": {"ΛΌΓΟΥ"}})
		assert.Equal(t, []string{"λόγου"}, surfacesOf(greekForm.Body.String()))
		assert.Equal(t, 0, lookupRowCount(get("el", url.Values{"term": {"λογου"}}).Body.String()), "Greek accents are meaningful")
	})

	t.Run("blank, multiword, unmatched, and malformed input are distinct", func(t *testing.T) {
		blank := get("de", url.Values{"term": {"   "}})
		assert.Equal(t, http.StatusOK, blank.Code)
		assert.NotContains(t, blank.Body.String(), `id="concordance-results"`)
		multi := get("de", url.Values{"term": {"steht auf"}})
		assert.Equal(t, http.StatusBadRequest, multi.Code)
		assert.Contains(t, multi.Body.String(), "Enter one lemma or word form.")
		assert.Contains(t, multi.Body.String(), `value="steht auf"`)
		assert.NotContains(t, multi.Body.String(), `id="concordance-results"`)
		unmatched := get("de", url.Values{"term": {"zzzyx"}})
		require.Equal(t, http.StatusOK, unmatched.Code)
		assert.Contains(t, unmatched.Body.String(), ">Word form zzzyx</h2>")
		assert.Contains(t, unmatched.Body.String(), "No analyzed occurrences of this word form")
		assert.NotContains(t, unmatched.Body.String()[strings.Index(unmatched.Body.String(), "<main"):], `role="alert"`)
		for name, values := range map[string]url.Values{
			"page zero":     {"term": {"gehen"}, "page": {"0"}},
			"page text":     {"term": {"gehen"}, "page": {"x"}},
			"unknown as":    {"term": {"gehen"}, "as": {"phrase"}},
			"orphan POS":    {"term": {"gehen"}, "upos": {"VERB"}},
			"form with POS": {"term": {"gehen"}, "as": {"form"}, "upos": {"VERB"}},
			"as without":    {"as": {"lemma"}},
		} {
			response := get("de", values)
			assert.Equal(t, http.StatusBadRequest, response.Code, name)
			assert.NotContains(t, response.Body.String(), `id="concordance-results"`, name)
		}
		outOfRange := get("de", url.Values{"term": {"gehen"}, "as": {"lemma"}, "page": {"99"}})
		require.Equal(t, http.StatusOK, outOfRange.Code)
		assert.Contains(t, outOfRange.Body.String(), "No results on this page.")
		assert.Contains(t, outOfRange.Body.String(), "Go to page 1")
		assert.NotContains(t, outOfRange.Body.String(), "No analyzed occurrences of this")
	})

	t.Run("owners never see each other's evidence", func(t *testing.T) {
		require.NoError(t, store.SetActiveStudyLanguage(ctx, bob.ID, "de"))
		bobCookies, _ := loginCookies(t, h, "lookup-bob", "bob-password")
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance?term=gehen&language=de", nil, bobCookies)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, []string{"Bobform"}, surfacesOf(response.Body.String()))
	})

	t.Run("paging and Study return carry the applied interpretation and a changed revision needs refresh", func(t *testing.T) {
		var base [][]lookupToken
		for range 26 {
			base = append(base, []lookupToken{{"Wir", "wir", "PRON"}, {"gehen", "gehen", "VERB"}})
		}
		// Appended after Alpha's existing sentences so ordinals stay unique.
		appendLookupSentences(t, ctx, store, alpha, "de", 7, 80, base)
		first := get("de", url.Values{"term": {"gehen"}})
		require.Equal(t, http.StatusOK, first.Code)
		assert.Equal(t, 25, lookupRowCount(first.Body.String()))
		assert.Contains(t, first.Body.String(), "Results 1–25 on page 1")
		assert.Contains(t, first.Body.String(), `href="/vocabulary/concordance?as=lemma&amp;language=de&amp;page=2&amp;priority=none&amp;rev=`)
		assert.Contains(t, first.Body.String(), "return=%2Fvocabulary%2Fconcordance%3Fas%3Dlemma")
		revision := first.Body.String()[strings.Index(first.Body.String(), "rev=")+4:]
		revision = revision[:strings.IndexAny(revision, "&\"")]
		second := get("de", url.Values{"term": {"gehen"}, "as": {"lemma"}, "page": {"2"}, "priority": {"none"}, "rev": {revision}})
		require.Equal(t, http.StatusOK, second.Code)
		assert.Contains(t, second.Body.String(), "Results 26–")
		_, err := store.Pool().Exec(ctx, `UPDATE books SET title='Alpha Retitled' WHERE owner_id=$1 AND id=$2`, alice.ID, alpha.ID)
		require.NoError(t, err)
		stale := get("de", url.Values{"term": {"gehen"}, "as": {"lemma"}, "page": {"2"}, "priority": {"none"}, "rev": {revision}})
		assert.Equal(t, http.StatusConflict, stale.Code)
		assert.Contains(t, stale.Body.String(), "Current evidence changed")
		assert.Contains(t, stale.Body.String(), "Refresh results")
		partial := performWithHeader(t, h, http.MethodGet, "/vocabulary/concordance?language=de&term=gehen&as=lemma&page=2&priority=none&rev="+revision, nil, cookies, "HX-Request-Type", "partial")
		assert.Equal(t, http.StatusConflict, partial.Code)
		assert.NotContains(t, partial.Body.String(), "<html")
		assert.NotContains(t, partial.Body.String(), `id="concordance-results"`)
	})

	t.Run("Study keeps raw analyzer, source, and syntax evidence for corrected and excluded occurrences", func(t *testing.T) {
		study := perform(t, h, http.MethodGet, fmt.Sprintf("/vocabulary/concordance/sentence?language=de&book=%s&run=%s&corpus=%s&unit=%s&sentence=0&target=1&target_surface=gingen", gamma.ID, runID, corpusID, unitID), nil, cookies)
		require.Equal(t, http.StatusOK, study.Code)
		assert.Contains(t, study.Body.String(), "Excluded from effective vocabulary; retained as syntax evidence.")
		assert.Contains(t, study.Body.String(), "lemma evidence <code>gehen</code>")
		corrected := perform(t, h, http.MethodGet, fmt.Sprintf("/vocabulary/concordance/sentence?language=de&book=%s&run=%s&corpus=%s&unit=%s&sentence=1&target=1&target_surface=Gang", gamma.ID, runID, corpusID, unitID), nil, cookies)
		require.Equal(t, http.StatusOK, corrected.Code)
		assert.Contains(t, corrected.Body.String(), "Corrected for this occurrence.")
		assert.Contains(t, corrected.Body.String(), "lemma evidence <code>gang</code>")
	})
}

// appendLookupSentences adds sentences after existing ordinals, offsetting
// source positions so they never collide with seeded evidence.
func appendLookupSentences(t *testing.T, ctx context.Context, store *persistence.PostgresStore, book domain.Book, language string, firstOrdinal, firstOffset int, sentences [][]lookupToken) {
	t.Helper()
	var owner, runID, corpusID, unitID string
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT cai.owner_id::text, cai.analysis_run_id::text, cai.corpus_id::text, u.unit_id
		  FROM current_analysis_identity cai
		  JOIN source_material_units u ON u.owner_id=cai.owner_id AND u.source_material_id=cai.source_material_id AND u.snapshot_id=cai.snapshot_id
		 WHERE cai.book_id=$1`, book.ID).Scan(&owner, &runID, &corpusID, &unitID))
	offset := firstOffset
	for i, tokens := range sentences {
		ordinal := firstOrdinal + i
		words := make([]string, len(tokens))
		for j, token := range tokens {
			words[j] = token.surface
		}
		text := strings.Join(words, " ")
		length := len([]rune(text))
		_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			owner, runID, corpusID, unitID, ordinal, text, offset, offset+length)
		require.NoError(t, err)
		position := offset
		for j, token := range tokens {
			end := position + len([]rune(token.surface))
			canonical := canonicalization.NormalizeWith(mustProfile(t, language), token.lemma).CanonicalLemma
			_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'dep',0,'{}',$11,$12)`,
				owner, language, runID, corpusID, ordinal, j, token.surface, token.lemma, canonical, token.upos, position, end)
			require.NoError(t, err)
			position = end + 1
		}
		offset += length + 1
	}
}
