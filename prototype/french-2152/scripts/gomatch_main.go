package main

import (
	"encoding/json"
	"fmt"
	"os"

	"probe/textmatch"
)

// Mirrors cardexport.targetIndex: clean the form, then FoldedWordSpan.
func main() {
	var cases [][]string
	raw, _ := os.ReadFile(os.Args[1])
	if err := json.Unmarshal(raw, &cases); err != nil {
		panic(err)
	}
	for _, c := range cases {
		form := textmatch.CleanLexicalSurface(c[2])
		_, _, okForm := textmatch.FoldedWordSpan(c[1], form)
		_, _, okLemma := textmatch.FoldedWordSpan(c[1], textmatch.CleanLexicalSurface(c[3]))
		fmt.Printf("%s\t%s\t%s\t%s\tform=%v\tlemma=%v\n", c[0], c[4], c[2], c[3], okForm, okLemma)
	}
}
