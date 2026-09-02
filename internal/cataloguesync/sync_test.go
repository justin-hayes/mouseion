package cataloguesync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestEligibleLanguagesIntersectsSavedReadyAndExcludesEnglish(t *testing.T) {
	profiles := []domain.LanguageProfile{
		{Language: "de-DE", DisplayName: "German"},
		{Language: "en", DisplayName: "English"},
		{Language: "it", DisplayName: "Italian"},
		{Language: "fr", DisplayName: "French"},
	}
	capabilities := analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", Ready: true},
		{Language: "en", Ready: true},
		{Language: "it", Ready: false},
		{Language: "fr", Ready: true},
	}}
	got := eligibleLanguages(profiles, capabilities)
	if len(got) != 2 || got[0].profile.Language != "de-DE" || got[1].profile.Language != "fr" {
		t.Fatalf("eligible languages=%+v", got)
	}
}

func TestSyncArgsNeverSerializeCredentials(t *testing.T) {
	args := SyncArgs{OwnerID: "owner", ConnectionID: "connection"}
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "password") || strings.Contains(text, "secret") {
		t.Fatalf("sync args contain credentials: %s", text)
	}
}

func TestSafeSyncErrorIsActionableWithoutCredential(t *testing.T) {
	secret := "plain-password-must-not-escape"
	err := safeSyncError(errors.New("opds: HTTP 401 Unauthorized: "+secret), domain.OpdsConnection{Name: "Home", URL: "https://catalog.example/opds", Password: secret})
	if !strings.Contains(err.Error(), "authentication failed") || strings.Contains(err.Error(), secret) {
		t.Fatalf("safe error=%q", err)
	}
}

func TestWorkerUnavailableDoesNotExposeInput(t *testing.T) {
	var worker *Worker
	if err := worker.Work(context.Background(), nil); err == nil || strings.Contains(err.Error(), "password") {
		t.Fatalf("worker error=%v", err)
	}
}
