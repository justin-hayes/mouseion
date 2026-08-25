package webapp

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
)

type testCapabilities struct {
	value analyzer.Capabilities
	err   error
}

func (t testCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return t.value, t.err
}

func TestSupportedNLPOnlyReturnsReadyLanguages(t *testing.T) {
	h := &Handler{services: Services{Capabilities: testCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", DisplayName: "German", Ready: true},
		{Language: "it", DisplayName: "Italian", Ready: true},
		{Language: "fr", DisplayName: "French", Ready: false},
	}}}}}
	languages, degraded := h.supportedNLP(context.Background())
	if degraded || len(languages) != 2 || languages[0].Language != "de" || languages[0].DisplayName != "German" || languages[1].Language != "it" || languages[1].DisplayName != "Italian" {
		t.Fatalf("languages = %+v, degraded = %v", languages, degraded)
	}
}

func TestSupportedNLPDegradesWhenDiscoveryFails(t *testing.T) {
	h := &Handler{services: Services{Capabilities: testCapabilities{err: errors.New("unavailable")}}}
	languages, degraded := h.supportedNLP(context.Background())
	if !degraded || len(languages) != 0 {
		t.Fatalf("languages = %+v, degraded = %v", languages, degraded)
	}
}

func TestSettingsPageKeepsSavedProfilesVisibleWhenDiscoveryIsDegraded(t *testing.T) {
	var output bytes.Buffer
	err := SettingsPage(
		domain.User{Username: "learner"}, "csrf", nil,
		[]domain.LanguageProfile{{Language: "de", DisplayName: "German"}}, true,
		"", nil, nil, "",
	).Render(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	body := output.String()
	if !bytes.Contains([]byte(body), []byte("Your saved study languages are unchanged")) ||
		!bytes.Contains([]byte(body), []byte("German")) {
		t.Fatalf("degraded settings omitted warning or persisted profile: %s", body)
	}
}
