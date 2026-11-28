package download

import (
	"crypto/md5" //nolint:gosec // KOReader identifies documents by partial MD5; this is identity, not security.
	"encoding/hex"
	"hash"
)

const (
	partialMD5SampleSize = 1024
	partialMD5Samples    = 12
)

// PartialMD5 is a streaming writer that computes KOReader's Document digest:
// the MD5 of up to twelve 1024-byte samples taken at offsets 0, 1 KiB, 4 KiB,
// 16 KiB and so on by factors of four up to 1 GiB. Those are 1024 << 2i for i
// from -1 to 10, where i = -1 gives 0 under LuaJIT's masked shift. An offset
// at or past the end of the stream contributes nothing and a sample cut short
// by the end contributes the bytes that exist. Offsets only increase, so the
// writer takes the bytes as they pass and needs no random access.
type PartialMD5 struct {
	hash     hash.Hash
	position int64
}

// NewPartialMD5 returns an empty writer.
func NewPartialMD5() *PartialMD5 {
	return &PartialMD5{hash: md5.New()} //nolint:gosec // see the import.
}

// partialMD5Offset returns the offset of sample index (0 to 11).
func partialMD5Offset(index int) int64 {
	if index == 0 {
		return 0
	}
	return int64(partialMD5SampleSize) << (2 * (index - 1))
}

// Write feeds the next bytes of the stream. It never fails.
func (p *PartialMD5) Write(data []byte) (int, error) {
	start, end := p.position, p.position+int64(len(data))
	for index := range partialMD5Samples {
		from := partialMD5Offset(index)
		to := from + partialMD5SampleSize
		if from >= end {
			break
		}
		if to <= start {
			continue
		}
		low, high := max(from, start), min(to, end)
		_, _ = p.hash.Write(data[low-start : high-start])
	}
	p.position = end
	return len(data), nil
}

// PartialMD5Of returns the Document digest of data held in memory.
func PartialMD5Of(data []byte) string {
	writer := NewPartialMD5()
	_, _ = writer.Write(data)
	return writer.Sum()
}

// Sum returns the 32 lower-case hex digits for the bytes written so far.
func (p *PartialMD5) Sum() string {
	return hex.EncodeToString(p.hash.Sum(nil))
}
