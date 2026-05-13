// Package store is the data-access layer over Postgres.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgxpool. Construct one per process; safe for concurrent use.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying pool for callers that need transactions.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping is a health check.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
