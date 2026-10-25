package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// EPUBContentDigest identifies the exact imported EPUB byte stream.
func EPUBContentDigest(content []byte) string {
	return EPUBContentDigestFromHash(sha256.Sum256(content))
}

// EPUBContentDigestFromHash formats a SHA-256 sum already computed while
// streaming the EPUB, so callers never need the whole body in memory to
// identify it.
func EPUBContentDigestFromHash(sum [sha256.Size]byte) string {
	return "sha256:" + hex.EncodeToString(sum[:])
}
