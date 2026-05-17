package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
)

const (
	// maxClaimFailures bounds consecutive transient ClaimNext errors before
	// Drain gives up (a single blip no longer aborts the whole queue).
	maxClaimFailures = 5
	// claimBackoff spaces out retries so a persistent error can't hot-loop.
	claimBackoff = 2 * time.Second
	// minTextMatchConfidence is the floor for accepting a title+author text
	// search result. A correct match (title substring + author match) scores
	// ~45-60; an unrelated result with only loose word overlap scores ~15.
	minTextMatchConfidence = 40
)

// rowAsCandidate projects the local ebook row into a Candidate so
// CalculateConfidence can compare the search result against existing data.
func rowAsCandidate(r EbookRow) Candidate {
	c := Candidate{Title: r.Title, ISBN: r.ISBN, ASIN: r.ASIN}
	for _, a := range strings.Split(r.Author, ",") {
		if s := strings.TrimSpace(a); s != "" {
			c.Authors = append(c.Authors, s)
		}
	}
	return c
}

// ErrNotFound is the sentinel sources return when a lookup yields no record.
// Defined here (in the metadata package) to avoid an import cycle with the
// sources sub-package. Main.go's registry adapter must map sources.ErrNotFound
// to this value when wrapping *sources.Registry for EnrichmentRegistry.
var ErrNotFound = errors.New("source: not found")

// EnrichmentStore is the subset of *store.Store the worker needs.
type EnrichmentStore interface {
	LoadEbookRow(ctx context.Context, id string) (EbookRow, error)
	UpdateEbookMetadata(ctx context.Context, row EbookRow) error
}

// EnrichmentRegistry is the minimal source-lookup surface the worker needs.
// Like SourceRegistry in aggregator.go, this is an interface because the
// concrete *sources.Registry can't be imported here (import cycle).
type EnrichmentRegistry interface {
	ForID(id string) Source // metadata.Source from aggregator.go
}

// EnrichmentWorker drains metadata_enrichment_job using a single configured
// source per the spec's scan-path trigger model.
type EnrichmentWorker struct {
	Queue    *Queue
	Store    EnrichmentStore
	Registry EnrichmentRegistry
	SourceID string
	Region   string
	Logger   hclog.Logger
}

// NewEnrichmentWorker constructs the worker.
func NewEnrichmentWorker(q *Queue, s EnrichmentStore, reg EnrichmentRegistry,
	sourceID, region string, logger hclog.Logger) *EnrichmentWorker {
	return &EnrichmentWorker{
		Queue: q, Store: s, Registry: reg,
		SourceID: sourceID, Region: region, Logger: logger,
	}
}

// Drain processes pending jobs until the queue is empty or ctx is canceled.
// The queue's FOR UPDATE SKIP LOCKED in ClaimNext is the concurrency guard.
func (w *EnrichmentWorker) Drain(ctx context.Context) error {
	consecFail := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		job, err := w.Queue.ClaimNext(ctx)
		if errors.Is(err, ErrQueueEmpty) {
			return nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// Transient (pool exhaustion, SKIP LOCKED serialization, brief
			// reset): retry with backoff instead of abandoning every
			// remaining pending job until the next scheduled drain.
			consecFail++
			w.Logger.Warn("claim next job", "err", err, "consecutive_failures", consecFail)
			if consecFail >= maxClaimFailures {
				return fmt.Errorf("drain aborted after %d consecutive claim failures: %w", consecFail, err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(claimBackoff):
			}
			continue
		}
		consecFail = 0
		if procErr := w.process(ctx, job); procErr != nil {
			// A ctx cancellation/deadline (shutdown, scheduled-task budget)
			// is not a job failure: don't burn an attempt or write a sticky
			// error. The lease keeps the job from being re-claimed until it
			// expires; the next drain retries it.
			if errors.Is(procErr, context.Canceled) || errors.Is(procErr, context.DeadlineExceeded) || ctx.Err() != nil {
				return ctx.Err()
			}
			_ = w.Queue.MarkFailed(ctx, job.EbookID, job.Attempts, procErr.Error())
			w.Logger.Warn("enrichment failed", "ebook_id", job.EbookID,
				"attempts", job.Attempts, "err", procErr)
			continue
		}
		if err := w.Queue.MarkCompleted(ctx, job.EbookID); err != nil {
			w.Logger.Warn("mark completed", "err", err)
		}
	}
}

// process runs the enrichment for a single job using the cascade:
// ISBN → ASIN (rare for ebooks) → (title + author) text. Single source per the spec.
func (w *EnrichmentWorker) process(ctx context.Context, j Job) error {
	src := w.Registry.ForID(w.SourceID)
	if src == nil {
		return errors.New("configured scan source not registered: " + w.SourceID)
	}
	row, err := w.Store.LoadEbookRow(ctx, j.EbookID)
	if err != nil {
		return err
	}

	var candidate *Candidate
	switch {
	case row.ISBN != "":
		cs, serr := src.Search(ctx, row.ISBN, w.Region)
		err = serr
		if len(cs) > 0 {
			candidate = &cs[0]
		}
	case row.ASIN != "":
		candidate, err = src.Get(ctx, row.ASIN, w.Region)
	default:
		q := strings.TrimSpace(row.Title + " " + row.Author)
		if q == "" {
			return errors.New("no enrichable identifier on ebook row")
		}
		cs, serr := src.Search(ctx, q, w.Region)
		err = serr
		if len(cs) > 0 {
			// A title+author text search can return a loosely-matching
			// first result for a *different* book; ApplyMatch would then
			// overwrite correct local metadata. Gate this fuzzy branch on a
			// confidence floor (a correct title-substring + author match
			// scores well above this; an unrelated result well below).
			orig := rowAsCandidate(row)
			if CalculateConfidence(q, cs[0], &orig) >= minTextMatchConfidence {
				candidate = &cs[0]
			}
		}
	}
	if errors.Is(err, ErrNotFound) {
		// Treat as completed-with-no-change rather than failed.
		return nil
	}
	if err != nil {
		return err
	}
	if candidate == nil {
		return nil
	}
	merged := ApplyMatch(row, *candidate)
	return w.Store.UpdateEbookMetadata(ctx, merged)
}
