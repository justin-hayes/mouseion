// Package olaarchive reads one pinned member of the Opera Latina Adnotata zip
// archive from a byte window fetched with an HTTP Range request. It parses the
// member's local header, inflates it and verifies it against the reviewed pin,
// so preparation never downloads the 3.9 GB archive.
package olaarchive

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/justin-hayes/mouseion/internal/classical"
)

const (
	localHeaderSignature = 0x04034b50
	localHeaderFixed     = 30
	methodStore          = 0
	methodDeflate        = 8
	flagDataDescriptor   = 1 << 3
	// extraSlack is how many bytes the requested window reserves for the local
	// header's extra field, which the pin does not record.
	extraSlack = 1024
)

// Window returns the inclusive byte range to request for a member: its local
// header, its name, room for an extra field and its compressed data, clamped to
// the archive. A member that does not lie inside the archive is a pin mismatch.
func Window(member classical.ArchiveMember, archiveSize int64) (start, end int64, err error) {
	if err := member.Validate(); err != nil {
		return 0, 0, mismatch("member pin is incomplete")
	}
	start = member.HeaderOffset
	end = start + localHeaderFixed + int64(len(member.Name)) + extraSlack + member.CompressedSize - 1
	if start >= archiveSize || start+localHeaderFixed+int64(len(member.Name))+member.CompressedSize > archiveSize {
		return 0, 0, mismatch("member lies outside the archive")
	}
	return start, min(end, archiveSize-1), nil
}

// Extract returns the member's uncompressed bytes from a window that starts at
// its local header. Every refusal is a pin mismatch: the header must name the
// pinned member, the data must inflate to exactly the pinned size, and its
// CRC-32 and SHA-256 must match.
func Extract(window []byte, member classical.ArchiveMember) ([]byte, error) {
	if len(window) < localHeaderFixed || binary.LittleEndian.Uint32(window) != localHeaderSignature {
		return nil, mismatch("no local header at the pinned offset")
	}
	flags := binary.LittleEndian.Uint16(window[6:])
	method := binary.LittleEndian.Uint16(window[8:])
	compressed := int64(binary.LittleEndian.Uint32(window[18:]))
	nameLength := int(binary.LittleEndian.Uint16(window[26:]))
	extraLength := int(binary.LittleEndian.Uint16(window[28:]))
	dataStart := localHeaderFixed + nameLength + extraLength
	if dataStart > len(window) || string(window[localHeaderFixed:localHeaderFixed+nameLength]) != member.Name {
		return nil, mismatch("local header does not name the pinned member")
	}
	if flags&flagDataDescriptor == 0 && compressed != member.CompressedSize {
		return nil, mismatch("local header size disagrees with the pin")
	}
	if int64(len(window)-dataStart) < member.CompressedSize {
		return nil, mismatch("window ends inside the member")
	}
	raw := window[dataStart : int64(dataStart)+member.CompressedSize]
	data, err := inflate(raw, method, member.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != member.Size {
		return nil, mismatch("member size disagrees with the pin")
	}
	if checksum := crc32.ChecksumIEEE(data); hex.EncodeToString(binary.BigEndian.AppendUint32(nil, checksum)) != member.CRC32 {
		return nil, mismatch("member CRC-32 disagrees with the pin")
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != member.SHA256 {
		return nil, mismatch("member SHA-256 disagrees with the pin")
	}
	return data, nil
}

// inflate decodes the member's data, never producing more than limit+1 bytes.
func inflate(raw []byte, method uint16, limit int64) ([]byte, error) {
	switch method {
	case methodStore:
		return raw, nil
	case methodDeflate:
		data, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), limit+1))
		if err != nil {
			return nil, mismatch("member data does not inflate")
		}
		return data, nil
	default:
		return nil, mismatch(fmt.Sprintf("unsupported compression method %d", method))
	}
}

func mismatch(detail string) error {
	return &classical.PreparationError{Code: classical.CodePinMismatch, Detail: detail}
}
