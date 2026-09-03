package webapp

import (
	"bytes"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

func explicitTestWebKey(purpose string) []byte { return []byte("012345678901234567890123456789" + purpose[:2]) }

func TestWebApplicationRequiresDurableProductionSecret(t *testing.T) {
	for _, test := range []struct {
		name   string
		secret string
		want   error
	}{
		{name: "missing", want: persistence.ErrSecretRequired},
		{name: "weak", secret: "too-short", want: persistence.ErrSecretWeak},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("MOUSEION_SECRET", test.secret)
			if _, err := NewWithError(Services{}); !errors.Is(err, test.want) {
				t.Fatalf("NewWithError() error=%v want %v", err, test.want)
			}
		})
	}
}

func TestConfiguredWebKeysAreStableAcrossHandlersAndRestarts(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "stable-web-application-secret-0123456789")
	first, err := NewWithError(Services{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewWithError(Services{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.acquisitionKey, second.acquisitionKey) || !bytes.Equal(first.targetKey, second.targetKey) {
		t.Fatal("configured web keys changed between handlers")
	}
	payload := acquisitionCookiePayload{OwnerID: "owner", SessionHash: "session", Entries: []acquisitionEntryState{{Connection: "connection", Language: "de", EntryID: "entry", SourceID: "source"}}}
	_, cookie, err := encodeAcquisitionCookie(first.acquisitionKey, payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.acquisitionStateFor(cookie, "owner", "session"); len(got) != 1 || got[0].SourceID != "source" {
		t.Fatalf("second handler could not read first handler cookie: %+v", got)
	}
	targetToken := first.clientTargetToken("connection", "de", nil, "https://catalog.example/book.epub")
	if _, err := decodeAcquisitionTarget(second.targetKey, targetToken); err != nil {
		t.Fatalf("second handler could not read first handler target: %v", err)
	}
}

func TestExplicitWebKeysAllowDeterministicNonProductionHandlers(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "")
	if _, err := NewWithError(Services{AcquisitionKey: explicitTestWebKey("cookie"), AcquisitionTargetKey: explicitTestWebKey("target")}); err != nil {
		t.Fatalf("explicit deterministic keys rejected: %v", err)
	}
}
