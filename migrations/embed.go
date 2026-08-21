// Package migrations exposes versioned SQL migrations to the application.
package migrations

import "embed"

// FS contains all forward and backward migrations.
//
//go:embed *.sql
var FS embed.FS
