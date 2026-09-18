package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// EPUBContentDigest identifies the exact imported EPUB byte stream.
func EPUBContentDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}
