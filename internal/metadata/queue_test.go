package metadata

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestQueuePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("EBOOKSDB_TEST_DSN")
	if dsn == "" {
		t.Skip("EBOOKSDB_TEST_DSN unset")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	pool.Exec(context.Background(),
		`TRUNCATE metadata_enrichment_job, ebook, library_path RESTART IDENTITY CASCADE`)
	pool.Exec(context.Background(), `INSERT INTO library_path (path) VALUES ('/test')`)
	pool.Exec(context.Background(),
		`INSERT INTO ebook (id, library_path_id, path, format, file_size, mtime)
		 VALUES ('test-id', 1, '/test/x.epub', 'epub', 0, now())`)
	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`TRUNCATE metadata_enrichment_job, ebook, library_path RESTART IDENTITY CASCADE`)
		pool.Close()
	})
	return pool
}

func TestQueue_EnqueueAndClaim(t *testing.T) {
	pool := newTestQueuePool(t)
	q := NewQueue(pool)
	ctx := context.Background()
	if err := q.Enqueue(ctx, "test-id"); err != nil {
		t.Fatal(err)
	}
	j, err := q.ClaimNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if j.EbookID != "test-id" {
		t.Errorf("got %q", j.EbookID)
	}
	if j.Attempts != 1 {
		t.Errorf("attempts %d", j.Attempts)
	}
}

func TestQueue_ClaimEmptyReturnsErrQueueEmpty(t *testing.T) {
	pool := newTestQueuePool(t)
	q := NewQueue(pool)
	if _, err := q.ClaimNext(context.Background()); err != ErrQueueEmpty {
		t.Errorf("expected ErrQueueEmpty, got %v", err)
	}
}

func TestQueue_FailedRetriesUntilMax(t *testing.T) {
	pool := newTestQueuePool(t)
	q := NewQueue(pool)
	ctx := context.Background()
	if err := q.Enqueue(ctx, "test-id"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < MaxAttempts; i++ {
		if err := q.MarkFailed(ctx, "test-id", i, "transient"); err != nil {
			t.Fatal(err)
		}
		pool.Exec(ctx, `UPDATE metadata_enrichment_job SET run_after = now() WHERE ebook_id='test-id'`)
		if _, err := q.ClaimNext(ctx); err != nil {
			t.Fatalf("attempt %d claim: %v", i+1, err)
		}
	}
	if err := q.MarkFailed(ctx, "test-id", MaxAttempts, "final"); err != nil {
		t.Fatal(err)
	}
	var status string
	pool.QueryRow(ctx, `SELECT status FROM metadata_enrichment_job WHERE ebook_id='test-id'`).Scan(&status)
	if status != "failed" {
		t.Errorf("expected failed, got %q", status)
	}
}

func TestQueue_MarkCompleted(t *testing.T) {
	pool := newTestQueuePool(t)
	q := NewQueue(pool)
	ctx := context.Background()
	if err := q.Enqueue(ctx, "test-id"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkCompleted(ctx, "test-id"); err != nil {
		t.Fatal(err)
	}
	var status string
	pool.QueryRow(ctx, `SELECT status FROM metadata_enrichment_job WHERE ebook_id='test-id'`).Scan(&status)
	if status != "completed" {
		t.Errorf("expected completed, got %q", status)
	}
}
