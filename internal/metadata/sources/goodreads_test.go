package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newGoodreadsFake(t *testing.T) (*httptest.Server, *Goodreads) {
	t.Helper()
	book := loadFixture(t, "goodreads_book.html")
	search := loadFixture(t, "goodreads_search.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/book/show/54493401"):
			w.Header().Set("Content-Type", "text/html")
			w.Write(book)
		case strings.HasPrefix(r.URL.Path, "/book/show/"):
			// any other book ID → 404
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/search":
			w.Header().Set("Content-Type", "text/html")
			w.Write(search)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	g := NewGoodreadsAt(srv.URL, "test-agent")
	g.http.Client = srv.Client()
	return srv, g
}

// TestGoodreads_GetByID verifies happy-path scraping of a book detail page.
// The fixture embeds JSON-LD with known fields; we assert each critical field
// so that a regression in the parser is immediately visible.
func TestGoodreads_GetByID(t *testing.T) {
	srv, g := newGoodreadsFake(t)
	defer srv.Close()

	c, err := g.Get(context.Background(), "54493401", "us")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil candidate")
	}
	if c.Title != "Project Hail Mary" {
		t.Errorf("title: got %q, want %q", c.Title, "Project Hail Mary")
	}
	if c.Source != goodreadsID {
		t.Errorf("source: got %q, want %q", c.Source, goodreadsID)
	}
	if c.ExternalID != "54493401" {
		t.Errorf("external_id: got %q, want %q", c.ExternalID, "54493401")
	}
	if c.ISBN != "9780593135204" {
		t.Errorf("isbn: got %q, want %q", c.ISBN, "9780593135204")
	}
	if len(c.Authors) == 0 || c.Authors[0] != "Andy Weir" {
		t.Errorf("authors: got %v, want [Andy Weir]", c.Authors)
	}
	if c.CoverURL == "" {
		t.Error("expected non-empty cover URL")
	}
	if c.PublishedAt != "2021-05-04" {
		t.Errorf("published_at: got %q, want %q", c.PublishedAt, "2021-05-04")
	}
	if c.Description == "" {
		t.Error("expected non-empty description")
	}
	if c.Publisher != "Ballantine Books" {
		t.Errorf("publisher: got %q, want %q", c.Publisher, "Ballantine Books")
	}
	if c.PageCount != 476 {
		t.Errorf("page_count: got %d, want 476", c.PageCount)
	}
}

// TestGoodreads_GetMissing verifies that a 404 response from the server
// surfaces as ErrNotFound rather than a nil candidate or an opaque error.
func TestGoodreads_GetMissing(t *testing.T) {
	srv, g := newGoodreadsFake(t)
	defer srv.Close()

	c, err := g.Get(context.Background(), "99999999", "us")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got err=%v c=%v", err, c)
	}
	if c != nil {
		t.Error("expected nil candidate on not-found")
	}
}

// TestGoodreads_SearchByText verifies that search results are parsed from the
// fixture HTML and returned as Candidates with correct titles and authors.
func TestGoodreads_SearchByText(t *testing.T) {
	srv, g := newGoodreadsFake(t)
	defer srv.Close()

	cs, err := g.Search(context.Background(), "project hail mary", "us")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(cs))
	}

	first := cs[0]
	if first.Title != "Project Hail Mary" {
		t.Errorf("first result title: got %q, want %q", first.Title, "Project Hail Mary")
	}
	if first.Source != goodreadsID {
		t.Errorf("first result source: got %q, want %q", first.Source, goodreadsID)
	}
	if first.ExternalID != "54493401" {
		t.Errorf("first result external_id: got %q, want %q", first.ExternalID, "54493401")
	}
	if len(first.Authors) == 0 || first.Authors[0] != "Andy Weir" {
		t.Errorf("first result authors: got %v, want [Andy Weir]", first.Authors)
	}
}

// TestGoodreads_NonNumericGetReturnsNil verifies that passing a non-numeric
// string (e.g. an ISBN or free-text) returns (nil, nil) without hitting the
// network, since Goodreads IDs are always numeric.
func TestGoodreads_NonNumericGetReturnsNil(t *testing.T) {
	srv, g := newGoodreadsFake(t)
	defer srv.Close()

	c, err := g.Get(context.Background(), "project-hail-mary", "us")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if c != nil {
		t.Errorf("expected nil candidate for non-numeric id, got %+v", c)
	}
}
