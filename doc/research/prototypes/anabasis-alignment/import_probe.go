//go:build ignore

// Throwaway, no-database probe. Explicitly run this file, never the server.
package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
)

type capture struct {
	path string
}

func (c capture) WriteImportedAnalysis(_ context.Context, req ancientgreek.ImportWriteRequest) (ancientgreek.ImportWriteResult, error) {
	// A local review asset, not operator output: keep Greek out of stdout/logs.
	data, err := json.Marshal(req.Result)
	if err != nil {
		return ancientgreek.ImportWriteResult{}, err
	}
	err = os.WriteFile(c.path, data, 0o600)
	return ancientgreek.ImportWriteResult{}, err
}

func main() {
	if len(os.Args) != 3 {
		panic("usage: go run import_probe.go PINNED_OGA_FILE PRIVATE_OUTPUT_JSON")
	}
	report, err := ancientgreek.Import(context.Background(), ancientgreek.ImportRequest{
		OwnerID: "throwaway-prototype", BookID: "throwaway-prototype",
		SourcePath: os.Args[1], Definition: ancientgreek.AnabasisBook1,
	}, capture{path: os.Args[2]})
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		panic(err)
	}
}
