//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// confirmLemmaDecision previews and confirms one decision on a δέω occurrence.
func confirmLemmaDecision(t *testing.T, h http.Handler, owner, bookID, decision, lemma, occurrenceID string) {
	t.Helper()
	confirmLemmaDecisionForm(t, h, owner, bookID, "δέω", decision, lemma, occurrenceID)
}

// confirmLemmaDecisionForm previews and confirms one decision on an occurrence
// of a surface form through the shared Lemma decision workflow the remediation
// screen uses, binding the confirm to the fingerprint the preview returned.
func confirmLemmaDecisionForm(t *testing.T, h http.Handler, owner, bookID, form, decision, lemma, occurrenceID string) {
	t.Helper()
	handler, ok := h.(*Handler)
	require.True(t, ok, "the test handler is the web handler")
	request := lemmaDecisionRequest(t, handler, owner, form, decision, lemma)
	step, err := handler.previewLemmaDecisionStep(request, owner, bookID, []string{occurrenceID})
	require.NoError(t, err, "preview %s", decision)
	request.Form.Set("fingerprint", step.Preview.Fingerprint)
	err = handler.confirmLemmaDecisionStep(request, owner, bookID, []string{occurrenceID})
	require.NoError(t, err, "confirm %s", decision)
}

// previewFingerprint reads the stale-state fingerprint a preview page carries.
func previewFingerprint(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(body)
	require.Len(t, match, 2, "preview body has no fingerprint: %s", body)
	return match[1]
}

// lemmaDecisionRequest is a decision form post carrying the owner's shell view,
// whose page language the workflow reads, as the user middleware sets it.
func lemmaDecisionRequest(t *testing.T, handler *Handler, owner, form, decision, lemma string) *http.Request {
	t.Helper()
	values := url.Values{"form": {form}, "decision": {decision}, "lemma": {lemma}}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	view, err := handler.loadShellView(request.Context(), owner)
	require.NoError(t, err)
	return request.WithContext(context.WithValue(request.Context(), shellViewContextKey{}, view))
}
