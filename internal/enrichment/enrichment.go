// Package enrichment defines vocabulary enrichment boundaries.
package enrichment

import "context"

// Provider enriches a lemma without coupling the core to a specific service.
type Provider interface {
	Enrich(context.Context, string, string) (string, error)
}
