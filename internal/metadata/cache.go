package metadata

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LookupKind partitions cache keys by query shape so an ASIN lookup and a
// text search with the same string don't collide.
type LookupKind string

const (
	LookupKindASIN   LookupKind = "asin"
	LookupKindISBN   LookupKind = "isbn"
	LookupKindSearch LookupKind = "search"
)

// NotFoundTTL is the short TTL applied to cached negative responses.
const NotFoundTTL = 24 * time.Hour

// Cache backs metadata_cache rows. TTL for hits is configured per-instance.
type Cache struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

// NewCache creates a Cache against the given pool. `ttl` is the TTL for
// positive (hit) entries; negative (not_found) entries use NotFoundTTL.
func NewCache(pool *pgxpool.Pool, ttl time.Duration) *Cache {
	return &Cache{pool: pool, ttl: ttl}
}

// Key formats a cache key as "<source>:<kind>:<sha1(region\x00query)>".
// region (operator/caller-controlled) is folded into the hash rather than
// interpolated raw, so a region containing the ':' delimiter can't shift
// field boundaries and collide distinct cached payloads. source and kind are
// fixed internal constants and stay readable for debugging.
func Key(source string, kind LookupKind, region, query string) string {
	sum := sha1.Sum([]byte(region + "\x00" + query))
	return fmt.Sprintf("%s:%s:%s", source, kind, hex.EncodeToString(sum[:]))
}

// ErrCacheMiss is returned by Get when no live (within-TTL) entry exists.
var ErrCacheMiss = errors.New("metadata cache miss")

// Entry is the unmarshalled cache row.
type Entry struct {
	Source    string
	Region    string
	NotFound  bool
	Response  json.RawMessage
	FetchedAt time.Time
}

// Get returns the entry for key if it's within TTL. ErrCacheMiss otherwise.
func (c *Cache) Get(ctx context.Context, key string) (Entry, error) {
	var e Entry
	err := c.pool.QueryRow(ctx, `
		SELECT source, region, not_found, response_json, fetched_at
		FROM metadata_cache WHERE cache_key = $1
	`, key).Scan(&e.Source, &e.Region, &e.NotFound, &e.Response, &e.FetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrCacheMiss
	}
	if err != nil {
		return Entry{}, err
	}
	ttl := c.ttl
	if e.NotFound {
		ttl = NotFoundTTL
	}
	if time.Since(e.FetchedAt) > ttl {
		return Entry{}, ErrCacheMiss
	}
	return e, nil
}

// PutHit caches a positive response.
func (c *Cache) PutHit(ctx context.Context, key, source, region string, response json.RawMessage) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO metadata_cache (cache_key, source, region, response_json, not_found, fetched_at)
		VALUES ($1, $2, $3, $4, FALSE, now())
		ON CONFLICT (cache_key) DO UPDATE
		  SET response_json = EXCLUDED.response_json,
		      not_found = FALSE,
		      fetched_at = now()
	`, key, source, region, response)
	return err
}

// PutNotFound caches a negative response (404 or empty result).
func (c *Cache) PutNotFound(ctx context.Context, key, source, region string) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO metadata_cache (cache_key, source, region, response_json, not_found, fetched_at)
		VALUES ($1, $2, $3, '{}'::jsonb, TRUE, now())
		ON CONFLICT (cache_key) DO UPDATE
		  SET not_found = TRUE,
		      response_json = '{}'::jsonb,
		      fetched_at = now()
	`, key, source, region)
	return err
}

// EvictExpired deletes rows older than the configured TTL (or NotFoundTTL for
// negatives). Returns the number of rows removed.
//
// make_interval(secs => ...) is used instead of $1::interval because Go's
// time.Duration.String() produces "720h0m0s" which Postgres rejects.
func (c *Cache) EvictExpired(ctx context.Context) (int64, error) {
	tag, err := c.pool.Exec(ctx, `
		DELETE FROM metadata_cache
		WHERE (NOT not_found AND fetched_at < now() - make_interval(secs => $1))
		   OR (    not_found AND fetched_at < now() - make_interval(secs => $2))
	`, c.ttl.Seconds(), NotFoundTTL.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
