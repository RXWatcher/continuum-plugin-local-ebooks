package store

import (
	"context"
	"time"
)

// LibraryPath is a configured root.
type LibraryPath struct {
	ID            int64
	Path          string
	Name          string
	MediaType     string
	Enabled       bool
	LastScannedAt *time.Time
}

type LibraryPathConfig struct {
	Path      string
	Name      string
	MediaType string
}

// ScanEvent is a recorded scanner run.
type ScanEvent struct {
	ID            int64      `json:"id"`
	LibraryPathID *int64     `json:"library_path_id,omitempty"`
	LibraryName   string     `json:"library_name,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	BooksAdded    int        `json:"books_added"`
	BooksChanged  int        `json:"books_changed"`
	BooksDeleted  int        `json:"books_deleted"`
	ErrorText     string     `json:"error_text,omitempty"`
}

// UpsertLibraryPath inserts the path if missing (returning the new id) or
// returns the existing id. Disabled rows are re-enabled on upsert.
func (s *Store) UpsertLibraryPath(ctx context.Context, path string) (int64, error) {
	return s.UpsertLibraryPathConfig(ctx, LibraryPathConfig{Path: path})
}

// UpsertLibraryPathConfig inserts or updates one configured library root. Empty
// names/media types preserve existing values on update and receive defaults on
// insert.
func (s *Store) UpsertLibraryPathConfig(ctx context.Context, cfg LibraryPathConfig) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO library_path (path, name, media_type, enabled)
		VALUES ($1, COALESCE(NULLIF($2, ''), $1), COALESCE(NULLIF($3, ''), 'book'), TRUE)
		ON CONFLICT (path) DO UPDATE SET
			name = COALESCE(NULLIF(EXCLUDED.name, ''), library_path.name),
			media_type = COALESCE(NULLIF(EXCLUDED.media_type, ''), library_path.media_type),
			enabled = TRUE
		RETURNING id
	`, cfg.Path, cfg.Name, cfg.MediaType).Scan(&id)
	return id, err
}

// ListLibraryPaths returns all configured roots.
func (s *Store) ListLibraryPaths(ctx context.Context) ([]LibraryPath, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, path, name, media_type, enabled, last_scanned_at
		FROM library_path
		ORDER BY name ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LibraryPath
	for rows.Next() {
		var lp LibraryPath
		if err := rows.Scan(&lp.ID, &lp.Path, &lp.Name, &lp.MediaType, &lp.Enabled, &lp.LastScannedAt); err != nil {
			return nil, err
		}
		out = append(out, lp)
	}
	return out, rows.Err()
}

// MarkLibraryScanned updates last_scanned_at to now().
func (s *Store) MarkLibraryScanned(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE library_path SET last_scanned_at = now() WHERE id = $1`, id)
	return err
}

// InsertScanEvent creates a started scan_event row; library_path_id is
// optional. Returns the new row id.
func (s *Store) InsertScanEvent(ctx context.Context, libraryPathID *int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO scan_event (library_path_id) VALUES ($1) RETURNING id
	`, libraryPathID).Scan(&id)
	return id, err
}

// FinishScanEvent closes a scan_event row with its result tallies and any
// error text.
func (s *Store) FinishScanEvent(ctx context.Context, id int64, added, changed, deleted int, errText string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE scan_event
		   SET finished_at = now(),
		       books_added = $2,
		       books_changed = $3,
		       books_deleted = $4,
		       error_text = NULLIF($5, '')
		 WHERE id = $1
	`, id, added, changed, deleted, errText)
	return err
}

// RecentScanEvents returns the most recent scanner runs.
func (s *Store) RecentScanEvents(ctx context.Context, limit int) ([]ScanEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT se.id, se.library_path_id, COALESCE(lp.name, ''),
		       se.started_at, se.finished_at, se.books_added, se.books_changed,
		       se.books_deleted, COALESCE(se.error_text, '')
		  FROM scan_event se
		  LEFT JOIN library_path lp ON lp.id = se.library_path_id
		 ORDER BY se.started_at DESC
		 LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScanEvent{}
	for rows.Next() {
		var ev ScanEvent
		if err := rows.Scan(
			&ev.ID, &ev.LibraryPathID, &ev.LibraryName, &ev.StartedAt, &ev.FinishedAt,
			&ev.BooksAdded, &ev.BooksChanged, &ev.BooksDeleted, &ev.ErrorText,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
