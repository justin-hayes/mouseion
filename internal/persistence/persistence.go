// Package persistence defines PostgreSQL-backed core storage boundaries.
package persistence

import "context"

// Store is the minimal lifecycle shared by persistence implementations.
type Store interface {
	Ping(context.Context) error
	Close() error
}
