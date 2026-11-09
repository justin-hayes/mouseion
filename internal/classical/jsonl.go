package classical

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ParseDefinitions reads the generated manifest's JSONL. It rejects unknown
// fields so a hand edit cannot silently add an unread field.
func ParseDefinitions(data []byte) ([]Definition, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	definitions := []Definition{}
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var definition Definition
		if err := decodeStrict(line, &definition); err != nil {
			return nil, fmt.Errorf("classical: manifest line %d: %w", len(definitions)+1, err)
		}
		if err := definition.Validate(); err != nil {
			return nil, fmt.Errorf("classical: manifest line %d: %w", len(definitions)+1, err)
		}
		definitions = append(definitions, definition)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("classical: read manifest: %w", err)
	}
	return definitions, nil
}

// ParseOverrides reads overrides.jsonl.
func ParseOverrides(data []byte) ([]Override, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	overrides := []Override{}
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var override Override
		if err := decodeStrict(line, &override); err != nil {
			return nil, fmt.Errorf("classical: overrides line %d: %w", len(overrides)+1, err)
		}
		if err := override.Validate(); err != nil {
			return nil, fmt.Errorf("classical: overrides line %d: %w", len(overrides)+1, err)
		}
		overrides = append(overrides, override)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("classical: read overrides: %w", err)
	}
	SortOverrides(overrides)
	return overrides, nil
}

// ParseLayers reads layers.json.
func ParseLayers(data []byte) (Layers, error) {
	var layers Layers
	if err := decodeStrict(data, &layers); err != nil {
		return Layers{}, fmt.Errorf("classical: layers.json: %w", err)
	}
	if err := layers.Validate(); err != nil {
		return Layers{}, err
	}
	return layers, nil
}

// RenderDefinitions writes definitions as deterministic JSONL in the order
// supplied. Callers sort by Key first. Standard-library encoding only.
func RenderDefinitions(definitions []Definition) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, definition := range definitions {
		if err := encoder.Encode(definition); err != nil {
			return nil, fmt.Errorf("classical: encode definition %q: %w", definition.Key, err)
		}
	}
	return buffer.Bytes(), nil
}

// decodeStrict decodes one value and rejects unknown fields.
func decodeStrict(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after JSON value")
	}
	return nil
}
