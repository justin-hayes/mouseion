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
		member, bytes, err := pinMember(directory, archive, info.Size(), []string{spec.Container, spec.CitationContainer}[index], name)
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

// pinMember pins one member: at the archive's top level, or inside a stored
// inner zip, where the pin is an absolute offset in the outer archive so the
// single-level reader fetches it unchanged.
func pinMember(directory *olaarchive.Directory, archive *os.File, size int64, container, name string) (classical.ArchiveMember, []byte, error) {
	if container == "" {
		return directory.Pin(archive, size, name)
	}
	return directory.PinNested(archive, size, container, name)
}

// admitLatinWork runs the text gates over a Work and records the report. A
// Work that fails a gate is refused unless its reviewer note clears it.
func (b *builder) admitLatinWork(override classical.Override, conllu, paula []byte) error {
	scan, err := ancientgreek.ScanLatinWork(conllu, paula)
	var report WorkGateReport
	if err != nil {
		// An unreadable Citation layer is still a result for the report.
		report = WorkGateReport{WorkKey: override.WorkKey, Evaluated: true, Gates: []GateResult{{Name: GateCitations, Detail: "the Citation layer or CoNLL-U is unreadable", Clearable: false}}}
	} else {
		report = evaluateLatinGates(override, scan)
	}
	if report.Admitted() {
		// The join gates still to run need the planned unit; they finish the report.
		b.pending[override.WorkKey] = report
		return nil
	}
	b.recordGates(report)
	return &GateError{Report: report}
}

// errNeedsLSIndex stops a generate run that has Latin Works and no dictionary
// index: gate (iv) cannot be skipped there.
var errNeedsLSIndex = errors.New("classical: generating a Latin Work needs the derived dictionary index (-ls-index) for the dictionary-match gate")

// admitLatinJoins runs gates (iii) and (iv) and the spot-check record over a
// Work whose text gates passed, then records its complete report. The Work is
// refused unless every gate passed or a reviewer note clears the failures.
func (b *builder) admitLatinJoins(override classical.Override, source *ancientgreek.ManifestSource, units []plannedUnit, translationLayer classical.Layer, hasTranslationLayer bool) error {
	report, ok := b.pending[override.WorkKey]
	if !ok {
		return nil
	}
	delete(b.pending, override.WorkKey)
	evidence, err := source.LatinEvidence()
	if err != nil {
		return err
	}
	if override.Translation != nil {
		report.Gates = append(report.Gates, b.translationGates(override, evidence, units[0], translationLayer, hasTranslationLayer)...)
	} else {
		report.Gates = append(report.Gates, GateResult{Name: GateTranslationJoin, Passed: true, NotEvaluated: true, Detail: "the Work declares no translation"})
	}
	switch {
	case b.options.LatinLS != nil:
		report.Gates = append(report.Gates, dictionaryMatchGate(evidence.Occurrences, b.options.LatinLS))
	case b.options.Generating:
		return errNeedsLSIndex
	default:
		report.Gates = append(report.Gates, GateResult{Name: GateDictionaryMatch, Passed: true, NotEvaluated: true, Detail: "no dictionary index was given; generation evaluates it"})
	}
	b.recordGates(report)
	if report.Admitted() {
		return nil
	}
	return &GateError{Report: report}
}

// translationGates evaluates gate (iii) and the spot check against the
// Work's pinned translation file. A file the run does not have is not
// evaluated, which verify mode allows; generation requires it elsewhere.
func (b *builder) translationGates(override classical.Override, evidence ancientgreek.LatinWorkEvidence, unit plannedUnit, layer classical.Layer, hasLayer bool) []GateResult {
	notEvaluated := func(detail string) []GateResult {
		return []GateResult{{Name: GateTranslationJoin, Passed: true, NotEvaluated: true, Detail: detail}}
	}
	if !hasLayer {
		return []GateResult{{Name: GateTranslationJoin, Detail: "the translation layer is not admissible"}}
	}
	path, pathErr := resolveUpstream(b.options.UpstreamDir, override.Translation.FileName)
	if pathErr != nil {
		return notEvaluated("the translation file was absent")
	}
	data, size, digest, err := readUpstream(path)
	if errors.Is(err, os.ErrNotExist) {
		return notEvaluated("the translation file was absent")
	}
	if err == nil {
		err = verifyLayerPin(layer, override.Translation.FileName, size, digest)
	}
	var segments []ancientgreek.ReferenceTranslation
	if err == nil {
		segments, err = ancientgreek.ParseReferenceTranslation(data, unit.start, unit.end)
	}
	if err != nil {
		return []GateResult{{Name: GateTranslationJoin, Detail: "the translation cannot be read against the Work"}}
	}
	return []GateResult{translationJoinGate(evidence.SentenceCitations, segments), spotCheckGate(override.SpotCheck, evidence.SentenceCitations)}
}
