package metadata

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const MaxAttempts = 5

type Job struct {
	EbookID  string
	Attempts int
}

type Queue struct {
	pool *pgxpool.Pool
}

func NewQueue(pool *pgxpool.Pool) *Queue { return &Queue{pool: pool} }

var ErrQueueEmpty = errors.New("metadata enrichment queue empty")

func (q *Queue) Enqueue(ctx context.Context, ebookID string) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO metadata_enrichment_job (ebook_id)
		VALUES ($1)
		ON CONFLICT (ebook_id) DO UPDATE
		  SET status = 'pending', attempts = 0,
		      run_after = now(), last_error = '', finished_at = NULL
	`, ebookID)
	return err
}

func (q *Queue) ClaimNext(ctx context.Context) (Job, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var j Job
	err = tx.QueryRow(ctx, `
		SELECT ebook_id, attempts FROM metadata_enrichment_job
		WHERE status = 'pending' AND run_after <= now()
		ORDER BY run_after, ebook_id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&j.EbookID, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrQueueEmpty
	}
	if err != nil {
		return Job{}, err
	}
	// Lease the job: push run_after into the future so the row is no longer
	// claimable while process() runs. SKIP LOCKED only serialized the
	// sub-second claim transaction; without the lease the still-'pending',
	// still-due row was immediately re-claimable by an overlapping drain and
	// double-processed. MarkCompleted/MarkFailed overwrite run_after on
	// finish; a worker that dies mid-process leaves the job to be re-picked
	// after the lease expires. ebook_id tiebreaker keeps ordering stable.
	if _, err := tx.Exec(ctx,
		`UPDATE metadata_enrichment_job
		   SET attempts = attempts + 1, run_after = now() + interval '5 minutes'
		 WHERE ebook_id = $1`,
		j.EbookID); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	j.Attempts++
	return j, nil
}

// The finishers guard on status = 'pending' so a racing/duplicate finisher
// (or a late call after the job was already re-armed) can't stamp a
// completed/failed row or flip a terminal state.
func (q *Queue) MarkCompleted(ctx context.Context, ebookID string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE metadata_enrichment_job
		SET status = 'completed', finished_at = now(), last_error = ''
		WHERE ebook_id = $1 AND status = 'pending'
	`, ebookID)
	return err
}

func (q *Queue) MarkFailed(ctx context.Context, ebookID string, attempts int, errText string) error {
	if attempts >= MaxAttempts {
		_, err := q.pool.Exec(ctx, `
			UPDATE metadata_enrichment_job
			SET status = 'failed', finished_at = now(), last_error = $2
			WHERE ebook_id = $1 AND status = 'pending'
		`, ebookID, errText)
		return err
	}
	backoff := time.Duration(1<<uint(attempts)) * time.Minute
	_, err := q.pool.Exec(ctx, `
		UPDATE metadata_enrichment_job
		SET run_after = now() + make_interval(secs => $2), last_error = $3
		WHERE ebook_id = $1 AND status = 'pending'
	`, ebookID, backoff.Seconds(), errText)
	return err
}
