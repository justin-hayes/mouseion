package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestKnownVocabularyFormatsOnlyGermanNouns(t *testing.T) {
	known := []domain.KnownVocabulary{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"},
		{Language: "en", CanonicalLemma: "house", UPOS: "NOUN"},
	}
	var output bytes.Buffer
	if err := KnownVocabResult("de", nil, known, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"<td>Haus</td>", "<td>gehen</td>", "<td>house</td>"} {
		if !strings.Contains(html, want) {
			t.Errorf("known vocabulary table missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "<td>haus</td>") {
		t.Errorf("German noun remained lowercase: %s", html)
	}
}
