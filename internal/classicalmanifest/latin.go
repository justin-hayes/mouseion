package classicalmanifest

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/olaarchive"
)

// readLatinSource pins and parses one OLA Work from the upstream archive. The
// zip directory supplies each member's offset, sizes and CRC-32, and the
// member bytes supply its SHA-256; layers.json holds the reviewed size and
// SHA-256 of both members. The Work then passes the text gates. A missing
// archive is reported through missing, not as an error.
func (b *builder) readLatinSource(override classical.Override, layer classical.Layer) (*ancientgreek.ManifestSource, classical.SourcePin, bool, error, error) {
	spec := *override.Source
	archivePath, pathErr := resolveUpstream(b.options.UpstreamDir, path.Base(layer.ArchiveURL))
	if pathErr != nil {
		return nil, classical.SourcePin{}, false, pathErr, nil
	}
	archive, err := os.Open(archivePath) //nolint:gosec // the path is resolved under the maintainer-supplied upstream root.
	if errors.Is(err, os.ErrNotExist) {
		return nil, classical.SourcePin{}, false, fmt.Errorf("classical: archive %q for %s is missing", path.Base(layer.ArchiveURL), override.WorkKey), nil
	}
	if err != nil {
		return nil, classical.SourcePin{}, false, nil, err
	}
	members, data, err := pinArchiveMembers(archive, layer, spec)
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, classical.SourcePin{}, false, nil, err
	}
	if err := b.admitLatinWork(override, data[0], data[1]); err != nil {
		return nil, classical.SourcePin{}, false, nil, err
	}
	source, err := ancientgreek.ParseLatinManifestSource(data[0], data[1])
	if err != nil {
		return nil, classical.SourcePin{}, false, nil, err
	}
	pin := classical.SourcePin{
		Layer: spec.Layer, FileName: members[0].Name, Size: members[0].Size, SHA256: members[0].SHA256,
		Format: spec.Format, Members: members[:],
	}
	return source, pin, true, nil, nil
}

// pinArchiveMembers reads the zip directory of the open archive and returns
// the CoNLL-U and PAULA members' pins and bytes, in that order. Each must match
// the reviewed size and SHA-256 in layers.json.
func pinArchiveMembers(archive *os.File, layer classical.Layer, spec classical.SourceSpec) ([2]classical.ArchiveMember, [2][]byte, error) {
	var members [2]classical.ArchiveMember
	var data [2][]byte
	info, err := archive.Stat()
	if err != nil {
		return members, data, err
	}
	if info.Size() != layer.ArchiveSize {
		return members, data, fmt.Errorf("classical: archive %q is %d bytes, layers.json pins %d", path.Base(layer.ArchiveURL), info.Size(), layer.ArchiveSize)
	}
	directory, err := olaarchive.ReadDirectory(archive, info.Size())
	if err != nil {
		return members, data, err
	}
	for index, name := range []string{spec.FileName, spec.CitationFileName} {
		member, bytes, err := directory.Pin(archive, info.Size(), name)
		if err != nil {
			return members, data, err
		}
		if err := verifyLayerPin(layer, name, member.Size, member.SHA256); err != nil {
			return members, data, err
		}
		members[index], data[index] = member, bytes
	}
	return members, data, nil
}

// admitLatinWork runs the text gates over a Work and records the report. A
// Work that fails a gate is refused unless its reviewer note clears it.
func (b *builder) admitLatinWork(override classical.Override, conllu, paula []byte) error {
	scan, err := ancientgreek.ScanLatinWork(conllu, paula)
	if err != nil {
		return err
	}
	report := evaluateLatinGates(override, scan)
	b.recordGates(report)
	if report.Admitted() {
		return nil
	}
	return &GateError{Report: report}
}
