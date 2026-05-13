package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Paged is a generic paginated result.
type Paged[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// ListParams controls list/browse pagination + search.
type ListParams struct {
	Page   int
	Limit  int
	Search string
	// Library, when non-empty, restricts results to the library path with the
	// given filesystem path. Empty matches all.
	Library string
}

// Ebook is a summary row for catalog listing.
type Ebook struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors,omitempty"`
	Series      string   `json:"series,omitempty"`
	SeriesIndex string   `json:"series_index,omitempty"`
	Year        string   `json:"year,omitempty"`
	Language    string   `json:"language,omitempty"`
	HasCover    bool     `json:"has_cover"`
	Format      string   `json:"format,omitempty"`
}

// EbookDetail is the full record returned by GetEbookByID.
type EbookDetail struct {
	Ebook
	Description string   `json:"description,omitempty"`
	ISBN        string   `json:"isbn,omitempty"`
	ASIN        string   `json:"asin,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
	FileSize    int64    `json:"file_size,omitempty"`
	Path        string   `json:"-"`
}

// Author / Series / Genre are aggregate rows for browse endpoints.
type Author struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Series struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Genre struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("ebook not found")

// normalizeLimit clamps a requested limit to [1, 200] with default 50.
func normalizeLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

func normalizePage(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// splitCSV splits a comma- or semicolon-separated field into trimmed tokens,
// dropping empties.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	// Accept comma OR semicolon as separators; some scanners use ; for authors.
	rep := strings.ReplaceAll(s, ";", ",")
	parts := strings.Split(rep, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ListEbooks returns a paginated list of non-deleted ebooks, optionally
// filtered by search (matches title/author ILIKE) and library path.
func (s *Store) ListEbooks(ctx context.Context, p ListParams) (Paged[Ebook], error) {
	limit := normalizeLimit(p.Limit)
	page := normalizePage(p.Page)
	offset := (page - 1) * limit

	where := []string{"e.deleted = FALSE"}
	args := []any{}
	if p.Search != "" {
		args = append(args, "%"+p.Search+"%")
		where = append(where, fmt.Sprintf("(e.title ILIKE $%d OR e.author ILIKE $%d)", len(args), len(args)))
	}
	if p.Library != "" {
		args = append(args, p.Library)
		where = append(where, fmt.Sprintf("lp.path = $%d", len(args)))
	}

	whereClause := strings.Join(where, " AND ")

	// Total count.
	var total int
	countSQL := `SELECT count(*) FROM ebook e JOIN library_path lp ON lp.id = e.library_path_id WHERE ` + whereClause
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return Paged[Ebook]{}, fmt.Errorf("count: %w", err)
	}

	// Page query.
	args = append(args, limit, offset)
	rowsSQL := `
		SELECT e.id, e.title, e.author, e.series, e.series_pos, e.year, e.language, e.format,
		       EXISTS(SELECT 1 FROM cover c WHERE c.ebook_id = e.id) AS has_cover
		FROM ebook e
		JOIN library_path lp ON lp.id = e.library_path_id
		WHERE ` + whereClause + `
		ORDER BY e.title ASC, e.id ASC
		LIMIT $` + fmt.Sprintf("%d", len(args)-1) + ` OFFSET $` + fmt.Sprintf("%d", len(args))
	rows, err := s.pool.Query(ctx, rowsSQL, args...)
	if err != nil {
		return Paged[Ebook]{}, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	items := []Ebook{}
	for rows.Next() {
		var (
			b              Ebook
			authorCSV      string
			series, pos    string
			year, language string
			format         string
			hasCover       bool
		)
		if err := rows.Scan(&b.ID, &b.Title, &authorCSV, &series, &pos, &year, &language, &format, &hasCover); err != nil {
			return Paged[Ebook]{}, fmt.Errorf("scan: %w", err)
		}
		b.Authors = splitCSV(authorCSV)
		b.Series = series
		b.SeriesIndex = pos
		b.Year = year
		b.Language = language
		b.Format = format
		b.HasCover = hasCover
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return Paged[Ebook]{}, err
	}

	return Paged[Ebook]{Items: items, Total: total, Page: page, Limit: limit}, nil
}

// GetEbookByID fetches a single ebook by id. Returns ErrNotFound if missing.
func (s *Store) GetEbookByID(ctx context.Context, id string) (EbookDetail, error) {
	var (
		d         EbookDetail
		authorCSV string
		genreCSV  string
		hasCover  bool
	)
	err := s.pool.QueryRow(ctx, `
		SELECT e.id, e.title, e.author, e.series, e.series_pos, e.year, e.language, e.format,
		       e.description, e.isbn, e.asin, e.publisher, e.genre, e.page_count, e.file_size, e.path,
		       EXISTS(SELECT 1 FROM cover c WHERE c.ebook_id = e.id) AS has_cover
		FROM ebook e
		WHERE e.id = $1 AND e.deleted = FALSE
	`, id).Scan(
		&d.ID, &d.Title, &authorCSV, &d.Series, &d.SeriesIndex, &d.Year, &d.Language, &d.Format,
		&d.Description, &d.ISBN, &d.ASIN, &d.Publisher, &genreCSV, &d.PageCount, &d.FileSize, &d.Path,
		&hasCover,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return EbookDetail{}, ErrNotFound
	}
	if err != nil {
		return EbookDetail{}, err
	}
	d.Authors = splitCSV(authorCSV)
	d.Genres = splitCSV(genreCSV)
	d.HasCover = hasCover
	return d, nil
}

// GetCover returns the raw cover bytes + content-type for an ebook.
// Returns ErrNotFound if the cover (or ebook) is missing.
func (s *Store) GetCover(ctx context.Context, id string) ([]byte, string, error) {
	var (
		bytes       []byte
		contentType string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT bytes, content_type FROM cover WHERE ebook_id = $1
	`, id).Scan(&bytes, &contentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return bytes, contentType, nil
}

// GetEbookPath returns the on-disk path + format for streaming the book file.
// Returns ErrNotFound if missing.
func (s *Store) GetEbookPath(ctx context.Context, id string) (string, string, error) {
	var path, format string
	err := s.pool.QueryRow(ctx, `
		SELECT e.path, e.format FROM ebook e
		WHERE e.id = $1 AND e.deleted = FALSE
	`, id).Scan(&path, &format)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	return path, format, nil
}

// splitColumnSQL produces a SELECT that splits a CSV column on , and ;,
// trims whitespace, and groups by the resulting name producing (name, count).
// Empty / whitespace-only names are filtered out.
func splitColumnSQL(column string) string {
	// nested SELECT with unnest, then aggregate.
	return `
		SELECT name, count(*)::int AS count FROM (
		  SELECT trim(t) AS name
		  FROM ebook e,
		       LATERAL unnest(string_to_array(replace(e.` + column + `, ';', ','), ',')) AS t
		  WHERE e.deleted = FALSE AND e.` + column + ` <> ''
		) sub
		WHERE name <> ''
		GROUP BY name
	`
}

// ListAuthors enumerates distinct authors (split from the CSV `author` column)
// with their book counts. Pagination is post-aggregation.
func (s *Store) ListAuthors(ctx context.Context, p ListParams) (Paged[Author], error) {
	limit := normalizeLimit(p.Limit)
	page := normalizePage(p.Page)
	return paginateAggregate[Author](ctx, s,
		splitColumnSQL("author"),
		p.Search, page, limit,
		func(name string, count int) Author { return Author{Name: name, Count: count} },
	)
}

// ListSeries enumerates distinct series with their book counts. The `series`
// column is a single value per row (not CSV), so no splitting required.
func (s *Store) ListSeries(ctx context.Context, p ListParams) (Paged[Series], error) {
	limit := normalizeLimit(p.Limit)
	page := normalizePage(p.Page)
	return paginateAggregate[Series](ctx, s,
		`SELECT e.series AS name, count(*)::int AS count
		 FROM ebook e WHERE e.deleted = FALSE AND e.series <> ''
		 GROUP BY e.series`,
		p.Search, page, limit,
		func(name string, count int) Series { return Series{Name: name, Count: count} },
	)
}

// ListGenres enumerates distinct genres (split from CSV `genre` column).
func (s *Store) ListGenres(ctx context.Context, p ListParams) (Paged[Genre], error) {
	limit := normalizeLimit(p.Limit)
	page := normalizePage(p.Page)
	return paginateAggregate[Genre](ctx, s,
		splitColumnSQL("genre"),
		p.Search, page, limit,
		func(name string, count int) Genre { return Genre{Name: name, Count: count} },
	)
}

// paginateAggregate wraps an aggregation subquery (yielding columns name,
// count) with optional name-filter + pagination + total count, projecting
// each row via mk.
func paginateAggregate[T any](
	ctx context.Context, s *Store, innerSQL, search string,
	page, limit int, mk func(name string, count int) T,
) (Paged[T], error) {
	offset := (page - 1) * limit
	args := []any{}
	where := ""
	if search != "" {
		args = append(args, "%"+search+"%")
		where = fmt.Sprintf(" WHERE name ILIKE $%d", len(args))
	}

	countSQL := `SELECT count(*) FROM (` + innerSQL + `) agg` + where
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return Paged[T]{}, fmt.Errorf("count: %w", err)
	}

	args = append(args, limit, offset)
	pageSQL := `SELECT name, count FROM (` + innerSQL + `) agg` + where +
		` ORDER BY name ASC LIMIT $` + fmt.Sprintf("%d", len(args)-1) +
		` OFFSET $` + fmt.Sprintf("%d", len(args))

	rows, err := s.pool.Query(ctx, pageSQL, args...)
	if err != nil {
		return Paged[T]{}, fmt.Errorf("page: %w", err)
	}
	defer rows.Close()

	items := []T{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return Paged[T]{}, err
		}
		items = append(items, mk(name, count))
	}
	if err := rows.Err(); err != nil {
		return Paged[T]{}, err
	}
	return Paged[T]{Items: items, Total: total, Page: page, Limit: limit}, nil
}
