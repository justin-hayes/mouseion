// Package cardexport contains Anki-compatible card export behavior.
package cardexport

// Card is the minimal portable representation of a study card.
type Card struct {
	Front string
	Back  string
}
