package olaarchive

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/justin-hayes/mouseion/internal/classical"
)

const (
	endRecordSignature  = 0x06054b50
	endRecordFixed      = 22
	maxCommentLength    = 1 << 16
	directorySignature  = 0x02014b50
	directoryFixed      = 46
	zip64Sentinel       = 0xFFFFFFFF
	maxDirectoryBytes   = 256 << 20
	maxPinnedMemberSize = 512 << 20
)

// Directory is the archive's central directory, read without the members.
type Directory struct {
	entries map[string]directoryEntry
}

// directoryEntry is what the central directory records about one member.
type directoryEntry struct {
	method         uint16
	headerOffset   int64
	compressedSize int64
	size           int64
	crc32          uint32
}

// ReadDirectory reads the central directory of a zip archive of the given
// size. It supports only archives below 4 GiB with fewer than 65,535 members:
// the pinned OLA archive is one, and a ZIP64 archive is refused rather than
// guessed at.
func ReadDirectory(archive io.ReaderAt, size int64) (*Directory, error) {
	tail := int64(endRecordFixed + maxCommentLength)
	if size < endRecordFixed {
		return nil, errors.New("olaarchive: archive is too small to be a zip file")
	}
	tail = min(tail, size)
	buffer := make([]byte, tail)
	if _, err := archive.ReadAt(buffer, size-tail); err != nil {
		return nil, fmt.Errorf("olaarchive: read the archive tail: %w", err)
	}
	end := -1
	for index := len(buffer) - endRecordFixed; index >= 0; index-- {
		if binary.LittleEndian.Uint32(buffer[index:]) == endRecordSignature {
			end = index
			break
		}
	}
	if end < 0 {
		return nil, errors.New("olaarchive: no zip end record")
	}
	count := int(binary.LittleEndian.Uint16(buffer[end+10:]))
	length := int64(binary.LittleEndian.Uint32(buffer[end+12:]))
	offset := int64(binary.LittleEndian.Uint32(buffer[end+16:]))
	if count == 0xFFFF || length == zip64Sentinel || offset == zip64Sentinel {
		return nil, errors.New("olaarchive: ZIP64 archives are not supported")
	}
	if length > maxDirectoryBytes || offset+length > size {
		return nil, errors.New("olaarchive: central directory lies outside the archive")
	}
	data := make([]byte, length)
	if _, err := archive.ReadAt(data, offset); err != nil {
		return nil, fmt.Errorf("olaarchive: read the central directory: %w", err)
	}
	return parseDirectory(data, count)
}

// parseDirectory reads count central-directory records.
func parseDirectory(data []byte, count int) (*Directory, error) {
	directory := &Directory{entries: make(map[string]directoryEntry, count)}
	for range count {
		if len(data) < directoryFixed || binary.LittleEndian.Uint32(data) != directorySignature {
			return nil, errors.New("olaarchive: malformed central directory")
		}
		nameLength := int(binary.LittleEndian.Uint16(data[28:]))
		skip := directoryFixed + nameLength + int(binary.LittleEndian.Uint16(data[30:])) + int(binary.LittleEndian.Uint16(data[32:]))
		if len(data) < skip {
			return nil, errors.New("olaarchive: malformed central directory")
		}
		entry := directoryEntry{
			method:         binary.LittleEndian.Uint16(data[10:]),
			crc32:          binary.LittleEndian.Uint32(data[16:]),
			compressedSize: int64(binary.LittleEndian.Uint32(data[20:])),
			size:           int64(binary.LittleEndian.Uint32(data[24:])),
			headerOffset:   int64(binary.LittleEndian.Uint32(data[42:])),
		}
		if entry.compressedSize == zip64Sentinel || entry.size == zip64Sentinel || entry.headerOffset == zip64Sentinel {
			return nil, errors.New("olaarchive: ZIP64 members are not supported")
		}
		name := string(data[directoryFixed : directoryFixed+nameLength])
		if _, duplicate := directory.entries[name]; duplicate {
			return nil, fmt.Errorf("olaarchive: the archive lists %q twice", name)
		}
		directory.entries[name] = entry
		data = data[skip:]
	}
	return directory, nil
}

// Pin returns the complete pin of one member and its uncompressed bytes. The
// central directory supplies the offset, sizes and CRC-32; the member's local
// header is then read and the data inflated, checked against the directory,
// and hashed to give the SHA-256.
func (d *Directory) Pin(archive io.ReaderAt, archiveSize int64, name string) (classical.ArchiveMember, []byte, error) {
	entry, ok := d.entries[name]
	if !ok {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: the archive has no member %q", name)
	}
	if entry.size > maxPinnedMemberSize {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: member %q is larger than %d bytes", name, maxPinnedMemberSize)
	}
	member := classical.ArchiveMember{
		Name:           name,
		HeaderOffset:   entry.headerOffset,
		CompressedSize: entry.compressedSize,
		Size:           entry.size,
		CRC32:          hex.EncodeToString(binary.BigEndian.AppendUint32(nil, entry.crc32)),
		SHA256:         "0000000000000000000000000000000000000000000000000000000000000000",
	}
	start, end, err := Window(member, archiveSize)
	if err != nil {
		return classical.ArchiveMember{}, nil, err
	}
	window := make([]byte, end-start+1)
	if read, err := archive.ReadAt(window, start); err != nil && (!errors.Is(err, io.EOF) || read != len(window)) {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: read member %q: %w", name, err)
	}
	data, err := extract(window, member, false)
	if err != nil {
		return classical.ArchiveMember{}, nil, err
	}
	member.SHA256 = sha256Hex(data)
	return member, data, nil
}

// PinNested pins a member of a stored inner zip held inside the archive. The
// real OLA archive keeps its per-Work members in stored inner zips, so the
// member's local header lies at an absolute offset in the outer archive: the
// inner zip's data start plus the member's offset within the inner zip. The
// returned pin carries that absolute offset, so the single-level reader
// fetches it unchanged. An inner zip that is compressed cannot be addressed by
// a byte window and is refused.
func (d *Directory) PinNested(archive io.ReaderAt, archiveSize int64, container, name string) (classical.ArchiveMember, []byte, error) {
	entry, ok := d.entries[container]
	if !ok {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: the archive has no member %q", container)
	}
	if entry.method != methodStore || entry.compressedSize != entry.size {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: container %q is not stored, so its members cannot be pinned by offset", container)
	}
	start, err := dataStart(archive, archiveSize, container, entry)
	if err != nil {
		return classical.ArchiveMember{}, nil, err
	}
	inner := io.NewSectionReader(archive, start, entry.size)
	directory, err := ReadDirectory(inner, entry.size)
	if err != nil {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: container %q: %w", container, err)
	}
	member, data, err := directory.Pin(inner, entry.size, name)
	if err != nil {
		return classical.ArchiveMember{}, nil, fmt.Errorf("olaarchive: container %q: %w", container, err)
	}
	member.HeaderOffset += start
	if _, _, err := Window(member, archiveSize); err != nil {
		return classical.ArchiveMember{}, nil, err
	}
	return member, data, nil
}

// dataStart returns the absolute offset of a member's data, read from its local
// header in the archive.
func dataStart(archive io.ReaderAt, archiveSize int64, name string, entry directoryEntry) (int64, error) {
	header := make([]byte, localHeaderFixed)
	if entry.headerOffset < 0 || entry.headerOffset+localHeaderFixed > archiveSize {
		return 0, fmt.Errorf("olaarchive: member %q lies outside the archive", name)
	}
	if _, err := archive.ReadAt(header, entry.headerOffset); err != nil {
		return 0, fmt.Errorf("olaarchive: read the local header of %q: %w", name, err)
	}
	if binary.LittleEndian.Uint32(header) != localHeaderSignature {
		return 0, fmt.Errorf("olaarchive: no local header for %q at its offset", name)
	}
	start := entry.headerOffset + localHeaderFixed + int64(binary.LittleEndian.Uint16(header[26:])) + int64(binary.LittleEndian.Uint16(header[28:]))
	if start+entry.size > archiveSize {
		return 0, fmt.Errorf("olaarchive: member %q lies outside the archive", name)
	}
	return start, nil
}
