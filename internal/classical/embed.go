package classical

import _ "embed"

// The release embeds the reviewed artifacts. Runtime reconciliation reads them
// from the binary; nothing is fetched at startup.
var (
	//go:embed manifest.jsonl
	ManifestJSONL []byte

	//go:embed overrides.jsonl
	OverridesJSONL []byte

	//go:embed layers.json
	LayersJSON []byte
)
