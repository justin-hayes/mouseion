package knownvocab

import (
	"encoding/json"
	"testing"
)

func TestJobArgsRoundTripKeepsOwnerAndFileContents(t *testing.T) {
	want := JobArgs{OwnerID: "owner-1", Language: "de", FileContents: "Haus\ngehen\n"}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got JobArgs
	if err = json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got != want || got.Kind() != "import_known_vocabulary" {
		t.Fatalf("args = %+v", got)
	}
}
