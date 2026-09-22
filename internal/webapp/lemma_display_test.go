package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKnownVocabularyFormatsOnlyGermanNouns(t *testing.T) {
	known := []domain.KnownVocabulary{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"},
		{Language: "en", CanonicalLemma: "house", UPOS: "NOUN"},
	}
	var output bytes.Buffer
	require.NoError(t, KnownVocabResult("de", nil, known, "").Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"<td>Haus</td>", "<td>gehen</td>", "<td>house</td>"} {
		assert.True(t, strings.Contains(html, want), "known vocabulary table missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, "<td>haus</td>"), "German noun remained lowercase: %s", html)
}
