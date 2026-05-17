package metadata

import (
	"context"
	"testing"

	"github.com/hashicorp/go-hclog"
)

// A low-confidence text-search result must NOT overwrite local metadata; a
// strong one still does.
func TestWorker_TextSearchConfidenceGate(t *testing.T) {
	t.Run("wrong match rejected", func(t *testing.T) {
		pool := newWorkerTestPool(t)
		q := NewQueue(pool)
		q.Enqueue(context.Background(), "a")
		reg := newFakeEnrichmentRegistry(&fakeWorkerSource{id: "src", out: &Candidate{
			Source: "src", Title: "Totally Unrelated Plumbing Manual",
			Authors: []string{"Someone Else"},
		}})
		fs := &fakeStore{row: EbookRow{ID: "a", Format: "epub", Title: "Foundation", Author: "Isaac Asimov"}}
		w := NewEnrichmentWorker(q, fs, reg, "src", "us", hclog.NewNullLogger())
		if err := w.Drain(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fs.wrote {
			t.Fatalf("low-confidence match overwrote local metadata: %+v", fs.written)
		}
	})

	t.Run("strong match applied", func(t *testing.T) {
		pool := newWorkerTestPool(t)
		q := NewQueue(pool)
		q.Enqueue(context.Background(), "a")
		reg := newFakeEnrichmentRegistry(&fakeWorkerSource{id: "src", out: &Candidate{
			Source: "src", Title: "Foundation", Authors: []string{"Isaac Asimov"},
			Publisher: "Gnome Press", Description: "A galactic empire saga.",
		}})
		fs := &fakeStore{row: EbookRow{ID: "a", Format: "epub", Title: "Foundation", Author: "Isaac Asimov"}}
		w := NewEnrichmentWorker(q, fs, reg, "src", "us", hclog.NewNullLogger())
		if err := w.Drain(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !fs.wrote || fs.written.Publisher != "Gnome Press" {
			t.Fatalf("strong match not applied: wrote=%v %+v", fs.wrote, fs.written)
		}
	})
}

// ClaimNext must lease the job: a second immediate claim sees an empty queue
// (the row's run_after was pushed forward) instead of re-claiming it.
func TestQueue_ClaimLeasesJob(t *testing.T) {
	pool := newWorkerTestPool(t)
	q := NewQueue(pool)
	ctx := context.Background()
	if err := q.Enqueue(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimNext(ctx); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := q.ClaimNext(ctx); err != ErrQueueEmpty {
		t.Fatalf("second claim must be ErrQueueEmpty (job leased), got %v", err)
	}
}

// MarkCompleted/MarkFailed must not flip a job that already left 'pending'.
func TestQueue_FinishersGuardStatus(t *testing.T) {
	pool := newWorkerTestPool(t)
	q := NewQueue(pool)
	ctx := context.Background()
	_ = q.Enqueue(ctx, "a")
	if _, err := q.ClaimNext(ctx); err != nil {
		t.Fatal(err)
	}
	// Terminal failure (attempts >= MaxAttempts).
	if err := q.MarkFailed(ctx, "a", MaxAttempts, "boom"); err != nil {
		t.Fatal(err)
	}
	// A racing/late MarkCompleted must NOT resurrect it to completed.
	if err := q.MarkCompleted(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM metadata_enrichment_job WHERE ebook_id='a'`).Scan(&status)
	if status != "failed" {
		t.Fatalf("terminal status flipped: got %q, want failed", status)
	}
}
