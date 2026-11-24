package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	constructionTestBook = "11111111-2222-3333-4444-555555555555"
	constructionTestRun  = "66666666-7777-8888-9999-000000000000"
)

func constructionTestQuery(overrides map[string]string) url.Values {
	values := url.Values{
		"as": {"construction"}, "term": {"genitive_absolute"}, "priority": {constructionTestBook},
		"from_run": {constructionTestRun}, "from_sentence": {"3"}, "from_anchor": {"2"},
	}
	for key, value := range overrides {
		if value == "" {
			values.Del(key)
			continue
		}
		values.Set(key, value)
	}
	return values
}

func parseConstructionTestRequest(t *testing.T, values url.Values) concordanceRequest {
	t.Helper()
	return parseConcordanceRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/vocabulary/concordance?"+values.Encode(), nil))
}

func TestConcordanceConstructionRequestParsesTheOrigin(t *testing.T) {
	request := parseConstructionTestRequest(t, constructionTestQuery(map[string]string{"subtype": "genitive"}))
	assert.Equal(t, domain.ConcordanceMatchConstruction, request.lookup.Match)
	assert.Equal(t, "genitive", request.lookup.Subtype)
	assert.Equal(t, constructionTestBook, request.lookup.Priority)
	require.NotNil(t, request.lookup.Origin)
	assert.Equal(t, domain.ConcordanceConstructionOrigin{RunID: constructionTestRun, Sentence: 3, Anchor: 2}, *request.lookup.Origin)
}

func TestConcordanceConstructionRequestDropsAMalformedOrigin(t *testing.T) {
	for name, overrides := range map[string]map[string]string{
		"missing run":      {"from_run": ""},
		"missing sentence": {"from_sentence": ""},
		"missing anchor":   {"from_anchor": ""},
		"run not a uuid":   {"from_run": "not-a-run"},
		"negative":         {"from_sentence": "-1"},
		"not a number":     {"from_anchor": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, parseConstructionTestRequest(t, constructionTestQuery(overrides)).lookup.Origin)
		})
	}
}

func TestConcordanceConstructionRejectionsSendTheSafeRestart(t *testing.T) {
	reject := func(t *testing.T, overrides map[string]string, language string) {
		t.Helper()
		request := parseConstructionTestRequest(t, constructionTestQuery(overrides))
		status, message, rejected := concordanceRequestRejection(request, request.lookup, language)
		assert.True(t, rejected)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, concordanceNoticeState, message)
	}
	t.Run("a valid request is not rejected", func(t *testing.T) {
		request := parseConstructionTestRequest(t, constructionTestQuery(map[string]string{"subtype": "genitive", "term": "articular_infinitive"}))
		_, _, rejected := concordanceRequestRejection(request, request.lookup, "grc")
		assert.False(t, rejected)
	})
	for name, overrides := range map[string]map[string]string{
		"a missing priority":           {"priority": ""},
		"the explicit absence":         {"priority": "none"},
		"a missing origin":             {"from_run": ""},
		"a malformed origin":           {"from_anchor": "x"},
		"an unknown construction":      {"term": "optative"},
		"an unlabelled subtype":        {"subtype": "natural"},
		"a subtype of another":         {"subtype": "infinitive"},
		"a part of speech":             {"upos": "VERB"},
		"a revision without priority":  {"priority": "", "rev": "r"},
		"a construction as its search": {"term": "genitive absolute"},
	} {
		t.Run(name, func(t *testing.T) { reject(t, overrides, "grc") })
	}
	t.Run("a language without a construction scheme", func(t *testing.T) {
		reject(t, nil, "de")
	})
	t.Run("a shipped subtype of its own construction", func(t *testing.T) {
		request := parseConstructionTestRequest(t, constructionTestQuery(map[string]string{"term": "result", "subtype": "natural"}))
		_, _, rejected := concordanceRequestRejection(request, request.lookup, "grc")
		assert.False(t, rejected)
	})
}

func TestConcordanceLemmaAndFormRequestsIgnoreConstructionParameters(t *testing.T) {
	request := parseConstructionTestRequest(t, url.Values{"as": {"form"}, "term": {"haus"}, "subtype": {"genitive"}, "from_run": {constructionTestRun}})
	_, _, rejected := concordanceRequestRejection(request, request.lookup, "de")
	assert.False(t, rejected)
}

func TestConcordanceConstructionURLsCarryTheOriginAndKeepThePriorityOnRestart(t *testing.T) {
	lookup := domain.ConcordanceLookup{
		Language: "grc", Term: "genitive_absolute", Match: domain.ConcordanceMatchConstruction, Subtype: "",
		Priority: constructionTestBook, Revision: "rev-1", Page: 2,
		Origin: &domain.ConcordanceConstructionOrigin{RunID: constructionTestRun, Sentence: 3, Anchor: 2},
	}
	page := parseURL(t, vocabularyConcordancePageURL(2, lookup))
	assert.Equal(t, url.Values{
		"language": {"grc"}, "term": {"genitive_absolute"}, "as": {"construction"}, "priority": {constructionTestBook},
		"rev": {"rev-1"}, "page": {"2"}, "from_run": {constructionTestRun}, "from_sentence": {"3"}, "from_anchor": {"2"},
	}, page)

	lookup.Subtype = "genitive"
	restart := parseURL(t, vocabularyConcordanceRestartURL(lookup))
	assert.Equal(t, url.Values{
		"language": {"grc"}, "term": {"genitive_absolute"}, "as": {"construction"}, "priority": {constructionTestBook},
		"subtype": {"genitive"}, "from_run": {constructionTestRun}, "from_sentence": {"3"}, "from_anchor": {"2"},
	}, restart, "a restart drops only rev and page, so it passes the origin rule again")

	assert.NotContains(t, vocabularyConcordanceRestartURL(lookup), "page=", "a construction restart omits the page")
	assert.Contains(t, vocabularyConcordancePageURL(2, lookup), "page=2")

	word := domain.ConcordanceLookup{Term: "haus", Match: domain.ConcordanceMatchForm, Priority: "book", Revision: "r"}
	assert.Equal(t, "/vocabulary/concordance?page=1&term=haus", vocabularyConcordanceRestartURL(word), "lemma and form restarts still blank the priority")
}

func parseURL(t *testing.T, raw string) url.Values {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed.Query()
}

func TestSafeConcordanceReturnURLKeepsAConstructionLookup(t *testing.T) {
	query := constructionTestQuery(map[string]string{"term": "articular_infinitive", "subtype": "genitive", "page": "2", "focus": "occurrence-b-1-2", "language": "grc"})
	valid := "/vocabulary/concordance?" + query.Encode() + "#occurrence-b-1-2"
	assert.Equal(t, valid, safeConcordanceReturnURL(valid))
	for name, overrides := range map[string]map[string]string{
		"a missing from_run":   {"from_run": ""},
		"a malformed anchor":   {"from_anchor": "-2"},
		"a non-uuid run":       {"from_run": "x"},
		"an unlabelled":        {"subtype": "natural"},
		"an unknown construct": {"term": "optative"},
	} {
		t.Run(name, func(t *testing.T) {
			query := constructionTestQuery(map[string]string{"page": "2", "focus": "occurrence-b-1-2", "language": "grc"})
			for key, value := range overrides {
				if value == "" {
					query.Del(key)
				} else {
					query.Set(key, value)
				}
			}
			assert.Equal(t, "/vocabulary/concordance", safeConcordanceReturnURL("/vocabulary/concordance?"+query.Encode()+"#occurrence-b-1-2"))
		})
	}
	t.Run("the explicit language and upos are checked", func(t *testing.T) {
		with := func(overrides map[string]string) string {
			query := constructionTestQuery(map[string]string{"term": "articular_infinitive", "page": "2", "focus": "occurrence-b-1-2"})
			for key, value := range overrides {
				query.Set(key, value)
			}
			return "/vocabulary/concordance?" + query.Encode() + "#occurrence-b-1-2"
		}
		valid := with(map[string]string{"language": "grc"})
		assert.Equal(t, valid, safeConcordanceReturnURL(valid))
		for name, overrides := range map[string]map[string]string{
			"another language": {"language": "de"}, "no language": {}, "a part of speech": {"language": "grc", "upos": "VERB"},
		} {
			assert.Equal(t, "/vocabulary/concordance", safeConcordanceReturnURL(with(overrides)), name)
		}
	})
	t.Run("origin keys outside a construction lookup", func(t *testing.T) {
		assert.Equal(t, "/vocabulary/concordance", safeConcordanceReturnURL("/vocabulary/concordance?term=haus&as=form&page=2&focus=x&from_run="+constructionTestRun+"&from_sentence=1&from_anchor=1#x"))
	})
}

func fixtureParallels(t *testing.T, term, subtype string) (domain.ConcordanceLookup, domain.ConcordanceResult) {
	t.Helper()
	lookup := domain.ConcordanceLookup{
		Language: "grc", Term: term, Match: domain.ConcordanceMatchConstruction, Subtype: subtype, Page: 1,
		Priority: fixtures.SyntaxBookID,
		Origin:   &domain.ConcordanceConstructionOrigin{RunID: fixtures.SyntaxRunID, Sentence: 0, Anchor: 2},
	}
	result, err := fixtures.NewStore().ListVocabularyConcordance(t.Context(), fixtures.OwnerID, "grc", lookup)
	require.NoError(t, err)
	require.False(t, result.InvalidOrigin)
	return lookup, result
}

func renderParallels(t *testing.T, lookup domain.ConcordanceLookup, result domain.ConcordanceResult) string {
	t.Helper()
	var rendered strings.Builder
	view := VocabularyConcordancePageView(domain.User{}, "", "grc", lookup, result, true, false, "", "", browseOrigin{})
	require.NoError(t, view.Render(t.Context(), &rendered))
	return rendered.String()
}

func TestConcordanceParallelsPageHeadsEachTierOnceAndReusesTheExtract(t *testing.T) {
	lookup, result := fixtureParallels(t, "genitive_absolute", "")
	body := renderParallels(t, lookup, result)
	tiers := regexp.MustCompile(`<h3 class="concordance-tier" id="concordance-tier-([a-z_]+)">([^<]+)</h3>`).FindAllStringSubmatch(body, -1)
	require.Len(t, tiers, 3)
	assert.Equal(t, []string{"This Book", "This author", "Other authors"}, []string{tiers[0][2], tiers[1][2], tiers[2][2]})
	assert.Contains(t, body, `<h2 id="concordance-summary" tabindex="-1">Parallels of Genitive absolute</h2>`)
	assert.Equal(t, 6, strings.Count(body, `<li class="concordance-result">`))
	assert.Contains(t, body, `<div class="concordance-construction">`)
	assert.Contains(t, body, `<p class="sr-only">Words: λέγει … ἄνδρας καλούς</p>`)
	assert.Contains(t, body, `value=""`, "a construction id is never a search term")
	assert.NotContains(t, body, "the Book you were reading", "the origin Book is not a reading priority")
	assert.NotContains(t, body, "concordance-parallels-widen", "an unfiltered lookup has nothing to widen")
	assert.Contains(t, body, `id="concordance-parallels-origin"`)
}

func TestConcordanceParallelsStudyLinkFocusesTheConstruction(t *testing.T) {
	lookup, result := fixtureParallels(t, "genitive_absolute", "")
	study := vocabularySentenceStudyURL(result.Occurrences[0], concordanceAppliedLookup(lookup, result), browseOrigin{})
	parsed, err := url.Parse(study)
	require.NoError(t, err)
	assert.Equal(t, "#construction-genitive_absolute-2", "#"+parsed.Fragment)
	assert.Equal(t, "genitive_absolute", parsed.Query().Get("construction"))
	assert.Equal(t, "2", parsed.Query().Get("target"))
	assert.Equal(t, safeConcordanceReturnURL(parsed.Query().Get("return")), parsed.Query().Get("return"), "the way back is a safe return")
}

func TestConcordanceParallelsWidenLinkDropsOnlyTheSubtype(t *testing.T) {
	lookup, result := fixtureParallels(t, "indirect_statement", "infinitive")
	body := renderParallels(t, lookup, result)
	assert.Contains(t, body, "Parallels of Indirect statement (infinitive)")
	assert.Contains(t, body, ">All Indirect statement</a>")
	widened := parseURL(t, concordanceWidenURL(lookup))
	assert.Empty(t, widened.Get("subtype"))
	assert.Equal(t, fixtures.SyntaxBookID, widened.Get("priority"))
	assert.Equal(t, fixtures.SyntaxRunID, widened.Get("from_run"))
	assert.Equal(t, "1", widened.Get("page"))
}

func TestConcordanceParallelsEmptyResultIsACorpusResult(t *testing.T) {
	lookup, result := fixtureParallels(t, "result", "")
	assert.Empty(t, result.Occurrences)
	assert.Contains(t, renderParallels(t, lookup, result), "No other analyzed occurrences of Result clause in your grc Books. This is a corpus result, not a lookup failure.")
}

func TestConcordanceTierGroupsKeepTheOrderOfOnePage(t *testing.T) {
	occurrence := func(tier string) domain.ConcordanceResultOccurrence {
		return domain.ConcordanceResultOccurrence{Tier: tier}
	}
	groups := concordanceTierGroups([]domain.ConcordanceResultOccurrence{
		occurrence(domain.ConcordanceTierThisAuthor), occurrence(domain.ConcordanceTierThisAuthor), occurrence(domain.ConcordanceTierOtherAuthors),
	})
	require.Len(t, groups, 2, "a page that starts mid-sequence still names its first tier")
	assert.Equal(t, domain.ConcordanceTierThisAuthor, groups[0].Tier)
	assert.Len(t, groups[0].Occurrences, 2)
	assert.Equal(t, "Other authors", concordanceTierLabel(groups[1].Tier))
	assert.Empty(t, concordanceTierGroups(nil))
}

func TestConcordanceOriginStudyURLNeedsAnOrigin(t *testing.T) {
	assert.Empty(t, concordanceOriginStudyURL(domain.ConcordanceLookup{Term: "purpose"}, domain.ConcordanceResult{}))
}

func TestSentenceStudyFocusesOnlyTheEntryWhoseAnchorIsTheTarget(t *testing.T) {
	study := fixtures.NewStore()
	got, err := study.GetVocabularySentenceStudy(t.Context(), fixtures.OwnerID, fixtures.SyntaxBookID, fixtures.SyntaxRunID, "corpus", fixtures.SyntaxUnitID, 0, 2, "ἄνδρας")
	require.NoError(t, err)
	focused := func(entries []sentenceStudyConstructionEntry) []string {
		var ids []string
		for _, entry := range entries {
			if entry.Focused {
				ids = append(ids, entry.ID)
			}
		}
		return ids
	}
	assert.Equal(t, []string{"genitive_absolute"}, focused(sentenceStudyFocusedConstructionEntries(got, "genitive_absolute")))
	assert.Empty(t, focused(sentenceStudyFocusedConstructionEntries(got, "purpose")), "the anchor is not the target")
	assert.Empty(t, focused(sentenceStudyFocusedConstructionEntries(got, "")))
	var order []string
	for _, entry := range sentenceStudyFocusedConstructionEntries(got, "genitive_absolute") {
		order = append(order, entry.ID)
	}
	assert.Equal(t, []string{"indirect_statement", "genitive_absolute", "purpose"}, order, "focus never reorders the list")
	var rendered strings.Builder
	require.NoError(t, VocabularySentenceStudyPageView(domain.User{}, "", got, "/vocabulary/concordance", "genitive_absolute").Render(t.Context(), &rendered))
	assert.Equal(t, 1, strings.Count(rendered.String(), "· from Concordance"))
	assert.Contains(t, rendered.String(), `<li id="construction-genitive_absolute-2" class="sentence-study-construction sentence-study-construction--focused">`)
}
