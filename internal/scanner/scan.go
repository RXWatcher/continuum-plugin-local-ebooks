// Package scanner walks library paths and ingests ebook files.
package scanner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/blake2b"

	"github.com/ContinuumApp/continuum-plugin-local-ebooks/internal/ebookparse"
)

// EnrichmentEnqueuer is the surface the scanner needs from metadata.Queue.
type EnrichmentEnqueuer interface {
	Enqueue(ctx context.Context, ebookID string) error
}

// Deps is the scanner's dependencies.
type Deps struct {
	Store           Store
	EnrichmentQueue EnrichmentEnqueuer // optional; nil-safe
	Logger          *slog.Logger
}

// Store is the surface the scanner needs.
type Store interface {
	UpsertEbook(ctx context.Context, libraryPathID int64, ebookID, path, format string,
		fileSize int64, mtime time.Time, p ebookparse.Parsed) (wasKnown bool, err error)
	UpsertCover(ctx context.Context, ebookID, contentType, source string, bytes []byte) error
	ListPaths(ctx context.Context, libraryPathID int64) (map[string]string, error)
	SoftDelete(ctx context.Context, ebookID string) error
}

// WalkResult summarizes a single library_path scan. Failed counts files
// that errored during upsert so the caller can record degradation in the
// scan_event audit row instead of reporting a clean success.
type WalkResult struct {
	Added   int
	Changed int
	Deleted int
	Failed  int
}

// Walk traverses `root` (with library_path_id `lpID` for FK), parses each
// recognized ebook, upserts it, and soft-deletes rows whose files disappeared.
func Walk(ctx context.Context, root string, lpID int64, deps Deps) (WalkResult, error) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	res := WalkResult{}

	knownByID, err := deps.Store.ListPaths(ctx, lpID)
	if err != nil {
		return res, fmt.Errorf("scanner: list paths: %w", err)
	}
	pathToID := map[string]string{}
	for id, p := range knownByID {
		pathToID[p] = id
	}

	seenIDs := map[string]struct{}{}
	walkHadErrors := false

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable entry/subtree means our view is incomplete; record
			// it so we don't soft-delete still-present rows we just couldn't see.
			walkHadErrors = true
			deps.Logger.Warn("walk entry error", "path", path, "err", err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !ebookparse.IsSupported(path) {
			return nil
		}
		// Mark a known path as seen up front so a transient stat error below
		// can't make a still-present file get spuriously soft-deleted.
		if priorID, ok := pathToID[path]; ok {
			seenIDs[priorID] = struct{}{}
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Only ingest regular files. d.Info() is lstat data, so a symlink
		// reports its own (non-regular) mode here; rejecting it prevents the
		// symlink-escape vector where a link inside a library root makes the
		// parser/os.ReadFile follow it and read arbitrary host files.
		if !info.Mode().IsRegular() {
			return nil
		}
		id := stableID(path, info.Size(), info.ModTime())
		seenIDs[id] = struct{}{}

		// Skip re-ingest if we've already seen this exact (path, id) pair.
		if priorID, ok := pathToID[path]; ok && priorID == id {
			return nil
		}

		p, err := parseRecovered(path)
		if err != nil {
			deps.Logger.Warn("parse failed; ingesting with empty metadata", "path", path, "err", err)
			p = ebookparse.Parsed{Format: strings.TrimPrefix(filepath.Ext(path), ".")}
		}

		wasKnown, upsertErr := deps.Store.UpsertEbook(ctx, lpID, id, path, p.Format, info.Size(), info.ModTime(), p)
		if upsertErr != nil {
			deps.Logger.Warn("upsert failed", "path", path, "err", upsertErr)
			res.Failed++
			return nil
		}
		if wasKnown {
			res.Changed++
		} else {
			res.Added++
		}

		// Cover: embedded first, then sidecar fallback.
		if p.Cover != nil && len(p.Cover.Bytes) > 0 {
			if err := deps.Store.UpsertCover(ctx, id, p.Cover.ContentType, "embedded", p.Cover.Bytes); err != nil {
				deps.Logger.Warn("cover write (embedded)", "err", err)
			}
		} else {
			if sc, ct, ok := findSidecarCover(filepath.Dir(path)); ok {
				if err := deps.Store.UpsertCover(ctx, id, ct, "sidecar", sc); err != nil {
					deps.Logger.Warn("cover write (sidecar)", "err", err)
				}
			}
		}

		if deps.EnrichmentQueue != nil {
			if err := deps.EnrichmentQueue.Enqueue(ctx, id); err != nil {
				deps.Logger.Warn("enqueue enrichment", "ebook_id", id, "err", err)
			}
		}
		return nil
	}

	if err := filepath.WalkDir(root, walkFn); err != nil {
		return res, fmt.Errorf("scanner: walk: %w", err)
	}

	// Only reconcile deletions when the walk saw the tree completely. If any
	// entry errored (e.g. a permissions blip on a subtree), the ids under it
	// are absent from seenIDs and would be mass soft-deleted spuriously.
	if walkHadErrors {
		deps.Logger.Warn("skipping soft-delete reconcile: walk had entry errors", "root", root)
		return res, nil
	}
	for id := range knownByID {
		if _, ok := seenIDs[id]; ok {
			continue
		}
		if err := deps.Store.SoftDelete(ctx, id); err != nil {
			deps.Logger.Warn("soft-delete", "id", id, "err", err)
			continue
		}
		res.Deleted++
	}

	return res, nil
}

// parseRecovered wraps ebookparse.Parse so a panic in any parser (e.g. the
// third-party PDF reader on a malformed file) is turned into an error and
// the file is ingested with empty metadata, instead of crashing the scan.
func parseRecovered(path string) (p ebookparse.Parsed, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic parsing %s: %v", path, r)
		}
	}()
	return ebookparse.Parse(path)
}

// stableID returns blake2b(path || size || mtime) truncated to 16 hex chars.
func stableID(path string, size int64, mtime time.Time) string {
	h, _ := blake2b.New(16, nil)
	h.Write([]byte(path))
	fmt.Fprintf(h, "|%d|%d", size, mtime.UnixNano())
	return hex.EncodeToString(h.Sum(nil))
}

// findSidecarCover looks for cover.jpg / cover.png / folder.jpg in dir.
func findSidecarCover(dir string) ([]byte, string, bool) {
	candidates := []struct {
		name        string
		contentType string
	}{
		{"cover.jpg", "image/jpeg"},
		{"cover.png", "image/png"},
		{"folder.jpg", "image/jpeg"},
	}
	for _, c := range candidates {
		p := filepath.Join(dir, c.name)
		// Lstat + regular-file check so a cover.jpg symlink can't exfiltrate
		// an arbitrary host file into the cover table.
		if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				continue
			}
			continue
		}
		if len(b) == 0 {
			continue
		}
		if len(b) > 5<<20 {
			b = b[:5<<20]
		}
		return b, c.contentType, true
	}
	return nil, "", false
}
