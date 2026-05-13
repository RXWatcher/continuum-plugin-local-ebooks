package metadata

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestCachePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("EBOOKSDB_TEST_DSN")
	if dsn == "" {
		t.Skip("EBOOKSDB_TEST_DSN unset; skipping integration cache test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `TRUNCATE metadata_cache`)
		pool.Close()
	})
	pool.Exec(context.Background(), `TRUNCATE metadata_cache`)
	return pool
}

func TestCache_PutAndGetHit(t *testing.T) {
	pool := newTestCachePool(t)
	c := NewCache(pool, 30*24*time.Hour)
	ctx := context.Background()
	key := Key("audnexus", LookupKindASIN, "us", "B0EXAMPLE")
	payload := json.RawMessage(`{"title":"X"}`)
	if err := c.PutHit(ctx, key, "audnexus", "us", payload); err != nil {
		t.Fatal(err)
	}
	e, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.NotFound {
		t.Errorf("expected hit, got not_found")
	}
	if string(e.Response) != string(payload) {
		t.Errorf("response mismatch: got %s", e.Response)
	}
}

func TestCache_NotFoundShortTTL(t *testing.T) {
	pool := newTestCachePool(t)
	c := NewCache(pool, 30*24*time.Hour)
	ctx := context.Background()
	key := Key("audnexus", LookupKindASIN, "us", "B0MISSING")
	if err := c.PutNotFound(ctx, key, "audnexus", "us"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE metadata_cache SET fetched_at = now() - interval '25 hours' WHERE cache_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, key); err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss for aged not_found, got %v", err)
	}
}

func TestCache_KeyIsStable(t *testing.T) {
	a := Key("audnexus", LookupKindASIN, "us", "B0EXAMPLE")
	b := Key("audnexus", LookupKindASIN, "us", "B0EXAMPLE")
	if a != b {
		t.Errorf("keys must be deterministic")
	}
	c := Key("audnexus", LookupKindASIN, "uk", "B0EXAMPLE")
	if a == c {
		t.Errorf("keys must vary by region")
	}
}

func TestCache_EvictExpired(t *testing.T) {
	pool := newTestCachePool(t)
	c := NewCache(pool, 30*24*time.Hour)
	ctx := context.Background()
	key := Key("audnexus", LookupKindASIN, "us", "B0OLD")
	if err := c.PutHit(ctx, key, "audnexus", "us", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE metadata_cache SET fetched_at = now() - interval '31 days' WHERE cache_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	n, err := c.EvictExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 row evicted, got %d", n)
	}
}
