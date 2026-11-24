package releasecheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/syntax/agdt"
)

// Fetcher returns the bytes of one pinned OGA file by name. The command backs
// it with a verified download and cache; tests back it with fixtures.
type Fetcher interface {
	Fetch(ctx context.Context, file classical.SourcePin) ([]byte, error)
}

// Verify checks a file's size and SHA-256 against its pin.
func Verify(data []byte, size int64, digest string) error {
	if int64(len(data)) != size {
		return fmt.Errorf("releasecheck: file is %d bytes, pin says %d", len(data), size)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return errors.New("releasecheck: file digest does not match its pin")
	}
	return nil
}

// WorkOf returns the research `work` id of a pinned file name: its two-part
// TLG prefix, such as tlg0059.tlg030.
func WorkOf(fileName string) string {
	parts := strings.SplitN(fileName, ".", 3)
	if len(parts) < 3 {
		return fileName
	}
	return parts[0] + "." + parts[1]
}

// DetectCorpus runs the ungated detectors over every pinned file the manifest
// names (deduplicated by file name) and returns the emitted occurrences.
func DetectCorpus(ctx context.Context, definitions []classical.Definition, fetcher Fetcher) (Detected, error) {
	detected := Detected{Emitted: map[Key]string{}, Hits: map[string]int{}, Subtypes: map[string]map[string]int{}}
	files := map[string][]classical.Definition{}
	var order []string
	for _, definition := range definitions {
		name := definition.Source.FileName
		if _, ok := files[name]; !ok {
			order = append(order, name)
		}
		files[name] = append(files[name], definition)
	}
	for _, name := range order {
		units := files[name]
		data, err := fetcher.Fetch(ctx, units[0].Source)
		if err != nil {
			return Detected{}, err
		}
		if err := Verify(data, units[0].Source.Size, units[0].Source.SHA256); err != nil {
			return Detected{}, fmt.Errorf("%s: %w", name, err)
		}
		parsed, err := ancientgreek.ParseOgaSource(data)
		if err != nil {
			return Detected{}, fmt.Errorf("%s: %w", name, err)
		}
		for _, definition := range units {
			if err := detectUnit(parsed, definition, detected); err != nil {
				return Detected{}, fmt.Errorf("%s: %w", definition.Key, err)
			}
		}
	}
	return detected, nil
}

func detectUnit(parsed *ancientgreek.ParsedWork, definition classical.Definition, detected Detected) error {
	unit, err := parsed.CutUnit(domain.ReadingUnitDefinition{
		Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle,
		WorkAuthor: definition.WorkAuthor, EditionLabel: definition.EditionLabel,
		CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
		SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
		SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
	})
	if err != nil {
		return err
	}
	work := WorkOf(definition.Source.FileName)
	for index, sentence := range unit.Result.Sentences {
		source := unit.Sources[index]
		for _, occurrence := range constructions.DetectUngated(agdt.FromSentence(sentence)) {
			key := Key{work, source.SentID, source.TokenRefs[occurrence.Anchor], string(occurrence.Construction)}
			detected.Emitted[key] = string(occurrence.Subtype)
			detected.Hits[key.Construction]++
			if detected.Subtypes[key.Construction] == nil {
				detected.Subtypes[key.Construction] = map[string]int{}
			}
			detected.Subtypes[key.Construction][string(occurrence.Subtype)]++
		}
	}
	return nil
}
