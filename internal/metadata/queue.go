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
	if err != nil { return Job{}, err }
	defer tx.Rollback(ctx) //nolint:errcheck
	var j Job
	err = tx.QueryRow(ctx, `
		SELECT ebook_id, attempts FROM metadata_enrichment_job
		WHERE status = 'pending' AND run_after <= now()
		ORDER BY run_after
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&j.EbookID, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrQueueEmpty
	}
	if err != nil { return Job{}, err }
	if _, err := tx.Exec(ctx,
		`UPDATE metadata_enrichment_job SET attempts = attempts + 1 WHERE ebook_id = $1`,
		j.EbookID); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil { return Job{}, err }
	j.Attempts++
	return j, nil
}

func (q *Queue) MarkCompleted(ctx context.Context, ebookID string) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE metadata_enrichment_job
		SET status = 'completed', finished_at = now(), last_error = ''
		WHERE ebook_id = $1
	`, ebookID)
	return err
}

func (q *Queue) MarkFailed(ctx context.Context, ebookID string, attempts int, errText string) error {
	if attempts >= MaxAttempts {
		_, err := q.pool.Exec(ctx, `
			UPDATE metadata_enrichment_job
			SET status = 'failed', finished_at = now(), last_error = $2
			WHERE ebook_id = $1
		`, ebookID, errText)
		return err
	}
	backoff := time.Duration(1<<uint(attempts)) * time.Minute
	_, err := q.pool.Exec(ctx, `
		UPDATE metadata_enrichment_job
		SET run_after = now() + make_interval(secs => $2), last_error = $3
		WHERE ebook_id = $1
	`, ebookID, backoff.Seconds(), errText)
	return err
}
