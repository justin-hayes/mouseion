package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
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

func parseConstructionTestRequest(values url.Values) concordanceRequest {
	return parseConcordanceRequest(httptest.NewRequest(http.MethodGet, "/vocabulary/concordance?"+values.Encode(), nil))
}

func TestConcordanceConstructionRequestParsesTheOrigin(t *testing.T) {
	request := parseConstructionTestRequest(constructionTestQuery(map[string]string{"subtype": "genitive"}))
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
			assert.Nil(t, parseConstructionTestRequest(constructionTestQuery(overrides)).lookup.Origin)
		})
	}
}

func TestConcordanceConstructionRejectionsSendTheSafeRestart(t *testing.T) {
	reject := func(t *testing.T, overrides map[string]string, language string) {
		t.Helper()
		request := parseConstructionTestRequest(constructionTestQuery(overrides))
		status, message, rejected := concordanceRequestRejection(request, request.lookup, language)
		assert.True(t, rejected)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, concordanceNoticeState, message)
	}
	t.Run("a valid request is not rejected", func(t *testing.T) {
		request := parseConstructionTestRequest(constructionTestQuery(map[string]string{"subtype": "genitive", "term": "articular_infinitive"}))
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
		request := parseConstructionTestRequest(constructionTestQuery(map[string]string{"term": "result", "subtype": "natural"}))
		_, _, rejected := concordanceRequestRejection(request, request.lookup, "grc")
		assert.False(t, rejected)
	})
}

func TestConcordanceLemmaAndFormRequestsIgnoreConstructionParameters(t *testing.T) {
	request := parseConstructionTestRequest(url.Values{"as": {"form"}, "term": {"haus"}, "subtype": {"genitive"}, "from_run": {constructionTestRun}})
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
		"page": {"1"}, "subtype": {"genitive"}, "from_run": {constructionTestRun}, "from_sentence": {"3"}, "from_anchor": {"2"},
	}, restart, "a restart drops only rev and page, so it passes the origin rule again")

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
	query := constructionTestQuery(map[string]string{"term": "articular_infinitive", "subtype": "genitive", "page": "2", "focus": "occurrence-b-1-2"})
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
			query := constructionTestQuery(map[string]string{"page": "2", "focus": "occurrence-b-1-2"})
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
	t.Run("origin keys outside a construction lookup", func(t *testing.T) {
		assert.Equal(t, "/vocabulary/concordance", safeConcordanceReturnURL("/vocabulary/concordance?term=haus&as=form&page=2&focus=x&from_run="+constructionTestRun+"&from_sentence=1&from_anchor=1#x"))
	})
}
