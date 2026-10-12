// Package deploysecret validates the deployment's MOUSEION_SECRET.
//
// OPDS passwords are encrypted with AES-256-GCM, and the key is derived from
// MOUSEION_SECRET, so a missing or weak secret would make existing credentials
// unreadable or guessable. Both server startup and persistence's credential
// encryption need this check, and it concerns deployment configuration rather
// than storage, so it lives in a leaf package that either may import
// (ADR 0091 §6).
package deploysecret

import (
	"errors"
	"strings"
)

// ErrSecretRequired reports a missing or whitespace-only MOUSEION_SECRET.
var ErrSecretRequired = errors.New("deploysecret: MOUSEION_SECRET is required for OPDS credentials")

// ErrSecretWeak reports a MOUSEION_SECRET shorter than 32 bytes.
var ErrSecretWeak = errors.New("deploysecret: MOUSEION_SECRET must be at least 32 bytes")

// ValidateSecret rejects a missing or weak MOUSEION_SECRET. The key for OPDS
// credential encryption is derived from it; changing the secret makes existing
// credentials unreadable.
func ValidateSecret(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return ErrSecretRequired
	}
	if len(secret) < 32 {
		return ErrSecretWeak
	}
	return nil
}
